package main

// ---- Expressions ----

type Expr interface{ exprNode() }

type IntLit struct {
	Val  int64
	Line, Col int
}
type FloatLit struct {
	Val  float64
	Line, Col int
}
type StrLit struct {
	Val  string
	Line, Col int
}
type BoolLit struct {
	Val  bool
	Line, Col int
}
type Ident struct {
	Name string
	Line, Col int
}
type Unary struct {
	Op   TokType // TMinus, TBang
	Rhs  Expr
	Line, Col int
}
type Binary struct {
	Op       TokType
	Lhs, Rhs Expr
	Line, Col int
}
type Logical struct { // && || with short circuit
	Op       TokType
	Lhs, Rhs Expr
	Line, Col int
}
type Call struct {
	Callee Expr
	Args   []Expr
	Line, Col int
}
type Index struct {
	Target Expr
	Idx    Expr
	Line, Col int
}
type ListLit struct {
	Elems []Expr
	Line, Col int
}
type FnLit struct { // fn(a, b) { ... }
	Params []string
	Body   *Block
	Name   string // "" for anonymous; set for fn declarations
	Line, Col int
}
type Block struct { // { stmts } ??value = value of final expression statement
	Stmts []Stmt
	Line, Col int
}
type IfExpr struct {
	Cond Expr
	Then *Block
	Else Expr // *Block, *IfExpr, or nil
	Line, Col int
}
type MatchExpr struct {
	Scrut Expr
	Arms  []MatchArm
	Line, Col int
}
type MatchArm struct {
	Pat  Pattern
	Body Expr
	Line, Col int
}
type VariantAccess struct { // Shape.Circle
	EnumName string
	Variant  string
	Line, Col int
}
type TryExpr struct { // expr?
	Inner Expr
	Line, Col int
}
type RecvExpr struct { // <-ch
	Chan Expr
	Line, Col int
}

func (*IntLit) exprNode()        {}
func (*FloatLit) exprNode()      {}
func (*StrLit) exprNode()        {}
func (*BoolLit) exprNode()       {}
func (*Ident) exprNode()         {}
func (*Unary) exprNode()         {}
func (*Binary) exprNode()        {}
func (*Logical) exprNode()       {}
func (*Call) exprNode()          {}
func (*Index) exprNode()         {}
func (*ListLit) exprNode()       {}
func (*FnLit) exprNode()         {}
func (*Block) exprNode()         {}
func (*IfExpr) exprNode()        {}
func (*MatchExpr) exprNode()     {}
func (*VariantAccess) exprNode() {}
func (*TryExpr) exprNode()       {}
func (*RecvExpr) exprNode()      {}

// ---- Patterns (for match) ----

type Pattern interface{ patNode() }

type PatWildcard struct{ Line, Col int }
type PatLiteral struct {
	Val  Expr // IntLit / StrLit / BoolLit / FloatLit
	Line, Col int
}
type PatBinding struct { // bare name: binds anything
	Name string
	Line, Col int
}
type PatVariant struct { // Shape.Circle(r) ??or bare variant name like Some(x)/None
	EnumName string // may be "" when written without qualifier
	Variant  string
	Binds    []Pattern // sub-patterns for fields (PatBinding or PatWildcard)
	Line, Col int
}

func (*PatWildcard) patNode() {}
func (*PatLiteral) patNode()  {}
func (*PatBinding) patNode()  {}
func (*PatVariant) patNode()  {}

// ---- Statements ----

type Stmt interface{ stmtNode() }

type LetStmt struct {
	Name  string
	Mut   bool
	Init  Expr
	Line, Col int
}
type AssignStmt struct {
	Target Expr // Ident or Index
	Val    Expr
	Line, Col int
}
type ExprStmt struct {
	E    Expr
	Line, Col int
}
type WhileStmt struct {
	Cond Expr
	Body *Block
	Line, Col int
}
type ForStmt struct {
	Var  string
	Iter Expr // must evaluate to a list
	Body *Block
	Line, Col int
}
type ReturnStmt struct {
	Val  Expr // nil for bare return
	Line, Col int
}
type FnDecl struct {
	Name string
	Fn   *FnLit
	Line, Col int
}
type EnumDecl struct {
	Name     string
	Params   []string // generic type parameters, e.g. enum Box(a) { ... }
	Variants []EnumVariantDecl
	Line, Col int
}
type EnumVariantDecl struct {
	Name  string
	Arity int
	Types []TypeExpr // field types
}

// ---- type expressions (v2 static syntax) ----

type TypeExpr interface{ typeNode() }

type TEName struct { // int / str / Shape / Option(int) / a type variable
	Name string
	Args []TypeExpr
	Line, Col int
}
type TEList struct { // [t]
	Elem TypeExpr
	Line, Col int
}
type TEFn struct { // fn(a, b) -> r
	Params []TypeExpr
	Ret    TypeExpr
	Line, Col int
}

func (*TEName) typeNode() {}
func (*TEList) typeNode() {}
func (*TEFn) typeNode()   {}
type SpawnStmt struct {
	CallE *Call
	Line, Col int
}
type SendStmt struct { // ch <- v
	Chan Expr
	Val  Expr
	Line, Col int
}
type BreakStmt struct{ Line, Col int }
type ContinueStmt struct{ Line, Col int }

func (*LetStmt) stmtNode()      {}
func (*AssignStmt) stmtNode()   {}
func (*ExprStmt) stmtNode()     {}
func (*WhileStmt) stmtNode()    {}
func (*ForStmt) stmtNode()      {}
func (*ReturnStmt) stmtNode()   {}
func (*FnDecl) stmtNode()       {}
func (*EnumDecl) stmtNode()     {}
func (*SpawnStmt) stmtNode()    {}
func (*SendStmt) stmtNode()     {}
func (*BreakStmt) stmtNode()    {}
func (*ContinueStmt) stmtNode() {}
