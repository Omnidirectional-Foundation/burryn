package main

import "fmt"

type EnumInfo struct {
	Name       string // qualified name: <pkg>.<Name>, or Name outside packages
	Prefix     string // import path of the defining package; "" for scripts and builtins
	Variants   []VariantInfo
	runtime    *OEnumType
	singletons []*OEnumInst    // cached nullary instances
	ctors      []*OVariantCtor // cached constructors
}

// Shared holds cross-function compile state.
type Shared struct {
	gc        *GC
	enums     map[string]*EnumInfo // keyed by qualified name
	globalMut map[string]bool      // declared globals (qualified names) -> is mutable
	lines     map[string]lineIndex // per source file, for disassembly
	curPkg    string               // import path of the package being compiled; "" for scripts
	curFile   string               // stamped onto compile diagnostics and functions
	pkgDecls  map[string]bool      // top-level names of the current package
}

// newShared builds compile state over the program's sources (file -> text);
// scripts pass their single source under the "" key.
func newShared(gc *GC, srcs map[string]string) *Shared {
	s := &Shared{gc: gc, enums: map[string]*EnumInfo{}, globalMut: map[string]bool{},
		lines: map[string]lineIndex{}}
	for file, src := range srcs {
		s.lines[file] = newLineIndex(src)
	}
	s.declareEnum("Option", "", []VariantInfo{{"Some", 1}, {"None", 0}})
	s.declareEnum("Result", "", []VariantInfo{{"Ok", 1}, {"Err", 1}})
	for _, n := range nativeNames() {
		s.globalMut[n] = false
	}
	return s
}

// qualify maps a top-level name of the current package to its globally
// unique global-table name.
func (s *Shared) qualify(name string) string {
	if s.curPkg == "" {
		return name
	}
	return s.curPkg + "." + name
}

// mangleIfDecl qualifies name when it is a top-level declaration of the
// current package; natives and script globals pass through unchanged.
func (s *Shared) mangleIfDecl(name string) string {
	if s.pkgDecls[name] {
		return s.qualify(name)
	}
	return name
}

func (s *Shared) declareEnum(qual, prefix string, variants []VariantInfo) *EnumInfo {
	rt := &OEnumType{Name: qual, Variants: variants}
	s.gc.alloc(rt)
	info := &EnumInfo{
		Name: qual, Prefix: prefix, Variants: variants, runtime: rt,
		singletons: make([]*OEnumInst, len(variants)),
		ctors:      make([]*OVariantCtor, len(variants)),
	}
	s.enums[qual] = info
	s.globalMut[qual] = false
	return info
}

// find a variant by bare name across the enums visible without a qualifier
// (the current package's and the builtins); ambiguous if several match
func (s *Shared) findVariant(name string) (info *EnumInfo, idx int, ambiguous bool) {
	idx = -1
	for _, e := range s.enums {
		if e.Prefix != s.curPkg && e.Prefix != "" {
			continue
		}
		for i, v := range e.Variants {
			if v.Name == name {
				if idx >= 0 {
					return nil, -1, true
				}
				info, idx = e, i
			}
		}
	}
	return info, idx, false
}

// localEnum resolves a bare enum name written in the current package; the
// package's own enums shadow the builtins.
func (s *Shared) localEnum(name string) (*EnumInfo, bool) {
	if info, ok := s.enums[s.qualify(name)]; ok {
		return info, true
	}
	info, ok := s.enums[name]
	return info, ok && info.Prefix == ""
}

type LocalVar struct {
	name       string
	depth      int
	slot       int
	mut        bool
	captured   bool
	tempsBelow int // c.temps at declaration; temps below a live local never change
}

type UpvalMeta struct {
	index   uint16
	isLocal bool
	mut     bool
}

type loopCtx struct {
	baseSlot      int // stack slot count at loop-body entry
	breakJumps    []int
	continueJumps []int
	continueTo    int // -1 if patched later via continueJumps
}

type Compiler struct {
	enclosing  *Compiler
	shared     *Shared
	fn         *OFunc
	locals     []LocalVar
	upvals     []UpvalMeta
	scopeDepth int
	temps      int // expression temporaries currently on the stack
	loops      []loopCtx
}

type compileErr Diag

// compileProgram compiles stmts, reporting compile errors as diagnostics.
// Compilation stops at the first error (the checker has already caught
// anything a user is likely to write; these are backstops and limits).
// A bytecode verifier failure is a compiler bug and panics.
func compileProgram(gc *GC, src string, stmts []Stmt) (fn *OFunc, shared *Shared, diags []Diag) {
	defer func() {
		if r := recover(); r != nil {
			ce, ok := r.(compileErr)
			if !ok {
				panic(r)
			}
			fn, shared = nil, nil
			diags = []Diag{Diag(ce)}
		}
	}()
	shared = newShared(gc, map[string]string{"": src})
	c := &Compiler{shared: shared, fn: &OFunc{Name: "<script>"}}
	gc.alloc(c.fn)
	c.locals = append(c.locals, LocalVar{name: "", depth: 0, slot: 0}) // slot 0: script itself
	for _, s := range stmts {
		c.stmt(s)
	}
	c.emit(OpUnit, Span{})
	c.emit(OpReturn, Span{})
	if err := verifyAll(c.fn); err != nil {
		panic(fmt.Sprintf("internal error: bytecode verifier: %v", err))
	}
	return c.fn, shared, nil
}

