package main

import "fmt"

type EnumInfo struct {
	Name       string
	Variants   []VariantInfo
	runtime    *OEnumType
	singletons []*OEnumInst   // cached nullary instances
	ctors      []*OVariantCtor // cached constructors
}

// Shared holds cross-function compile state.
type Shared struct {
	gc        *GC
	enums     map[string]*EnumInfo
	globalMut map[string]bool // declared globals -> is mutable
}

func newShared(gc *GC) *Shared {
	s := &Shared{gc: gc, enums: map[string]*EnumInfo{}, globalMut: map[string]bool{}}
	s.declareEnum("Option", []VariantInfo{{"Some", 1}, {"None", 0}})
	s.declareEnum("Result", []VariantInfo{{"Ok", 1}, {"Err", 1}})
	for _, n := range nativeNames() {
		s.globalMut[n] = false
	}
	return s
}

func (s *Shared) declareEnum(name string, variants []VariantInfo) *EnumInfo {
	rt := &OEnumType{Name: name, Variants: variants}
	s.gc.alloc(rt)
	info := &EnumInfo{
		Name: name, Variants: variants, runtime: rt,
		singletons: make([]*OEnumInst, len(variants)),
		ctors:      make([]*OVariantCtor, len(variants)),
	}
	s.enums[name] = info
	s.globalMut[name] = false
	return info
}

// find a variant by bare name across all enums; ambiguous if several match
func (s *Shared) findVariant(name string) (info *EnumInfo, idx int, ambiguous bool) {
	idx = -1
	for _, e := range s.enums {
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

type compileErr string

func compileProgram(gc *GC, stmts []Stmt) (fn *OFunc, shared *Shared, err error) {
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(compileErr); ok {
				err = fmt.Errorf("%s", string(ce))
				return
			}
			panic(r)
		}
	}()
	shared = newShared(gc)
	c := &Compiler{shared: shared, fn: &OFunc{Name: "<script>"}}
	gc.alloc(c.fn)
	c.locals = append(c.locals, LocalVar{name: "", depth: 0, slot: 0}) // slot 0: script itself
	for _, s := range stmts {
		c.stmt(s)
	}
	c.emit(OpUnit, 0)
	c.emit(OpReturn, 0)
	return c.fn, shared, nil
}

func (c *Compiler) fail(line int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	panic(compileErr(fmt.Sprintf("compile error at line %d: %s", line, msg)))
}

func (c *Compiler) chunk() *Chunk { return &c.fn.Chunk }

func (c *Compiler) emit(b byte, line int)        { c.chunk().write(b, line) }
func (c *Compiler) emitU16(v uint16, line int)   { c.chunk().writeU16(v, line) }
func (c *Compiler) emitConst(v Value, line int) {
	idx := c.chunk().addConst(v)
	c.emit(OpConst, line)
	c.emitU16(idx, line)
}

func (c *Compiler) emitJump(op Op, line int) int {
	c.emit(op, line)
	c.emitU16(0xffff, line)
	return len(c.chunk().Code) - 2
}

func (c *Compiler) patchJump(at int) {
	dist := len(c.chunk().Code) - (at + 2)
	if dist > 0xffff {
		c.fail(0, "jump too large")
	}
	c.chunk().Code[at] = byte(dist >> 8)
	c.chunk().Code[at+1] = byte(dist & 0xff)
}

