package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ---- type representation ----

type Ty interface{ tyNode() }

// TCon: type constructor. Prims: int float bool str unit; "list"/"chan" take
// one arg; enum names take their declared parameters.
type TCon struct {
	Name string
	Args []Ty
}

type TFunc struct {
	Params []Ty
	Ret    Ty
}

// TV: type variable with Rémy-style levels for efficient generalization and
// an optional overloading constraint ("num" = int|float, "addord" = int|float|str).
type TV struct {
	id    int
	level int
	con   string
	link  Ty // non-nil once bound
}

func (*TCon) tyNode()  {}
func (*TFunc) tyNode() {}
func (*TV) tyNode()    {}

var conSets = map[string][]string{
	"num":    {"int", "float"},
	"addord": {"int", "float", "str"},
	"key":    {"int", "str"},
	// singletons name the intersections that merging can produce (e.g.
	// num ∩ key = {int}); a variable pinned to one of these can only be
	// that type.
	"int":   {"int"},
	"float": {"float"},
	"str":   {"str"},
}

func resolve(t Ty) Ty {
	for {
		tv, ok := t.(*TV)
		if !ok || tv.link == nil {
			return t
		}
		t = tv.link
	}
}

type Scheme struct {
	vars []*TV // quantified
	t    Ty
}

// ---- checker ----

type binding struct {
	scheme  *Scheme
	mut     bool
	used    bool
	isFn    bool
	pub     bool
	special string // "chan", "printf" for irregular natives
	span    Span
	file    string // file the binding was declared in
	kind    string // "local", "param", "global", "native"
}

type VariantSig struct {
	Name   string
	Fields []TypeExpr
}

type EnumSig struct {
	Name     string
	Qual     string // globally unique type-constructor name: <pkg>.<Name>, or Name outside packages
	Prefix   string // import path of the defining package; "" for scripts and builtins
	Pub      bool
	Params   []string
	Variants []VariantSig
}

// pkgSymbols is what one checked package offers to its importers.
type pkgSymbols struct {
	vals  map[string]*binding // every top-level value binding; pub gates access
	enums map[string]*EnumSig // every enum; Pub gates access
}

type Checker struct {
	diags     []Diag
	scopes    []map[string]*binding
	enums     map[string]*EnumSig    // keyed by EnumSig.Qual
	pkgs      map[string]*pkgSymbols // import path -> checked package symbols
	pkgPrefix string                 // import path of the package being checked; "" for scripts
	curFile   string                 // stamped onto diagnostics; "" for scripts
	nextID    int
	level     int
	retTys    []Ty // stack of enclosing function return types
	loop      int
}

func newChecker() *Checker {
	c := &Checker{enums: map[string]*EnumSig{}, pkgs: map[string]*pkgSymbols{}}
	c.pushScope()
	c.declareBuiltins()
	return c
}

// qualName maps a bare enum name written in the current package to its
// globally unique type-constructor name.
func (c *Checker) qualName(name string) string {
	if c.pkgPrefix == "" {
		return name
	}
	return c.pkgPrefix + "." + name
}

// enumSig resolves a bare enum name: the current package's enums shadow the
// builtin ones.
func (c *Checker) enumSig(name string) (*EnumSig, bool) {
	if sig, ok := c.enums[c.qualName(name)]; ok {
		return sig, true
	}
	sig, ok := c.enums[name]
	return sig, ok && sig.Prefix == ""
}

func typecheck(stmts []Stmt) []Diag {
	c := newChecker()

	// pass 1: register enums, pre-declare every top-level name (monomorphic)
	for _, s := range stmts {
		switch st := s.(type) {
		case *EnumDecl:
			c.registerEnum(st)
		}
	}
	pre := map[string]*TV{}
	for _, s := range stmts {
		switch st := s.(type) {
		case *FnDecl:
			tv := c.fresh("")
			pre[st.Name] = tv
			c.declare(st.Name, &Scheme{t: tv}, false, "global", st.NameSpan)
			c.scopes[len(c.scopes)-1][st.Name].isFn = true
		case *LetStmt:
			tv := c.fresh("")
			pre[st.Name] = tv
			c.declare(st.Name, &Scheme{t: tv}, st.Mut, "global", st.NameSpan)
		}
	}

	// pass 2: infer in order
	for _, s := range stmts {
		switch st := s.(type) {
		case *FnDecl:
			if st.Pub {
				c.errPubScript(st.Span)
			}
			c.level++
			ft := c.inferExpr(st.Fn)
			c.level--
			// generalize BEFORE linking the level-0 placeholder: unify would
			// pin every inner var to level 0 and kill polymorphism. Vars
			// already pinned by earlier (mutual-recursive) uses resolve to
			// their concrete types during instantiation, which stays sound.
			b := c.lookup(st.Name)
			b.scheme = c.generalize(ft)
			c.unifyAt(pre[st.Name], ft, st.NameSpan, "in this function")
		case *LetStmt:
			if st.Pub {
				c.errPubScript(st.Span)
			}
			t := c.inferLetInit(st)
			c.unifyAt(pre[st.Name], t, st.NameSpan, "in this binding")
		case *EnumDecl:
			if st.Pub {
				c.errPubScript(st.Span)
			}
			// already registered
		default:
			c.inferStmt(s)
		}
	}

	// lint unused top-level bindings (skip _-prefixed)
	for name, b := range c.scopes[0] {
		if b.kind == "global" && !b.used && !strings.HasPrefix(name, "_") {
			what := "variable"
			if b.isFn {
				what = "function"
			}
			c.warnf(b.span, "unused_variable",
				fmt.Sprintf("if this is intentional, prefix it with an underscore: `_%s`", name),
				"unused %s: `%s`", what, name)
		}
	}

	sort.SliceStable(c.diags, func(i, j int) bool {
		return c.diags[i].Span.Start < c.diags[j].Span.Start
	})
	return c.diags
}

// typecheckModule checks every package of a loaded module in dependency
// order (imports before importers) and validates the entry package's main.
func typecheckModule(m *Module) []Diag {
	c := newChecker()
	fileOrder := map[string]int{}
	for _, pkg := range m.Packages {
		for _, f := range pkg.Files {
			fileOrder[f.Path] = len(fileOrder) + 1
		}
	}
	for _, pkg := range m.Packages {
		c.checkPackage(pkg, pkg == m.Entry)
	}
	sort.SliceStable(c.diags, func(i, j int) bool {
		if fileOrder[c.diags[i].File] != fileOrder[c.diags[j].File] {
			return fileOrder[c.diags[i].File] < fileOrder[c.diags[j].File]
		}
		return c.diags[i].Span.Start < c.diags[j].Span.Start
	})
	return c.diags
}