// compileModule compiles every package of a loaded module into one program:
// per-file init functions define each package's globals in dependency order,
// then the entry package's main runs.
func compileModule(gc *GC, m *Module) (fn *OFunc, shared *Shared, diags []Diag) {
	defer func() {
		if r := recover(); r != nil {
			ce, ok := r.(compileErr)
			if !ok {
				panic(r)
			}
			fn, shared = nil, nil
			diags = []Diag{Diag(ce)}
		}
	}()
	shared = newShared(gc, m.Srcs)
	c := &Compiler{shared: shared, fn: &OFunc{Name: "<module>"}}
	gc.alloc(c.fn)
	c.locals = append(c.locals, LocalVar{name: "", depth: 0, slot: 0}) // slot 0: the program itself
	for _, pkg := range m.Packages {
		shared.curPkg = pkg.ImportPath
		shared.pkgDecls = pkgDeclNames(pkg)
		// register every enum of the package before compiling any body, so
		// variant references resolve across files regardless of order
		for _, f := range pkg.Files {
			shared.curFile = f.Path
			for _, s := range f.Stmts {
				if st, ok := s.(*EnumDecl); ok {
					c.registerEnumDecl(st)
				}
			}
		}
		for _, f := range pkg.Files {
			shared.curFile = f.Path
			c.fileInit(f)
		}
	}
	shared.curPkg, shared.curFile, shared.pkgDecls = "", "", nil

	mainName := m.Entry.ImportPath + ".main"
	idx := c.chunk().addConst(ObjV(gc.newString(mainName)))
	c.emit(OpGetGlobal, Span{})
	c.emitU16(idx, Span{})
	c.emit(OpCall, Span{})
	c.emit(0, Span{})
	c.emit(OpPop, Span{})
	c.emit(OpUnit, Span{})
	c.emit(OpReturn, Span{})
	if err := verifyAll(c.fn); err != nil {
		panic(fmt.Sprintf("internal error: bytecode verifier: %v", err))
	}
	return c.fn, shared, nil
}

// pkgDeclNames collects the package's top-level names; references to them
// compile to qualified globals.
func pkgDeclNames(pkg *Package) map[string]bool {
	names := map[string]bool{}
	for _, f := range pkg.Files {
		for _, s := range f.Stmts {
			switch st := s.(type) {
			case *FnDecl:
				names[st.Name] = true
			case *EnumDecl:
				names[st.Name] = true
			case *LetStmt:
				names[st.Name] = true
			}
		}
	}
	return names
}

// fileInit compiles one package file's declarations into an init function
// and emits a call to it, so runtime errors during initialization attribute
// to the right file.
func (c *Compiler) fileInit(f *SourceFile) {
	sub := &Compiler{
		shared: c.shared,
		fn:     &OFunc{Name: "<init>", File: f.Path},
	}
	c.shared.gc.alloc(sub.fn)
	sub.locals = append(sub.locals, LocalVar{name: "", depth: 0, slot: 0})
	for _, s := range f.Stmts {
		sub.stmt(s)
	}
	sub.emit(OpUnit, Span{})
	sub.emit(OpReturn, Span{})
	if len(sub.upvals) > 0 {
		panic("internal error: package file init captured variables")
	}
	idx := c.chunk().addConst(ObjV(sub.fn))
	c.emit(OpClosure, Span{})
	c.emitU16(idx, Span{})
	c.emit(OpCall, Span{})
	c.emit(0, Span{})
	c.emit(OpPop, Span{})
}

func (c *Compiler) fail(sp Span, code, help, format string, args ...any) {
	panic(compileErr(Diag{IsErr: true, Code: code, Msg: fmt.Sprintf(format, args...),
		Help: help, File: c.shared.curFile, Span: sp}))
}

func (c *Compiler) chunk() *Chunk { return &c.fn.Chunk }

func (c *Compiler) emit(b byte, sp Span)      { c.chunk().write(b, sp) }
func (c *Compiler) emitU16(v uint16, sp Span) { c.chunk().writeU16(v, sp) }
func (c *Compiler) emitConst(v Value, sp Span) {
	idx := c.chunk().addConst(v)
	c.emit(OpConst, sp)
	c.emitU16(idx, sp)
}

func (c *Compiler) emitJump(op Op, sp Span) int {
	c.emit(op, sp)
	c.emitU16(0xffff, sp)
	return len(c.chunk().Code) - 2
}

