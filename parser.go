package main

import (
	"fmt"
	"strconv"
)

type Parser struct {
	toks []Token
	pos  int
}

func parse(toks []Token) (stmts []Stmt, err error) {
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(parseErr); ok {
				err = fmt.Errorf("%s", string(pe))
				return
			}
			panic(r)
		}
	}()
	p := &Parser{toks: toks}
	p.skipSemis()
	for !p.check(TEOF) {
		stmts = append(stmts, p.statement())
		p.skipSemis()
	}
	return stmts, nil
}

type parseErr string

func (p *Parser) fail(line int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	panic(parseErr(fmt.Sprintf("parse error at line %d: %s", line, msg)))
}

func (p *Parser) peek() Token     { return p.toks[p.pos] }
func (p *Parser) prev() Token     { return p.toks[p.pos-1] }
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
	p.fail(tok.Line, "expected %s, got %q", what, tok.Lex)
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
	p.fail(tok.Line, "expected end of statement, got %q", tok.Lex)
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
		var val Expr
		if !p.check(TSemi) && !p.check(TRBrace) && !p.check(TEOF) {
			val = p.expression()
		}
		p.endStmt()
		return &ReturnStmt{Val: val, Line: tok.Line, Col: tok.Col}
	case p.check(TSpawn):
		return p.spawnStmt()
	case p.check(TBreak):
		tok := p.advance()
		p.endStmt()
		return &BreakStmt{Line: tok.Line, Col: tok.Col}
	case p.check(TContinue):
		tok := p.advance()
		p.endStmt()
		return &ContinueStmt{Line: tok.Line, Col: tok.Col}
	}
	// expression statement / assignment / send
	first := p.peek()
	e := p.expression()
	line, col := first.Line, first.Col
	if p.match(TEq) {
		val := p.expression()
		switch e.(type) {
		case *Ident, *Index:
		default:
			p.fail(line, "invalid assignment target")
		}
		p.endStmt()
		return &AssignStmt{Target: e, Val: val, Line: line, Col: col}
	}
	if p.match(TLArrow) {
		val := p.expression()
		p.endStmt()
		return &SendStmt{Chan: e, Val: val, Line: line, Col: col}
	}
	p.endStmt()
	return &ExprStmt{E: e, Line: line, Col: col}
}

func (p *Parser) letStmt() Stmt {
	tok := p.advance() // let
	mut := p.match(TMut)
	name := p.expect(TIdent, "variable name")
	p.expect(TEq, "'=' in let binding")
	init := p.expression()
	p.endStmt()
	return &LetStmt{Name: name.Lex, Mut: mut, Init: init, Line: tok.Line, Col: tok.Col}
}

func (p *Parser) fnDecl() Stmt {
	tok := p.advance() // fn
	name := p.expect(TIdent, "function name")
	lit := p.fnRest(name.Lex, tok.Line, tok.Col)
	return &FnDecl{Name: name.Lex, Fn: lit, Line: tok.Line, Col: tok.Col}
}

// params and body, after 'fn [name]'
func (p *Parser) fnRest(name string, line, col int) *FnLit {
	p.expect(TLParen, "'(' after fn")
	var params []string
	p.skipSemis()
	for !p.check(TRParen) {
		params = append(params, p.expect(TIdent, "parameter name").Lex)
		if !p.match(TComma) {
			break
		}
		p.skipSemis()
	}
	p.skipSemis()
	p.expect(TRParen, "')' after parameters")
	body := p.block()
	return &FnLit{Params: params, Body: body, Name: name, Line: line, Col: col}
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
			p.fail(p.peek().Line, "expected ',' or newline between enum variants")
		}
		p.skipSemis()
	}
	p.expect(TRBrace, "'}' after enum variants")
	return &EnumDecl{Name: name.Lex, Params: params, Variants: variants, Line: tok.Line, Col: tok.Col}
}

// typeExpr := "[" typeExpr "]" | "fn" "(" list ")" "->" typeExpr | IDENT ["(" list ")"]
func (p *Parser) typeExpr() TypeExpr {
	tok := p.peek()
	switch tok.Type {
	case TLBracket:
		p.advance()
		el := p.typeExpr()
		p.expect(TRBracket, "']' in list type")
		return &TEList{Elem: el, Line: tok.Line, Col: tok.Col}
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
		return &TEFn{Params: ps, Ret: ret, Line: tok.Line, Col: tok.Col}
	case TIdent:
		p.advance()
		te := &TEName{Name: tok.Lex, Line: tok.Line, Col: tok.Col}
		if p.match(TLParen) {
			for !p.check(TRParen) {
				te.Args = append(te.Args, p.typeExpr())
				if !p.match(TComma) {
					break
				}
			}
			p.expect(TRParen, "')' in type arguments")
		}
		return te
	}
	p.fail(tok.Line, "expected a type, got %q", tok.Lex)
	return nil
}

