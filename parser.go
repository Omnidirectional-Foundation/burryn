package main

import (
	"fmt"
	"strconv"
)

type Parser struct {
	toks []Token
	pos  int
}

// maxParseDiags stops error recovery once a source is hopelessly broken.
const maxParseDiags = 20

// parse builds statements from toks, collecting parse errors as diagnostics.
// After an error it synchronizes to the next statement boundary and keeps
// parsing, so several errors can be reported in one pass.
func parse(toks []Token) (stmts []Stmt, diags []Diag) {
	p := &Parser{toks: toks}
	p.skipSemis()
	for !p.check(TEOF) && len(diags) < maxParseDiags {
		if s, d := p.statementRecover(); d != nil {
			diags = append(diags, *d)
			p.synchronize()
		} else {
			stmts = append(stmts, s)
		}
		p.skipSemis()
	}
	return stmts, diags
}

// parseInterface parses an interface file (a syntax subset: imports, enums,
// exported signatures).
func parseInterface(toks []Token) (stmts []Stmt, diags []Diag) {
	p := &Parser{toks: toks}
	p.skipSemis()
	for !p.check(TEOF) && len(diags) < maxParseDiags {
		if s, d := p.statementRecoverIface(); d != nil {
			diags = append(diags, *d)
			p.synchronize()
		} else {
			stmts = append(stmts, s)
		}
		p.skipSemis()
	}
	return stmts, diags
}

func (p *Parser) statementRecover() (s Stmt, d *Diag) {
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(parseErr)
			if !ok {
				panic(r)
			}
			dd := Diag(pe)
			d = &dd
		}
	}()
	return p.statement(), nil
}

func (p *Parser) statementRecoverIface() (s Stmt, d *Diag) {
	defer func() {
		if r := recover(); r != nil {
			pe, ok := r.(parseErr)
			if !ok {
				panic(r)
			}
			dd := Diag(pe)
			d = &dd
		}
	}()
	return p.interfaceStatement(), nil
}

// synchronize skips tokens until the next statement boundary: a semicolon at
// the current brace depth (closers of blocks we bailed out of are swallowed
// as residue) or a token that starts a statement.
func (p *Parser) synchronize() {
	depth := 0
	for !p.check(TEOF) {
		switch p.advance().Type {
		case TLBrace:
			depth++
		case TRBrace:
			if depth > 0 {
				depth--
			}
		case TSemi:
			if depth == 0 && !p.check(TRBrace) {
				return
			}
		}
		if depth == 0 {
			switch p.peek().Type {
			case TLet, TConst, TFn, TEnum, TType, TWhile, TFor, TReturn, TSpawn, TSelect, TDefer, TBreak, TContinue, TPub, TImport:
				return
			}
		}
	}
}

type parseErr Diag

func (p *Parser) fail(sp Span, code, help, format string, args ...any) {
	panic(parseErr(Diag{IsErr: true, Code: code, Msg: fmt.Sprintf(format, args...), Help: help, Span: sp}))
}

func (p *Parser) peek() Token          { return p.toks[p.pos] }
func (p *Parser) prev() Token          { return p.toks[p.pos-1] }
func (p *Parser) check(t TokType) bool { return p.peek().Type == t }

func (p *Parser) advance() Token {
	t := p.toks[p.pos]
	if t.Type != TEOF {
		p.pos++
	}
	return t
}