// checkPackage runs the two-pass top-level inference over all files of one
// package sharing a single package scope, then snapshots its symbols for
// importers.
func (c *Checker) checkPackage(pkg *Package, isEntry bool) {
	c.pkgPrefix = pkg.ImportPath
	sym := &pkgSymbols{vals: map[string]*binding{}, enums: map[string]*EnumSig{}}
	c.pkgs[pkg.ImportPath] = sym
	c.pushScope()

	// pass 1: register enums from every file
	for _, f := range pkg.Files {
		c.curFile = f.Path
		for _, s := range f.Stmts {
			if st, ok := s.(*EnumDecl); ok {
				c.registerEnum(st)
				if sig, ok := c.enums[c.qualName(st.Name)]; ok {
					sym.enums[st.Name] = sig
				}
			}
		}
	}
	// pass 1.5: pre-declare every package-level value name (monomorphic)
	pre := map[string]*TV{}
	top := c.scopes[len(c.scopes)-1]
	for _, f := range pkg.Files {
		c.curFile = f.Path
		for _, s := range f.Stmts {
			switch st := s.(type) {
			case *FnDecl:
				tv := c.fresh("")
				pre[st.Name] = tv
				c.declare(st.Name, &Scheme{t: tv}, false, "global", st.NameSpan)
				top[st.Name].isFn = true
				top[st.Name].pub = st.Pub
			case *LetStmt:
				tv := c.fresh("")
				pre[st.Name] = tv
				c.declare(st.Name, &Scheme{t: tv}, st.Mut, "global", st.NameSpan)
				top[st.Name].pub = st.Pub
			}
		}
	}
	// pass 2: infer file by file, in load order
	for _, f := range pkg.Files {
		c.curFile = f.Path
		for _, s := range f.Stmts {
			switch st := s.(type) {
			case *FnDecl:
				c.level++
				ft := c.inferExpr(st.Fn)
				c.level--
				// generalize before linking the placeholder: see typecheck
				b := c.lookup(st.Name)
				b.scheme = c.generalize(ft)
				c.unifyAt(pre[st.Name], ft, st.NameSpan, "in this function")
			case *LetStmt:
				t := c.inferLetInit(st)
				c.unifyAt(pre[st.Name], t, st.NameSpan, "in this binding")
			case *EnumDecl:
				// registered in pass 1
			default:
				c.inferStmt(s) // unreachable: the loader admits only declarations
			}
		}
	}

	if isEntry {
		if b, ok := top["main"]; ok && b.isFn {
			b.used = true
			if err := c.unify(c.instantiate(b.scheme), &TFunc{Ret: c.fresh("")}); err != nil {
				c.curFile = b.file
				c.errorf(b.span, "E0580", "declare it as `fn main() { ... }`",
					"`main` must take no parameters")
			}
		} else {
			c.curFile = pkg.Files[0].Path
			c.errorf(Span{}, "E0601", "declare `fn main() { ... }` in the package you run",
				"no `main` function in package %q", pkg.ImportPath)
		}
	}

	// lint unused non-pub bindings (pub ones are the package's API), then
	// snapshot the scope for importers and pop it without local/param lint
	for name, b := range top {
		if b.kind == "global" && !b.pub && !b.used && !strings.HasPrefix(name, "_") {
			what := "variable"
			if b.isFn {
				what = "function"
			}
			c.curFile = b.file
			c.warnf(b.span, "unused_variable",
				fmt.Sprintf("if this is intentional, prefix it with an underscore: `_%s`", name),
				"unused %s: `%s`", what, name)
		}
		sym.vals[name] = b
	}
	c.scopes = c.scopes[:len(c.scopes)-1]
	c.curFile, c.pkgPrefix = "", ""
}

func (c *Checker) errorf(sp Span, code, help, format string, args ...any) {
	c.diags = append(c.diags, Diag{
		IsErr: true, Code: code, File: c.curFile, Span: sp,
		Msg: fmt.Sprintf(format, args...), Help: help,
	})
}

// errPubScript rejects `pub` outside a package: single-file scripts and
// nested scopes have nothing to export.
func (c *Checker) errPubScript(sp Span) {
	c.errorf(sp, "E0449",
		"`pub` exports items from a package (a directory with bur.mod); remove it here",
		"`pub` is not allowed here")
}

func (c *Checker) warnf(sp Span, code, help, format string, args ...any) {
	c.diags = append(c.diags, Diag{
		IsErr: false, Code: code, File: c.curFile, Span: sp,
		Msg: fmt.Sprintf(format, args...), Help: help,
	})
}

// ---- scopes ----

func (c *Checker) pushScope() { c.scopes = append(c.scopes, map[string]*binding{}) }

func (c *Checker) popScope() {
	top := c.scopes[len(c.scopes)-1]
	for name, b := range top {
		if !b.used && !strings.HasPrefix(name, "_") && (b.kind == "local" || b.kind == "param") {
			what := "variable"
			if b.kind == "param" {
				what = "parameter"
			}
			c.warnf(b.span, "unused_variable",
				fmt.Sprintf("if this is intentional, prefix it with an underscore: `_%s`", name),
				"unused %s: `%s`", what, name)
		}
	}
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *Checker) declare(name string, s *Scheme, mut bool, kind string, sp Span) {
	c.scopes[len(c.scopes)-1][name] = &binding{scheme: s, mut: mut, kind: kind, span: sp, file: c.curFile}
}

func (c *Checker) lookup(name string) *binding {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if b, ok := c.scopes[i][name]; ok {
			return b
		}
	}
	return nil
}

// ---- type variable machinery ----

func (c *Checker) fresh(con string) *TV {
	c.nextID++
	return &TV{id: c.nextID, level: c.level, con: con}
}

func adjustLevel(t Ty, level int) {
	switch x := resolve(t).(type) {
	case *TV:
		if x.level > level {
			x.level = level
		}
	case *TCon:
		for _, a := range x.Args {
			adjustLevel(a, level)
		}
	case *TFunc:
		for _, p := range x.Params {
			adjustLevel(p, level)
		}
		adjustLevel(x.Ret, level)
	}
}

func occurs(v *TV, t Ty) bool {
	switch x := resolve(t).(type) {
	case *TV:
		return x == v
	case *TCon:
		for _, a := range x.Args {
			if occurs(v, a) {
				return true
			}
		}
	case *TFunc:
		for _, p := range x.Params {
			if occurs(v, p) {
				return true
			}
		}
		return occurs(v, x.Ret)
	}
	return false
}

func conAllows(con, name string) bool {
	if con == "" {
		return true
	}
	for _, n := range conSets[con] {
		if n == name {
			return true
		}
	}
	return false
}

// mergeCon intersects two overloading constraints, returning the name of the
// set equal to their intersection; ok is false when the intersection is empty
// (the two constraints are incompatible).
func mergeCon(a, b string) (string, bool) {
	if a == "" {
		return b, true
	}
	if b == "" || a == b {
		return a, true
	}
	var inter []string
	for _, x := range conSets[a] {
		for _, y := range conSets[b] {
			if x == y {
				inter = append(inter, x)
				break
			}
		}
	}
	if len(inter) == 0 {
		return "", false
	}
	for name, set := range conSets {
		if len(set) == len(inter) {
			same := true
			for _, x := range inter {
				if !slices.Contains(set, x) {
					same = false
					break
				}
			}
			if same {
				return name, true
			}
		}
	}
	return "", false
}

