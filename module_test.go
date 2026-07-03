package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTree materializes a file tree under a temp dir; keys use / separators.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func loadTree(t *testing.T, files map[string]string, entry string) (*Module, []Diag) {
	t.Helper()
	dir := writeTree(t, files)
	return loadModule(filepath.Join(dir, filepath.FromSlash(entry)))
}

func hasDiagCode(diags []Diag, code string, isErr bool) bool {
	for _, d := range diags {
		if d.Code == code && d.IsErr == isErr {
			return true
		}
	}
	return false
}

func expectLoadDiag(t *testing.T, files map[string]string, entry, code string) {
	t.Helper()
	_, diags := loadTree(t, files, entry)
	if !hasDiagCode(diags, code, true) {
		t.Fatalf("expected a %s error, got %v", code, diags)
	}
}

const modHeader = "module example.com/m\n"

func TestLoadModuleHappyPath(t *testing.T) {
	m, diags := loadTree(t, map[string]string{
		"bur.mod": modHeader,
		"main.bur": `import "example.com/m/util"

fn main() {
    println(util.double(2))
}
`,
		"util/u.bur": "pub fn double(x) { x * 2 }\n",
	}, ".")
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("unexpected error: [%s] %s", d.Code, d.Msg)
		}
	}
	if m.Path != "example.com/m" {
		t.Fatalf("module path = %q", m.Path)
	}
	if len(m.Packages) != 2 || m.Packages[1] != m.Entry {
		t.Fatalf("want [util, entry] load order, got %d packages, entry last = %v",
			len(m.Packages), len(m.Packages) > 0 && m.Packages[len(m.Packages)-1] == m.Entry)
	}
	if m.Packages[0].ImportPath != "example.com/m/util" || m.Packages[0].Name != "util" {
		t.Fatalf("dep package = %+v", m.Packages[0])
	}
	// util.double must have been rewritten to a PkgAccess carrying the import path
	fnMain := m.Entry.Files[0].Stmts[0].(*FnDecl)
	call := fnMain.Fn.Body.Stmts[0].(*ExprStmt).E.(*Call)
	inner := call.Args[0].(*Call)
	pa, ok := inner.Callee.(*PkgAccess)
	if !ok || pa.Pkg != "example.com/m/util" || pa.Name != "double" {
		t.Fatalf("callee not rewritten to PkgAccess: %#v", inner.Callee)
	}
}

func TestLoadModuleExplicitAlias(t *testing.T) {
	m, diags := loadTree(t, map[string]string{
		"bur.mod":    modHeader,
		"main.bur":   "import u \"example.com/m/util\"\n\nfn main() {\n    println(u.double(2))\n}\n",
		"util/u.bur": "pub fn double(x) { x * 2 }\n",
	}, ".")
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("unexpected error: [%s] %s", d.Code, d.Msg)
		}
	}
	if m.Entry.Files[0].Aliases["u"] != "example.com/m/util" {
		t.Fatalf("aliases = %v", m.Entry.Files[0].Aliases)
	}
}

func TestLoadModuleMissingBurMod(t *testing.T) {
	expectLoadDiag(t, map[string]string{"main.bur": "fn main() { }\n"}, ".", "E0432")
}

func TestLoadModuleBadBurMod(t *testing.T) {
	expectLoadDiag(t, map[string]string{"bur.mod": "flavor grape\n", "main.bur": "fn main() { }\n"}, ".", "E0432")
	expectLoadDiag(t, map[string]string{"bur.mod": "", "main.bur": "fn main() { }\n"}, ".", "E0432")
	expectLoadDiag(t, map[string]string{
		"bur.mod": modHeader + "require example.com/lib 1.2.0\n", "main.bur": "fn main() { }\n"}, ".", "E0432")
}

func TestLoadModuleImportOutsideModule(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "import \"other.com/lib\"\n\nfn main() { }\n",
	}, ".", "E0432")
}

func TestLoadModuleImportCycle(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "import \"example.com/m/a\"\n\nfn main() { println(a.f()) }\n",
		"a/a.bur":  "import \"example.com/m/b\"\n\npub fn f() { b.g() }\n",
		"b/b.bur":  "import \"example.com/m/a\"\n\npub fn g() { a.f() }\n",
	}, ".", "E0391")
}