func (p *Parser) whileStmt() Stmt {
	tok := p.advance()
	cond := p.expression()
	body := p.block()
	return &WhileStmt{Cond: cond, Body: body, Line: tok.Line, Col: tok.Col}
}

func (p *Parser) forStmt() Stmt {
	tok := p.advance()
	v := p.expect(TIdent, "loop variable")
	p.expect(TIn, "'in' in for loop")
	iter := p.expression()
	body := p.block()
	return &ForStmt{Var: v.Lex, Iter: iter, Body: body, Line: tok.Line, Col: tok.Col}
}

func (p *Parser) spawnStmt() Stmt {
	tok := p.advance()
	e := p.expression()
	call, ok := e.(*Call)
	if !ok {
		p.fail(tok.Line, "spawn expects a function call, e.g. `spawn worker(ch)`")
	}
	p.endStmt()
	return &SpawnStmt{CallE: call, Line: tok.Line, Col: tok.Col}
}

func (p *Parser) block() *Block {
	lb := p.expect(TLBrace, "'{'")
	b := &Block{Line: lb.Line, Col: lb.Col}
	p.skipSemis()
	for !p.check(TRBrace) && !p.check(TEOF) {
		b.Stmts = append(b.Stmts, p.statement())
		p.skipSemis()
	}
	p.expect(TRBrace, "'}'")
	return b
}

// ---- expressions (precedence climbing) ----

func (p *Parser) expression() Expr { return p.orExpr() }

func (p *Parser) orExpr() Expr {
	e := p.andExpr()
	for p.check(TOrOr) {
		op := p.advance()
		rhs := p.andExpr()
		e = &Logical{Op: TOrOr, Lhs: e, Rhs: rhs, Line: op.Line, Col: op.Col}
	}
	return e
}

func (p *Parser) andExpr() Expr {
	e := p.equality()
	for p.check(TAndAnd) {
		op := p.advance()
		rhs := p.equality()
		e = &Logical{Op: TAndAnd, Lhs: e, Rhs: rhs, Line: op.Line, Col: op.Col}
	}
	return e
}

func (p *Parser) equality() Expr {
	e := p.comparison()
	for p.check(TEqEq) || p.check(TBangEq) {
		op := p.advance()
		rhs := p.comparison()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Line: op.Line, Col: op.Col}
	}
	return e
}

func (p *Parser) comparison() Expr {
	e := p.term()
	for p.check(TLt) || p.check(TLtEq) || p.check(TGt) || p.check(TGtEq) {
		op := p.advance()
		rhs := p.term()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Line: op.Line, Col: op.Col}
	}
	return e
}

func (p *Parser) term() Expr {
	e := p.factor()
	for p.check(TPlus) || p.check(TMinus) {
		op := p.advance()
		rhs := p.factor()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Line: op.Line, Col: op.Col}
	}
	return e
}

func (p *Parser) factor() Expr {
	e := p.unary()
	for p.check(TStar) || p.check(TSlash) || p.check(TPercent) {
		op := p.advance()
		rhs := p.unary()
		e = &Binary{Op: op.Type, Lhs: e, Rhs: rhs, Line: op.Line, Col: op.Col}
	}
	return e
}

func (p *Parser) unary() Expr {
	switch {
	case p.check(TBang), p.check(TMinus):
		op := p.advance()
		rhs := p.unary()
		return &Unary{Op: op.Type, Rhs: rhs, Line: op.Line, Col: op.Col}
	case p.check(TLArrow): // <-ch  (receive)
		op := p.advance()
		rhs := p.unary()
		return &RecvExpr{Chan: rhs, Line: op.Line, Col: op.Col}
	}
	return p.postfix()
}