func (c *Checker) unify(a, b Ty) error {
	a, b = resolve(a), resolve(b)
	if a == b {
		return nil
	}
	if av, ok := a.(*TV); ok {
		if occurs(av, b) {
			return fmt.Errorf("recursive type detected")
		}
		if bc, ok := b.(*TCon); ok && len(bc.Args) == 0 {
			if !conAllows(av.con, bc.Name) {
				return fmt.Errorf("`%s` is not valid here: this operator needs %s", bc.Name, strings.Join(conSets[av.con], " or "))
			}
		}
		if bc, ok := b.(*TCon); ok && len(bc.Args) > 0 && av.con != "" {
			return fmt.Errorf("`%s` is not valid here: this operator needs %s", pretty(bc), strings.Join(conSets[av.con], " or "))
		}
		if _, ok := b.(*TFunc); ok && av.con != "" {
			return fmt.Errorf("a function is not valid here: this operator needs %s", strings.Join(conSets[av.con], " or "))
		}
		if bv, ok := b.(*TV); ok {
			merged, ok := mergeCon(av.con, bv.con)
			if !ok {
				return fmt.Errorf("no type satisfies both %s and %s",
					strings.Join(conSets[av.con], "|"), strings.Join(conSets[bv.con], "|"))
			}
			bv.con = merged
		}
		adjustLevel(b, av.level)
		av.link = b
		return nil
	}
	if _, ok := b.(*TV); ok {
		return c.unify(b, a)
	}
	ac, aok := a.(*TCon)
	bc, bok := b.(*TCon)
	if aok && bok {
		if ac.Name != bc.Name || len(ac.Args) != len(bc.Args) {
			return fmt.Errorf("expected `%s`, found `%s`", pretty(a), pretty(b))
		}
		for i := range ac.Args {
			if err := c.unify(ac.Args[i], bc.Args[i]); err != nil {
				return err
			}
		}
		return nil
	}
	af, aok2 := a.(*TFunc)
	bf, bok2 := b.(*TFunc)
	if aok2 && bok2 {
		if len(af.Params) != len(bf.Params) {
			return fmt.Errorf("expected a function with %d parameter(s), found one with %d", len(af.Params), len(bf.Params))
		}
		for i := range af.Params {
			if err := c.unify(af.Params[i], bf.Params[i]); err != nil {
				return err
			}
		}
		return c.unify(af.Ret, bf.Ret)
	}
	return fmt.Errorf("expected `%s`, found `%s`", pretty(a), pretty(b))
}

func (c *Checker) unifyAt(a, b Ty, sp Span, context string) {
	if err := c.unify(a, b); err != nil {
		c.errorf(sp, "E0308", "", "mismatched types %s: %s", context, err.Error())
	}
}

func (c *Checker) generalize(t Ty) *Scheme {
	var vars []*TV
	seen := map[*TV]bool{}
	var walk func(Ty)
	walk = func(t Ty) {
		switch x := resolve(t).(type) {
		case *TV:
			if x.level > c.level && !seen[x] {
				seen[x] = true
				vars = append(vars, x)
			}
		case *TCon:
			for _, a := range x.Args {
				walk(a)
			}
		case *TFunc:
			for _, p := range x.Params {
				walk(p)
			}
			walk(x.Ret)
		}
	}
	walk(t)
	return &Scheme{vars: vars, t: t}
}

func (c *Checker) instantiate(s *Scheme) Ty {
	if len(s.vars) == 0 {
		return s.t
	}
	subst := map[*TV]Ty{}
	for _, v := range s.vars {
		subst[v] = c.fresh(v.con)
	}
	var cp func(Ty) Ty
	cp = func(t Ty) Ty {
		switch x := resolve(t).(type) {
		case *TV:
			if n, ok := subst[x]; ok {
				return n
			}
			return x
		case *TCon:
			args := make([]Ty, len(x.Args))
			for i, a := range x.Args {
				args[i] = cp(a)
			}
			return &TCon{Name: x.Name, Args: args}
		case *TFunc:
			ps := make([]Ty, len(x.Params))
			for i, p := range x.Params {
				ps[i] = cp(p)
			}
			return &TFunc{Params: ps, Ret: cp(x.Ret)}
		}
		return t
	}
	return cp(s.t)
}

// ---- enums ----

func (c *Checker) registerEnum(st *EnumDecl) {
	if _, exists := c.enums[c.qualName(st.Name)]; exists {
		c.errorf(st.Span, "E0428", "", "enum `%s` is declared more than once", st.Name)
		return
	}
	sig := &EnumSig{Name: st.Name, Qual: c.qualName(st.Name), Prefix: c.pkgPrefix,
		Pub: st.Pub, Params: st.Params}
	for _, v := range st.Variants {
		sig.Variants = append(sig.Variants, VariantSig{Name: v.Name, Fields: v.Types})
	}
	c.enums[sig.Qual] = sig
	// validate field type expressions
	pm := map[string]bool{}
	for _, p := range st.Params {
		pm[p] = true
	}
	for _, v := range st.Variants {
		for _, te := range v.Types {
			c.validateTypeExpr(te, pm)
		}
	}
}

func (c *Checker) validateTypeExpr(te TypeExpr, params map[string]bool) {
	switch x := te.(type) {
	case *TEName:
		if x.Pkg != "" {
			if sig := c.lookupPkgEnum(x.Pkg, x.Name, x.Span); sig != nil {
				if len(x.Args) != len(sig.Params) {
					c.errorf(x.Span, "E0107", "",
						"enum `%s` takes %d type argument(s), %d given", x.Name, len(sig.Params), len(x.Args))
				}
			}
			for _, a := range x.Args {
				c.validateTypeExpr(a, params)
			}
			return
		}
		switch x.Name {
		case "int", "float", "bool", "str", "unit":
			if len(x.Args) > 0 {
				c.errorf(x.Span, "E0109", "", "`%s` takes no type arguments", x.Name)
			}
		case "chan":
			if len(x.Args) != 1 {
				c.errorf(x.Span, "E0107", "", "`chan` takes exactly one type argument, e.g. chan(int)")
			}
		case "map":
			if len(x.Args) != 2 {
				c.errorf(x.Span, "E0107", "", "`map` takes two type arguments, e.g. map(str, int)")
			}
		default:
			if params[x.Name] {
				if len(x.Args) > 0 {
					c.errorf(x.Span, "E0109", "", "type parameter `%s` takes no arguments", x.Name)
				}
			} else if sig, ok := c.enumSig(x.Name); ok {
				if len(x.Args) != len(sig.Params) {
					c.errorf(x.Span, "E0107", "",
						"enum `%s` takes %d type argument(s), %d given", x.Name, len(sig.Params), len(x.Args))
				}
			} else {
				c.errorf(x.Span, "E0412", "", "cannot find type `%s`", x.Name)
			}
		}
		for _, a := range x.Args {
			c.validateTypeExpr(a, params)
		}
	case *TEList:
		c.validateTypeExpr(x.Elem, params)
	case *TEFn:
		for _, p := range x.Params {
			c.validateTypeExpr(p, params)
		}
		c.validateTypeExpr(x.Ret, params)
	}
}