func TestLoadModuleDuplicateAlias(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":     modHeader,
		"main.bur":    "import \"example.com/m/util\"\nimport util \"example.com/m/util2\"\n\nfn main() { println(util.f()) }\n",
		"util/u.bur":  "pub fn f() { 1 }\n",
		"util2/u.bur": "pub fn f() { 2 }\n",
	}, ".", "E0252")
}

func TestLoadModuleAliasConflictsDecl(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":    modHeader,
		"main.bur":   "import \"example.com/m/util\"\n\nfn util() { 1 }\nfn main() { println(util.f()) }\n",
		"util/u.bur": "pub fn f() { 1 }\n",
	}, ".", "E0252")
}

func TestLoadModuleImportAfterDecl(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":    modHeader,
		"main.bur":   "fn main() { println(util.f()) }\n\nimport \"example.com/m/util\"\n",
		"util/u.bur": "pub fn f() { 1 }\n",
	}, ".", "E0432")
}

func TestLoadModuleTopLevelStatement(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "println(1)\n\nfn main() { }\n",
	}, ".", "E0801")
}

func TestLoadModuleNonConstLet(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "let x = range(0, 3)\n\nfn main() { println(x) }\n",
	}, ".", "E0802")
}

func TestLoadModuleConstLetForms(t *testing.T) {
	_, diags := loadTree(t, map[string]string{
		"bur.mod": modHeader,
		"main.bur": `let a = 1 + 2 * 3
let b = [1, 2, 3]
let c = "x" + "y"
let d = true && false
let e = -5
let f = fn(x) { x + 1 }

fn main() {
    println(a, b, c, d, e, f(1))
}
`,
	}, ".")
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("unexpected error: [%s] %s", d.Code, d.Msg)
		}
	}
}

func TestLoadModuleDuplicateDeclAcrossFiles(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod": modHeader,
		"a.bur":   "fn helper() { 1 }\nfn main() { println(helper()) }\n",
		"b.bur":   "fn helper() { 2 }\n",
	}, ".", "E0428")
}

func TestLoadModuleUnknownQualifier(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "fn main() {\n    let _ = geo.Shape.Circle(1.0)\n}\n",
	}, ".", "E0433")
}

func TestLoadModuleMissingPackageDir(t *testing.T) {
	expectLoadDiag(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "import \"example.com/m/nope\"\n\nfn main() { println(nope.f()) }\n",
	}, ".", "E0433")
}

func TestLoadModuleUnusedImportWarns(t *testing.T) {
	_, diags := loadTree(t, map[string]string{
		"bur.mod":    modHeader,
		"main.bur":   "import \"example.com/m/util\"\n\nfn main() { }\n",
		"util/u.bur": "pub fn f() { 1 }\n",
	}, ".")
	if !hasDiagCode(diags, "unused_import", false) {
		t.Fatalf("expected an unused_import warning, got %v", diags)
	}
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("unexpected error: [%s] %s", d.Code, d.Msg)
		}
	}
}

func TestLoadModuleEntryIsSubdir(t *testing.T) {
	m, diags := loadTree(t, map[string]string{
		"bur.mod":     modHeader,
		"cmd/app.bur": "import \"example.com/m/util\"\n\nfn main() { println(util.f()) }\n",
		"util/u.bur":  "pub fn f() { 1 }\n",
	}, "cmd")
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("unexpected error: [%s] %s", d.Code, d.Msg)
		}
	}
	if m.Entry.ImportPath != "example.com/m/cmd" {
		t.Fatalf("entry path = %q", m.Entry.ImportPath)
	}
}

// checkTree loads a module (failing the test on load errors) and typechecks it.
func checkTree(t *testing.T, files map[string]string, entry string) []Diag {
	t.Helper()
	m, diags := loadTree(t, files, entry)
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("load error: [%s] %s", d.Code, d.Msg)
		}
	}
	return typecheckModule(m)
}

func expectModuleError(t *testing.T, files map[string]string, code string) {
	t.Helper()
	diags := checkTree(t, files, ".")
	if !hasDiagCode(diags, code, true) {
		t.Fatalf("expected a %s error, got %v", code, diags)
	}
}

