package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---- module model ----

// A Require is one dependency line from bur.mod. Parsed and validated now;
// resolution (MVS + fetching) is a later milestone.
type Require struct {
	Path    string
	Version string
}

// A SourceFile is one parsed .bur file inside a package. Imports are
// file-scoped: each file names its own dependencies.
type SourceFile struct {
	Path    string // display path used in diagnostics
	Src     string
	Imports []*ImportDecl     // leading import declarations, in order
	Aliases map[string]string // import alias -> import path
	Stmts   []Stmt            // top-level declarations after the imports
}

// A Package is a directory of .bur files: directory = package.
type Package struct {
	ImportPath string
	Name       string // default qualifier: last path segment
	Dir        string
	Files      []*SourceFile
	Deps       []string // import paths of direct dependencies, deduped
}

// A Module is a bur.mod file plus every package loaded through it.
type Module struct {
	Path     string // module path from the module directive
	Root     string // directory containing bur.mod
	Requires []Require
	Entry    *Package
	Packages []*Package          // dependency order: imports first, entry last
	ByPath   map[string]*Package // import path -> package
	Srcs     map[string]string   // display path -> source, for renderDiags

	cwd   string
	diags []Diag
	state map[string]int // import path -> loading state
}

const (
	pkgUnseen = iota
	pkgLoading
	pkgDone
)

func (m *Module) errorf(file string, sp Span, code, help, format string, args ...any) {
	m.diags = append(m.diags, Diag{
		IsErr: true, Code: code, File: file, Span: sp,
		Msg: fmt.Sprintf(format, args...), Help: help,
	})
}

func (m *Module) warnf(file string, sp Span, code, help, format string, args ...any) {
	m.diags = append(m.diags, Diag{
		IsErr: false, Code: code, File: file, Span: sp,
		Msg: fmt.Sprintf(format, args...), Help: help,
	})
}