// convertTypeExpr lowers a written type to a Ty. prefix is the import path of
// the package the type expression was written in: bare enum names resolve
// against it, so an enum type keeps its identity across packages.
func (c *Checker) convertTypeExpr(te TypeExpr, subst map[string]Ty, prefix string) Ty {
	switch x := te.(type) {
	case *TEName:
		if x.Pkg == "" {
			if t, ok := subst[x.Name]; ok {
				return t
			}
		}
		args := make([]Ty, len(x.Args))
		for i, a := range x.Args {
			args[i] = c.convertTypeExpr(a, subst, prefix)
		}
		switch {
		case x.Pkg != "":
			return &TCon{Name: x.Pkg + "." + x.Name, Args: args}
		case x.Name == "int" || x.Name == "float" || x.Name == "bool" ||
			x.Name == "str" || x.Name == "unit" || x.Name == "chan" || x.Name == "map":
			return &TCon{Name: x.Name, Args: args}
		default:
			name := x.Name
			if prefix != "" {
				if _, local := c.enums[prefix+"."+x.Name]; local {
					name = prefix + "." + x.Name
				}
			}
			return &TCon{Name: name, Args: args}
		}
	case *TEList:
		return &TCon{Name: "list", Args: []Ty{c.convertTypeExpr(x.Elem, subst, prefix)}}
	case *TEFn:
		ps := make([]Ty, len(x.Params))
		for i, p := range x.Params {
			ps[i] = c.convertTypeExpr(p, subst, prefix)
		}
		return &TFunc{Params: ps, Ret: c.convertTypeExpr(x.Ret, subst, prefix)}
	}
	return &TCon{Name: "unit"}
}

// instantiateEnum returns the enum type with fresh parameters and a substitution
// for converting variant field types.
func (c *Checker) instantiateEnum(sig *EnumSig) (Ty, map[string]Ty) {
	subst := map[string]Ty{}
	args := make([]Ty, len(sig.Params))
	for i, p := range sig.Params {
		v := c.fresh("")
		subst[p] = v
		args[i] = v
	}
	return &TCon{Name: sig.Qual, Args: args}, subst
}

// lookupPkgEnum resolves pkgPath.name to another package's enum, reporting
// resolution and visibility errors; nil means an error was already emitted.
func (c *Checker) lookupPkgEnum(pkgPath, name string, sp Span) *EnumSig {
	sym, ok := c.pkgs[pkgPath]
	if !ok {
		c.errorf(sp, "E0433", "", "cannot find package `%s`", pkgPath)
		return nil
	}
	sig, ok := sym.enums[name]
	if !ok {
		c.errorf(sp, "E0412", "", "cannot find enum `%s` in package `%s`", name, pkgPath)
		return nil
	}
	if !sig.Pub {
		c.errorf(sp, "E0603", fmt.Sprintf("mark it `pub enum %s` to export it", name),
			"enum `%s` is private to package `%s`", name, pkgPath)
		return nil
	}
	return sig
}

// findVariantSig resolves a bare variant name against the enums visible
// without a qualifier: the current package's and the builtins.
func (c *Checker) findVariantSig(name string) (*EnumSig, int, bool) {
	var found *EnumSig
	idx := -1
	for _, sig := range c.enums {
		if sig.Prefix != c.pkgPrefix && sig.Prefix != "" {
			continue
		}
		for i, v := range sig.Variants {
			if v.Name == name {
				if idx >= 0 {
					return nil, -1, true // ambiguous
				}
				found, idx = sig, i
			}
		}
	}
	return found, idx, false
}

// ---- statements ----

func (c *Checker) inferLetInit(st *LetStmt) Ty {
	if fl, ok := st.Init.(*FnLit); ok {
		c.level++
		t := c.inferExpr(fl)
		c.level--
		return t
	}
	return c.inferExpr(st.Init)
}

func (c *Checker) inferStmt(s Stmt) {
	switch st := s.(type) {
	case *LetStmt:
		if st.Pub {
			c.errPubScript(st.Span)
		}
		t := c.inferLetInit(st)
		var sch *Scheme
		if _, isFn := st.Init.(*FnLit); isFn {
			sch = c.generalize(t)
		} else {
			sch = &Scheme{t: t}
		}
		c.declare(st.Name, sch, st.Mut, "local", st.NameSpan)
	case *FnDecl:
		if st.Pub {
			c.errPubScript(st.Span)
		}
		tv := c.fresh("")
		c.declare(st.Name, &Scheme{t: tv}, false, "local", st.NameSpan)
		b := c.lookup(st.Name)
		b.isFn = true
		b.used = true // don't lint named helper fns
		c.level++
		ft := c.inferExpr(st.Fn)
		c.level--
		b.scheme = c.generalize(ft) // before unify: see top-level FnDecl note
		c.unifyAt(tv, ft, st.NameSpan, "in this function")
	case *EnumDecl:
		c.errorf(st.Span, "E0637", "", "enum declarations are only allowed at top level")
	case *ImportDecl:
		c.errorf(st.Span, "E0432",
			"imports go at the top of a file in a module (a directory with bur.mod); single-file scripts cannot import",
			"`import` is not allowed here")
	case *ExprStmt:
		t := c.inferExpr(st.E)
		switch r := resolve(t).(type) {
		case *TCon:
			if r.Name == "Result" || r.Name == "Option" {
				c.errorf(st.Span, "unused_must_use",
					"handle it with `match`/`?`, or discard explicitly with `let _ = ...`",
					"unused `%s` value must be handled", r.Name)
			}
		}
	case *AssignStmt:
		switch tgt := st.Target.(type) {
		case *Ident:
			b := c.lookup(tgt.Name)
			if b == nil {
				c.errorf(tgt.Span, "E0425", "declare it first: `let mut "+tgt.Name+" = ...`",
					"cannot assign to undeclared variable `%s`", tgt.Name)
				c.inferExpr(st.Val)
				return
			}
			if !b.mut {
				help := fmt.Sprintf("make this binding mutable: `let mut %s`", tgt.Name)
				if b.kind == "param" {
					help = fmt.Sprintf("declare the parameter as mutable: `mut %s`", tgt.Name)
				}
				c.errorf(tgt.Span, "E0384", help,
					"cannot assign twice to immutable variable `%s`", tgt.Name)
			}
			vt := c.inferExpr(st.Val)
			c.unifyAt(c.instantiate(b.scheme), vt, st.Val.span(), "in this assignment")
		case *Index:
			c.requireMutRoot(tgt, "assign to this element")
			el := c.fresh("")
			tt := c.inferExpr(tgt.Target)
			c.unifyAt(tt, &TCon{Name: "list", Args: []Ty{el}}, tgt.Target.span(), "when indexing")
			it := c.inferExpr(tgt.Idx)
			c.unifyAt(it, tInt, tgt.Idx.span(), "in this index")
			vt := c.inferExpr(st.Val)
			c.unifyAt(el, vt, st.Val.span(), "in this assignment")
		}
	case *WhileStmt:
		ct := c.inferExpr(st.Cond)
		c.unifyAt(ct, tBool, st.Cond.span(), "in this `while` condition")
		c.loop++
		c.pushScope()
		c.inferBlockStmts(st.Body)
		c.popScope()
		c.loop--
	case *ForStmt:
		el := c.fresh("")
		it := c.inferExpr(st.Iter)
		// a channel iterable receives until closed; anything else must be a
		// list. The channel type must be statically evident here (same
		// limitation as elsewhere: unannotated params default to list).
		if ch, ok := resolve(it).(*TCon); ok && ch.Name == "chan" {
			c.unifyAt(el, ch.Args[0], st.Iter.span(), "in this `for` loop (receive from a channel)")
			st.IterIsChan = true
		} else {
			c.unifyAt(it, &TCon{Name: "list", Args: []Ty{el}}, st.Iter.span(), "in this `for` loop (iterate over a list or channel)")
		}
		c.loop++
		c.pushScope()
		c.declare(st.Var, &Scheme{t: el}, false, "local", st.VarSpan)
		c.inferBlockStmts(st.Body)
		c.popScope()
		c.loop--
	case *ReturnStmt:
		var t Ty = tUnit
		if st.Val != nil {
			t = c.inferExpr(st.Val)
		}
		if len(c.retTys) == 0 {
			c.errorf(st.Span, "E0572", "", "`return` outside of a function")
			return
		}
		c.unifyAt(c.retTys[len(c.retTys)-1], t, st.Span, "in this `return`")
	case *SpawnStmt:
		c.inferExpr(st.CallE)
	case *SelectStmt:
		for i := range st.Arms {
			arm := &st.Arms[i]
			el := c.fresh("")
			ct := c.inferExpr(arm.Chan)
			c.unifyAt(ct, &TCon{Name: "chan", Args: []Ty{el}}, arm.Chan.span(),
				"in this `select` arm (need a channel)")
			if arm.IsSend {
				vt := c.inferExpr(arm.Val)
				c.unifyAt(el, vt, arm.Val.span(), "in this `select` send")
				c.inferExpr(arm.Body)
			} else {
				c.pushScope()
				if arm.Bind != "" && arm.Bind != "_" {
					c.declare(arm.Bind, &Scheme{t: el}, false, "local", arm.BindSpan)
				}
				c.inferExpr(arm.Body)
				c.popScope()
			}
		}
		if st.HasDefault {
			c.inferExpr(st.Default)
		}
	case *SendStmt:
		el := c.fresh("")
		ct := c.inferExpr(st.Chan)
		c.unifyAt(ct, &TCon{Name: "chan", Args: []Ty{el}}, st.Chan.span(), "on the left of `<-` (need a channel)")
		vt := c.inferExpr(st.Val)
		c.unifyAt(el, vt, st.Val.span(), "in this send")
	case *BreakStmt, *ContinueStmt:
		// loop nesting validated by the compiler
	}
}