func expectModuleClean(t *testing.T, files map[string]string) {
	t.Helper()
	for _, d := range checkTree(t, files, ".") {
		t.Fatalf("expected no diagnostics, got: [%s] %s", d.Code, d.Msg)
	}
}

const geoSrc = `pub enum Shape {
    Circle(float),
    Rect(int, int),
}

pub fn area(s) {
    match s {
        Circle(r) => 3.14 * r * r,
        Rect(w, h) => to_float(w * h),
    }
}

pub let inc = fn(x) { x + 1 }
`

func TestModuleCheckCleanCrossPackage(t *testing.T) {
	expectModuleClean(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur": `import "example.com/m/geo"

fn main() {
    let s = geo.Shape.Circle(2.0)
    println(geo.area(s))
    match s {
        geo.Shape.Circle(r) => println(r),
        geo.Shape.Rect(w, h) => println(w, h),
    }
    println(geo.inc(41))
}
`,
	})
}

func TestModuleCheckCrossPackageTypeError(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    println(geo.area(1))\n}\n",
	}, "E0308")
}

func TestModuleCheckPrivateFn(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": "fn secret() { 1 }\npub fn ok() { secret() }\n",
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    println(geo.secret())\n}\n",
	}, "E0603")
}

func TestModuleCheckPrivateEnum(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": "enum Hidden { X }\npub fn tag() { X }\n",
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    let _ = geo.Hidden.X\n}\n",
	}, "E0603")
}

func TestModuleCheckUnknownMember(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    println(geo.nope())\n}\n",
	}, "E0425")
}

func TestModuleCheckEnumAsValue(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    let _ = geo.Shape\n}\n",
	}, "E0423")
}

func TestModuleCheckMainMissing(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "pub fn helper() { 1 }\n",
	}, "E0601")
}

func TestModuleCheckMainWithParams(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":  modHeader,
		"main.bur": "fn main(x) { println(x) }\n",
	}, "E0580")
}

func TestModuleCheckCrossPackagePushImmutable(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": "pub let xs = [1, 2]\n",
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    push(geo.xs, 3)\n}\n",
	}, "E0596")
}

func TestModuleCheckCrossPackagePushMutable(t *testing.T) {
	expectModuleClean(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": "pub let mut xs = [1, 2]\n",
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    push(geo.xs, 3)\n    println(geo.xs)\n}\n",
	})
}

func TestModuleCheckUnusedPrivateWarns(t *testing.T) {
	diags := checkTree(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": "fn dead() { 1 }\npub fn ok() { 2 }\n",
		"main.bur":  "import \"example.com/m/geo\"\n\nfn main() {\n    println(geo.ok())\n}\n",
	}, ".")
	if !hasDiagCode(diags, "unused_variable", false) {
		t.Fatalf("expected an unused_variable warning for `dead`, got %v", diags)
	}
	for _, d := range diags {
		if d.IsErr {
			t.Fatalf("unexpected error: [%s] %s", d.Code, d.Msg)
		}
	}
}

func TestModuleCheckSameEnumNameIsDistinct(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur": `import "example.com/m/geo"

enum Shape {
    Circle(float),
}

fn main() {
    println(geo.area(Circle(1.0)))
}
`,
	}, "E0308")
}

func TestModuleCheckQualifiedTypeInEnumField(t *testing.T) {
	expectModuleClean(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur": `import "example.com/m/geo"

enum Wrap {
    W(geo.Shape),
}

fn main() {
    match Wrap.W(geo.Shape.Circle(1.0)) {
        Wrap.W(s) => println(geo.area(s)),
    }
}
`,
	})
}

func TestModuleCheckExhaustivenessAcrossPackages(t *testing.T) {
	expectModuleError(t, map[string]string{
		"bur.mod":   modHeader,
		"geo/g.bur": geoSrc,
		"main.bur": `import "example.com/m/geo"

fn main() {
    match geo.Shape.Circle(1.0) {
        geo.Shape.Circle(r) => println(r),
    }
}
`,
	}, "E0004")
}