// displayPath shortens an absolute path to a cwd-relative one when that does
// not escape upward, keeping diagnostics stable and readable.
func (m *Module) displayPath(p string) string {
	if m.cwd == "" {
		return p
	}
	rel, err := filepath.Rel(m.cwd, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

// loadModule loads the module that contains entryDir and every package
// reachable from the entry package. Any returned error diagnostics mean the
// module must not be checked or compiled.
func loadModule(entryDir string) (*Module, []Diag) {
	m := &Module{
		ByPath: map[string]*Package{},
		Srcs:   map[string]string{},
		state:  map[string]int{},
	}
	m.cwd, _ = os.Getwd()

	absDir, err := filepath.Abs(entryDir)
	if err != nil {
		m.errorf("", Span{}, "E0432", "", "cannot resolve directory %q: %v", entryDir, err)
		return m, m.diags
	}
	root, ok := findBurMod(absDir)
	if !ok {
		m.errorf("", Span{}, "E0432",
			"create one with a module directive, e.g. `module example.com/you/hello`",
			"bur.mod not found in %s or any parent directory", m.displayPath(absDir))
		return m, m.diags
	}
	m.Root = root
	modFile := m.displayPath(filepath.Join(root, "bur.mod"))
	modBytes, err := os.ReadFile(filepath.Join(root, "bur.mod"))
	if err != nil {
		m.errorf("", Span{}, "E0432", "", "cannot read %s: %v", modFile, err)
		return m, m.diags
	}
	m.Srcs[modFile] = string(modBytes)
	m.parseBurMod(modFile, string(modBytes))
	if len(m.diags) > 0 {
		return m, m.diags
	}

	entryPath := m.Path
	if rel, err := filepath.Rel(root, absDir); err == nil && rel != "." {
		entryPath = m.Path + "/" + filepath.ToSlash(rel)
	}
	m.Entry = m.loadPackage(entryPath, "", Span{}, nil)
	return m, m.diags
}

// findBurMod walks upward from dir looking for a bur.mod file.
func findBurMod(dir string) (root string, ok bool) {
	for {
		if st, err := os.Stat(filepath.Join(dir, "bur.mod")); err == nil && !st.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// parseBurMod reads the line-oriented bur.mod format:
//
//	module example.com/you/name
//	require example.com/other/lib v1.2.0
func (m *Module) parseBurMod(file, src string) {
	off := 0
	for _, line := range strings.SplitAfter(src, "\n") {
		lineOff := off
		off += len(line)
		text := strings.TrimSuffix(line, "\n")
		if i := strings.Index(text, "//"); i >= 0 {
			text = text[:i]
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		start := lineOff + strings.Index(line, fields[0])
		sp := Span{Start: start, End: lineOff + len(strings.TrimRight(text, " \t"))}
		switch fields[0] {
		case "module":
			if len(fields) != 2 {
				m.errorf(file, sp, "E0432", "write `module <path>`, e.g. `module example.com/you/hello`",
					"malformed module directive")
				continue
			}
			if m.Path != "" {
				m.errorf(file, sp, "E0432", "", "duplicate module directive")
				continue
			}
			if !validImportPath(fields[1]) {
				m.errorf(file, sp, "E0432", "", "invalid module path %q", fields[1])
				continue
			}
			m.Path = fields[1]
		case "require":
			if len(fields) != 3 {
				m.errorf(file, sp, "E0432", "write `require <path> <version>`, e.g. `require example.com/lib v1.2.0`",
					"malformed require directive")
				continue
			}
			if !validImportPath(fields[1]) {
				m.errorf(file, sp, "E0432", "", "invalid require path %q", fields[1])
				continue
			}
			if !strings.HasPrefix(fields[2], "v") {
				m.errorf(file, sp, "E0432", "versions look like v1.2.0", "invalid version %q", fields[2])
				continue
			}
			m.Requires = append(m.Requires, Require{Path: fields[1], Version: fields[2]})
		default:
			m.errorf(file, sp, "E0432", "bur.mod knows the directives `module` and `require`",
				"unknown directive %q", fields[0])
		}
	}
	if m.Path == "" && len(m.diags) == 0 {
		m.errorf(file, Span{}, "E0432",
			"add a module directive, e.g. `module example.com/you/hello`",
			"missing module directive in %s", file)
	}
}

func validImportPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return false
	}
	for _, el := range strings.Split(p, "/") {
		if el == "" || el == "." || el == ".." {
			return false
		}
	}
	return !strings.ContainsAny(p, "\\ \t\"'`")
}

// loadPackage loads path and, depth-first, everything it imports, appending
// each package to m.Packages after its dependencies (a ready topological
// order). fromFile/fromSpan point at the import that requested the package.
func (m *Module) loadPackage(path, fromFile string, fromSpan Span, stack []string) *Package {
	switch m.state[path] {
	case pkgDone:
		return m.ByPath[path]
	case pkgLoading:
		cycle := append(stack[indexOf(stack, path):], path)
		m.errorf(fromFile, fromSpan, "E0391", "",
			"import cycle: %s", strings.Join(cycle, " -> "))
		return nil
	}
	m.state[path] = pkgLoading
	stack = append(stack, path)

	pkg := m.readPackage(path, fromFile, fromSpan)
	if pkg != nil {
		for _, f := range pkg.Files {
			for _, imp := range f.Imports {
				target := f.Aliases[importAlias(imp)]
				if target == "" {
					continue // resolution already failed with a diagnostic
				}
				m.loadPackage(target, f.Path, imp.PathSpan, stack)
			}
		}
		m.state[path] = pkgDone
		m.ByPath[path] = pkg
		m.Packages = append(m.Packages, pkg)
	} else {
		m.state[path] = pkgDone
	}
	return pkg
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return 0
}

func importAlias(d *ImportDecl) string {
	if d.Alias != "" {
		return d.Alias
	}
	if i := strings.LastIndex(d.Path, "/"); i >= 0 {
		return d.Path[i+1:]
	}
	return d.Path
}

// readPackage parses every .bur file in the package directory, enforces the
// package-file shape (imports first, then declarations only), resolves import
// aliases, and rewrites alias-qualified paths to import paths.
func (m *Module) readPackage(path, fromFile string, fromSpan Span) *Package {
	if path != m.Path && !strings.HasPrefix(path, m.Path+"/") {
		m.errorf(fromFile, fromSpan, "E0432",
			fmt.Sprintf("only packages inside module `%s` can be imported for now; external dependencies arrive with MVS resolution", m.Path),
			"cannot resolve import %q", path)
		return nil
	}
	dir := filepath.Join(m.Root, filepath.FromSlash(strings.TrimPrefix(path, m.Path)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.errorf(fromFile, fromSpan, "E0433", "", "cannot find package %q: %v", path, err)
		return nil
	}
	pkg := &Package{ImportPath: path, Name: path[strings.LastIndex(path, "/")+1:], Dir: dir}
	burFiles := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bur") {
			continue
		}
		burFiles++
		full := filepath.Join(dir, e.Name())
		disp := m.displayPath(full)
		srcBytes, err := os.ReadFile(full)
		if err != nil {
			m.errorf(fromFile, fromSpan, "E0433", "", "cannot read %s: %v", disp, err)
			continue
		}
		src := string(srcBytes)
		m.Srcs[disp] = src
		f := m.parseFile(disp, src)
		if f != nil {
			pkg.Files = append(pkg.Files, f)
		}
	}
	if burFiles == 0 {
		m.errorf(fromFile, fromSpan, "E0433", "", "no .bur files in package %q (%s)", path, m.displayPath(dir))
		return nil
	}
	m.checkPackageDecls(pkg)
	for _, f := range pkg.Files {
		m.resolveFile(pkg, f)
	}
	sort.Strings(pkg.Deps)
	return pkg
}

// parseFile lexes and parses one package file and splits its leading imports
// from the declarations.
func (m *Module) parseFile(disp, src string) *SourceFile {
	toks, lexDiags := lex(src)
	if len(lexDiags) > 0 {
		stampFile(lexDiags, disp)
		m.diags = append(m.diags, lexDiags...)
		return nil
	}
	stmts, parseDiags := parse(toks)
	if len(parseDiags) > 0 {
		stampFile(parseDiags, disp)
		m.diags = append(m.diags, parseDiags...)
		return nil
	}
	f := &SourceFile{Path: disp, Src: src, Aliases: map[string]string{}}
	importsDone := false
	for _, s := range stmts {
		imp, isImport := s.(*ImportDecl)
		if !isImport {
			importsDone = true
			f.Stmts = append(f.Stmts, s)
			continue
		}
		if importsDone {
			m.errorf(disp, imp.Span, "E0432",
				"move it to the top of the file, above the declarations", "`import` must come before any declarations")
			continue
		}
		f.Imports = append(f.Imports, imp)
	}
	return f
}

// checkPackageDecls enforces the package-file shape across all files: only
// declarations at top level, const-expression lets, and no duplicate names.
func (m *Module) checkPackageDecls(pkg *Package) {
	declared := map[string]string{} // name -> file it was first declared in
	for _, f := range pkg.Files {
		for _, s := range f.Stmts {
			var name string
			var nameSpan Span
			switch st := s.(type) {
			case *FnDecl:
				name, nameSpan = st.Name, st.NameSpan
			case *EnumDecl:
				name, nameSpan = st.Name, st.Span
			case *LetStmt:
				name, nameSpan = st.Name, st.NameSpan
				if !isConstExpr(st.Init) {
					m.errorf(f.Path, st.Init.span(), "E0802",
						"package-level `let` allows literals, arithmetic on literals, list literals, and `fn` literals; compute anything else inside a function",
						"package-level `let` must be initialized with a constant expression")
				}
			default:
				m.errorf(f.Path, s.span(), "E0801",
					"executable code lives inside functions; the program starts at `fn main`",
					"only declarations are allowed at the top level of a package file")
				continue
			}
			if prev, dup := declared[name]; dup {
				where := ""
				if prev != f.Path {
					where = fmt.Sprintf(" (first declared in %s)", prev)
				}
				m.errorf(f.Path, nameSpan, "E0428", "",
					"`%s` is declared more than once in package %q%s", name, pkg.ImportPath, where)
				continue
			}
			declared[name] = f.Path
		}
	}
}

// isConstExpr reports whether e can initialize a package-level let: no calls,
// no names, nothing that runs user code at package init.
func isConstExpr(e Expr) bool {
	switch x := e.(type) {
	case *IntLit, *FloatLit, *StrLit, *BoolLit, *FnLit:
		return true
	case *Unary:
		return isConstExpr(x.Rhs)
	case *Binary:
		return isConstExpr(x.Lhs) && isConstExpr(x.Rhs)
	case *Logical:
		return isConstExpr(x.Lhs) && isConstExpr(x.Rhs)
	case *ListLit:
		for _, el := range x.Elems {
			if !isConstExpr(el) {
				return false
			}
		}
		return true
	}
	return false
}

// ---- per-file alias resolution ----

// resolveFile fills f.Aliases from f.Imports, then rewrites every
// alias-qualified path in the file's AST to carry the target import path.
func (m *Module) resolveFile(pkg *Package, f *SourceFile) {
	seenDep := map[string]bool{}
	for _, imp := range f.Imports {
		alias := importAlias(imp)
		aliasSpan := imp.AliasSpan
		if imp.Alias == "" {
			aliasSpan = imp.PathSpan
		}
		if !validImportPath(imp.Path) {
			m.errorf(f.Path, imp.PathSpan, "E0432", "", "invalid import path %q", imp.Path)
			continue
		}
		if alias == "_" || alias == "Option" || alias == "Result" {
			m.errorf(f.Path, aliasSpan, "E0252",
				"rename the import: `import other \""+imp.Path+"\"`",
				"cannot use `%s` as an import name", alias)
			continue
		}
		if _, dup := f.Aliases[alias]; dup {
			m.errorf(f.Path, aliasSpan, "E0252",
				"rename one of them: `import other \""+imp.Path+"\"`",
				"`%s` is imported more than once in this file", alias)
			continue
		}
		f.Aliases[alias] = imp.Path
		if imp.Path != pkg.ImportPath && !seenDep[imp.Path] {
			seenDep[imp.Path] = true
			pkg.Deps = append(pkg.Deps, imp.Path)
		}
	}
	// import aliases and package-level names share the qualifier position, so
	// they must not collide
	for _, other := range pkg.Files {
		for _, s := range other.Stmts {
			var name string
			switch st := s.(type) {
			case *FnDecl:
				name = st.Name
			case *EnumDecl:
				name = st.Name
			case *LetStmt:
				name = st.Name
			}
			if name == "" {
				continue
			}
			if _, clash := f.Aliases[name]; clash {
				m.errorf(f.Path, importSpanFor(f, name), "E0252",
					"rename the import with an alias: `import other \"...\"`",
					"import `%s` conflicts with a declaration in this package", name)
				delete(f.Aliases, name)
			}
		}
	}
	r := &fileResolver{m: m, f: f, used: map[string]bool{}}
	for i, s := range f.Stmts {
		f.Stmts[i] = r.stmt(s)
	}
	for _, imp := range f.Imports {
		alias := importAlias(imp)
		if _, ok := f.Aliases[alias]; ok && !r.used[alias] {
			m.warnf(f.Path, imp.Span, "unused_import", "remove the import", "unused import: `%s`", alias)
		}
	}
}

func importSpanFor(f *SourceFile, alias string) Span {
	for _, imp := range f.Imports {
		if importAlias(imp) == alias {
			return imp.Span
		}
	}
	return Span{}
}

// fileResolver rewrites one file's AST: two-segment paths whose head is an
// import alias become PkgAccess, and the alias in three-segment variant
// paths, variant patterns, and qualified type names becomes the import path.
type fileResolver struct {
	m    *Module
	f    *SourceFile
	used map[string]bool
}

// resolveQual maps an alias written in source to its import path.
func (r *fileResolver) resolveQual(alias string, sp Span) (string, bool) {
	if path, ok := r.f.Aliases[alias]; ok {
		r.used[alias] = true
		return path, true
	}
	r.m.errorf(r.f.Path, sp, "E0433",
		fmt.Sprintf("import it first: `import \"%s/%s\"`", r.m.Path, alias),
		"cannot find package `%s`", alias)
	return "", false
}

func (r *fileResolver) stmt(s Stmt) Stmt {
	switch st := s.(type) {
	case *LetStmt:
		st.Init = r.expr(st.Init)
	case *AssignStmt:
		st.Target = r.expr(st.Target)
		st.Val = r.expr(st.Val)
	case *ExprStmt:
		st.E = r.expr(st.E)
	case *WhileStmt:
		st.Cond = r.expr(st.Cond)
		r.block(st.Body)
	case *ForStmt:
		st.Iter = r.expr(st.Iter)
		r.block(st.Body)
	case *ReturnStmt:
		if st.Val != nil {
			st.Val = r.expr(st.Val)
		}
	case *FnDecl:
		r.expr(st.Fn)
	case *EnumDecl:
		for _, v := range st.Variants {
			for _, te := range v.Types {
				r.typeExpr(te)
			}
		}
	case *SpawnStmt:
		st.CallE = r.expr(st.CallE).(*Call)
	case *SendStmt:
		st.Chan = r.expr(st.Chan)
		st.Val = r.expr(st.Val)
	}
	return s
}

func (r *fileResolver) block(b *Block) {
	for i, s := range b.Stmts {
		b.Stmts[i] = r.stmt(s)
	}
}

func (r *fileResolver) expr(e Expr) Expr {
	switch ex := e.(type) {
	case *VariantAccess:
		if path, ok := r.f.Aliases[ex.EnumName]; ok {
			r.used[ex.EnumName] = true
			return &PkgAccess{Pkg: path, Name: ex.Variant, Span: ex.Span}
		}
	case *QualVariantAccess:
		if path, ok := r.resolveQual(ex.Pkg, ex.Span); ok {
			ex.Pkg = path
		}
	case *Unary:
		ex.Rhs = r.expr(ex.Rhs)
	case *Binary:
		ex.Lhs = r.expr(ex.Lhs)
		ex.Rhs = r.expr(ex.Rhs)
	case *Logical:
		ex.Lhs = r.expr(ex.Lhs)
		ex.Rhs = r.expr(ex.Rhs)
	case *Call:
		ex.Callee = r.expr(ex.Callee)
		for i, a := range ex.Args {
			ex.Args[i] = r.expr(a)
		}
	case *Index:
		ex.Target = r.expr(ex.Target)
		ex.Idx = r.expr(ex.Idx)
	case *ListLit:
		for i, el := range ex.Elems {
			ex.Elems[i] = r.expr(el)
		}
	case *FnLit:
		r.block(ex.Body)
	case *Block:
		r.block(ex)
	case *IfExpr:
		ex.Cond = r.expr(ex.Cond)
		r.block(ex.Then)
		if ex.Else != nil {
			ex.Else = r.expr(ex.Else)
		}
	case *MatchExpr:
		ex.Scrut = r.expr(ex.Scrut)
		for i := range ex.Arms {
			r.pattern(ex.Arms[i].Pat)
			ex.Arms[i].Body = r.expr(ex.Arms[i].Body)
		}
	case *TryExpr:
		ex.Inner = r.expr(ex.Inner)
	case *RecvExpr:
		ex.Chan = r.expr(ex.Chan)
	}
	return e
}

func (r *fileResolver) pattern(p Pattern) {
	if pv, ok := p.(*PatVariant); ok {
		if pv.Pkg != "" {
			if path, ok := r.resolveQual(pv.Pkg, pv.Span); ok {
				pv.Pkg = path
			}
		}
		for _, sub := range pv.Binds {
			r.pattern(sub)
		}
	}
}

func (r *fileResolver) typeExpr(te TypeExpr) {
	switch x := te.(type) {
	case *TEName:
		if x.Pkg != "" {
			if path, ok := r.resolveQual(x.Pkg, x.Span); ok {
				x.Pkg = path
			}
		}
		for _, a := range x.Args {
			r.typeExpr(a)
		}
	case *TEList:
		r.typeExpr(x.Elem)
	case *TEFn:
		for _, p := range x.Params {
			r.typeExpr(p)
		}
		r.typeExpr(x.Ret)
	}
}