func (c *Checker) inferBlockStmts(b *Block) {
	for _, s := range b.Stmts {
		c.inferStmt(s)
	}
}

// ---- expressions ----

var (
	tInt   = &TCon{Name: "int"}
	tFloat = &TCon{Name: "float"}
	tBool  = &TCon{Name: "bool"}
	tStr   = &TCon{Name: "str"}
	tUnit  = &TCon{Name: "unit"}
)

func (c *Checker) inferExpr(e Expr) Ty {
	switch ex := e.(type) {
	case *IntLit:
		return tInt
	case *FloatLit:
		return tFloat
	case *StrLit:
		return tStr
	case *BoolLit:
		return tBool
	case *Ident:
		if b := c.lookup(ex.Name); b != nil {
			b.used = true
			return c.instantiate(b.scheme)
		}
		// bare enum variant
		if sig, idx, amb := c.findVariantSig(ex.Name); amb {
			c.errorf(ex.Span, "E0659",
				fmt.Sprintf("qualify it, e.g. `SomeEnum.%s`", ex.Name),
				"`%s` is a variant of several enums", ex.Name)
			return c.fresh("")
		} else if idx >= 0 {
			return c.variantType(sig, idx)
		}
		c.errorf(ex.Span, "E0425", "", "cannot find `%s` in this scope", ex.Name)
		return c.fresh("")
	case *VariantAccess:
		sig, ok := c.enumSig(ex.EnumName)
		if !ok {
			c.errorf(ex.Span, "E0412", "", "cannot find enum `%s`", ex.EnumName)
			return c.fresh("")
		}
		for i, v := range sig.Variants {
			if v.Name == ex.Variant {
				return c.variantType(sig, i)
			}
		}
		c.errorf(ex.Span, "E0599", "", "enum `%s` has no variant `%s`", ex.EnumName, ex.Variant)
		return c.fresh("")
	case *PkgAccess:
		sym, ok := c.pkgs[ex.Pkg]
		if !ok {
			c.errorf(ex.Span, "E0433", "", "cannot find package `%s`", ex.Pkg)
			return c.fresh("")
		}
		b, ok := sym.vals[ex.Name]
		if !ok {
			if _, isEnum := sym.enums[ex.Name]; isEnum {
				c.errorf(ex.Span, "E0423", "name a variant: `"+ex.Pkg+"."+ex.Name+".SomeVariant`",
					"enum `%s` cannot be used as a value", ex.Name)
			} else {
				c.errorf(ex.Span, "E0425", "", "cannot find `%s` in package `%s`", ex.Name, ex.Pkg)
			}
			return c.fresh("")
		}
		if !b.pub {
			c.errorf(ex.Span, "E0603", fmt.Sprintf("mark it `pub` in package `%s` to export it", ex.Pkg),
				"`%s` is private to package `%s`", ex.Name, ex.Pkg)
		}
		b.used = true
		return c.instantiate(b.scheme)
	case *QualVariantAccess:
		sig := c.lookupPkgEnum(ex.Pkg, ex.Enum, ex.Span)
		if sig == nil {
			return c.fresh("")
		}
		for i, v := range sig.Variants {
			if v.Name == ex.Variant {
				return c.variantType(sig, i)
			}
		}
		c.errorf(ex.Span, "E0599", "", "enum `%s` has no variant `%s`", ex.Enum, ex.Variant)
		return c.fresh("")
	case *Unary:
		t := c.inferExpr(ex.Rhs)
		if ex.Op == TMinus {
			c.unifyAt(t, c.fresh("num"), ex.Span, "with unary `-`")
			return t
		}
		c.unifyAt(t, tBool, ex.Span, "with `!`")
		return tBool
	case *Binary:
		lt := c.inferExpr(ex.Lhs)
		rt := c.inferExpr(ex.Rhs)
		switch ex.Op {
		case TPlus:
			c.unifyAt(lt, rt, ex.Span, "with `+` (no implicit conversions: use to_float()/trunc())")
			c.unifyAt(lt, c.fresh("addord"), ex.Span, "with `+`")
			return lt
		case TMinus, TStar, TSlash:
			c.unifyAt(lt, rt, ex.Span, "with this operator (no implicit conversions: use to_float()/trunc())")
			c.unifyAt(lt, c.fresh("num"), ex.Span, "with this arithmetic operator")
			return lt
		case TPercent:
			c.unifyAt(lt, tInt, ex.Span, "with `%` (int only)")
			c.unifyAt(rt, tInt, ex.Span, "with `%` (int only)")
			return tInt
		case TLt, TLtEq, TGt, TGtEq:
			c.unifyAt(lt, rt, ex.Span, "in this comparison (no implicit conversions)")
			c.unifyAt(lt, c.fresh("addord"), ex.Span, "in this comparison")
			return tBool
		case TEqEq, TBangEq:
			c.unifyAt(lt, rt, ex.Span, "in this equality (both sides must be the same type)")
			return tBool
		}
		return c.fresh("")
	case *Logical:
		lt := c.inferExpr(ex.Lhs)
		rt := c.inferExpr(ex.Rhs)
		c.unifyAt(lt, tBool, ex.Lhs.span(), "with `&&`/`||`")
		c.unifyAt(rt, tBool, ex.Rhs.span(), "with `&&`/`||`")
		return tBool
	case *Call:
		return c.inferCall(ex)
	case *Index:
		el := c.fresh("")
		tt := c.inferExpr(ex.Target)
		c.unifyAt(tt, &TCon{Name: "list", Args: []Ty{el}}, ex.Target.span(),
			"when indexing (only lists; for strings use char_at(), for maps use get())")
		it := c.inferExpr(ex.Idx)
		c.unifyAt(it, tInt, ex.Idx.span(), "in this index")
		return el
	case *ListLit:
		el := c.fresh("")
		for _, e := range ex.Elems {
			t := c.inferExpr(e)
			c.unifyAt(el, t, e.span(), "in this list (all elements must share one type)")
		}
		return &TCon{Name: "list", Args: []Ty{el}}
	case *FnLit:
		c.pushScope()
		params := make([]Ty, len(ex.Params))
		for i, p := range ex.Params {
			params[i] = c.fresh("")
			c.declare(p, &Scheme{t: params[i]}, ex.ParamMuts[i], "param", ex.ParamSpans[i])
		}
		ret := c.fresh("")
		c.retTys = append(c.retTys, ret)
		bt := c.inferBlockExpr(ex.Body)
		c.unifyAt(ret, bt, ex.Body.Span, "as this function's result")
		c.retTys = c.retTys[:len(c.retTys)-1]
		c.popScope()
		return &TFunc{Params: params, Ret: ret}
	case *Block:
		c.pushScope()
		t := c.inferBlockExprNoScope(ex)
		c.popScope()
		return t
	case *IfExpr:
		ct := c.inferExpr(ex.Cond)
		c.unifyAt(ct, tBool, ex.Cond.span(), "in this `if` condition")
		tt := c.inferExpr(ex.Then)
		if ex.Else == nil {
			c.unifyAt(tt, tUnit, ex.Then.Span,
				"in this `if` (without an `else`, the branch must be unit)")
			return tUnit
		}
		et := c.inferExpr(ex.Else)
		c.unifyAt(tt, et, ex.Else.span(), "between `if` and `else` branches")
		return tt
	case *MatchExpr:
		return c.inferMatch(ex)
	case *TryExpr:
		return c.inferTry(ex)
	case *RecvExpr:
		el := c.fresh("")
		ct := c.inferExpr(ex.Chan)
		c.unifyAt(ct, &TCon{Name: "chan", Args: []Ty{el}}, ex.Span, "with `<-` (need a channel)")
		return el
	}
	return c.fresh("")
}