func (c *Compiler) patchJump(at int) {
	dist := len(c.chunk().Code) - (at + 2)
	if dist > 0xffff {
		c.fail(Span{}, "E2001", "", "jump too large")
	}
	c.chunk().Code[at] = byte(dist >> 8)
	c.chunk().Code[at+1] = byte(dist & 0xff)
}

func (c *Compiler) emitLoop(to int, sp Span) {
	c.emit(OpLoop, sp)
	dist := len(c.chunk().Code) + 2 - to
	if dist > 0xffff {
		c.fail(sp, "E2002", "", "loop body too large")
	}
	c.emitU16(uint16(dist), sp)
}

// ---- scopes and locals ----

func (c *Compiler) nextSlot() int {
	if len(c.locals) > 0 {
		last := c.locals[len(c.locals)-1]
		return last.slot + 1 + (c.temps - last.tempsBelow)
	}
	return c.temps
}

// declareLocal assumes the value is already on the stack top; nextSlot()
// computes from the locals list (which excludes that value) plus any
// temporaries pushed since the previous local, which is exactly the slot
// the value landed on
func (c *Compiler) declareLocal(name string, mut bool, sp Span) {
	slot := c.nextSlot()
	if slot > 0xfffe {
		c.fail(sp, "E2003", "", "too many locals")
	}
	c.locals = append(c.locals, LocalVar{
		name: name, depth: c.scopeDepth, slot: slot, mut: mut, tempsBelow: c.temps,
	})
}

func (c *Compiler) beginScope() { c.scopeDepth++ }

// endScope for statement contexts: emits pop/close for each local
func (c *Compiler) endScope(sp Span) {
	c.scopeDepth--
	for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth > c.scopeDepth {
		if c.locals[len(c.locals)-1].captured {
			c.emit(OpCloseUpvalue, sp)
		} else {
			c.emit(OpPop, sp)
		}
		c.locals = c.locals[:len(c.locals)-1]
	}
}

// endScopeExpr for expression blocks: result is on top; OpEndBlock removes
// the scope's locals underneath it (closing captured ones)
func (c *Compiler) endScopeExpr(sp Span) {
	c.scopeDepth--
	n := 0
	for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth > c.scopeDepth {
		n++
		c.locals = c.locals[:len(c.locals)-1]
	}
	if n > 0 {
		if n > 255 {
			c.fail(sp, "E2003", "", "too many locals in block")
		}
		c.emit(OpEndBlock, sp)
		c.emit(byte(n), sp)
	}
}

func (c *Compiler) resolveLocal(name string) int {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].name == name {
			return i
		}
	}
	return -1
}

func (c *Compiler) addUpvalue(index uint16, isLocal, mut bool, sp Span) int {
	for i, uv := range c.upvals {
		if uv.index == index && uv.isLocal == isLocal {
			return i
		}
	}
	if len(c.upvals) >= 255 {
		c.fail(sp, "E2004", "", "too many captured variables")
	}
	c.upvals = append(c.upvals, UpvalMeta{index: index, isLocal: isLocal, mut: mut})
	c.fn.NumUpvals = len(c.upvals)
	return len(c.upvals) - 1
}

func (c *Compiler) resolveUpvalue(name string, sp Span) int {
	if c.enclosing == nil {
		return -1
	}
	if li := c.enclosing.resolveLocal(name); li >= 0 {
		l := &c.enclosing.locals[li]
		l.captured = true
		return c.addUpvalue(uint16(l.slot), true, l.mut, sp)
	}
	if ui := c.enclosing.resolveUpvalue(name, sp); ui >= 0 {
		return c.addUpvalue(uint16(ui), false, c.enclosing.upvals[ui].mut, sp)
	}
	return -1
}

// ---- statements ----

func (c *Compiler) stmt(s Stmt) {
	switch st := s.(type) {
	case *LetStmt:
		c.expr(st.Init)
		if c.scopeDepth == 0 {
			c.defineGlobal(st.Name, st.Mut, st.Span)
		} else {
			c.declareLocal(st.Name, st.Mut, st.Span)
		}
	case *FnDecl:
		if c.scopeDepth == 0 {
			c.function(st.Fn)
			c.defineGlobal(st.Name, false, st.Span)
		} else {
			// declare first so the body can refer to itself
			c.emit(OpUnit, st.Span)
			c.declareLocal(st.Name, true, st.Span) // internally mutable for the fixup below
			c.function(st.Fn)
			slot := c.locals[c.resolveLocal(st.Name)].slot
			c.emit(OpSetLocal, st.Span)
			c.emitU16(uint16(slot), st.Span)
			c.emit(OpPop, st.Span)
			c.locals[c.resolveLocal(st.Name)].mut = false
		}
	case *EnumDecl:
		c.enumDecl(st)
	case *ExprStmt:
		c.expr(st.E)
		c.emit(OpPop, st.Span)
	case *AssignStmt:
		c.assign(st)
	case *ReturnStmt:
		if st.Val != nil {
			c.expr(st.Val)
		} else {
			c.emit(OpUnit, st.Span)
		}
		c.emit(OpReturn, st.Span)
	case *WhileStmt:
		c.whileStmt(st)
	case *ForStmt:
		c.forStmt(st)
	case *BreakStmt:
		c.breakStmt(st.Span)
	case *ContinueStmt:
		c.continueStmt(st.Span)
	case *SpawnStmt:
		c.expr(st.CallE.Callee)
		c.temps++
		for _, a := range st.CallE.Args {
			c.expr(a)
			c.temps++
		}
		c.temps -= len(st.CallE.Args) + 1
		if len(st.CallE.Args) > 255 {
			c.fail(st.Span, "E2005", "", "too many arguments")
		}
		c.emit(OpSpawn, st.Span)
		c.emit(byte(len(st.CallE.Args)), st.Span)
	case *SendStmt:
		c.expr(st.Chan)
		c.temps++
		c.expr(st.Val)
		c.temps--
		c.emit(OpSend, st.Span)
	default:
		panic(fmt.Sprintf("unhandled stmt %T", s))
	}
}

