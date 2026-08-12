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
type Pipe struct { // lhs |> target(args...)
	Lhs, Target Expr
	Args        []Expr
	Span        Span
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
type FnLit struct { // fn(a, b) capture(ref x) { ... }
	Params     []string
	ParamMuts  []bool // parallel to Params: `fn f(mut xs)` marks xs mutable
	ParamSpans []Span // parallel to Params
	ParamTys   []TypeExpr
	RetTy      TypeExpr // nil when not annotated
	Body       *Block
	Name       string   // "" for anonymous; set for fn declarations
	Crefs      []string // capture(ref ...) names
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
	Pat   Pattern
	Guard Expr // `pat if guard => body`; nil when absent
	Body  Expr
	Span  Span
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
type RecordLit struct { // record { a: 1, b: 2 }
	Names []string
	Vals  []Expr
	Span  Span
}
type RecordUpdate struct { // record { base | a: 1 }
	Base  Expr
	Names []string
	Vals  []Expr
	Span  Span
}
type FieldAccess struct { // base.field
	Base  Expr
	Field string
	Span  Span
}
type TupleLit struct { // (a, b)
	Elems []Expr
	Span  Span
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
func (*Pipe) exprNode()              {}
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
func (*RecordLit) exprNode()         {}
func (*RecordUpdate) exprNode()      {}
func (*FieldAccess) exprNode()       {}
func (*TupleLit) exprNode()          {}

func (e *IntLit) span() Span            { return e.Span }
func (e *FloatLit) span() Span          { return e.Span }
func (e *StrLit) span() Span            { return e.Span }
func (e *BoolLit) span() Span           { return e.Span }
func (e *Ident) span() Span             { return e.Span }
func (e *Unary) span() Span             { return e.Span }
func (e *Binary) span() Span            { return e.Span }
func (e *Logical) span() Span           { return e.Span }
func (e *Call) span() Span              { return e.Span }
func (e *Pipe) span() Span              { return e.Span }
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
func (e *RecordLit) span() Span         { return e.Span }
func (e *RecordUpdate) span() Span      { return e.Span }
func (e *FieldAccess) span() Span       { return e.Span }
func (e *TupleLit) span() Span          { return e.Span }

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
type PatVariant struct { // Shape::Circle(r) or bare variant name like Some(x)/None
	Pkg      string // geo::Shape::Circle(r): import alias until the loader resolves it
	EnumName string // may be "" when written without qualifier
	Variant  string
	Binds    []Pattern // sub-patterns for fields (PatBinding or PatWildcard)
	Span     Span
}
type PatRecord struct { // record { x, y }
	Names []string
	Span  Span
}

func (*PatWildcard) patNode() {}
func (*PatLiteral) patNode()  {}
func (*PatBinding) patNode()  {}
func (*PatVariant) patNode()  {}
func (*PatRecord) patNode()   {}

func (p *PatWildcard) span() Span { return p.Span }
func (p *PatLiteral) span() Span  { return p.Span }
func (p *PatBinding) span() Span  { return p.Span }
func (p *PatVariant) span() Span  { return p.Span }
func (p *PatRecord) span() Span   { return p.Span }

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
	Ty       TypeExpr // optional type annotation
	Init     Expr
	Span     Span
}
type DestructLet struct { // let (a, b) = tuple
	Names     []string
	NameSpans []Span
	Mut       bool
	Init      Expr
	Span      Span
}
type ConstDecl struct { // const x = ... (folded at compile time)
	Name     string
	NameSpan Span
	Pub      bool
	Ty       TypeExpr
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
	Var       string
	VarSpan   Span // the loop variable identifier
	Idx       string // index variable ("for i, x in xs"); "" when absent
	IdxSpan   Span
	Iter      Expr // evaluates to a list or a channel
	IterIsChan bool // set by the checker: iterate by receiving until closed
	Body      *Block
	Span      Span
}
type LabelStmt struct { // label: while ... { break label }
	Label string
	Inner Stmt
	Span  Span
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
type MethodDecl struct { // fn (recv: Type) name(...) { ... }
	RecvTy   string // dispatch key: the receiver type name
	Name     string
	NameSpan Span
	Fn       *FnLit
	Span     Span
}
type DeferStmt struct { // defer { ... } / defer capture(ref x) { ... }
	Body  Expr
	Crefs []string
	Span  Span
}
type TypeAliasDecl struct { // type X = [int]
	Name     string
	Ty       TypeExpr
	Pub      bool
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

type TEName struct { // int / str / Shape / Option(int) / a type variable / pkg::Name
	Pkg        string // pkg::Shape: import alias until the loader resolves it; "" if unqualified
	Name       string
	Constraint string // `Name: Constraint` (row-poly records); "" when absent
	Args       []TypeExpr
	Span       Span
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
type TERecord struct { // record { a: int | r }
	Names   []string
	Types   []TypeExpr
	RowVar  TypeExpr // trailing row variable (open record); nil for closed
	Span    Span
}
type TETuple struct { // (int, str)
	Types []TypeExpr
	Span  Span
}

func (*TEName) typeNode()    {}
func (*TEList) typeNode()    {}
func (*TEFn) typeNode()      {}
func (*TERecord) typeNode()  {}
func (*TETuple) typeNode()   {}

func (t *TEName) span() Span   { return t.Span }
func (t *TEList) span() Span   { return t.Span }
func (t *TEFn) span() Span     { return t.Span }
func (t *TERecord) span() Span { return t.Span }
func (t *TETuple) span() Span  { return t.Span }

type SpawnStmt struct {
	CallE *Call
	Span  Span
}

// SelectArm is one communication clause of a `select`.
type SelectArm struct {
	IsSend bool // true: `ch <- v`; false: receive
	Bind   string // recv only: name bound to the received value ("" or "_" = discard)
	BindSpan Span
	Chan   Expr
	Val    Expr // send only
	Body   Expr
	Span   Span
}

// SelectStmt waits on several channels, running the first ready arm in
// declaration order; an optional `default` arm runs when none are ready.
type SelectStmt struct {
	Arms       []SelectArm
	HasDefault bool
	Default    Expr
	DefaultSpan Span
	Span       Span
}
type SendStmt struct { // ch <- v
	Chan Expr
	Val  Expr
	Span Span
}
type BreakStmt struct {
	Label string // "" = unlabeled
	Span  Span
}
type ContinueStmt struct {
	Label string // "" = unlabeled
	Span  Span
}

func (*LetStmt) stmtNode()       {}
func (*DestructLet) stmtNode()   {}
func (*ConstDecl) stmtNode()     {}
func (*AssignStmt) stmtNode()    {}
func (*ExprStmt) stmtNode()      {}
func (*WhileStmt) stmtNode()     {}
func (*ForStmt) stmtNode()       {}
func (*LabelStmt) stmtNode()     {}
func (*ReturnStmt) stmtNode()    {}
func (*FnDecl) stmtNode()        {}
func (*MethodDecl) stmtNode()    {}
func (*DeferStmt) stmtNode()     {}
func (*TypeAliasDecl) stmtNode() {}
func (*EnumDecl) stmtNode()      {}
func (*ImportDecl) stmtNode()    {}
func (*SpawnStmt) stmtNode()     {}
func (*SelectStmt) stmtNode()    {}
func (*SendStmt) stmtNode()      {}
func (*BreakStmt) stmtNode()     {}
func (*ContinueStmt) stmtNode()  {}

func (s *LetStmt) span() Span      { return s.Span }
func (s *DestructLet) span() Span  { return s.Span }
func (s *ConstDecl) span() Span    { return s.Span }
func (s *AssignStmt) span() Span   { return s.Span }
func (s *ExprStmt) span() Span     { return s.Span }
func (s *WhileStmt) span() Span    { return s.Span }
func (s *ForStmt) span() Span      { return s.Span }
func (s *LabelStmt) span() Span    { return s.Span }
func (s *ReturnStmt) span() Span   { return s.Span }
func (s *FnDecl) span() Span       { return s.Span }
func (s *MethodDecl) span() Span   { return s.Span }
func (s *DeferStmt) span() Span    { return s.Span }
func (s *TypeAliasDecl) span() Span { return s.Span }
func (s *EnumDecl) span() Span     { return s.Span }
func (s *ImportDecl) span() Span   { return s.Span }
func (s *SpawnStmt) span() Span    { return s.Span }
func (s *SelectStmt) span() Span   { return s.Span }
func (s *SendStmt) span() Span     { return s.Span }
func (s *BreakStmt) span() Span    { return s.Span }
func (s *ContinueStmt) span() Span { return s.Span }
