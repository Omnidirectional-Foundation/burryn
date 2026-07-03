package main

type TokType uint8

const (
	// single char
	TLParen TokType = iota
	TRParen
	TLBrace
	TRBrace
	TLBracket
	TRBracket
	TComma
	TDot
	TMinus
	TPlus
	TSlash
	TStar
	TPercent
	TSemi // explicit or auto-inserted at newline
	TQuestion
	TColon

	// one or two char
	TBang
	TBangEq
	TEq
	TEqEq
	TGt
	TGtEq
	TLt
	TLtEq
	TAndAnd
	TOrOr
	TArrow     // =>
	TLArrow    // <-
	TThinArrow // -> (type syntax)

	// literals
	TIdent
	TString
	TInt
	TFloat

	// keywords
	TLet
	TMut
	TFn
	TIf
	TElse
	TWhile
	TFor
	TIn
	TReturn
	TTrue
	TFalse
	TEnum
	TMatch
	TSpawn
	TBreak
	TContinue

	TEOF
)

// Span is a half-open byte range [Start, End) into the source.
type Span struct {
	Start, End int
}

// union returns the smallest span covering both s and o.
func (s Span) union(o Span) Span {
	if o.Start < s.Start {
		s.Start = o.Start
	}
	if o.End > s.End {
		s.End = o.End
	}
	return s
}

type Token struct {
	Type TokType
	Lex  string
	Span Span
}

var keywords = map[string]TokType{
	"let":      TLet,
	"mut":      TMut,
	"fn":       TFn,
	"if":       TIf,
	"else":     TElse,
	"while":    TWhile,
	"for":      TFor,
	"in":       TIn,
	"return":   TReturn,
	"true":     TTrue,
	"false":    TFalse,
	"enum":     TEnum,
	"match":    TMatch,
	"spawn":    TSpawn,
	"break":    TBreak,
	"continue": TContinue,
}
