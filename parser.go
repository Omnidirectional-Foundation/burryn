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

// statementRecover parses one statement, catching a parse failure anywhere
// inside it and returning it as a diagnostic.
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
			case TLet, TFn, TEnum, TWhile, TFor, TReturn, TSpawn, TBreak, TContinue:
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
	switch {
	case p.check(TLet):
		return p.letStmt()
	case p.check(TFn) && p.toks[p.pos+1].Type == TIdent:
		return p.fnDecl()
	case p.check(TEnum):
		return p.enumDecl()
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
	case p.check(TBreak):
		tok := p.advance()
		p.endStmt()
		return &BreakStmt{Span: tok.Span}
	case p.check(TContinue):
		tok := p.advance()
		p.endStmt()
		return &ContinueStmt{Span: tok.Span}
	}
	// expression statement / assignment / send
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
	if p.match(TLArrow) {
		val := p.expression()
		p.endStmt()
		return &SendStmt{Chan: e, Val: val, Span: e.span().union(val.span())}
	}
	p.endStmt()
	return &ExprStmt{E: e, Span: e.span()}
}

func (p *Parser) letStmt() Stmt {
	tok := p.advance() // let
	mut := p.match(TMut)
	name := p.expect(TIdent, "variable name")
	p.expect(TEq, "'=' in let binding")
	init := p.expression()
	p.endStmt()
	return &LetStmt{Name: name.Lex, NameSpan: name.Span, Mut: mut, Init: init,
		Span: tok.Span.union(init.span())}
}

func (p *Parser) fnDecl() Stmt {
	tok := p.advance() // fn
	name := p.expect(TIdent, "function name")
	lit := p.fnRest(name.Lex, tok.Span)
	return &FnDecl{Name: name.Lex, NameSpan: name.Span, Fn: lit, Span: lit.Span}
}

// params and body, after 'fn [name]'; fnSpan is the span of the `fn` keyword
func (p *Parser) fnRest(name string, fnSpan Span) *FnLit {
	p.expect(TLParen, "'(' after fn")
	var params []string
	var paramSpans []Span
	p.skipSemis()
	for !p.check(TRParen) {
		ptok := p.expect(TIdent, "parameter name")
		params = append(params, ptok.Lex)
		paramSpans = append(paramSpans, ptok.Span)
		if !p.match(TComma) {
			break
		}
		p.skipSemis()
	}
	p.skipSemis()
	p.expect(TRParen, "')' after parameters")
	body := p.block()
	return &FnLit{Params: params, ParamSpans: paramSpans, Body: body, Name: name,
		Span: fnSpan.union(body.Span)}
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
		if p.match(TLParen) {
			for !p.check(TRParen) {
				v.Types = append(v.Types, p.typeExpr())
				if !p.match(TComma) {
					break
				}
			}
			p.expect(TRParen, "')' after variant field types")
			v.Arity = len(v.Types)
		}
		variants = append(variants, v)
		if !p.match(TComma) && !p.check(TSemi) && !p.check(TRBrace) {
			p.fail(p.peek().Span, "E1104", "", "expected ',' or newline between enum variants")
		}
		p.skipSemis()
	}
	rb := p.expect(TRBrace, "'}' after enum variants")
	return &EnumDecl{Name: name.Lex, Params: params, Variants: variants,
		Span: tok.Span.union(rb.Span)}
}

// typeExpr := "[" typeExpr "]" | "fn" "(" list ")" "->" typeExpr | IDENT ["(" list ")"]
func (p *Parser) typeExpr() TypeExpr {
	tok := p.peek()
	switch tok.Type {
	case TLBracket:
		p.advance()
		el := p.typeExpr()
		rb := p.expect(TRBracket, "']' in list type")
		return &TEList{Elem: el, Span: tok.Span.union(rb.Span)}
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

func (p *Parser) forStmt() Stmt {
	tok := p.advance()
	v := p.expect(TIdent, "loop variable")
	p.expect(TIn, "'in' in for loop")
	iter := p.expression()
	body := p.block()
	return &ForStmt{Var: v.Lex, VarSpan: v.Span, Iter: iter, Body: body,
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

func (p *Parser) expression() Expr { return p.orExpr() }

func (p *Parser) orExpr() Expr {
	e := p.andExpr()
	for p.check(TOrOr) {
		p.advance()
		rhs := p.andExpr()
		e = &Logical{Op: TOrOr, Lhs: e, Rhs: rhs, Span: e.span().union(rhs.span())}
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
		case p.check(TDot):
			d := p.advance()
			id, ok := e.(*Ident)
			if !ok {
				p.fail(d.Span, "E1107", "", "'.' is only used for enum variants (Enum.Variant)")
			}
			v := p.expect(TIdent, "variant name after '.'")
			e = &VariantAccess{EnumName: id.Name, Variant: v.Lex, Span: id.Span.union(v.Span)}
		default:
			return e
		}
	}
}

func (p *Parser) primary() Expr {
	tok := p.peek()
	switch tok.Type {
	case TInt:
		p.advance()
		n, err := strconv.ParseInt(tok.Lex, 10, 64)
		if err != nil {
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
	case TTrue:
		p.advance()
		return &BoolLit{Val: true, Span: tok.Span}
	case TFalse:
		p.advance()
		return &BoolLit{Val: false, Span: tok.Span}
	case TIdent:
		p.advance()
		return &Ident{Name: tok.Lex, Span: tok.Span}
	case TLParen:
		p.advance()
		e := p.expression()
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
		return p.fnRest("", tok.Span)
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
		p.expect(TArrow, "'=>' after pattern")
		p.skipSemis()
		body := p.expression()
		m.Arms = append(m.Arms, MatchArm{Pat: pat, Body: body, Span: pat.span().union(body.span())})
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

func (p *Parser) pattern() Pattern {
	tok := p.peek()
	switch tok.Type {
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
		enumName, variant := "", tok.Lex
		sp := tok.Span
		if p.match(TDot) {
			enumName = tok.Lex
			vtok := p.expect(TIdent, "variant name after '.'")
			variant = vtok.Lex
			sp = sp.union(vtok.Span)
		}
		if p.match(TLParen) {
			pv := &PatVariant{EnumName: enumName, Variant: variant}
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
			return &PatVariant{EnumName: enumName, Variant: variant, Span: sp}
		}
		// bare name: binding, or a nullary variant like None ??compiler decides
		return &PatBinding{Name: variant, Span: sp}
	}
	p.fail(tok.Span, "E1113", "", "invalid pattern %q", tok.Lex)
	return nil
}
