package main

// ---- Expressions ----

type Expr interface {
	exprNode()
	span() Span
}

type IntLit struct {
	Val  int64
	Span Span
}
type FloatLit struct {
	Val  float64
	Span Span
}
type StrLit struct {
	Val  string
	Span Span
}
type BoolLit struct {
	Val  bool
	Span Span
}
type Ident struct {
	Name string
	Span Span
}
type Unary struct {
	Op   TokType // TMinus, TBang
	Rhs  Expr
	Span Span
}
type Binary struct {
	Op       TokType
	Lhs, Rhs Expr
	Span     Span
}
type Logical struct { // && || with short circuit
	Op       TokType
	Lhs, Rhs Expr
	Span     Span
}
type Call struct {
	Callee Expr
	Args   []Expr
	Span   Span
}
type Index struct {
	Target Expr
	Idx    Expr
	Span   Span
}
type ListLit struct {
	Elems []Expr
	Span  Span
}
type FnLit struct { // fn(a, b) { ... }
	Params     []string
	ParamSpans []Span // parallel to Params
	Body       *Block
	Name       string // "" for anonymous; set for fn declarations
	Span       Span
}
type Block struct { // { stmts } ??value = value of final expression statement
	Stmts []Stmt
	Span  Span
}
type IfExpr struct {
	Cond Expr
	Then *Block
	Else Expr // *Block, *IfExpr, or nil
	Span Span
}
type MatchExpr struct {
	Scrut Expr
	Arms  []MatchArm
	Span  Span
}
type MatchArm struct {
	Pat  Pattern
	Body Expr
	Span Span
}
type VariantAccess struct { // Shape.Circle
	EnumName string
	Variant  string
	Span     Span
}
type PkgAccess struct { // geo.area — created by the module loader when the
	// head of a two-segment path resolves to a file's import alias
	Pkg  string // import path of the target package
	Name string
	Span Span
}
type QualVariantAccess struct { // geo.Shape.Circle
	Pkg     string // import alias until the loader resolves it to an import path
	Enum    string
	Variant string
	Span    Span
}
type TryExpr struct { // expr?
	Inner Expr
	Span  Span
}
type RecvExpr struct { // <-ch
	Chan Expr
	Span Span
}

func (*IntLit) exprNode()            {}
func (*FloatLit) exprNode()          {}
func (*StrLit) exprNode()            {}
func (*BoolLit) exprNode()           {}
func (*Ident) exprNode()             {}
func (*Unary) exprNode()             {}
func (*Binary) exprNode()            {}
func (*Logical) exprNode()           {}
func (*Call) exprNode()              {}
func (*Index) exprNode()             {}
func (*ListLit) exprNode()           {}
func (*FnLit) exprNode()             {}
func (*Block) exprNode()             {}
func (*IfExpr) exprNode()            {}
func (*MatchExpr) exprNode()         {}
func (*VariantAccess) exprNode()     {}
func (*PkgAccess) exprNode()         {}
func (*QualVariantAccess) exprNode() {}
func (*TryExpr) exprNode()           {}
func (*RecvExpr) exprNode()          {}

func (e *IntLit) span() Span            { return e.Span }
func (e *FloatLit) span() Span          { return e.Span }
func (e *StrLit) span() Span            { return e.Span }
func (e *BoolLit) span() Span           { return e.Span }
func (e *Ident) span() Span             { return e.Span }
func (e *Unary) span() Span             { return e.Span }
func (e *Binary) span() Span            { return e.Span }
func (e *Logical) span() Span           { return e.Span }
func (e *Call) span() Span              { return e.Span }
func (e *Index) span() Span             { return e.Span }
func (e *ListLit) span() Span           { return e.Span }
func (e *FnLit) span() Span             { return e.Span }
func (e *Block) span() Span             { return e.Span }
func (e *IfExpr) span() Span            { return e.Span }
func (e *MatchExpr) span() Span         { return e.Span }
func (e *VariantAccess) span() Span     { return e.Span }
func (e *PkgAccess) span() Span         { return e.Span }
func (e *QualVariantAccess) span() Span { return e.Span }
func (e *TryExpr) span() Span           { return e.Span }
func (e *RecvExpr) span() Span          { return e.Span }

// ---- Patterns (for match) ----

type Pattern interface {
	patNode()
	span() Span
}

type PatWildcard struct{ Span Span }
type PatLiteral struct {
	Val  Expr // IntLit / StrLit / BoolLit / FloatLit
	Span Span
}
type PatBinding struct { // bare name: binds anything
	Name string
	Span Span
}
type PatVariant struct { // Shape.Circle(r) ??or bare variant name like Some(x)/None
	Pkg      string // geo.Shape.Circle(r): import alias until the loader resolves it
	EnumName string // may be "" when written without qualifier
	Variant  string
	Binds    []Pattern // sub-patterns for fields (PatBinding or PatWildcard)
	Span     Span
}

