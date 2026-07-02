package main

import (
	"fmt"
	"strings"
)

// Lexer with Go-style automatic semicolon insertion: a newline terminates a
// statement when the previous token could legally end one.
type Lexer struct {
	src       string
	start     int
	pos       int
	line      int
	lineStart int // offset of the current line, for column tracking
	last      TokType // last token type emitted (for semicolon insertion)
	begun     bool    // whether any token was emitted yet
	toks      []Token
	err       error
}

func lex(src string) ([]Token, error) {
	src = strings.TrimPrefix(src, string(rune(0xFEFF))) // tolerate a UTF-8 BOM
	l := &Lexer{src: src, line: 1}
	for l.err == nil && !l.atEnd() {
		l.start = l.pos
		l.scan()
	}
	l.maybeSemi() // final newline may be missing
	l.emit(TEOF, "")
	return l.toks, l.err
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
	col := l.start - l.lineStart + 1
	if col < 1 {
		col = 1
	}
	l.toks = append(l.toks, Token{Type: t, Lex: lexeme, Line: l.line, Col: col})
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
		l.line++
		l.lineStart = l.pos
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
			l.err = fmt.Errorf("line %d: unexpected '&'", l.line)
		}
	case '|':
		if l.match('|') {
			l.emit(TOrOr, "||")
		} else {
			l.err = fmt.Errorf("line %d: unexpected '|'", l.line)
		}
	case '"':
		l.scanString()
	default:
		if isDigit(c) {
			l.scanNumber()
		} else if isAlpha(c) {
			l.scanIdent()
		} else {
			l.err = fmt.Errorf("line %d: unexpected character %q", l.line, string(c))
		}
	}
}

func (l *Lexer) scanString() {
	var buf []byte
	for !l.atEnd() && l.peek() != '"' {
		c := l.advance()
		if c == '\n' {
			l.line++
		}
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
				l.err = fmt.Errorf("line %d: bad escape \\%s", l.line, string(e))
				return
			}
		} else {
			buf = append(buf, c)
		}
	}
	if l.atEnd() {
		l.err = fmt.Errorf("line %d: unterminated string", l.line)
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
