package main

import (
	"fmt"
	"strings"
)

// Lexer with Go-style automatic semicolon insertion: a newline terminates a
// statement when the previous token could legally end one.
type Lexer struct {
	src   string
	start int
	pos   int
	last  TokType // last token type emitted (for semicolon insertion)
	begun bool    // whether any token was emitted yet
	toks  []Token
	diags []Diag
}

// lex scans src into tokens, collecting lex errors as diagnostics; scanning
// continues past errors so several can be reported at once.
func lex(src string) ([]Token, []Diag) {
	src = strings.TrimPrefix(src, string(rune(0xFEFF))) // tolerate a UTF-8 BOM
	l := &Lexer{src: src}
	for !l.atEnd() {
		l.start = l.pos
		l.scan()
	}
	l.start = l.pos // spans of the synthetic trailing tokens sit at EOF
	l.maybeSemi()   // final newline may be missing
	l.emit(TEOF, "")
	return l.toks, l.diags
}

func (l *Lexer) fail(sp Span, code, help, format string, args ...any) {
	l.diags = append(l.diags, Diag{
		IsErr: true, Code: code, Msg: fmt.Sprintf(format, args...), Help: help, Span: sp,
	})
}

func (l *Lexer) atEnd() bool { return l.pos >= len(l.src) }

func (l *Lexer) peek() byte {
	if l.atEnd() {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peek2() byte {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	return c
}

func (l *Lexer) match(c byte) bool {
	if l.peek() == c {
		l.pos++
		return true
	}
	return false
}

func (l *Lexer) emit(t TokType, lexeme string) {
	l.toks = append(l.toks, Token{Type: t, Lex: lexeme, Span: Span{Start: l.start, End: l.pos}})
	l.last = t
	l.begun = true
}

// statement-ending token types trigger semicolon insertion at newline
func canEndStmt(t TokType) bool {
	switch t {
	case TIdent, TString, TInt, TFloat, TTrue, TFalse,
		TRParen, TRBracket, TRBrace, TQuestion, TReturn, TBreak, TContinue:
		return true
	}
	return false
}

func (l *Lexer) maybeSemi() {
	if l.begun && canEndStmt(l.last) {
		l.emit(TSemi, "\n")
	}
}

func (l *Lexer) scan() {
	c := l.advance()
	switch c {
	case ' ', '\t', '\r':
	case '\n':
		l.maybeSemi()
	case '/':
		if l.match('/') {
			for !l.atEnd() && l.peek() != '\n' {
				l.pos++
			}
		} else {
			l.emit(TSlash, "/")
		}
	case '(':
		l.emit(TLParen, "(")
	case ')':
		l.emit(TRParen, ")")
	case '{':
		l.emit(TLBrace, "{")
	case '}':
		l.emit(TRBrace, "}")
	case '[':
		l.emit(TLBracket, "[")
	case ']':
		l.emit(TRBracket, "]")
	case ',':
		l.emit(TComma, ",")
	case '.':
		l.emit(TDot, ".")
	case '-':
		if l.match('>') {
			l.emit(TThinArrow, "->")
		} else {
			l.emit(TMinus, "-")
		}
	case '+':
		l.emit(TPlus, "+")
	case '*':
		l.emit(TStar, "*")
	case '%':
		l.emit(TPercent, "%")
	case ';':
		l.emit(TSemi, ";")
	case '?':
		l.emit(TQuestion, "?")
	case ':':
		l.emit(TColon, ":")
	case '!':
		if l.match('=') {
			l.emit(TBangEq, "!=")
		} else {
			l.emit(TBang, "!")
		}
	case '=':
		if l.match('=') {
			l.emit(TEqEq, "==")
		} else if l.match('>') {
			l.emit(TArrow, "=>")
		} else {
			l.emit(TEq, "=")
		}
	case '>':
		if l.match('=') {
			l.emit(TGtEq, ">=")
		} else {
			l.emit(TGt, ">")
		}
	case '<':
		if l.match('=') {
			l.emit(TLtEq, "<=")
		} else if l.match('-') {
			l.emit(TLArrow, "<-")
		} else {
			l.emit(TLt, "<")
		}
	case '&':
		if l.match('&') {
			l.emit(TAndAnd, "&&")
		} else {
			l.fail(Span{Start: l.start, End: l.pos}, "E1001",
				"Burryn has no bitwise operators; logical and is `&&`", "unexpected '&'")
		}
	case '|':
		if l.match('|') {
			l.emit(TOrOr, "||")
		} else {
			l.fail(Span{Start: l.start, End: l.pos}, "E1001",
				"Burryn has no bitwise operators; logical or is `||`", "unexpected '|'")
		}
	case '"':
		l.scanString()
	default:
		if isDigit(c) {
			l.scanNumber()
		} else if isAlpha(c) {
			l.scanIdent()
		} else {
			l.fail(Span{Start: l.start, End: l.pos}, "E1001", "",
				"unexpected character %q", string(c))
		}
	}
}

func (l *Lexer) scanString() {
	var buf []byte
	for !l.atEnd() && l.peek() != '"' {
		c := l.advance()
		if c == '\\' && !l.atEnd() {
			e := l.advance()
			switch e {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case '"':
				buf = append(buf, '"')
			case '\\':
				buf = append(buf, '\\')
			default:
				l.fail(Span{Start: l.pos - 2, End: l.pos}, "E1003",
					`supported escapes: \n \t \" \\`, `bad escape \%s`, string(e))
				buf = append(buf, e) // keep scanning the rest of the string
			}
		} else {
			buf = append(buf, c)
		}
	}
	if l.atEnd() {
		l.fail(Span{Start: l.start, End: l.start + 1}, "E1002",
			"add a closing '\"'", "unterminated string")
		return
	}
	l.pos++ // closing quote
	l.emit(TString, string(buf))
}

func (l *Lexer) scanNumber() {
	for isDigit(l.peek()) {
		l.pos++
	}
	isFloat := false
	if l.peek() == '.' && isDigit(l.peek2()) {
		isFloat = true
		l.pos++
		for isDigit(l.peek()) {
			l.pos++
		}
	}
	if isFloat {
		l.emit(TFloat, l.src[l.start:l.pos])
	} else {
		l.emit(TInt, l.src[l.start:l.pos])
	}
}

func (l *Lexer) scanIdent() {
	for isAlpha(l.peek()) || isDigit(l.peek()) {
		l.pos++
	}
	word := l.src[l.start:l.pos]
	if kw, ok := keywords[word]; ok {
		l.emit(kw, word)
	} else {
		l.emit(TIdent, word)
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