func (c *Checker) inferBlockExpr(b *Block) Ty {
	c.pushScope()
	t := c.inferBlockExprNoScope(b)
	c.popScope()
	return t
}

func (c *Checker) inferBlockExprNoScope(b *Block) Ty {
	n := len(b.Stmts)
	for i, s := range b.Stmts {
		if i == n-1 {
			if es, ok := s.(*ExprStmt); ok {
				return c.inferExpr(es.E)
			}
		}
		c.inferStmt(s)
	}
	return tUnit
}

func (c *Checker) variantType(sig *EnumSig, idx int) Ty {
	enumTy, subst := c.instantiateEnum(sig)
	v := sig.Variants[idx]
	if len(v.Fields) == 0 {
		return enumTy
	}
	fields := make([]Ty, len(v.Fields))
	for i, f := range v.Fields {
		fields[i] = c.convertTypeExpr(f, subst, sig.Prefix)
	}
	return &TFunc{Params: fields, Ret: enumTy}
}

// mutatingNatives are builtins that mutate their first argument in place.
// Calling them on a place rooted in an immutable binding is an error:
// `mut` is deep (GOALS: default immutability must mean what it says).
var mutatingNatives = map[string]bool{"push": true, "pop": true, "put": true, "delete": true}

// requireMutRoot walks an index chain (`xs`, `xs[i]`, `xs[i][j]`, ...) back
// to its root binding — a local, a global, or an imported package member —
// and enforces deep `mut`: the root binding of a mutated place
// must be declared mutable. what describes the mutation for the message.
func (c *Checker) requireMutRoot(e Expr, what string) {
	root := e
	for {
		if ix, ok := root.(*Index); ok {
			root = ix.Target
			continue
		}
		break
	}
	var name string
	var b *binding
	switch r := root.(type) {
	case *Ident:
		name = r.Name
		b = c.lookup(name)
	case *PkgAccess:
		name = r.Name
		if sym, ok := c.pkgs[r.Pkg]; ok {
			b = sym.vals[name]
		}
	default:
		return // unnamed temporary: no binding to check
	}
	if b == nil || b.mut {
		return
	}
	help := fmt.Sprintf("make this binding mutable: `let mut %s`", name)
	if b.kind == "param" {
		help = fmt.Sprintf("declare the parameter as mutable: `mut %s`", name)
	}
	c.errorf(e.span(), "E0596", help,
		"cannot %s, as `%s` is not declared as mutable", what, name)
}

func (c *Checker) inferCall(ex *Call) Ty {
	if id, ok := ex.Callee.(*Ident); ok && mutatingNatives[id.Name] && len(ex.Args) > 0 {
		if b := c.lookup(id.Name); b != nil && b.kind == "native" {
			c.requireMutRoot(ex.Args[0], fmt.Sprintf("mutate this list with `%s()`", id.Name))
		}
	}
	// special-cased natives
	if id, ok := ex.Callee.(*Ident); ok {
		if b := c.lookup(id.Name); b != nil && b.special != "" {
			b.used = true
			switch b.special {
			case "printf": // print/println: variadic, any types
				for _, a := range ex.Args {
					c.inferExpr(a)
				}
				return tUnit
			case "chan":
				if len(ex.Args) > 1 {
					c.errorf(ex.Span, "E0061", "", "chan() takes at most one argument (the capacity)")
				}
				for _, a := range ex.Args {
					at := c.inferExpr(a)
					c.unifyAt(at, tInt, a.span(), "as the channel capacity")
				}
				return &TCon{Name: "chan", Args: []Ty{c.fresh("")}}
			}
		}
	}
	ct := c.inferExpr(ex.Callee)
	args := make([]Ty, len(ex.Args))
	for i, a := range ex.Args {
		args[i] = c.inferExpr(a)
	}
	// better arity errors when the callee type is already concrete
	if f, ok := resolve(ct).(*TFunc); ok && len(f.Params) != len(args) {
		c.errorf(ex.Span, "E0061", "",
			"this call takes %d argument(s) but %d were supplied", len(f.Params), len(args))
		return f.Ret
	}
	ret := c.fresh("")
	c.unifyAt(ct, &TFunc{Params: args, Ret: ret}, ex.Span, "in this call")
	return ret
}