func (c *Compiler) defineGlobal(name string, mut bool, sp Span) {
	qual := c.shared.qualify(name)
	if _, exists := c.shared.globalMut[qual]; exists {
		if _, isEnum := c.shared.enums[qual]; isEnum {
			c.fail(sp, "E2007", "", "cannot redeclare enum %q", name)
		}
	}
	c.shared.globalMut[qual] = mut
	idx := c.chunk().addConst(ObjV(c.shared.gc.newString(qual)))
	c.emit(OpDefGlobal, sp)
	c.emitU16(idx, sp)
}

func (c *Compiler) assign(st *AssignStmt) {
	switch t := st.Target.(type) {
	case *Ident:
		c.expr(st.Val)
		if li := c.resolveLocal(t.Name); li >= 0 {
			l := c.locals[li]
			if !l.mut {
				c.fail(st.Span, "E2010", "declare it with `let mut`", "cannot assign to immutable variable %q", t.Name)
			}
			c.emit(OpSetLocal, st.Span)
			c.emitU16(uint16(l.slot), st.Span)
		} else if ui := c.resolveUpvalue(t.Name, st.Span); ui >= 0 {
			if !c.upvals[ui].mut {
				c.fail(st.Span, "E2010", "declare it with `let mut`", "cannot assign to immutable variable %q", t.Name)
			}
			c.emit(OpSetUpval, st.Span)
			c.emitU16(uint16(ui), st.Span)
		} else if mut, ok := c.shared.globalMut[c.shared.mangleIfDecl(t.Name)]; ok {
			if !mut {
				c.fail(st.Span, "E2010", "declare it with `let mut`", "cannot assign to immutable variable %q", t.Name)
			}
			idx := c.chunk().addConst(ObjV(c.shared.gc.newString(c.shared.mangleIfDecl(t.Name))))
			c.emit(OpSetGlobal, st.Span)
			c.emitU16(idx, st.Span)
		} else {
			c.fail(st.Span, "E2011", "", "cannot assign to undeclared variable %q", t.Name)
		}
		c.emit(OpPop, st.Span)
	case *Index:
		c.expr(t.Target)
		c.temps++
		c.expr(t.Idx)
		c.temps++
		c.expr(st.Val)
		c.temps -= 2
		c.emit(OpIndexSet, st.Span)
	default:
		c.fail(st.Span, "E2012", "", "invalid assignment target")
	}
}

func (c *Compiler) whileStmt(st *WhileStmt) {
	start := len(c.chunk().Code)
	c.loops = append(c.loops, loopCtx{baseSlot: c.nextSlot(), continueTo: start})
	c.expr(st.Cond)
	exitJ := c.emitJump(OpJumpIfFalsePop, st.Span)
	c.blockAsStmts(st.Body)
	c.emitLoop(start, st.Span)
	c.patchJump(exitJ)
	lp := c.loops[len(c.loops)-1]
	for _, j := range lp.breakJumps {
		c.patchJump(j)
	}
	c.loops = c.loops[:len(c.loops)-1]
}