func (p *Parser) postfix() Expr {
	e := p.primary()
	for {
		switch {
		case p.check(TLParen):
			lp := p.advance()
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
			p.expect(TRParen, "')' after arguments")
			e = &Call{Callee: e, Args: args, Line: lp.Line, Col: lp.Col}
		case p.check(TLBracket):
			lb := p.advance()
			idx := p.expression()
			p.expect(TRBracket, "']' after index")
			e = &Index{Target: e, Idx: idx, Line: lb.Line, Col: lb.Col}
		case p.check(TQuestion):
			q := p.advance()
			e = &TryExpr{Inner: e, Line: q.Line, Col: q.Col}
		case p.check(TDot):
			d := p.advance()
			id, ok := e.(*Ident)
			if !ok {
				p.fail(d.Line, "'.' is only used for enum variants (Enum.Variant)")
			}
			v := p.expect(TIdent, "variant name after '.'")
			e = &VariantAccess{EnumName: id.Name, Variant: v.Lex, Line: d.Line, Col: d.Col}
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
			p.fail(tok.Line, "bad integer literal %q", tok.Lex)
		}
		return &IntLit{Val: n, Line: tok.Line, Col: tok.Col}
	case TFloat:
		p.advance()
		f, err := strconv.ParseFloat(tok.Lex, 64)
		if err != nil {
			p.fail(tok.Line, "bad float literal %q", tok.Lex)
		}
		return &FloatLit{Val: f, Line: tok.Line, Col: tok.Col}
	case TString:
		p.advance()
		return &StrLit{Val: tok.Lex, Line: tok.Line, Col: tok.Col}
	case TTrue:
		p.advance()
		return &BoolLit{Val: true, Line: tok.Line, Col: tok.Col}
	case TFalse:
		p.advance()
		return &BoolLit{Val: false, Line: tok.Line, Col: tok.Col}
	case TIdent:
		p.advance()
		return &Ident{Name: tok.Lex, Line: tok.Line, Col: tok.Col}
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
		p.expect(TRBracket, "']' after list elements")
		return &ListLit{Elems: elems, Line: tok.Line, Col: tok.Col}
	case TFn:
		p.advance()
		return p.fnRest("", tok.Line, tok.Col)
	case TIf:
		return p.ifExpr()
	case TMatch:
		return p.matchExpr()
	case TLBrace:
		return p.block()
	}
	p.fail(tok.Line, "unexpected token %q", tok.Lex)
	return nil
}

func (p *Parser) ifExpr() Expr {
	tok := p.advance() // if
	cond := p.expression()
	then := p.block()
	var els Expr
	if p.match(TElse) {
		if p.check(TIf) {
			els = p.ifExpr()
		} else {
			els = p.block()
		}
	}
	return &IfExpr{Cond: cond, Then: then, Else: els, Line: tok.Line, Col: tok.Col}
}

func (p *Parser) matchExpr() Expr {
	tok := p.advance() // match
	scrut := p.expression()
	p.expect(TLBrace, "'{' after match scrutinee")
	m := &MatchExpr{Scrut: scrut, Line: tok.Line, Col: tok.Col}
	p.skipSemis()
	for !p.check(TRBrace) && !p.check(TEOF) {
		pat := p.pattern()
		p.expect(TArrow, "'=>' after pattern")
		p.skipSemis()
		body := p.expression()
		m.Arms = append(m.Arms, MatchArm{Pat: pat, Body: body, Line: tok.Line, Col: tok.Col})
		if !p.match(TComma) && !p.check(TSemi) && !p.check(TRBrace) {
			p.fail(p.peek().Line, "expected ',' or newline between match arms")
		}
		p.skipSemis()
	}
	p.expect(TRBrace, "'}' after match arms")
	if len(m.Arms) == 0 {
		p.fail(tok.Line, "match must have at least one arm")
	}
	return m
}

func (p *Parser) pattern() Pattern {
	tok := p.peek()
	switch tok.Type {
	case TInt, TFloat, TString, TTrue, TFalse:
		lit := p.primary()
		return &PatLiteral{Val: lit, Line: tok.Line, Col: tok.Col}
	case TMinus: // negative number literal
		p.advance()
		lit := p.primary()
		switch l := lit.(type) {
		case *IntLit:
			l.Val = -l.Val
		case *FloatLit:
			l.Val = -l.Val
		default:
			p.fail(tok.Line, "'-' in pattern must precede a number")
		}
		return &PatLiteral{Val: lit, Line: tok.Line, Col: tok.Col}
	case TIdent:
		p.advance()
		if tok.Lex == "_" {
			return &PatWildcard{Line: tok.Line, Col: tok.Col}
		}
		enumName, variant := "", tok.Lex
		if p.match(TDot) {
			enumName = tok.Lex
			variant = p.expect(TIdent, "variant name after '.'").Lex
		}
		if p.match(TLParen) {
			pv := &PatVariant{EnumName: enumName, Variant: variant, Line: tok.Line, Col: tok.Col}
			for !p.check(TRParen) {
				sub := p.pattern()
				switch sub.(type) {
				case *PatBinding, *PatWildcard:
				default:
					p.fail(tok.Line, "variant fields may only bind names or use '_'")
				}
				pv.Binds = append(pv.Binds, sub)
				if !p.match(TComma) {
					break
				}
			}
			p.expect(TRParen, "')' after variant pattern")
			return pv
		}
		if enumName != "" {
			return &PatVariant{EnumName: enumName, Variant: variant, Line: tok.Line, Col: tok.Col}
		}
		// bare name: binding, or a nullary variant like None ??compiler decides
		return &PatBinding{Name: variant, Line: tok.Line, Col: tok.Col}
	}
	p.fail(tok.Line, "invalid pattern %q", tok.Lex)
	return nil
}