func (c *Checker) inferTry(ex *TryExpr) Ty {
	t := c.inferExpr(ex.Inner)
	if len(c.retTys) == 0 {
		c.errorf(ex.Span, "E0277", "", "`?` can only be used inside a function")
		return c.fresh("")
	}
	ret := c.retTys[len(c.retTys)-1]
	switch r := resolve(t).(type) {
	case *TCon:
		switch r.Name {
		case "Result":
			e := c.fresh("")
			ok := c.fresh("")
			c.unifyAt(t, &TCon{Name: "Result", Args: []Ty{ok, e}}, ex.Span, "with `?`")
			b := c.fresh("")
			c.unifyAt(ret, &TCon{Name: "Result", Args: []Ty{b, e}}, ex.Span,
				"with `?` (the enclosing function must return a Result with the same error type)")
			return ok
		case "Option":
			v := c.fresh("")
			c.unifyAt(t, &TCon{Name: "Option", Args: []Ty{v}}, ex.Span, "with `?`")
			b := c.fresh("")
			c.unifyAt(ret, &TCon{Name: "Option", Args: []Ty{b}}, ex.Span,
				"with `?` (the enclosing function must return an Option)")
			return v
		}
	}
	c.errorf(ex.Span, "E0277", "", "`?` needs an `Option` or `Result`, found `%s`", pretty(t))
	return c.fresh("")
}

// ---- match & exhaustiveness ----

func (c *Checker) inferMatch(m *MatchExpr) Ty {
	scrutT := c.inferExpr(m.Scrut)
	resT := c.fresh("")

	covered := map[string]bool{} // variant names / "true"/"false"
	hasCatchAll := false

	for _, arm := range m.Arms {
		c.pushScope()
		switch pat := arm.Pat.(type) {
		case *PatWildcard:
			hasCatchAll = true
		case *PatLiteral:
			var lt Ty
			switch pat.Val.(type) {
			case *IntLit:
				lt = tInt
			case *FloatLit:
				lt = tFloat
			case *StrLit:
				lt = tStr
			case *BoolLit:
				lt = tBool
				if b, ok := pat.Val.(*BoolLit); ok {
					covered[fmt.Sprintf("%v", b.Val)] = true
				}
			}
			c.unifyAt(scrutT, lt, pat.Span, "between this pattern and the matched value")
		case *PatBinding:
			if sig, idx, amb := c.findVariantSig(pat.Name); amb {
				c.errorf(pat.Span, "E0659",
					fmt.Sprintf("qualify it, e.g. `SomeEnum.%s`", pat.Name),
					"`%s` is a variant of several enums", pat.Name)
			} else if idx >= 0 && len(sig.Variants[idx].Fields) == 0 {
				enumTy, _ := c.instantiateEnum(sig)
				c.unifyAt(scrutT, enumTy, pat.Span, "between this pattern and the matched value")
				covered[pat.Name] = true
			} else if idx >= 0 {
				c.errorf(pat.Span, "E0532", "",
					"variant `%s` has %d field(s); bind them: `%s(...)`", pat.Name, len(sig.Variants[idx].Fields), pat.Name)
			} else {
				hasCatchAll = true
				c.declare(pat.Name, &Scheme{t: scrutT}, false, "local", pat.Span)
			}
		case *PatVariant:
			var sig *EnumSig
			idx := -1
			if pat.Pkg != "" {
				if s := c.lookupPkgEnum(pat.Pkg, pat.EnumName, pat.Span); s != nil {
					sig = s
					for i, v := range sig.Variants {
						if v.Name == pat.Variant {
							idx = i
							break
						}
					}
					if idx < 0 {
						c.errorf(pat.Span, "E0599", "", "enum `%s` has no variant `%s`", pat.EnumName, pat.Variant)
						sig = nil
					}
				}
			} else if pat.EnumName != "" {
				s, ok := c.enumSig(pat.EnumName)
				if !ok {
					c.errorf(pat.Span, "E0412", "", "cannot find enum `%s`", pat.EnumName)
				} else {
					sig = s
					for i, v := range sig.Variants {
						if v.Name == pat.Variant {
							idx = i
							break
						}
					}
					if idx < 0 {
						c.errorf(pat.Span, "E0599", "", "enum `%s` has no variant `%s`", pat.EnumName, pat.Variant)
						sig = nil
					}
				}
			} else {
				s, i, amb := c.findVariantSig(pat.Variant)
				if amb {
					c.errorf(pat.Span, "E0659",
						fmt.Sprintf("qualify it, e.g. `SomeEnum.%s`", pat.Variant),
						"`%s` is a variant of several enums", pat.Variant)
				} else if i < 0 {
					c.errorf(pat.Span, "E0425", "", "cannot find variant `%s`", pat.Variant)
				} else {
					sig, idx = s, i
				}
			}
			if sig != nil && idx >= 0 {
				enumTy, subst := c.instantiateEnum(sig)
				c.unifyAt(scrutT, enumTy, pat.Span, "between this pattern and the matched value")
				fields := sig.Variants[idx].Fields
				if len(pat.Binds) != len(fields) {
					c.errorf(pat.Span, "E0023", "",
						"variant `%s` has %d field(s), pattern binds %d", pat.Variant, len(fields), len(pat.Binds))
				} else {
					for i, sub := range pat.Binds {
						ft := c.convertTypeExpr(fields[i], subst, sig.Prefix)
						if b, ok := sub.(*PatBinding); ok {
							c.declare(b.Name, &Scheme{t: ft}, false, "local", b.Span)
						}
					}
				}
				covered[pat.Variant] = true
			}
		}
		bt := c.inferExpr(arm.Body)
		c.unifyAt(resT, bt, arm.Body.span(), "between the arms of this `match` (all arms must produce one type)")
		c.popScope()
	}

	// exhaustiveness
	if !hasCatchAll {
		switch s := resolve(scrutT).(type) {
		case *TCon:
			if sig, ok := c.enums[s.Name]; ok {
				var missing []string
				for _, v := range sig.Variants {
					if !covered[v.Name] {
						missing = append(missing, "`"+v.Name+"`")
					}
				}
				if len(missing) > 0 {
					c.errorf(m.Span, "E0004",
						"add the missing arms, or a `_ => ...` catch-all",
						"non-exhaustive `match`: variant(s) %s not covered", strings.Join(missing, ", "))
				}
			} else if s.Name == "bool" {
				if !covered["true"] || !covered["false"] {
					c.errorf(m.Span, "E0004",
						"cover both `true` and `false`, or add `_ => ...`",
						"non-exhaustive `match` on bool")
				}
			} else {
				c.errorf(m.Span, "E0004",
					"add a final binding or `_ => ...` arm",
					"non-exhaustive `match`: `%s` cannot be enumerated", pretty(scrutT))
			}
		default:
			c.errorf(m.Span, "E0004",
				"add a final binding or `_ => ...` arm",
				"non-exhaustive `match`")
		}
	}
	return resT
}

// ---- builtins ----