func (c *Compiler) forStmt(st *ForStmt) {
	c.beginScope()
	c.expr(st.Iter)
	c.declareLocal("(iter)", false, st.Span)
	iterSlot := c.locals[len(c.locals)-1].slot
	c.emitConst(IntV(0), st.Span)
	c.declareLocal("(idx)", false, st.Span)
	idxSlot := c.locals[len(c.locals)-1].slot

	c.loops = append(c.loops, loopCtx{baseSlot: c.nextSlot(), continueTo: -1})

	condStart := len(c.chunk().Code)
	c.emit(OpGetLocal, st.Span)
	c.emitU16(uint16(idxSlot), st.Span)
	c.emit(OpGetLocal, st.Span)
	c.emitU16(uint16(iterSlot), st.Span)
	c.emit(OpLen, st.Span)
	c.emit(OpLt, st.Span)
	exitJ := c.emitJump(OpJumpIfFalsePop, st.Span)

	c.beginScope()
	c.emit(OpGetLocal, st.Span)
	c.emitU16(uint16(iterSlot), st.Span)
	c.emit(OpGetLocal, st.Span)
	c.emitU16(uint16(idxSlot), st.Span)
	c.emit(OpIndexGet, st.Span)
	c.declareLocal(st.Var, false, st.Span)
	for _, s := range st.Body.Stmts {
		c.stmt(s)
	}
	c.endScope(st.Span)

	// continue lands here: increment
	incr := len(c.chunk().Code)
	lp := &c.loops[len(c.loops)-1]
	for _, j := range lp.continueJumps {
		c.patchJump(j)
	}
	_ = incr
	c.emit(OpGetLocal, st.Span)
	c.emitU16(uint16(idxSlot), st.Span)
	c.emitConst(IntV(1), st.Span)
	c.emit(OpAdd, st.Span)
	c.emit(OpSetLocal, st.Span)
	c.emitU16(uint16(idxSlot), st.Span)
	c.emit(OpPop, st.Span)
	c.emitLoop(condStart, st.Span)

	c.patchJump(exitJ)
	for _, j := range lp.breakJumps {
		c.patchJump(j)
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.endScope(st.Span) // pops idx, iter
}

// emit pops for locals above the innermost loop's base without altering
// compiler bookkeeping (execution continues past the jump for other paths)
func (c *Compiler) unwindToLoop(sp Span) *loopCtx {
	if len(c.loops) == 0 {
		c.fail(sp, "E2013", "", "break/continue outside of a loop")
	}
	lp := &c.loops[len(c.loops)-1]
	for i := len(c.locals) - 1; i >= 0 && c.locals[i].slot >= lp.baseSlot; i-- {
		if c.locals[i].captured {
			c.emit(OpCloseUpvalue, sp)
		} else {
			c.emit(OpPop, sp)
		}
	}
	return lp
}

func (c *Compiler) breakStmt(sp Span) {
	lp := c.unwindToLoop(sp)
	lp.breakJumps = append(lp.breakJumps, c.emitJump(OpJump, sp))
}

func (c *Compiler) continueStmt(sp Span) {
	lp := c.unwindToLoop(sp)
	if lp.continueTo >= 0 {
		c.emitLoop(lp.continueTo, sp)
	} else {
		lp.continueJumps = append(lp.continueJumps, c.emitJump(OpJump, sp))
	}
}

func (c *Compiler) enumDecl(st *EnumDecl) {
	if c.scopeDepth > 0 {
		c.fail(st.Span, "E2008", "", "enum declarations are only allowed at top level")
	}
	qual := c.shared.qualify(st.Name)
	info, registered := c.shared.enums[qual]
	if !registered {
		info = c.registerEnumDecl(st)
	} else if c.shared.curPkg == "" {
		// module builds pre-register every enum; in a script an existing
		// entry is a redeclaration
		c.fail(st.Span, "E2007", "", "enum %q is already declared", st.Name)
	}
	c.emitConst(ObjV(info.runtime), st.Span)
	idx := c.chunk().addConst(ObjV(c.shared.gc.newString(qual)))
	c.emit(OpDefGlobal, st.Span)
	c.emitU16(idx, st.Span)
}

// registerEnumDecl records an enum's compile-time info without emitting
// code; module builds run it over every file before compiling bodies so
// cross-file variant references resolve regardless of order.
func (c *Compiler) registerEnumDecl(st *EnumDecl) *EnumInfo {
	qual := c.shared.qualify(st.Name)
	if _, exists := c.shared.enums[qual]; exists {
		c.fail(st.Span, "E2007", "", "enum %q is already declared", st.Name)
	}
	seen := map[string]bool{}
	var variants []VariantInfo
	for _, v := range st.Variants {
		if seen[v.Name] {
			c.fail(st.Span, "E2009", "", "duplicate variant %q in enum %q", v.Name, st.Name)
		}
		seen[v.Name] = true
		variants = append(variants, VariantInfo{Name: v.Name, Arity: v.Arity})
	}
	return c.shared.declareEnum(qual, c.shared.curPkg, variants)
}

// ---- expressions ----

func (c *Compiler) expr(e Expr) {
	switch ex := e.(type) {
	case *IntLit:
		c.emitConst(IntV(ex.Val), ex.Span)
	case *FloatLit:
		c.emitConst(FloatV(ex.Val), ex.Span)
	case *StrLit:
		c.emitConst(ObjV(c.shared.gc.newString(ex.Val)), ex.Span)
	case *BoolLit:
		if ex.Val {
			c.emit(OpTrue, ex.Span)
		} else {
			c.emit(OpFalse, ex.Span)
		}
	case *Ident:
		c.identExpr(ex)
	case *Unary:
		c.expr(ex.Rhs)
		if ex.Op == TMinus {
			c.emit(OpNeg, ex.Span)
		} else {
			c.emit(OpNot, ex.Span)
		}
	case *Binary:
		c.expr(ex.Lhs)
		c.temps++
		c.expr(ex.Rhs)
		c.temps--
		switch ex.Op {
		case TPlus:
			c.emit(OpAdd, ex.Span)
		case TMinus:
			c.emit(OpSub, ex.Span)
		case TStar:
			c.emit(OpMul, ex.Span)
		case TSlash:
			c.emit(OpDiv, ex.Span)
		case TPercent:
			c.emit(OpMod, ex.Span)
		case TEqEq:
			c.emit(OpEq, ex.Span)
		case TBangEq:
			c.emit(OpNeq, ex.Span)
		case TGt:
			c.emit(OpGt, ex.Span)
		case TGtEq:
			c.emit(OpGtEq, ex.Span)
		case TLt:
			c.emit(OpLt, ex.Span)
		case TLtEq:
			c.emit(OpLtEq, ex.Span)
		}
	case *Logical:
		c.expr(ex.Lhs)
		var end int
		if ex.Op == TAndAnd {
			end = c.emitJump(OpJumpIfFalse, ex.Span)
		} else {
			end = c.emitJump(OpJumpIfTrue, ex.Span)
		}
		c.emit(OpPop, ex.Span)
		c.expr(ex.Rhs)
		c.patchJump(end)
	case *Call:
		c.expr(ex.Callee)
		c.temps++
		for _, a := range ex.Args {
			c.expr(a)
			c.temps++
		}
		c.temps -= len(ex.Args) + 1
		if len(ex.Args) > 255 {
			c.fail(ex.Span, "E2005", "", "too many arguments")
		}
		c.emit(OpCall, ex.Span)
		c.emit(byte(len(ex.Args)), ex.Span)
	case *Index:
		c.expr(ex.Target)
		c.temps++
		c.expr(ex.Idx)
		c.temps--
		c.emit(OpIndexGet, ex.Span)
	case *ListLit:
		for _, el := range ex.Elems {
			c.expr(el)
			c.temps++
		}
		c.temps -= len(ex.Elems)
		if len(ex.Elems) > 0xffff {
			c.fail(ex.Span, "E2006", "", "list literal too large")
		}
		c.emit(OpList, ex.Span)
		c.emitU16(uint16(len(ex.Elems)), ex.Span)
	case *FnLit:
		c.function(ex)
	case *Block:
		c.blockExpr(ex)
	case *IfExpr:
		c.ifExpr(ex)
	case *MatchExpr:
		c.matchExpr(ex)
	case *VariantAccess:
		c.variantAccess(ex.EnumName, ex.Variant, ex.Span)
	case *PkgAccess:
		idx := c.chunk().addConst(ObjV(c.shared.gc.newString(ex.Pkg + "." + ex.Name)))
		c.emit(OpGetGlobal, ex.Span)
		c.emitU16(idx, ex.Span)
	case *QualVariantAccess:
		info, ok := c.shared.enums[ex.Pkg+"."+ex.Enum]
		if !ok {
			c.fail(ex.Span, "E2015", "", "unknown enum %q", ex.Pkg+"."+ex.Enum)
		}
		c.emitVariantOf(info, ex.Variant, ex.Span)
	case *TryExpr:
		c.expr(ex.Inner)
		c.emit(OpTry, ex.Span)
	case *RecvExpr:
		c.expr(ex.Chan)
		c.emit(OpRecv, ex.Span)
	default:
		panic(fmt.Sprintf("unhandled expr %T", e))
	}
}

func (c *Compiler) identExpr(ex *Ident) {
	if li := c.resolveLocal(ex.Name); li >= 0 {
		c.emit(OpGetLocal, ex.Span)
		c.emitU16(uint16(c.locals[li].slot), ex.Span)
		return
	}
	if ui := c.resolveUpvalue(ex.Name, ex.Span); ui >= 0 {
		c.emit(OpGetUpval, ex.Span)
		c.emitU16(uint16(ui), ex.Span)
		return
	}
	name := c.shared.mangleIfDecl(ex.Name)
	if _, declared := c.shared.globalMut[name]; !declared {
		// bare enum variant like Some / None / Ok / Err
		if info, idx, amb := c.shared.findVariant(ex.Name); amb {
			c.fail(ex.Span, "E2014", fmt.Sprintf("qualify it as Enum.%s", ex.Name), "variant %q exists in several enums", ex.Name)
		} else if idx >= 0 {
			c.emitVariant(info, idx, ex.Span)
			return
		}
	}
	// global (possibly defined later in the script)
	idx := c.chunk().addConst(ObjV(c.shared.gc.newString(name)))
	c.emit(OpGetGlobal, ex.Span)
	c.emitU16(idx, ex.Span)
}

func (c *Compiler) variantAccess(enumName, variant string, sp Span) {
	info, ok := c.shared.localEnum(enumName)
	if !ok {
		c.fail(sp, "E2015", "", "unknown enum %q", enumName)
	}
	c.emitVariantOf(info, variant, sp)
}

// emitVariantOf emits the constructor or singleton for a named variant.
func (c *Compiler) emitVariantOf(info *EnumInfo, variant string, sp Span) {
	for i, v := range info.Variants {
		if v.Name == variant {
			c.emitVariant(info, i, sp)
			return
		}
	}
	c.fail(sp, "E2016", "", "enum %q has no variant %q", info.Name, variant)
}

// nullary variants become singleton instance constants; variants with fields
// become constructor constants (called like functions)
func (c *Compiler) emitVariant(info *EnumInfo, idx int, sp Span) {
	if info.Variants[idx].Arity == 0 {
		if info.singletons[idx] == nil {
			inst := &OEnumInst{Enum: info.runtime, Variant: idx}
			c.shared.gc.alloc(inst)
			info.singletons[idx] = inst
		}
		c.emitConst(ObjV(info.singletons[idx]), sp)
	} else {
		if info.ctors[idx] == nil {
			ctor := &OVariantCtor{Enum: info.runtime, Idx: idx}
			c.shared.gc.alloc(ctor)
			info.ctors[idx] = ctor
		}
		c.emitConst(ObjV(info.ctors[idx]), sp)
	}
}

func (c *Compiler) function(lit *FnLit) {
	sub := &Compiler{
		enclosing: c,
		shared:    c.shared,
		fn:        &OFunc{Name: lit.Name, File: c.shared.curFile, Arity: len(lit.Params)},
	}
	c.shared.gc.alloc(sub.fn)
	sub.beginScope()
	selfName := ""
	if lit.Name != "" {
		selfName = lit.Name // recursion through slot 0
	}
	sub.locals = append(sub.locals, LocalVar{name: selfName, depth: sub.scopeDepth, slot: 0})
	if len(lit.Params) > 255 {
		sub.fail(lit.Span, "E2019", "", "too many parameters")
	}
	seen := map[string]bool{}
	for _, p := range lit.Params {
		if seen[p] {
			sub.fail(lit.Span, "E2020", "", "duplicate parameter %q", p)
		}
		seen[p] = true
		sub.locals = append(sub.locals, LocalVar{
			name: p, depth: sub.scopeDepth, slot: sub.locals[len(sub.locals)-1].slot + 1,
		})
	}
	sub.blockExpr(lit.Body)
	sub.emit(OpReturn, lit.Span)

	idx := c.chunk().addConst(ObjV(sub.fn))
	c.emit(OpClosure, lit.Span)
	c.emitU16(idx, lit.Span)
	for _, uv := range sub.upvals {
		if uv.isLocal {
			c.emit(1, lit.Span)
		} else {
			c.emit(0, lit.Span)
		}
		c.emitU16(uv.index, lit.Span)
	}
}

// blockExpr compiles a block leaving its value on the stack
func (c *Compiler) blockExpr(b *Block) {
	c.beginScope()
	n := len(b.Stmts)
	valueProduced := false
	for i, s := range b.Stmts {
		if i == n-1 {
			if es, ok := s.(*ExprStmt); ok {
				c.expr(es.E)
				valueProduced = true
				break
			}
		}
		c.stmt(s)
	}
	if !valueProduced {
		c.emit(OpUnit, b.Span)
	}
	c.endScopeExpr(b.Span)
}

// blockAsStmts compiles a block purely for effect (loop bodies)
func (c *Compiler) blockAsStmts(b *Block) {
	c.beginScope()
	for _, s := range b.Stmts {
		c.stmt(s)
	}
	c.endScope(b.Span)
}

func (c *Compiler) ifExpr(ex *IfExpr) {
	c.expr(ex.Cond)
	elseJ := c.emitJump(OpJumpIfFalsePop, ex.Span)
	c.blockExpr(ex.Then)
	endJ := c.emitJump(OpJump, ex.Span)
	c.patchJump(elseJ)
	switch els := ex.Else.(type) {
	case nil:
		c.emit(OpUnit, ex.Span)
	case *Block:
		c.blockExpr(els)
	case *IfExpr:
		c.ifExpr(els)
	default:
		c.fail(ex.Span, "E2018", "", "invalid else branch")
	}
	c.patchJump(endJ)
}

func (c *Compiler) matchExpr(m *MatchExpr) {
	c.beginScope()
	c.expr(m.Scrut)
	c.declareLocal("(match)", false, m.Span)
	scrutSlot := c.locals[len(c.locals)-1].slot

	var endJumps []int
	for _, arm := range m.Arms {
		var failJ = -1
		c.beginScope()
		armLocals := 0

		switch pat := arm.Pat.(type) {
		case *PatWildcard:
			// always matches
		case *PatLiteral:
			c.emit(OpGetLocal, arm.Span)
			c.emitU16(uint16(scrutSlot), arm.Span)
			c.temps++
			c.expr(pat.Val)
			c.temps--
			c.emit(OpEq, arm.Span)
			failJ = c.emitJump(OpJumpIfFalsePop, arm.Span)
		case *PatBinding:
			// a bare name is a nullary-variant test if such a variant exists,
			// otherwise a catch-all binding
			if info, idx, amb := c.shared.findVariant(pat.Name); amb {
				c.fail(arm.Span, "E2014", fmt.Sprintf("qualify it as Enum.%s", pat.Name), "variant %q exists in several enums", pat.Name)
			} else if idx >= 0 && info.Variants[idx].Arity == 0 {
				failJ = c.emitVariantTest(info, idx, scrutSlot, arm.Span)
			} else if idx >= 0 {
				c.fail(arm.Span, "E2017", fmt.Sprintf("bind them: %s(...)", pat.Name), "variant %q has %d field(s)", pat.Name, info.Variants[idx].Arity)
			} else {
				c.emit(OpGetLocal, arm.Span)
				c.emitU16(uint16(scrutSlot), arm.Span)
				c.declareLocal(pat.Name, false, arm.Span)
				armLocals++
			}
		case *PatVariant:
			var info *EnumInfo
			var idx int
			if pat.Pkg != "" {
				var ok bool
				info, ok = c.shared.enums[pat.Pkg+"."+pat.EnumName]
				if !ok {
					c.fail(arm.Span, "E2015", "", "unknown enum %q", pat.Pkg+"."+pat.EnumName)
				}
				idx = -1
				for i, v := range info.Variants {
					if v.Name == pat.Variant {
						idx = i
						break
					}
				}
				if idx < 0 {
					c.fail(arm.Span, "E2016", "", "enum %q has no variant %q", pat.EnumName, pat.Variant)
				}
			} else if pat.EnumName != "" {
				var ok bool
				info, ok = c.shared.localEnum(pat.EnumName)
				if !ok {
					c.fail(arm.Span, "E2015", "", "unknown enum %q", pat.EnumName)
				}
				idx = -1
				for i, v := range info.Variants {
					if v.Name == pat.Variant {
						idx = i
						break
					}
				}
				if idx < 0 {
					c.fail(arm.Span, "E2016", "", "enum %q has no variant %q", pat.EnumName, pat.Variant)
				}
			} else {
				var amb bool
				info, idx, amb = c.shared.findVariant(pat.Variant)
				if amb {
					c.fail(arm.Span, "E2014", fmt.Sprintf("qualify it as Enum.%s", pat.Variant), "variant %q exists in several enums", pat.Variant)
				}
				if idx < 0 {
					c.fail(arm.Span, "E2016", "", "unknown variant %q", pat.Variant)
				}
			}
			arity := info.Variants[idx].Arity
			if len(pat.Binds) != arity {
				c.fail(arm.Span, "E2017", "", "variant %s has %d field(s), pattern binds %d", pat.Variant, arity, len(pat.Binds))
			}
			failJ = c.emitVariantTest(info, idx, scrutSlot, arm.Span)
			for i, sub := range pat.Binds {
				name := "_"
				if b, ok := sub.(*PatBinding); ok {
					name = b.Name
				}
				if name == "_" {
					continue
				}
				c.emit(OpGetLocal, arm.Span)
				c.emitU16(uint16(scrutSlot), arm.Span)
				c.emit(OpGetField, arm.Span)
				c.emit(byte(i), arm.Span)
				c.declareLocal(name, false, arm.Span)
				armLocals++
			}
		}

		c.expr(arm.Body)
		if armLocals > 0 {
			c.emit(OpEndBlock, arm.Span)
			c.emit(byte(armLocals), arm.Span)
		}
		c.scopeDepth--
		for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth > c.scopeDepth {
			c.locals = c.locals[:len(c.locals)-1]
		}
		endJumps = append(endJumps, c.emitJump(OpJump, arm.Span))
		if failJ >= 0 {
			c.patchJump(failJ)
		}
	}
	c.emit(OpNoMatch, m.Span)
	for _, j := range endJumps {
		c.patchJump(j)
	}
	// squash the scrutinee slot, leaving the arm value
	c.scopeDepth--
	c.locals = c.locals[:len(c.locals)-1]
	c.emit(OpEndBlock, m.Span)
	c.emit(1, m.Span)
}

func (c *Compiler) emitVariantTest(info *EnumInfo, idx int, scrutSlot int, sp Span) int {
	c.emit(OpGetLocal, sp)
	c.emitU16(uint16(scrutSlot), sp)
	c.emitConst(ObjV(info.runtime), sp)
	c.emit(OpTestVariant, sp)
	c.emit(byte(idx), sp)
	return c.emitJump(OpJumpIfFalsePop, sp)
}