func (p *Parser) match(types ...TokType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

func (p *Parser) expect(t TokType, what string) Token {
	if p.check(t) {
		return p.advance()
	}
	tok := p.peek()
	p.fail(tok.Span, "E1101", "", "expected %s, got %q", what, tok.Lex)
	return Token{}
}

func (p *Parser) skipSemis() {
	for p.check(TSemi) {
		p.advance()
	}
}

// terminate a statement: semi, or lookahead at a closer/EOF
func (p *Parser) endStmt() {
	if p.check(TSemi) {
		p.advance()
		return
	}
	if p.check(TRBrace) || p.check(TEOF) {
		return
	}
	tok := p.peek()
	p.fail(tok.Span, "E1102", "separate statements with a newline or ';'", "expected end of statement, got %q", tok.Lex)
}

// ---- statements ----

func (p *Parser) statement() Stmt {
	// labeled loops first: ident : while/for
	if p.check(TIdent) && p.toks[p.pos+1].Type == TColon &&
		(p.toks[p.pos+2].Type == TWhile || p.toks[p.pos+2].Type == TFor) {
		ltok := p.advance()
		p.advance() // ':'
		inner := p.statement()
		return &LabelStmt{Label: ltok.Lex, Inner: inner, Span: ltok.Span.union(inner.span())}
	}
	switch {
	case p.check(TLet):
		return p.letStmt()
	case p.check(TConst):
		return p.constStmt()
	case p.check(TFn) && p.toks[p.pos+1].Type == TIdent:
		return p.fnDecl()
	case p.check(TFn) && p.toks[p.pos+1].Type == TLParen && p.toks[p.pos+2].Type == TIdent && p.toks[p.pos+3].Type == TColon:
		return p.methodDecl()
	case p.check(TEnum):
		return p.enumDecl()
	case p.check(TType):
		return p.typeAliasDecl(false)
	case p.check(TPub):
		return p.pubDecl()
	case p.check(TImport):
		return p.importDecl()
	case p.check(TWhile):
		return p.whileStmt()
	case p.check(TFor):
		return p.forStmt()
	case p.check(TReturn):
		tok := p.advance()
		sp := tok.Span
		var val Expr
		if !p.check(TSemi) && !p.check(TRBrace) && !p.check(TEOF) {
			val = p.expression()
			sp = sp.union(val.span())
		}
		p.endStmt()
		return &ReturnStmt{Val: val, Span: sp}
	case p.check(TSpawn):
		return p.spawnStmt()
	case p.check(TSelect):
		return p.selectStmt()
	case p.check(TDefer):
		return p.deferStmt()
	case p.check(TBreak):
		tok := p.advance()
		lb := ""
		if p.check(TIdent) {
			lb = p.advance().Lex
		}
		p.endStmt()
		return &BreakStmt{Label: lb, Span: tok.Span}
	case p.check(TContinue):
		tok := p.advance()
		lb := ""
		if p.check(TIdent) {
			lb = p.advance().Lex
		}
		p.endStmt()
		return &ContinueStmt{Label: lb, Span: tok.Span}
	}
	// expression statement / assignment / compound assignment / send
	e := p.expression()
	if p.match(TEq) {
		val := p.expression()
		switch e.(type) {
		case *Ident, *Index:
		default:
			p.fail(e.span(), "E1103", "only a variable or an index expression can be assigned to", "invalid assignment target")
		}
		p.endStmt()
		return &AssignStmt{Target: e, Val: val, Span: e.span().union(val.span())}
	}
	if p.check(TPlusEq) || p.check(TMinusEq) || p.check(TStarEq) || p.check(TSlashEq) || p.check(TPercentEq) {
		op := p.advance()
		var binop TokType
		switch op.Type {
		case TPlusEq:
			binop = TPlus
		case TMinusEq:
			binop = TMinus
		case TStarEq:
			binop = TStar
		case TSlashEq:
			binop = TSlash
		default:
			binop = TPercent
		}
		val := p.expression()
		switch e.(type) {
		case *Ident, *Index:
		default:
			p.fail(e.span(), "E1103", "only a variable or an index expression can be assigned to", "invalid assignment target")
		}
		p.endStmt()
		sp := e.span().union(val.span())
		return &AssignStmt{Target: e, Val: &Binary{Op: binop, Lhs: e, Rhs: val, Span: sp}, Span: sp}
	}
	if p.match(TLArrow) {
		val := p.expression()
		p.endStmt()
		return &SendStmt{Chan: e, Val: val, Span: e.span().union(val.span())}
	}
	p.endStmt()
	return &ExprStmt{E: e, Span: e.span()}
}

// interfaceStatement parses the interface subset: import, enum, pub declarations.
func (p *Parser) interfaceStatement() Stmt {
	switch {
	case p.check(TImport):
		return p.importDecl()
	case p.check(TEnum):
		return p.enumDecl()
	case p.check(TPub):
		tok := p.advance()
		switch {
		case p.check(TFn):
			p.advance()
			name := p.expect(TIdent, "function name")
			lit := p.fnRest(name.Lex, tok.Span, true)
			return &FnDecl{Name: name.Lex, NameSpan: name.Span, Pub: true, Fn: lit,
				Span: tok.Span.union(lit.Span)}
		case p.check(TLet):
			p.advance()
			name := p.expect(TIdent, "variable name")
			p.expect(TColon, "':' before the exported value type")
			te := p.typeExpr()
			p.endStmt()
			return &LetStmt{Name: name.Lex, NameSpan: name.Span, Pub: true, Ty: te,
				Span: tok.Span.union(te.span())}
		case p.check(TEnum):
			s := p.enumDecl().(*EnumDecl)
			s.Pub = true
			s.Span = tok.Span.union(s.Span)
			return s
		}
		p.fail(tok.Span, "E1114", "interface files contain imports, enums, and exported signatures",
			"expected `fn`, `enum`, or `let` after `pub`")
	}
	p.fail(p.peek().Span, "E1114", "interface files contain imports, enums, and exported signatures",
		"expected an interface declaration, got %q", p.peek().Lex)
	return nil
}

func (p *Parser) letStmt() Stmt {
	tok := p.advance() // let
	mut := p.match(TMut)
	// tuple destructuring: let (a, b) = ...
	if p.check(TLParen) {
		p.advance()
		var names []string
		var nspans []Span
		p.skipSemis()
		for !p.check(TRParen) {
			nm := p.expect(TIdent, "name in destructuring let")
			names = append(names, nm.Lex)
			nspans = append(nspans, nm.Span)
			if !p.match(TComma) {
				break
			}
			p.skipSemis()
		}
		p.expect(TRParen, "')' in destructuring let")
		p.expect(TEq, "'=' in let binding")
		init := p.expression()
		p.endStmt()
		return &DestructLet{Names: names, NameSpans: nspans, Mut: mut, Init: init,
			Span: tok.Span.union(init.span())}
	}
	name := p.expect(TIdent, "variable name")
	var ty TypeExpr
	if p.match(TColon) {
		ty = p.typeExpr()
	}
	p.expect(TEq, "'=' in let binding")
	init := p.expression()
	p.endStmt()
	return &LetStmt{Name: name.Lex, NameSpan: name.Span, Mut: mut, Ty: ty, Init: init,
		Span: tok.Span.union(init.span())}
}

func (p *Parser) constStmt() Stmt {
	tok := p.advance() // const
	name := p.expect(TIdent, "constant name")
	var ty TypeExpr
	if p.match(TColon) {
		ty = p.typeExpr()
	}
	p.expect(TEq, "'=' in const declaration")
	init := p.expression()
	p.endStmt()
	return &ConstDecl{Name: name.Lex, NameSpan: name.Span, Ty: ty, Init: init,
		Span: tok.Span.union(init.span())}
}

// pubDecl parses `pub` followed by an exportable declaration.
func (p *Parser) pubDecl() Stmt {
	tok := p.advance() // pub
	switch {
	case p.check(TLet):
		s := p.letStmt().(*LetStmt)
		s.Pub = true
		s.Span = tok.Span.union(s.Span)
		return s
	case p.check(TConst):
		s := p.constStmt().(*ConstDecl)
		s.Pub = true
		s.Span = tok.Span.union(s.Span)
		return s
	case p.check(TFn) && p.toks[p.pos+1].Type == TIdent:
		s := p.fnDecl().(*FnDecl)
		s.Pub = true
		s.Span = tok.Span.union(s.Span)
		return s
	case p.check(TEnum):
		s := p.enumDecl().(*EnumDecl)
		s.Pub = true
		s.Span = tok.Span.union(s.Span)
		return s
	case p.check(TType):
		return p.typeAliasDecl(true)
	}
	p.fail(tok.Span, "E1114", "`pub` exports a top-level declaration: `pub fn`, `pub enum`, `pub let`, `pub const`, or `pub type`",
		"expected `fn`, `enum`, `let`, `const`, or `type` after `pub`, got %q", p.peek().Lex)
	return nil
}

// importDecl parses `import ["alias"] "path"`.
func (p *Parser) importDecl() Stmt {
	tok := p.advance() // import
	d := &ImportDecl{Span: tok.Span}
	if p.check(TIdent) {
		a := p.advance()
		d.Alias, d.AliasSpan = a.Lex, a.Span
	}
	pathTok := p.expect(TString, "an import path string")
	d.Path, d.PathSpan = pathTok.Lex, pathTok.Span
	d.Span = tok.Span.union(pathTok.Span)
	p.endStmt()
	return d
}

func (p *Parser) fnDecl() Stmt {
	tok := p.advance() // fn
	name := p.expect(TIdent, "function name")
	lit := p.fnRest(name.Lex, tok.Span, false)
	return &FnDecl{Name: name.Lex, NameSpan: name.Span, Fn: lit, Span: lit.Span}
}

// methodDecl parses `fn (recv: Type) name(...) { ... }`; the receiver joins
// the parameter list first.
func (p *Parser) methodDecl() Stmt {
	tok := p.advance() // fn
	p.expect(TLParen, "'(' after fn in method")
	recv := p.expect(TIdent, "receiver name")
	p.expect(TColon, "':' after receiver name")
	rty := p.typeExpr()
	p.expect(TRParen, "')' after receiver")
	name := p.expect(TIdent, "method name")
	p.expect(TLParen, "'(' after method name")
	params := []string{recv.Lex}
	muts := []bool{false}
	pspans := []Span{recv.Span}
	paramTys := []TypeExpr{rty}
	p.skipSemis()
	for !p.check(TRParen) {
		isMut := p.match(TMut)
		ptok := p.expect(TIdent, "parameter name")
		params = append(params, ptok.Lex)
		muts = append(muts, isMut)
		pspans = append(pspans, ptok.Span)
		var pty TypeExpr
		if p.match(TColon) {
			pty = p.typeExpr()
		}
		paramTys = append(paramTys, pty)
		if !p.match(TComma) {
			break
		}
		p.skipSemis()
	}
	p.skipSemis()
	p.expect(TRParen, "')' after parameters")
	var retTy TypeExpr
	if p.match(TThinArrow) {
		retTy = p.typeExpr()
	}
	body := p.block()
	lit := &FnLit{Params: params, ParamMuts: muts, ParamSpans: pspans, ParamTys: paramTys,
		RetTy: retTy, Body: body, Name: name.Lex, Span: tok.Span.union(body.Span)}
	return &MethodDecl{RecvTy: rtyName(rty), Name: name.Lex, NameSpan: name.Span, Fn: lit, Span: lit.Span}
}

// rtyName extracts the receiver type name (the method dispatch key); ""
// for non-name types.
func rtyName(ty TypeExpr) string {
	if te, ok := ty.(*TEName); ok {
		return te.Name
	}
	return ""
}

// params, capture clause, and body, after 'fn [name]'; fnSpan is the span of
// the `fn` keyword. interfaceOnly takes the signature without a body.
func (p *Parser) fnRest(name string, fnSpan Span, interfaceOnly bool) *FnLit {
	p.expect(TLParen, "'(' after fn")
	var params []string
	var paramMuts []bool
	var paramSpans []Span
	var paramTys []TypeExpr
	p.skipSemis()
	for !p.check(TRParen) {
		mut := p.match(TMut)
		ptok := p.expect(TIdent, "parameter name")
		params = append(params, ptok.Lex)
		paramMuts = append(paramMuts, mut)
		paramSpans = append(paramSpans, ptok.Span)
		var pty TypeExpr
		if p.match(TColon) {
			pty = p.typeExpr()
		} else if interfaceOnly {
			p.fail(ptok.Span, "E1101", "interface parameters require explicit types",
				"expected ':' after interface parameter `%s`", ptok.Lex)
		}
		paramTys = append(paramTys, pty)
		if !p.match(TComma) {
			break
		}
		p.skipSemis()
	}
	p.skipSemis()
	rp := p.expect(TRParen, "')' after parameters")
	var retTy TypeExpr
	if p.match(TThinArrow) {
		retTy = p.typeExpr()
	} else if interfaceOnly {
		p.fail(rp.Span, "E1101", "interface functions require an explicit return type",
			"expected '->' after interface parameters")
	}
	var crefs []string
	if !interfaceOnly && p.match(TCapture) {
		crefs = p.captureList()
	}
	if interfaceOnly {
		sp := fnSpan
		if retTy != nil {
			sp = sp.union(retTy.span())
		}
		return &FnLit{Params: params, ParamMuts: paramMuts, ParamSpans: paramSpans,
			ParamTys: paramTys, RetTy: retTy, Body: nil, Name: name, Span: sp}
	}
	body := p.block()
	return &FnLit{Params: params, ParamMuts: paramMuts, ParamSpans: paramSpans,
		ParamTys: paramTys, RetTy: retTy, Body: body, Name: name, Crefs: crefs,
		Span: fnSpan.union(body.Span)}
}

// captureList parses `(ref a, b)`; only ref-captured names are recorded
// (value-captured items are the default and not recorded).
func (p *Parser) captureList() []string {
	p.expect(TLParen, "'(' after capture")
	var crefs []string
	p.skipSemis()
	for !p.check(TRParen) {
		isRef := p.check(TIdent) && p.peek().Lex == "ref"
		if isRef {
			p.advance()
		}
		ptok := p.expect(TIdent, "captured name")
		if isRef {
			crefs = append(crefs, ptok.Lex)
		}
		if !p.match(TComma) {
			break
		}
		p.skipSemis()
	}
	p.expect(TRParen, "')' after capture list")
	return crefs
}

func (p *Parser) enumDecl() Stmt {
	tok := p.advance() // enum
	name := p.expect(TIdent, "enum name")
	var params []string
	if p.match(TLParen) { // generic parameters: enum Box(a) { ... }
		for !p.check(TRParen) {
			params = append(params, p.expect(TIdent, "type parameter").Lex)
			if !p.match(TComma) {
				break
			}
		}
		p.expect(TRParen, "')' after type parameters")
	}
	p.expect(TLBrace, "'{' after enum name")
	var variants []EnumVariantDecl
	p.skipSemis()
	for !p.check(TRBrace) {
		vname := p.expect(TIdent, "variant name")
		v := EnumVariantDecl{Name: vname.Lex}
		vsp := vname.Span
		if p.match(TLParen) {
			for !p.check(TRParen) {
				v.Types = append(v.Types, p.typeExpr())
				if !p.match(TComma) {
					break
				}
			}
			vrp := p.expect(TRParen, "')' after variant field types")
			v.Arity = len(v.Types)
			vsp = vsp.union(vrp.Span)
		}
		variants = append(variants, v)
		_ = vsp
		if !p.match(TComma) && !p.check(TSemi) && !p.check(TRBrace) {
			p.fail(p.peek().Span, "E1104", "", "expected ',' or newline between enum variants")
		}
		p.skipSemis()
	}
	rb := p.expect(TRBrace, "'}' after enum variants")
	return &EnumDecl{Name: name.Lex, Params: params, Variants: variants,
		Span: tok.Span.union(rb.Span)}
}

func (p *Parser) typeAliasDecl(isPub bool) Stmt {
	tok := p.advance() // type
	name := p.expect(TIdent, "type alias name")
	p.expect(TEq, "'=' after type alias name")
	te := p.typeExpr()
	p.endStmt()
	return &TypeAliasDecl{Name: name.Lex, Ty: te, Pub: isPub, Span: tok.Span.union(te.span())}
}

// typeExpr := "[" type "]" | "record" "{" fields ["|" rowvar] "}" |
// "(" tuple ")" | "fn" "(" list ")" "->" type | NAME ["::" NAME] [":" constraint] ["(" args ")"]
func (p *Parser) typeExpr() TypeExpr {
	tok := p.peek()
	switch tok.Type {
	case TLBracket:
		p.advance()
		el := p.typeExpr()
		rb := p.expect(TRBracket, "']' in list type")
		return &TEList{Elem: el, Span: tok.Span.union(rb.Span)}
	case TRecord:
		p.advance()
		p.expect(TLBrace, "'{' after `record` in type")
		var names []string
		var types []TypeExpr
		var rowVar TypeExpr
		p.skipSemis()
		for !p.check(TRBrace) {
			if p.match(TBar) {
				rv := p.expect(TIdent, "row variable after '|'")
				rowVar = &TEName{Name: rv.Lex, Span: rv.Span}
				break
			}
			fname := p.expect(TIdent, "field name in record type")
			p.expect(TColon, "':' after field name")
			ft := p.typeExpr()
			names = append(names, fname.Lex)
			types = append(types, ft)
			if !p.match(TComma) {
				if p.match(TBar) {
					rv := p.expect(TIdent, "row variable after '|'")
					rowVar = &TEName{Name: rv.Lex, Span: rv.Span}
				}
				break
			}
			p.skipSemis()
		}
		rb := p.expect(TRBrace, "'}' after record type fields")
		return &TERecord{Names: names, Types: types, RowVar: rowVar, Span: tok.Span.union(rb.Span)}
	case TLParen:
		lp := p.advance()
		var ets []TypeExpr
		for !p.check(TRParen) {
			ets = append(ets, p.typeExpr())
			if !p.match(TComma) {
				break
			}
		}
		rp := p.expect(TRParen, "')' in tuple type")
		return &TETuple{Types: ets, Span: lp.Span.union(rp.Span)}
	case TFn:
		p.advance()
		p.expect(TLParen, "'(' in fn type")
		var ps []TypeExpr
		for !p.check(TRParen) {
			ps = append(ps, p.typeExpr())
			if !p.match(TComma) {
				break
			}
		}
		p.expect(TRParen, "')' in fn type")
		p.expect(TThinArrow, "'->' in fn type")
		ret := p.typeExpr()
		return &TEFn{Params: ps, Ret: ret, Span: tok.Span.union(ret.span())}
	case TIdent:
		p.advance()
		te := &TEName{Name: tok.Lex, Span: tok.Span}
		if p.match(TColonColon) { // qualified type: pkg::Name
			n := p.expect(TIdent, "a type name after '::'")
			te.Pkg, te.Name = tok.Lex, n.Lex
			te.Span = te.Span.union(n.Span)
		}
		if p.match(TColon) { // type constraint: Name: Constraint
			ctok := p.expect(TIdent, "a type constraint after ':'")
			te.Constraint = ctok.Lex
			te.Span = te.Span.union(ctok.Span)
		}
		if p.match(TLParen) {
			for !p.check(TRParen) {
				te.Args = append(te.Args, p.typeExpr())
				if !p.match(TComma) {
					break
				}
			}
			rp := p.expect(TRParen, "')' in type arguments")
			te.Span = te.Span.union(rp.Span)
		}
		return te
	}
	p.fail(tok.Span, "E1105", "", "expected a type, got %q", tok.Lex)
	return nil
}

func (p *Parser) whileStmt() Stmt {
	tok := p.advance()
	cond := p.expression()
	body := p.block()
	return &WhileStmt{Cond: cond, Body: body, Span: tok.Span.union(body.Span)}
}

// forStmt parses `for v in xs` and `for i, v in xs` (channels reject the
// index form, checked later).
func (p *Parser) forStmt() Stmt {
	tok := p.advance()
	first := p.expect(TIdent, "loop variable")
	v := first.Lex
	vsp := first.Span
	var idx string
	var isp Span
	if p.match(TComma) {
		idx = v
		isp = vsp
		e := p.expect(TIdent, "element variable after ','")
		v = e.Lex
		vsp = e.Span
	}
	p.expect(TIn, "'in' in for loop")
	iter := p.expression()
	body := p.block()
	return &ForStmt{Var: v, VarSpan: vsp, Idx: idx, IdxSpan: isp, Iter: iter, Body: body,
		Span: tok.Span.union(body.Span)}
}

func (p *Parser) spawnStmt() Stmt {
	tok := p.advance()
	e := p.expression()
	call, ok := e.(*Call)
	if !ok {
		p.fail(tok.Span, "E1106", "", "spawn expects a function call, e.g. `spawn worker(ch)`")
	}
	p.endStmt()
	return &SpawnStmt{CallE: call, Span: tok.Span.union(call.Span)}
}

// deferStmt parses `defer { ... }` with an optional capture clause.
func (p *Parser) deferStmt() Stmt {
	tok := p.advance()
	if !p.check(TLBrace) && !p.check(TCapture) {
		p.fail(p.peek().Span, "E1119", "defer takes a block, e.g. `defer { close(ch) }`",
			"expected '{' after defer, got %q", p.peek().Lex)
	}
	var crefs []string
	if p.match(TCapture) {
		crefs = p.captureList()
	}
	body := p.block()
	return &DeferStmt{Body: body, Crefs: crefs, Span: tok.Span.union(body.Span)}
}

// selectStmt parses `select { arm, arm, default => body }`. Each arm is a
// receive (`v = <-ch => body` / `<-ch => body`), a send (`ch <- v => body`),
// or `default => body` (at most one).
func (p *Parser) selectStmt() Stmt {
	tok := p.advance() // select
	p.expect(TLBrace, "'{' after select")
	sel := &SelectStmt{Span: tok.Span}
	p.skipSemis()
	for !p.check(TRBrace) && !p.check(TEOF) {
		if p.check(TIdent) && p.peek().Lex == "default" {
			d := p.advance()
			if sel.HasDefault {
				p.fail(d.Span, "E1115", "", "`select` may have at most one `default` arm")
			}
			p.expect(TArrow, "'=>' after default")
			p.skipSemis()
			sel.HasDefault = true
			sel.DefaultSpan = d.Span
			sel.Default = p.expression()
		} else {
			sel.Arms = append(sel.Arms, p.selectArm())
		}
		if !p.match(TComma) && !p.check(TSemi) && !p.check(TRBrace) {
			p.fail(p.peek().Span, "E1104", "", "expected ',' or newline between select arms")
		}
		p.skipSemis()
	}
	rb := p.expect(TRBrace, "'}' after select arms")
	sel.Span = tok.Span.union(rb.Span)
	if len(sel.Arms) == 0 && !sel.HasDefault {
		p.fail(sel.Span, "E1116", "", "`select` needs at least one communication arm")
	}
	return sel
}

func (p *Parser) selectArm() SelectArm {
	arm := SelectArm{}
	if p.check(TLArrow) { // <-ch => body  (receive, value discarded)
		start := p.advance()
		arm.Chan = p.expression()
		arm.Span = start.Span
	} else {
		e := p.expression()
		arm.Span = e.span()
		if p.match(TEq) { // v = <-ch => body  (receive, bound)
			id, ok := e.(*Ident)
			if !ok {
				p.fail(e.span(), "E1117", "", "only a name can bind a received value: `name = <-ch`")
			}
			arm.Bind, arm.BindSpan = id.Name, id.Span
			p.expect(TLArrow, "'<-' after `=` in a select receive arm")
			arm.Chan = p.expression()
		} else if p.match(TLArrow) { // ch <- v => body  (send)
			arm.IsSend = true
			arm.Chan = e
			arm.Val = p.expression()
		} else {
			p.fail(p.peek().Span, "E1118", "a select arm is a receive (`v = <-ch`), a send (`ch <- v`), or `default`",
				"expected `=`, `<-`, or `=>` in this select arm")
		}
	}
	p.expect(TArrow, "'=>' after a select arm")
	p.skipSemis()
	arm.Body = p.expression()
	arm.Span = arm.Span.union(arm.Body.span())
	return arm
}

func (p *Parser) block() *Block {
	lb := p.expect(TLBrace, "'{'")
	b := &Block{Span: lb.Span}
	p.skipSemis()
	for !p.check(TRBrace) && !p.check(TEOF) {
		b.Stmts = append(b.Stmts, p.statement())
		p.skipSemis()
	}
	rb := p.expect(TRBrace, "'}'")
	b.Span = b.Span.union(rb.Span)
	return b
}

// ---- expressions (precedence climbing) ----

func (p *Parser) expression() Expr { return p.pipeExpr() }

// pipeExpr is the entry: the lowest-precedence operator is the pipe
// (left-assoc; the target is a name path, optionally with arguments).
func (p *Parser) pipeExpr() Expr {
	e := p.orExpr()
	for p.check(TPipe) {
		p.advance()
		e = p.pipeTarget(e)
	}
	return e
}

func (p *Parser) pipeTarget(lhs Expr) Expr {
	first := p.expect(TIdent, "a pipe target")
	path := Expr(&Ident{Name: first.Lex, Span: first.Span})
	for p.match(TColonColon) {
		part := p.expect(TIdent, "a name after '::' in pipe target")
		switch head := path.(type) {
		case *Ident:
			path = &VariantAccess{EnumName: head.Name, Variant: part.Lex, Span: head.Span.union(part.Span)}
		case *VariantAccess:
			path = &QualVariantAccess{Pkg: head.EnumName, Enum: head.Variant, Variant: part.Lex,
				Span: head.Span.union(part.Span)}
		default:
			p.fail(part.Span, "E1107", "", "a pipe target path has at most three names")
		}
	}
	var args []Expr
	endSp := path.span()
	if p.match(TLParen) {
		p.skipSemis()
		for !p.check(TRParen) {
			args = append(args, p.expression())
			if !p.match(TComma) {
				break
			}
			p.skipSemis()
		}
		p.skipSemis()
		rp := p.expect(TRParen, "')' after pipe target arguments")
		endSp = endSp.union(rp.Span)
	}
	return &Pipe{Lhs: lhs, Target: path, Args: args, Span: lhs.span().union(endSp)}
}

func (p *Parser) orExpr() Expr {
	e := p.andExpr()
	for p.check(TOrOr) {
		p.advance()
		rhs := p.andExpr()
		e = &Logical{Op: TOrOr, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
	}
	if p.match(TDotDot) { // a..b range literal desugars to range(a, b)
		rhs := p.andExpr()
		sp := e.span().union(rhs.span())
		e = &Call{Callee: &Ident{Name: "range", Span: sp}, Args: []Expr{e, rhs}, Span: sp}
	}
	return e
}

func (p *Parser) andExpr() Expr {
	e := p.equality()
	for p.check(TAndAnd) {
		p.advance()
		rhs := p.equality()
		e = &Logical{Op: TAndAnd, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
	}
	return e
}

func (p *Parser) equality() Expr {
	e := p.comparison()
	for p.check(TEqEq) || p.check(TBangEq) {
		op := p.advance()
		rhs := p.comparison()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
	}
	return e
}

func (p *Parser) comparison() Expr {
	e := p.term()
	for p.check(TLt) || p.check(TLtEq) || p.check(TGt) || p.check(TGtEq) {
		op := p.advance()
		rhs := p.term()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
	}
	return e
}

func (p *Parser) term() Expr {
	e := p.factor()
	for p.check(TPlus) || p.check(TMinus) {
		op := p.advance()
		rhs := p.factor()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
	}
	return e
}

func (p *Parser) factor() Expr {
	e := p.unary()
	for p.check(TStar) || p.check(TSlash) || p.check(TPercent) {
		op := p.advance()
		rhs := p.unary()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
	}
	return e
}

func (p *Parser) unary() Expr {
	switch {
	case p.check(TBang), p.check(TMinus):
		op := p.advance()
		rhs := p.unary()
		return &Unary{Op: op.Type, Rhs: rhs, Span: op.Span.union(rhs.span())}
	case p.check(TLArrow): // <-ch  (receive)
		op := p.advance()
		rhs := p.unary()
		return &RecvExpr{Chan: rhs, Span: op.Span.union(rhs.span())}
	}
	return p.postfix()
}

// postfix parses postfix chains: calls, indexing, try, :: qualification,
// field access, record updates.
func (p *Parser) postfix() Expr {
	e := p.primary()
	for {
		switch {
		case p.check(TLParen):
			p.advance()
			var args []Expr
			p.skipSemis()
			for !p.check(TRParen) {
				args = append(args, p.expression())
				if !p.match(TComma) {
					break
				}
				p.skipSemis()
			}
			p.skipSemis()
			rp := p.expect(TRParen, "')' after arguments")
			e = &Call{Callee: e, Args: args, Span: e.span().union(rp.Span)}
		case p.check(TLBracket):
			p.advance()
			idx := p.expression()
			rb := p.expect(TRBracket, "']' after index")
			e = &Index{Target: e, Idx: idx, Span: e.span().union(rb.Span)}
		case p.check(TQuestion):
			q := p.advance()
			e = &TryExpr{Inner: e, Span: e.span().union(q.Span)}
		case p.check(TColonColon):
			p.advance()
			switch head := e.(type) {
			case *Ident:
				v := p.expect(TIdent, "a name after '::'")
				e = &VariantAccess{EnumName: head.Name, Variant: v.Lex, Span: head.Span.union(v.Span)}
			case *VariantAccess: // third segment: pkg::Enum::Variant
				v := p.expect(TIdent, "a name after '::'")
				e = &QualVariantAccess{Pkg: head.EnumName, Enum: head.Variant, Variant: v.Lex,
					Span: head.Span.union(v.Span)}
			default:
				p.fail(p.peek().Span, "E1120", "`::` qualifies a package or enum name; write `pkg::name` or `Shape::Variant`",
					"'::' cannot follow this expression")
			}
		case p.check(TDot):
			p.advance()
			v := p.expect(TIdent, "a field name after '.'")
			e = &FieldAccess{Base: e, Field: v.Lex, Span: e.span().union(v.Span)}
		default:
			return e
		}
	}
}

// litToInt converts a number literal lexeme to int (hex/binary/decimal,
// underscore separators, overflow-checked); ok=false on overflow.
func litToInt(lexeme string) (int64, bool) {
	l := len(lexeme)
	if l > 2 && lexeme[:2] == "0x" {
		var v int64
		for i := 2; i < l; i++ {
			c := lexeme[i]
			if c == '_' {
				continue
			}
			var d int64
			switch {
			case c >= '0' && c <= '9':
				d = int64(c - '0')
			case c >= 'a' && c <= 'f':
				d = int64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = int64(c-'A') + 10
			default:
				return 0, false
			}
			if d < 0 || v > (9223372036854775807-d)/16 {
				return 0, false
			}
			v = v*16 + d
		}
		return v, true
	}
	if l > 2 && lexeme[:2] == "0b" {
		var v int64
		for i := 2; i < l; i++ {
			c := lexeme[i]
			if c == '_' {
				continue
			}
			var d int64
			if c == '0' || c == '1' {
				d = int64(c - '0')
			} else {
				return 0, false
			}
			if v > (9223372036854775807-d)/2 {
				return 0, false
			}
			v = v*2 + d
		}
		return v, true
	}
	var clean []byte
	for i := 0; i < l; i++ {
		if lexeme[i] != '_' {
			clean = append(clean, lexeme[i])
		}
	}
	n, err := strconv.ParseInt(string(clean), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (p *Parser) primary() Expr {
	tok := p.peek()
	switch tok.Type {
	case TInt:
		p.advance()
		n, ok := litToInt(tok.Lex)
		if !ok {
			p.fail(tok.Span, "E1108", "i64 holds -9223372036854775808..9223372036854775807", "bad integer literal %q", tok.Lex)
		}
		return &IntLit{Val: n, Span: tok.Span}
	case TFloat:
		p.advance()
		f, err := strconv.ParseFloat(tok.Lex, 64)
		if err != nil {
			p.fail(tok.Span, "E1108", "", "bad float literal %q", tok.Lex)
		}
		return &FloatLit{Val: f, Span: tok.Span}
	case TString:
		p.advance()
		return &StrLit{Val: tok.Lex, Span: tok.Span}
	case TInterpStart:
		return p.interpolated()
	case TTrue:
		p.advance()
		return &BoolLit{Val: true, Span: tok.Span}
	case TFalse:
		p.advance()
		return &BoolLit{Val: false, Span: tok.Span}
	case TRecord:
		return p.recordLit()
	case TIdent:
		p.advance()
		return &Ident{Name: tok.Lex, Span: tok.Span}
	case TLParen:
		lp := p.advance()
		e := p.expression()
		if p.match(TComma) { // tuple literal
			elems := []Expr{e}
			p.skipSemis()
			for !p.check(TRParen) {
				elems = append(elems, p.expression())
				if !p.match(TComma) {
					break
				}
				p.skipSemis()
			}
			rp := p.expect(TRParen, "')' after tuple")
			return &TupleLit{Elems: elems, Span: lp.Span.union(rp.Span)}
		}
		p.expect(TRParen, "')'")
		return e
	case TLBracket:
		p.advance()
		var elems []Expr
		p.skipSemis()
		for !p.check(TRBracket) {
			elems = append(elems, p.expression())
			if !p.match(TComma) {
				break
			}
			p.skipSemis()
		}
		p.skipSemis()
		rb := p.expect(TRBracket, "']' after list elements")
		return &ListLit{Elems: elems, Span: tok.Span.union(rb.Span)}
	case TFn:
		p.advance()
		return p.fnRest("", tok.Span, false)
	case TIf:
		return p.ifExpr()
	case TMatch:
		return p.matchExpr()
	case TLBrace:
		return p.block()
	}
	p.fail(tok.Span, "E1109", "", "unexpected token %q", tok.Lex)
	return nil
}

// recordLit parses `record { a: 1, b: 2 }` and the update form
// `record { base | a: 1 }`.
func (p *Parser) recordLit() Expr {
	tok := p.advance() // record
	p.expect(TLBrace, "'{' after `record`")
	p.skipSemis()
	if p.check(TRBrace) {
		rb := p.advance()
		return &RecordLit{Span: tok.Span.union(rb.Span)}
	}
	firstName := p.expect(TIdent, "field name in record literal")
	if p.match(TBar) { // record update: base | fields
		base := Expr(&Ident{Name: firstName.Lex, Span: firstName.Span})
		var names []string
		var vals []Expr
		p.skipSemis()
		for !p.check(TRBrace) {
			fname := p.expect(TIdent, "field name in record update")
			p.expect(TColon, "':' after field name")
			fv := p.expression()
			names = append(names, fname.Lex)
			vals = append(vals, fv)
			if !p.match(TComma) {
				break
			}
			p.skipSemis()
		}
		rb := p.expect(TRBrace, "'}' after record update fields")
		return &RecordUpdate{Base: base, Names: names, Vals: vals, Span: tok.Span.union(rb.Span)}
	}
	p.expect(TColon, "':' after field name")
	firstVal := p.expression()
	names := []string{firstName.Lex}
	vals := []Expr{firstVal}
	if p.match(TComma) {
		p.skipSemis()
		for !p.check(TRBrace) {
			fname := p.expect(TIdent, "field name in record literal")
			p.expect(TColon, "':' after field name")
			fv := p.expression()
			names = append(names, fname.Lex)
			vals = append(vals, fv)
			if !p.match(TComma) {
				break
			}
			p.skipSemis()
		}
	}
	rb := p.expect(TRBrace, "'}' after record fields")
	return &RecordLit{Names: names, Vals: vals, Span: tok.Span.union(rb.Span)}
}

// interpolated parses an interpolated string: segments and expressions
// become a + chain.
func (p *Parser) interpolated() Expr {
	start := p.advance()
	out := Expr(&StrLit{Val: start.Lex, Span: start.Span})
	done := false
	for !done {
		inner := p.expression()
		out = &Binary{Op: TPlus, Lhs: out, Rhs: inner, Span: out.span().union(inner.span())}
		p.skipSemis()
		next := p.peek()
		var part Token
		switch next.Type {
		case TInterpMid, TInterpEnd:
			part = p.advance()
		default:
			p.fail(next.Span, "E1101", "close the interpolation with '}'",
				"expected the end of a string interpolation, got %q", next.Lex)
		}
		out = &Binary{Op: TPlus, Lhs: out, Rhs: &StrLit{Val: part.Lex, Span: part.Span},
			Span: start.Span.union(part.Span)}
		done = next.Type == TInterpEnd
	}
	return out
}

func (p *Parser) ifExpr() Expr {
	tok := p.advance() // if
	cond := p.expression()
	then := p.block()
	sp := tok.Span.union(then.Span)
	var els Expr
	if p.match(TElse) {
		if p.check(TIf) {
			els = p.ifExpr()
		} else {
			els = p.block()
		}
		sp = sp.union(els.span())
	}
	return &IfExpr{Cond: cond, Then: then, Else: els, Span: sp}
}

func (p *Parser) matchExpr() Expr {
	tok := p.advance() // match
	scrut := p.expression()
	p.expect(TLBrace, "'{' after match scrutinee")
	m := &MatchExpr{Scrut: scrut, Span: tok.Span}
	p.skipSemis()
	for !p.check(TRBrace) && !p.check(TEOF) {
		pat := p.pattern()
		pats := []Pattern{pat}
		for p.match(TBar) { // or-patterns expand into separate arms
			pats = append(pats, p.pattern())
		}
		hasGuard := p.match(TIf)
		var guard Expr
		if hasGuard {
			guard = p.expression()
		}
		if hasGuard {
			p.expect(TArrow, "'=>' after match guard")
		} else {
			p.expect(TArrow, "'=>' after pattern")
		}
		p.skipSemis()
		body := p.expression()
		for _, pt := range pats {
			m.Arms = append(m.Arms, MatchArm{Pat: pt, Guard: guard, Body: body, Span: pt.span().union(body.span())})
		}
		if !p.match(TComma) && !p.check(TSemi) && !p.check(TRBrace) {
			p.fail(p.peek().Span, "E1104", "", "expected ',' or newline between match arms")
		}
		p.skipSemis()
	}
	rb := p.expect(TRBrace, "'}' after match arms")
	m.Span = m.Span.union(rb.Span)
	if len(m.Arms) == 0 {
		p.fail(m.Span, "E1110", "", "match must have at least one arm")
	}
	return m
}

// pattern parses patterns: record, literals, negated numbers, identifiers
// (wildcard, binding, variant), variants with field binds.
func (p *Parser) pattern() Pattern {
	tok := p.peek()
	switch tok.Type {
	case TRecord:
		rk := p.advance()
		p.expect(TLBrace, "'{' after record pattern")
		var names []string
		p.skipSemis()
		for !p.check(TRBrace) {
			fname := p.expect(TIdent, "field name in record pattern")
			names = append(names, fname.Lex)
			if !p.match(TComma) {
				break
			}
			p.skipSemis()
		}
		rb := p.expect(TRBrace, "'}' after record pattern fields")
		return &PatRecord{Names: names, Span: rk.Span.union(rb.Span)}
	case TInt, TFloat, TString, TTrue, TFalse:
		lit := p.primary()
		return &PatLiteral{Val: lit, Span: lit.span()}
	case TMinus: // negative number literal
		p.advance()
		lit := p.primary()
		switch l := lit.(type) {
		case *IntLit:
			l.Val = -l.Val
		case *FloatLit:
			l.Val = -l.Val
		default:
			p.fail(tok.Span, "E1111", "", "'-' in pattern must precede a number")
		}
		return &PatLiteral{Val: lit, Span: tok.Span.union(lit.span())}
	case TIdent:
		p.advance()
		if tok.Lex == "_" {
			return &PatWildcard{Span: tok.Span}
		}
		pkg, enumName, variant := "", "", tok.Lex
		sp := tok.Span
		if p.match(TColonColon) {
			enumName = tok.Lex
			vtok := p.expect(TIdent, "variant name after '::'")
			variant = vtok.Lex
			sp = sp.union(vtok.Span)
			if p.match(TColonColon) { // third segment: pkg::Enum::Variant
				pkg, enumName = enumName, variant
				vtok = p.expect(TIdent, "variant name after '::'")
				variant = vtok.Lex
				sp = sp.union(vtok.Span)
			}
		}
		if p.match(TLParen) {
			pv := &PatVariant{Pkg: pkg, EnumName: enumName, Variant: variant}
			for !p.check(TRParen) {
				sub := p.pattern()
				switch sub.(type) {
				case *PatBinding, *PatWildcard:
				default:
					p.fail(sub.span(), "E1112", "", "variant fields may only bind names or use '_'")
				}
				pv.Binds = append(pv.Binds, sub)
				if !p.match(TComma) {
					break
				}
			}
			rp := p.expect(TRParen, "')' after variant pattern")
			pv.Span = sp.union(rp.Span)
			return pv
		}
		if enumName != "" {
			return &PatVariant{Pkg: pkg, EnumName: enumName, Variant: variant, Span: sp}
		}
		// bare name: binding, or a nullary variant like None ??compiler decides
		return &PatBinding{Name: variant, Span: sp}
	}
	p.fail(tok.Span, "E1113", "", "invalid pattern %q", tok.Lex)
	return nil
}
