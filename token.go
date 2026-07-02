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

type Token struct {
	Type TokType
	Lex  string
	Line int
	Col  int
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