func (c *Compiler) emitLoop(to int, line int) {
	c.emit(OpLoop, line)
	dist := len(c.chunk().Code) + 2 - to
	if dist > 0xffff {
		c.fail(line, "loop body too large")
	}
	c.emitU16(uint16(dist), line)
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
func (c *Compiler) declareLocal(name string, mut bool, line int) {
	slot := c.nextSlot()
	if slot > 0xfffe {
		c.fail(line, "too many locals")
	}
	c.locals = append(c.locals, LocalVar{
		name: name, depth: c.scopeDepth, slot: slot, mut: mut, tempsBelow: c.temps,
	})
}

func (c *Compiler) beginScope() { c.scopeDepth++ }

// endScope for statement contexts: emits pop/close for each local
func (c *Compiler) endScope(line int) {
	c.scopeDepth--
	for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth > c.scopeDepth {
		if c.locals[len(c.locals)-1].captured {
			c.emit(OpCloseUpvalue, line)
		} else {
			c.emit(OpPop, line)
		}
		c.locals = c.locals[:len(c.locals)-1]
	}
}

// endScopeExpr for expression blocks: result is on top; OpEndBlock removes
// the scope's locals underneath it (closing captured ones)
func (c *Compiler) endScopeExpr(line int) {
	c.scopeDepth--
	n := 0
	for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth > c.scopeDepth {
		n++
		c.locals = c.locals[:len(c.locals)-1]
	}
	if n > 0 {
		if n > 255 {
			c.fail(line, "too many locals in block")
		}
		c.emit(OpEndBlock, line)
		c.emit(byte(n), line)
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

func (c *Compiler) addUpvalue(index uint16, isLocal, mut bool, line int) int {
	for i, uv := range c.upvals {
		if uv.index == index && uv.isLocal == isLocal {
			return i
		}
	}
	if len(c.upvals) >= 255 {
		c.fail(line, "too many captured variables")
	}
	c.upvals = append(c.upvals, UpvalMeta{index: index, isLocal: isLocal, mut: mut})
	c.fn.NumUpvals = len(c.upvals)
	return len(c.upvals) - 1
}

func (c *Compiler) resolveUpvalue(name string, line int) int {
	if c.enclosing == nil {
		return -1
	}
	if li := c.enclosing.resolveLocal(name); li >= 0 {
		l := &c.enclosing.locals[li]
		l.captured = true
		return c.addUpvalue(uint16(l.slot), true, l.mut, line)
	}
	if ui := c.enclosing.resolveUpvalue(name, line); ui >= 0 {
		return c.addUpvalue(uint16(ui), false, c.enclosing.upvals[ui].mut, line)
	}
	return -1
}

// ---- statements ----

func (c *Compiler) stmt(s Stmt) {
	switch st := s.(type) {
	case *LetStmt:
		c.expr(st.Init)
		if c.scopeDepth == 0 {
			c.defineGlobal(st.Name, st.Mut, st.Line)
		} else {
			c.declareLocal(st.Name, st.Mut, st.Line)
		}
	case *FnDecl:
		if c.scopeDepth == 0 {
			c.function(st.Fn)
			c.defineGlobal(st.Name, false, st.Line)
		} else {
			// declare first so the body can refer to itself
			c.emit(OpUnit, st.Line)
			c.declareLocal(st.Name, true, st.Line) // internally mutable for the fixup below
			c.function(st.Fn)
			slot := c.locals[c.resolveLocal(st.Name)].slot
			c.emit(OpSetLocal, st.Line)
			c.emitU16(uint16(slot), st.Line)
			c.emit(OpPop, st.Line)
			c.locals[c.resolveLocal(st.Name)].mut = false
		}
	case *EnumDecl:
		c.enumDecl(st)
	case *ExprStmt:
		c.expr(st.E)
		c.emit(OpPop, st.Line)
	case *AssignStmt:
		c.assign(st)
	case *ReturnStmt:
		if st.Val != nil {
			c.expr(st.Val)
		} else {
			c.emit(OpUnit, st.Line)
		}
		c.emit(OpReturn, st.Line)
	case *WhileStmt:
		c.whileStmt(st)
	case *ForStmt:
		c.forStmt(st)
	case *BreakStmt:
		c.breakStmt(st.Line)
	case *ContinueStmt:
		c.continueStmt(st.Line)
	case *SpawnStmt:
		c.expr(st.CallE.Callee)
		c.temps++
		for _, a := range st.CallE.Args {
			c.expr(a)
			c.temps++
		}
		c.temps -= len(st.CallE.Args) + 1
		if len(st.CallE.Args) > 255 {
			c.fail(st.Line, "too many arguments")
		}
		c.emit(OpSpawn, st.Line)
		c.emit(byte(len(st.CallE.Args)), st.Line)
	case *SendStmt:
		c.expr(st.Chan)
		c.temps++
		c.expr(st.Val)
		c.temps--
		c.emit(OpSend, st.Line)
	default:
		panic(fmt.Sprintf("unhandled stmt %T", s))
	}
}

func (c *Compiler) defineGlobal(name string, mut bool, line int) {
	if _, exists := c.shared.globalMut[name]; exists {
		if _, isEnum := c.shared.enums[name]; isEnum {
			c.fail(line, "cannot redeclare enum %q", name)
		}
	}
	c.shared.globalMut[name] = mut
	idx := c.chunk().addConst(ObjV(c.shared.gc.newString(name)))
	c.emit(OpDefGlobal, line)
	c.emitU16(idx, line)
}

func (c *Compiler) assign(st *AssignStmt) {
	switch t := st.Target.(type) {
	case *Ident:
		c.expr(st.Val)
		if li := c.resolveLocal(t.Name); li >= 0 {
			l := c.locals[li]
			if !l.mut {
				c.fail(st.Line, "cannot assign to immutable variable %q (declare it with `let mut`)", t.Name)
			}
			c.emit(OpSetLocal, st.Line)
			c.emitU16(uint16(l.slot), st.Line)
		} else if ui := c.resolveUpvalue(t.Name, st.Line); ui >= 0 {
			if !c.upvals[ui].mut {
				c.fail(st.Line, "cannot assign to immutable variable %q (declare it with `let mut`)", t.Name)
			}
			c.emit(OpSetUpval, st.Line)
			c.emitU16(uint16(ui), st.Line)
		} else if mut, ok := c.shared.globalMut[t.Name]; ok {
			if !mut {
				c.fail(st.Line, "cannot assign to immutable variable %q (declare it with `let mut`)", t.Name)
			}
			idx := c.chunk().addConst(ObjV(c.shared.gc.newString(t.Name)))
			c.emit(OpSetGlobal, st.Line)
			c.emitU16(idx, st.Line)
		} else {
			c.fail(st.Line, "cannot assign to undeclared variable %q", t.Name)
		}
		c.emit(OpPop, st.Line)
	case *Index:
		c.expr(t.Target)
		c.temps++
		c.expr(t.Idx)
		c.temps++
		c.expr(st.Val)
		c.temps -= 2
		c.emit(OpIndexSet, st.Line)
	default:
		c.fail(st.Line, "invalid assignment target")
	}
}

func (c *Compiler) whileStmt(st *WhileStmt) {
	start := len(c.chunk().Code)
	c.loops = append(c.loops, loopCtx{baseSlot: c.nextSlot(), continueTo: start})
	c.expr(st.Cond)
	exitJ := c.emitJump(OpJumpIfFalsePop, st.Line)
	c.blockAsStmts(st.Body)
	c.emitLoop(start, st.Line)
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
	c.declareLocal("(iter)", false, st.Line)
	iterSlot := c.locals[len(c.locals)-1].slot
	c.emitConst(IntV(0), st.Line)
	c.declareLocal("(idx)", false, st.Line)
	idxSlot := c.locals[len(c.locals)-1].slot

	c.loops = append(c.loops, loopCtx{baseSlot: c.nextSlot(), continueTo: -1})

	condStart := len(c.chunk().Code)
	c.emit(OpGetLocal, st.Line)
	c.emitU16(uint16(idxSlot), st.Line)
	c.emit(OpGetLocal, st.Line)
	c.emitU16(uint16(iterSlot), st.Line)
	c.emit(OpLen, st.Line)
	c.emit(OpLt, st.Line)
	exitJ := c.emitJump(OpJumpIfFalsePop, st.Line)

	c.beginScope()
	c.emit(OpGetLocal, st.Line)
	c.emitU16(uint16(iterSlot), st.Line)
	c.emit(OpGetLocal, st.Line)
	c.emitU16(uint16(idxSlot), st.Line)
	c.emit(OpIndexGet, st.Line)
	c.declareLocal(st.Var, false, st.Line)
	for _, s := range st.Body.Stmts {
		c.stmt(s)
	}
	c.endScope(st.Line)

	// continue lands here: increment
	incr := len(c.chunk().Code)
	lp := &c.loops[len(c.loops)-1]
	for _, j := range lp.continueJumps {
		c.patchJump(j)
	}
	_ = incr
	c.emit(OpGetLocal, st.Line)
	c.emitU16(uint16(idxSlot), st.Line)
	c.emitConst(IntV(1), st.Line)
	c.emit(OpAdd, st.Line)
	c.emit(OpSetLocal, st.Line)
	c.emitU16(uint16(idxSlot), st.Line)
	c.emit(OpPop, st.Line)
	c.emitLoop(condStart, st.Line)

	c.patchJump(exitJ)
	for _, j := range lp.breakJumps {
		c.patchJump(j)
	}
	c.loops = c.loops[:len(c.loops)-1]
	c.endScope(st.Line) // pops idx, iter
}

// emit pops for locals above the innermost loop's base without altering
// compiler bookkeeping (execution continues past the jump for other paths)
func (c *Compiler) unwindToLoop(line int) *loopCtx {
	if len(c.loops) == 0 {
		c.fail(line, "break/continue outside of a loop")
	}
	lp := &c.loops[len(c.loops)-1]
	for i := len(c.locals) - 1; i >= 0 && c.locals[i].slot >= lp.baseSlot; i-- {
		if c.locals[i].captured {
			c.emit(OpCloseUpvalue, line)
		} else {
			c.emit(OpPop, line)
		}
	}
	return lp
}

func (c *Compiler) breakStmt(line int) {
	lp := c.unwindToLoop(line)
	lp.breakJumps = append(lp.breakJumps, c.emitJump(OpJump, line))
}

func (c *Compiler) continueStmt(line int) {
	lp := c.unwindToLoop(line)
	if lp.continueTo >= 0 {
		c.emitLoop(lp.continueTo, line)
	} else {
		lp.continueJumps = append(lp.continueJumps, c.emitJump(OpJump, line))
	}
}

func (c *Compiler) enumDecl(st *EnumDecl) {
	if c.scopeDepth > 0 {
		c.fail(st.Line, "enum declarations are only allowed at top level")
	}
	if _, exists := c.shared.enums[st.Name]; exists {
		c.fail(st.Line, "enum %q is already declared", st.Name)
	}
	seen := map[string]bool{}
	var variants []VariantInfo
	for _, v := range st.Variants {
		if seen[v.Name] {
			c.fail(st.Line, "duplicate variant %q in enum %q", v.Name, st.Name)
		}
		seen[v.Name] = true
		variants = append(variants, VariantInfo{Name: v.Name, Arity: v.Arity})
	}
	info := c.shared.declareEnum(st.Name, variants)
	c.emitConst(ObjV(info.runtime), st.Line)
	idx := c.chunk().addConst(ObjV(c.shared.gc.newString(st.Name)))
	c.emit(OpDefGlobal, st.Line)
	c.emitU16(idx, st.Line)
}

// ---- expressions ----

func (c *Compiler) expr(e Expr) {
	switch ex := e.(type) {
	case *IntLit:
		c.emitConst(IntV(ex.Val), ex.Line)
	case *FloatLit:
		c.emitConst(FloatV(ex.Val), ex.Line)
	case *StrLit:
		c.emitConst(ObjV(c.shared.gc.newString(ex.Val)), ex.Line)
	case *BoolLit:
		if ex.Val {
			c.emit(OpTrue, ex.Line)
		} else {
			c.emit(OpFalse, ex.Line)
		}
	case *Ident:
		c.identExpr(ex)
	case *Unary:
		c.expr(ex.Rhs)
		if ex.Op == TMinus {
			c.emit(OpNeg, ex.Line)
		} else {
			c.emit(OpNot, ex.Line)
		}
	case *Binary:
		c.expr(ex.Lhs)
		c.temps++
		c.expr(ex.Rhs)
		c.temps--
		switch ex.Op {
		case TPlus:
			c.emit(OpAdd, ex.Line)
		case TMinus:
			c.emit(OpSub, ex.Line)
		case TStar:
			c.emit(OpMul, ex.Line)
		case TSlash:
			c.emit(OpDiv, ex.Line)
		case TPercent:
			c.emit(OpMod, ex.Line)
		case TEqEq:
			c.emit(OpEq, ex.Line)
		case TBangEq:
			c.emit(OpNeq, ex.Line)
		case TGt:
			c.emit(OpGt, ex.Line)
		case TGtEq:
			c.emit(OpGtEq, ex.Line)
		case TLt:
			c.emit(OpLt, ex.Line)
		case TLtEq:
			c.emit(OpLtEq, ex.Line)
		}
	case *Logical:
		c.expr(ex.Lhs)
		var end int
		if ex.Op == TAndAnd {
			end = c.emitJump(OpJumpIfFalse, ex.Line)
		} else {
			end = c.emitJump(OpJumpIfTrue, ex.Line)
		}
		c.emit(OpPop, ex.Line)
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
			c.fail(ex.Line, "too many arguments")
		}
		c.emit(OpCall, ex.Line)
		c.emit(byte(len(ex.Args)), ex.Line)
	case *Index:
		c.expr(ex.Target)
		c.temps++
		c.expr(ex.Idx)
		c.temps--
		c.emit(OpIndexGet, ex.Line)
	case *ListLit:
		for _, el := range ex.Elems {
			c.expr(el)
			c.temps++
		}
		c.temps -= len(ex.Elems)
		if len(ex.Elems) > 0xffff {
			c.fail(ex.Line, "list literal too large")
		}
		c.emit(OpList, ex.Line)
		c.emitU16(uint16(len(ex.Elems)), ex.Line)
	case *FnLit:
		c.function(ex)
	case *Block:
		c.blockExpr(ex)
	case *IfExpr:
		c.ifExpr(ex)
	case *MatchExpr:
		c.matchExpr(ex)
	case *VariantAccess:
		c.variantAccess(ex.EnumName, ex.Variant, ex.Line)
	case *TryExpr:
		c.expr(ex.Inner)
		c.emit(OpTry, ex.Line)
	case *RecvExpr:
		c.expr(ex.Chan)
		c.emit(OpRecv, ex.Line)
	default:
		panic(fmt.Sprintf("unhandled expr %T", e))
	}
}

func (c *Compiler) identExpr(ex *Ident) {
	if li := c.resolveLocal(ex.Name); li >= 0 {
		c.emit(OpGetLocal, ex.Line)
		c.emitU16(uint16(c.locals[li].slot), ex.Line)
		return
	}
	if ui := c.resolveUpvalue(ex.Name, ex.Line); ui >= 0 {
		c.emit(OpGetUpval, ex.Line)
		c.emitU16(uint16(ui), ex.Line)
		return
	}
	if _, declared := c.shared.globalMut[ex.Name]; !declared {
		// bare enum variant like Some / None / Ok / Err
		if info, idx, amb := c.shared.findVariant(ex.Name); amb {
			c.fail(ex.Line, "variant %q exists in several enums; qualify it as Enum.%s", ex.Name, ex.Name)
		} else if idx >= 0 {
			c.emitVariant(info, idx, ex.Line)
			return
		}
	}
	// global (possibly defined later in the script)
	idx := c.chunk().addConst(ObjV(c.shared.gc.newString(ex.Name)))
	c.emit(OpGetGlobal, ex.Line)
	c.emitU16(idx, ex.Line)
}

func (c *Compiler) variantAccess(enumName, variant string, line int) {
	info, ok := c.shared.enums[enumName]
	if !ok {
		c.fail(line, "unknown enum %q", enumName)
	}
	for i, v := range info.Variants {
		if v.Name == variant {
			c.emitVariant(info, i, line)
			return
		}
	}
	c.fail(line, "enum %q has no variant %q", enumName, variant)
}

// nullary variants become singleton instance constants; variants with fields
// become constructor constants (called like functions)
func (c *Compiler) emitVariant(info *EnumInfo, idx int, line int) {
	if info.Variants[idx].Arity == 0 {
		if info.singletons[idx] == nil {
			inst := &OEnumInst{Enum: info.runtime, Variant: idx}
			c.shared.gc.alloc(inst)
			info.singletons[idx] = inst
		}
		c.emitConst(ObjV(info.singletons[idx]), line)
	} else {
		if info.ctors[idx] == nil {
			ctor := &OVariantCtor{Enum: info.runtime, Idx: idx}
			c.shared.gc.alloc(ctor)
			info.ctors[idx] = ctor
		}
		c.emitConst(ObjV(info.ctors[idx]), line)
	}
}

func (c *Compiler) function(lit *FnLit) {
	sub := &Compiler{
		enclosing: c,
		shared:    c.shared,
		fn:        &OFunc{Name: lit.Name, Arity: len(lit.Params)},
	}
	c.shared.gc.alloc(sub.fn)
	sub.beginScope()
	selfName := ""
	if lit.Name != "" {
		selfName = lit.Name // recursion through slot 0
	}
	sub.locals = append(sub.locals, LocalVar{name: selfName, depth: sub.scopeDepth, slot: 0})
	if len(lit.Params) > 255 {
		sub.fail(lit.Line, "too many parameters")
	}
	seen := map[string]bool{}
	for _, p := range lit.Params {
		if seen[p] {
			sub.fail(lit.Line, "duplicate parameter %q", p)
		}
		seen[p] = true
		sub.locals = append(sub.locals, LocalVar{
			name: p, depth: sub.scopeDepth, slot: sub.locals[len(sub.locals)-1].slot + 1,
		})
	}
	sub.blockExpr(lit.Body)
	sub.emit(OpReturn, lit.Line)

	idx := c.chunk().addConst(ObjV(sub.fn))
	c.emit(OpClosure, lit.Line)
	c.emitU16(idx, lit.Line)
	for _, uv := range sub.upvals {
		if uv.isLocal {
			c.emit(1, lit.Line)
		} else {
			c.emit(0, lit.Line)
		}
		c.emitU16(uv.index, lit.Line)
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
		c.emit(OpUnit, b.Line)
	}
	c.endScopeExpr(b.Line)
}

// blockAsStmts compiles a block purely for effect (loop bodies)
func (c *Compiler) blockAsStmts(b *Block) {
	c.beginScope()
	for _, s := range b.Stmts {
		c.stmt(s)
	}
	c.endScope(b.Line)
}

func (c *Compiler) ifExpr(ex *IfExpr) {
	c.expr(ex.Cond)
	elseJ := c.emitJump(OpJumpIfFalsePop, ex.Line)
	c.blockExpr(ex.Then)
	endJ := c.emitJump(OpJump, ex.Line)
	c.patchJump(elseJ)
	switch els := ex.Else.(type) {
	case nil:
		c.emit(OpUnit, ex.Line)
	case *Block:
		c.blockExpr(els)
	case *IfExpr:
		c.ifExpr(els)
	default:
		c.fail(ex.Line, "invalid else branch")
	}
	c.patchJump(endJ)
}

func (c *Compiler) matchExpr(m *MatchExpr) {
	c.beginScope()
	c.expr(m.Scrut)
	c.declareLocal("(match)", false, m.Line)
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
			c.emit(OpGetLocal, arm.Line)
			c.emitU16(uint16(scrutSlot), arm.Line)
			c.temps++
			c.expr(pat.Val)
			c.temps--
			c.emit(OpEq, arm.Line)
			failJ = c.emitJump(OpJumpIfFalsePop, arm.Line)
		case *PatBinding:
			// a bare name is a nullary-variant test if such a variant exists,
			// otherwise a catch-all binding
			if info, idx, amb := c.shared.findVariant(pat.Name); amb {
				c.fail(arm.Line, "variant %q exists in several enums; qualify it as Enum.%s", pat.Name, pat.Name)
			} else if idx >= 0 && info.Variants[idx].Arity == 0 {
				failJ = c.emitVariantTest(info, idx, scrutSlot, arm.Line)
			} else if idx >= 0 {
				c.fail(arm.Line, "variant %q has %d field(s); bind them: %s(...)", pat.Name, info.Variants[idx].Arity, pat.Name)
			} else {
				c.emit(OpGetLocal, arm.Line)
				c.emitU16(uint16(scrutSlot), arm.Line)
				c.declareLocal(pat.Name, false, arm.Line)
				armLocals++
			}
		case *PatVariant:
			var info *EnumInfo
			var idx int
			if pat.EnumName != "" {
				var ok bool
				info, ok = c.shared.enums[pat.EnumName]
				if !ok {
					c.fail(arm.Line, "unknown enum %q", pat.EnumName)
				}
				idx = -1
				for i, v := range info.Variants {
					if v.Name == pat.Variant {
						idx = i
						break
					}
				}
				if idx < 0 {
					c.fail(arm.Line, "enum %q has no variant %q", pat.EnumName, pat.Variant)
				}
			} else {
				var amb bool
				info, idx, amb = c.shared.findVariant(pat.Variant)
				if amb {
					c.fail(arm.Line, "variant %q exists in several enums; qualify it as Enum.%s", pat.Variant, pat.Variant)
				}
				if idx < 0 {
					c.fail(arm.Line, "unknown variant %q", pat.Variant)
				}
			}
			arity := info.Variants[idx].Arity
			if len(pat.Binds) != arity {
				c.fail(arm.Line, "variant %s has %d field(s), pattern binds %d", pat.Variant, arity, len(pat.Binds))
			}
			failJ = c.emitVariantTest(info, idx, scrutSlot, arm.Line)
			for i, sub := range pat.Binds {
				name := "_"
				if b, ok := sub.(*PatBinding); ok {
					name = b.Name
				}
				if name == "_" {
					continue
				}
				c.emit(OpGetLocal, arm.Line)
				c.emitU16(uint16(scrutSlot), arm.Line)
				c.emit(OpGetField, arm.Line)
				c.emit(byte(i), arm.Line)
				c.declareLocal(name, false, arm.Line)
				armLocals++
			}
		}

		c.expr(arm.Body)
		if armLocals > 0 {
			c.emit(OpEndBlock, arm.Line)
			c.emit(byte(armLocals), arm.Line)
		}
		c.scopeDepth--
		for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth > c.scopeDepth {
			c.locals = c.locals[:len(c.locals)-1]
		}
		endJumps = append(endJumps, c.emitJump(OpJump, arm.Line))
		if failJ >= 0 {
			c.patchJump(failJ)
		}
	}
	c.emit(OpNoMatch, m.Line)
	for _, j := range endJumps {
		c.patchJump(j)
	}
	// squash the scrutinee slot, leaving the arm value
	c.scopeDepth--
	c.locals = c.locals[:len(c.locals)-1]
	c.emit(OpEndBlock, m.Line)
	c.emit(1, m.Line)
}

func (c *Compiler) emitVariantTest(info *EnumInfo, idx int, scrutSlot int, line int) int {
	c.emit(OpGetLocal, line)
	c.emitU16(uint16(scrutSlot), line)
	c.emitConst(ObjV(info.runtime), line)
	c.emit(OpTestVariant, line)
	c.emit(byte(idx), line)
	return c.emitJump(OpJumpIfFalsePop, line)
}