func (*PatWildcard) patNode() {}
func (*PatLiteral) patNode()  {}
func (*PatBinding) patNode()  {}
func (*PatVariant) patNode()  {}

func (p *PatWildcard) span() Span { return p.Span }
func (p *PatLiteral) span() Span  { return p.Span }
func (p *PatBinding) span() Span  { return p.Span }
func (p *PatVariant) span() Span  { return p.Span }

// ---- Statements ----

type Stmt interface {
	stmtNode()
	span() Span
}

type LetStmt struct {
	Name     string
	NameSpan Span // the bound identifier itself
	Mut      bool
	Pub      bool
	Init     Expr
	Span     Span
}
type AssignStmt struct {
	Target Expr // Ident or Index
	Val    Expr
	Span   Span
}
type ExprStmt struct {
	E    Expr
	Span Span
}
type WhileStmt struct {
	Cond Expr
	Body *Block
	Span Span
}
type ForStmt struct {
	Var     string
	VarSpan Span // the loop variable identifier
	Iter    Expr // must evaluate to a list
	Body    *Block
	Span    Span
}
type ReturnStmt struct {
	Val  Expr // nil for bare return
	Span Span
}
type FnDecl struct {
	Name     string
	NameSpan Span // the function name identifier
	Pub      bool
	Fn       *FnLit
	Span     Span
}
type EnumDecl struct {
	Name     string
	Params   []string // generic type parameters, e.g. enum Box(a) { ... }
	Variants []EnumVariantDecl
	Pub      bool
	Span     Span
}
type ImportDecl struct {
	Alias     string // explicit alias; "" = last path segment
	AliasSpan Span
	Path      string // import path as written
	PathSpan  Span
	Span      Span
}
type EnumVariantDecl struct {
	Name  string
	Arity int
	Types []TypeExpr // field types
}

// ---- type expressions (v2 static syntax) ----

type TypeExpr interface {
	typeNode()
	span() Span
}

type TEName struct { // int / str / Shape / Option(int) / a type variable
	Pkg  string // geo.Shape: import alias until the loader resolves it; "" if unqualified
	Name string
	Args []TypeExpr
	Span Span
}
type TEList struct { // [t]
	Elem TypeExpr
	Span Span
}
type TEFn struct { // fn(a, b) -> r
	Params []TypeExpr
	Ret    TypeExpr
	Span   Span
}

func (*TEName) typeNode() {}
func (*TEList) typeNode() {}
func (*TEFn) typeNode()   {}

func (t *TEName) span() Span { return t.Span }
func (t *TEList) span() Span { return t.Span }
func (t *TEFn) span() Span   { return t.Span }

type SpawnStmt struct {
	CallE *Call
	Span  Span
}
type SendStmt struct { // ch <- v
	Chan Expr
	Val  Expr
	Span Span
}
type BreakStmt struct{ Span Span }
type ContinueStmt struct{ Span Span }

func (*LetStmt) stmtNode()      {}
func (*AssignStmt) stmtNode()   {}
func (*ExprStmt) stmtNode()     {}
func (*WhileStmt) stmtNode()    {}
func (*ForStmt) stmtNode()      {}
func (*ReturnStmt) stmtNode()   {}
func (*FnDecl) stmtNode()       {}
func (*EnumDecl) stmtNode()     {}
func (*ImportDecl) stmtNode()   {}
func (*SpawnStmt) stmtNode()    {}
func (*SendStmt) stmtNode()     {}
func (*BreakStmt) stmtNode()    {}
func (*ContinueStmt) stmtNode() {}

func (s *LetStmt) span() Span      { return s.Span }
func (s *AssignStmt) span() Span   { return s.Span }
func (s *ExprStmt) span() Span     { return s.Span }
func (s *WhileStmt) span() Span    { return s.Span }
func (s *ForStmt) span() Span      { return s.Span }
func (s *ReturnStmt) span() Span   { return s.Span }
func (s *FnDecl) span() Span       { return s.Span }
func (s *EnumDecl) span() Span     { return s.Span }
func (s *ImportDecl) span() Span   { return s.Span }
func (s *SpawnStmt) span() Span    { return s.Span }
func (s *SendStmt) span() Span     { return s.Span }
func (s *BreakStmt) span() Span    { return s.Span }
func (s *ContinueStmt) span() Span { return s.Span }
