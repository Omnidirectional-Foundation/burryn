package main

import (
	"fmt"
	"strings"
)

// Lexer with Go-style automatic semicolon insertion: a newline terminates a
// statement when the previous token could legally end one. Mirrors
// compiler/frontend/lexer.bur: comments are trivia, strings may carry
// interpolation segments, multiline strings use triple quotes.
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

func (l *Lexer) peekAt(off int) byte {
	if l.pos+off >= len(l.src) {
		return 0
	}
	return l.src[l.pos+off]
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
	case TIdent, TString, TInterpEnd, TInt, TFloat, TTrue, TFalse,
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
	case '#':
		// shebang line at the very start of the file
		if l.start == 0 && l.peek() == '!' {
			for !l.atEnd() && l.peek() != '\n' {
				l.pos++
			}
		} else {
			l.fail(Span{Start: l.start, End: l.pos}, "E1001", "", "unexpected character %q", string(c))
		}
	case '/':
		if l.match('/') {
			for !l.atEnd() && l.peek() != '\n' {
				l.pos++
			}
		} else if l.match('*') {
			for !l.atEnd() {
				if l.peek() == '*' && l.peek2() == '/' {
					l.pos += 2
					break
				}
				l.pos++
			}
		} else if l.match('=') {
			l.emit(TSlashEq, "/=")
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
		if l.match('.') {
			l.emit(TDotDot, "..")
		} else {
			l.emit(TDot, ".")
		}
	case '-':
		if l.match('>') {
			l.emit(TThinArrow, "->")
		} else if l.match('=') {
			l.emit(TMinusEq, "-=")
		} else {
			l.emit(TMinus, "-")
		}
	case '+':
		if l.match('=') {
			l.emit(TPlusEq, "+=")
		} else {
			l.emit(TPlus, "+")
		}
	case '*':
		if l.match('=') {
			l.emit(TStarEq, "*=")
		} else {
			l.emit(TStar, "*")
		}
	case '%':
		if l.match('=') {
			l.emit(TPercentEq, "%=")
		} else {
			l.emit(TPercent, "%")
		}
	case ';':
		l.emit(TSemi, ";")
	case '?':
		l.emit(TQuestion, "?")
	case ':':
		if l.match(':') {
			l.emit(TColonColon, "::")
		} else {
			l.emit(TColon, ":")
		}
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
		if l.match('>') {
			l.emit(TPipe, "|>")
		} else if l.match('|') {
			l.emit(TOrOr, "||")
		} else {
			l.emit(TBar, "|")
		}
	case '"':
		if l.peek() == '"' && l.peek2() == '"' {
			l.mlString()
		} else {
			l.scanString()
		}
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

// scanString scans an ordinary string; stringPart is the segment scanner.
func (l *Lexer) scanString() { l.stringPart(true) }

// stringPart scans one string segment (escapes and interpolation switching),
// mirroring lex_string_part in lexer.bur. first distinguishes the opening
// segment from one resumed after an interpolation-closing `}`; the latter
// ends in TInterpEnd.
func (l *Lexer) stringPart(first bool) {
	var parts []string
	seg := l.pos
	for !l.atEnd() {
		c := l.peek()
		if c == '"' {
			parts = append(parts, l.src[seg:l.pos])
			l.pos++
			t := TString
			if !first {
				t = TInterpEnd
			}
			l.emit(t, strings.Join(parts, ""))
			return
		} else if c == '{' {
			if l.peek2() == '{' {
				parts = append(parts, l.src[seg:l.pos])
				parts = append(parts, "{")
				l.pos += 2
				seg = l.pos
			} else {
				parts = append(parts, l.src[seg:l.pos])
				l.pos++
				t := TInterpStart
				if !first {
					t = TInterpMid
				}
				l.emit(t, strings.Join(parts, ""))
				l.interpolation(func() { l.stringPart(false) })
				return
			}
		} else {
			l.advance()
			if c == '\\' && !l.atEnd() {
				parts = append(parts, l.src[seg:l.pos-1])
				e := l.advance()
				switch e {
				case 'n':
					parts = append(parts, "\n")
				case 't':
					parts = append(parts, "\t")
				case '"':
					parts = append(parts, "\"")
				case '\\':
					parts = append(parts, "\\")
				case 'r':
					parts = append(parts, "\r")
				default:
					l.fail(Span{Start: l.pos - 2, End: l.pos}, "E1003",
						`supported escapes: \n \t \" \\ \r`, `bad escape \%s`, string(e))
					parts = append(parts, string(e))
				}
				seg = l.pos
			}
		}
	}
	l.fail(Span{Start: l.start, End: l.start + 1}, "E1002", "add a closing '\"'", "unterminated string")
}

// mlString scans a triple-quoted multiline string: a newline must follow
// """ ; leading whitespace is ignored, then a \ content marker; content runs
// to end of line; blank lines are empty rows; the final newline before the
// closing """ is stripped; escapes and interpolation behave as usual.
func (l *Lexer) mlString() {
	l.pos += 2
	if l.peek() != '\n' {
		l.fail(Span{Start: l.pos, End: l.pos + 1}, "E1006",
			"put the content on the lines below", `expected a newline after """`)
	}
	l.pos++
	var seg []string
	first := true
	anyInterp := false
	for !l.atEnd() {
		for l.peek() == ' ' || l.peek() == '\t' {
			l.pos++
		}
		if l.peek() == '"' && l.peek2() == '"' && l.peekAt(2) == '"' {
			l.pos += 3
			if len(seg) > 0 && strings.HasSuffix(seg[0], "\n") {
				seg[0] = seg[0][:len(seg[0])-1]
			}
			if anyInterp {
				l.emit(TInterpEnd, seg[0])
			} else {
				l.emit(TString, seg[0])
			}
			return
		}
		if l.peek() == '\n' {
			seg[0] += "\n"
			l.pos++
			continue
		}
		if l.peek() != '\\' {
			l.fail(Span{Start: l.pos, End: l.pos + 1}, "E1007",
				"mark content lines with a leading `\\`", `expected `+"`\\`"+` to start a content line in a """ string`)
			l.pos++
			continue
		}
		l.pos++
		l.mlInline(&seg, &first, &anyInterp)
	}
	l.fail(Span{Start: l.start, End: l.start + 1}, "E1002", "add a closing '\"'", "unterminated string")
}

// mlInline scans one multiline row: literal bytes to end-of-line or a nested
// interpolation; resumes here after interpolation (lex_ml_inline).
func (l *Lexer) mlInline(seg *[]string, first *bool, anyInterp *bool) {
	for !l.atEnd() {
		c := l.peek()
		if c == '\n' {
			(*seg)[0] += "\n"
			l.pos++
			return
		}
		if c == '{' && l.peek2() != '{' {
			*anyInterp = true
			l.pos++
			t := TInterpStart
			if !*first {
				t = TInterpMid
			}
			l.emit(t, (*seg)[0])
			*first = false
			(*seg)[0] = ""
			l.interpolation(func() { l.mlInline(seg, first, anyInterp) })
			return
		}
		if c == '{' && l.peek2() == '{' {
			(*seg)[0] += "{"
			l.pos += 2
			continue
		}
		if c == '\\' && l.pos+1 < len(l.src) {
			e := l.src[l.pos+1]
			switch e {
			case 'n':
				(*seg)[0] += "\n"
				l.pos += 2
				continue
			case 't':
				(*seg)[0] += "\t"
				l.pos += 2
				continue
			case '"':
				(*seg)[0] += "\""
				l.pos += 2
				continue
			case '\\':
				(*seg)[0] += "\\"
				l.pos += 2
				continue
			case 'r':
				(*seg)[0] += "\r"
				l.pos += 2
				continue
			default:
				l.fail(Span{Start: l.pos, End: l.pos + 2}, "E1003",
					`supported escapes: \n \t \" \\ \r`, `bad escape \%s`, string(e))
				l.pos += 2
				continue
			}
		}
		(*seg)[0] += string(c)
		l.pos++
	}
}

// interpolation scans the interpolation expression up to the matching `}`;
// braces inside blocks, match arms, and nested interpolations balance before
// the outer string resumes; resume continues the outer scan.
func (l *Lexer) interpolation(resume func()) {
	open := l.pos
	depth := 0
	for !l.atEnd() {
		if l.peek() == '}' && depth == 0 {
			l.start = l.pos
			l.pos++
			resume()
			return
		}
		l.start = l.pos
		before := len(l.toks)
		l.scan()
		for i := before; i < len(l.toks); i++ {
			switch l.toks[i].Type {
			case TLBrace:
				depth++
			case TRBrace:
				depth--
			}
		}
		if depth < 0 {
			resume()
			return
		}
	}
	l.fail(Span{Start: open, End: open + 1}, "E1004",
		"add a closing '}' before the end of the string", "unterminated string interpolation")
}

// scanNumber scans number literals: hex/binary prefixes 0x 0b (lexeme kept
// verbatim, converted by the parser), underscore separators, float point and
// exponents (lookahead so 1e/1ex stays an identifier).
func (l *Lexer) scanNumber() {
	c0 := l.src[l.start]
	c1 := l.peek()
	if c0 == '0' && (c1 == 'x' || c1 == 'X' || c1 == 'b' || c1 == 'B') {
		isHex := c1 == 'x' || c1 == 'X'
		l.pos++
		any := false
		for {
			ch := l.peek()
			if ch == '_' {
				ok := isHex && isHexDigit(l.peek2()) || !isHex && (l.peek2() == '0' || l.peek2() == '1')
				if !ok {
					l.fail(Span{Start: l.start, End: l.pos}, "E1005",
						"a digit must follow '_' in a number literal", "bad '_' in number")
				}
				l.pos++
			} else if isHex && isHexDigit(ch) || !isHex && (ch == '0' || ch == '1') {
				any = true
				l.pos++
			} else {
				break
			}
		}
		if !any {
			l.fail(Span{Start: l.start, End: l.pos}, "E1005",
				"write digits after the prefix, e.g. `0xFF`", "expected digits after %s", l.src[l.start:l.pos])
		}
		l.emit(TInt, l.src[l.start:l.pos])
		return
	}
	for isDigit(l.peek()) || l.peek() == '_' {
		if l.peek() == '_' && !isDigit(l.peek2()) {
			l.fail(Span{Start: l.start, End: l.pos}, "E1005",
				"a digit must follow '_' in a number literal", "bad '_' in number")
		}
		l.pos++
	}
	isFloat := false
	if l.peek() == '.' && isDigit(l.peek2()) {
		isFloat = true
		l.pos++
		for isDigit(l.peek()) || l.peek() == '_' {
			if l.peek() == '_' && !isDigit(l.peek2()) {
				l.fail(Span{Start: l.start, End: l.pos}, "E1005",
					"a digit must follow '_' in a number literal", "bad '_' in number")
			}
			l.pos++
		}
	}
	ec := l.peek()
	if ec == 'e' || ec == 'E' {
		p1 := l.peek2()
		p2 := l.peekAt(2)
		exp := (p1 == '+' || p1 == '-') && isDigit(p2) || isDigit(p1)
		if exp {
			isFloat = true
			l.pos++
			if l.peek() == '+' || l.peek() == '-' {
				l.pos++
			}
			for isDigit(l.peek()) || l.peek() == '_' {
				if l.peek() == '_' && !isDigit(l.peek2()) {
					l.fail(Span{Start: l.start, End: l.pos}, "E1005",
						"a digit must follow '_' in a number literal", "bad '_' in number")
				}
				l.pos++
			}
		}
	}
	word := l.src[l.start:l.pos]
	if isFloat {
		l.emit(TFloat, word)
	} else {
		l.emit(TInt, word)
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
func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func isAlpha(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