func (c *Checker) declareBuiltins() {
	c.enums["Option"] = &EnumSig{
		Name: "Option", Qual: "Option", Pub: true, Params: []string{"a"},
		Variants: []VariantSig{
			{Name: "Some", Fields: []TypeExpr{&TEName{Name: "a"}}},
			{Name: "None"},
		},
	}
	c.enums["Result"] = &EnumSig{
		Name: "Result", Qual: "Result", Pub: true, Params: []string{"a", "e"},
		Variants: []VariantSig{
			{Name: "Ok", Fields: []TypeExpr{&TEName{Name: "a"}}},
			{Name: "Err", Fields: []TypeExpr{&TEName{Name: "e"}}},
		},
	}
	// Output(code, stdout, stderr): the single-variant result of exec()
	c.enums["Output"] = &EnumSig{
		Name: "Output", Qual: "Output", Pub: true,
		Variants: []VariantSig{
			{Name: "Output", Fields: []TypeExpr{
				&TEName{Name: "int"}, &TEName{Name: "str"}, &TEName{Name: "str"},
			}},
		},
	}

	mono := func(t Ty) *Scheme { return &Scheme{t: t} }
	fn := func(ret Ty, ps ...Ty) *TFunc { return &TFunc{Params: ps, Ret: ret} }
	list := func(t Ty) Ty { return &TCon{Name: "list", Args: []Ty{t}} }
	mapt := func(k, v Ty) Ty { return &TCon{Name: "map", Args: []Ty{k, v}} }
	opt := func(t Ty) Ty { return &TCon{Name: "Option", Args: []Ty{t}} }
	res := func(a, e Ty) Ty { return &TCon{Name: "Result", Args: []Ty{a, e}} }

	poly1 := func(mk func(a *TV) *TFunc) *Scheme {
		a := &TV{id: -1, level: 1}
		return &Scheme{vars: []*TV{a}, t: mk(a)}
	}
	// poly2map quantifies a key-constrained K and an unconstrained V.
	poly2map := func(mk func(k, v *TV) *TFunc) *Scheme {
		k := &TV{id: -1, level: 1, con: "key"}
		v := &TV{id: -1, level: 1}
		return &Scheme{vars: []*TV{k, v}, t: mk(k, v)}
	}

	decl := func(name string, s *Scheme) {
		c.declare(name, s, false, "native", Span{})
	}

	decl("len", poly1(func(a *TV) *TFunc { return fn(tInt, list(a)) }))
	decl("push", poly1(func(a *TV) *TFunc { return fn(tUnit, list(a), a) }))
	decl("pop", poly1(func(a *TV) *TFunc { return fn(a, list(a)) }))
	decl("map", poly2map(func(k, v *TV) *TFunc { return fn(mapt(k, v)) }))
	decl("get", poly2map(func(k, v *TV) *TFunc { return fn(opt(v), mapt(k, v), k) }))
	decl("put", poly2map(func(k, v *TV) *TFunc { return fn(tUnit, mapt(k, v), k, v) }))
	decl("delete", poly2map(func(k, v *TV) *TFunc { return fn(tUnit, mapt(k, v), k) }))
	decl("keys", poly2map(func(k, v *TV) *TFunc { return fn(list(k), mapt(k, v)) }))
	decl("str", poly1(func(a *TV) *TFunc { return fn(tStr, a) }))
	decl("type_of", poly1(func(a *TV) *TFunc { return fn(tStr, a) }))
	decl("str_len", mono(fn(tInt, tStr)))
	decl("char_at", mono(fn(tStr, tStr, tInt)))
	decl("trunc", mono(fn(tInt, tFloat)))
	decl("to_float", mono(fn(tFloat, tInt)))
	decl("parse_int", mono(fn(&TCon{Name: "Option", Args: []Ty{tInt}}, tStr)))
	decl("parse_float", mono(fn(&TCon{Name: "Option", Args: []Ty{tFloat}}, tStr)))
	decl("range", mono(fn(list(tInt), tInt, tInt)))
	decl("split", mono(fn(list(tStr), tStr, tStr)))
	decl("join", mono(fn(tStr, list(tStr), tStr)))
	decl("substr", mono(fn(tStr, tStr, tInt, tInt)))
	decl("str_contains", mono(fn(tBool, tStr, tStr)))
	decl("str_index_of", mono(fn(opt(tInt), tStr, tStr)))
	decl("trim", mono(fn(tStr, tStr)))
	decl("slice", poly1(func(a *TV) *TFunc { return fn(list(a), list(a), tInt, tInt) }))
	decl("concat", poly1(func(a *TV) *TFunc { return fn(list(a), list(a), list(a)) }))
	decl("contains", poly1(func(a *TV) *TFunc { return fn(tBool, list(a), a) }))
	decl("read_file", mono(fn(res(tStr, tStr), tStr)))
	decl("write_file", mono(fn(res(tUnit, tStr), tStr, tStr)))
	decl("file_exists", mono(fn(tBool, tStr)))
	decl("read_dir", mono(fn(res(list(tStr), tStr), tStr)))
	decl("exec", mono(fn(res(&TCon{Name: "Output"}, tStr), tStr, list(tStr))))
	decl("args", mono(fn(list(tStr))))
	// exit never returns, so it unifies with any expected type
	decl("exit", poly1(func(a *TV) *TFunc { return fn(a, tInt) }))
	decl("clock", mono(fn(tFloat)))
	decl("assert", mono(fn(tUnit, tBool, tStr)))
	decl("gc", mono(fn(tInt)))
	decl("heap_objects", mono(fn(tInt)))
	decl("gc_cycles", mono(fn(tInt)))
	decl("chr", mono(fn(tStr, tInt)))
	decl("ord", mono(fn(tInt, tStr)))
	decl("yield", mono(fn(tUnit)))
	decl("close", poly1(func(a *TV) *TFunc { return fn(tUnit, &TCon{Name: "chan", Args: []Ty{a}}) }))
	decl("recv", poly1(func(a *TV) *TFunc { return fn(opt(a), &TCon{Name: "chan", Args: []Ty{a}}) }))

	decl("print", mono(fn(tUnit)))
	c.scopes[0]["print"].special = "printf"
	decl("println", mono(fn(tUnit)))
	c.scopes[0]["println"].special = "printf"
	decl("chan", mono(fn(&TCon{Name: "chan", Args: []Ty{tUnit}})))
	c.scopes[0]["chan"].special = "chan"
}

// ---- pretty-printing ----

func pretty(t Ty) string {
	names := map[*TV]string{}
	return prettyWith(t, names)
}

func prettyWith(t Ty, names map[*TV]string) string {
	switch x := resolve(t).(type) {
	case *TV:
		n, ok := names[x]
		if !ok {
			n = "'" + string(rune('a'+len(names)%26))
			names[x] = n
		}
		if x.con != "" {
			return n + ":" + x.con
		}
		return n
	case *TCon:
		switch x.Name {
		case "list":
			return "[" + prettyWith(x.Args[0], names) + "]"
		case "chan":
			return "chan(" + prettyWith(x.Args[0], names) + ")"
		}
		if len(x.Args) == 0 {
			return x.Name
		}
		parts := make([]string, len(x.Args))
		for i, a := range x.Args {
			parts[i] = prettyWith(a, names)
		}
		return x.Name + "(" + strings.Join(parts, ", ") + ")"
	case *TFunc:
		parts := make([]string, len(x.Params))
		for i, p := range x.Params {
			parts[i] = prettyWith(p, names)
		}
		return "fn(" + strings.Join(parts, ", ") + ") -> " + prettyWith(x.Ret, names)
	}
	return "?"
}
