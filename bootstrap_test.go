package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file checks the bootstrap compiler (burc/, written in Burryn) against
// the Go implementation, stage by stage. Each stage dumps its result in a
// stable text format; the Go side renders the same format here and the two
// must match byte for byte over the whole corpus.

// tokDumpNames mirrors burc/token.bur's tok_names table (and token.go's
// TokType order); keep all three in sync.
var tokDumpNames = []string{"TLParen", "TRParen", "TLBrace", "TRBrace",
	"TLBracket", "TRBracket", "TComma", "TDot", "TMinus", "TPlus", "TSlash",
	"TStar", "TPercent", "TSemi", "TQuestion", "TColon", "TBang", "TBangEq",
	"TEq", "TEqEq", "TGt", "TGtEq", "TLt", "TLtEq", "TAndAnd", "TOrOr",
	"TArrow", "TLArrow", "TThinArrow", "TIdent", "TString", "TInt", "TFloat",
	"TLet", "TMut", "TFn", "TIf", "TElse", "TWhile", "TFor", "TIn", "TReturn",
	"TTrue", "TFalse", "TEnum", "TMatch", "TSpawn", "TBreak", "TContinue",
	"TPub", "TImport", "TSelect", "TEOF"}

// dumpEscape mirrors burc/dump.bur's dump_escape: quote the string, escape
// backslash/newline/tab/quote, pass every other byte through raw.
func dumpEscape(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

// dumpDiags mirrors burc/dump.bur's dump_diag: diagnostic text stays outside
// the parity contract, so only kind, code and span are compared.
func dumpDiags(b *strings.Builder, diags []Diag) {
	for _, d := range diags {
		kind := "warning"
		if d.IsErr {
			kind = "error"
		}
		fmt.Fprintf(b, "diag %s %s %d %d\n", kind, d.Code, d.Span.Start, d.Span.End)
	}
}

func dumpGoLex(src string) string {
	toks, diags := lex(src)
	var b strings.Builder
	for _, t := range toks {
		fmt.Fprintf(&b, "%s %d %d %s\n", tokDumpNames[t.Type], t.Span.Start, t.Span.End, dumpEscape(t.Lex))
	}
	dumpDiags(&b, diags)
	return b.String()
}

// burcProgram loads, checks and compiles burc/ once per test run; individual
// corpus files then run on a fresh VM each.
type burcProgram struct {
	gc     *GC
	fn     *OFunc
	shared *Shared
}

func loadBurc(t *testing.T) *burcProgram {
	t.Helper()
	m, diags := loadModule("burc")
	diags = append(diags, typecheckModule(m)...)
	for _, d := range diags {
		t.Errorf("burc is not diagnostic-clean: [%s] %s", d.Code, d.Msg)
	}
	if t.Failed() {
		t.Fatal("aborting: burc must be diagnostic-clean")
	}
	gc := newGC()
	fn, shared, compDiags := compileModule(gc, m)
	if len(compDiags) > 0 {
		t.Fatalf("burc compile error: [%s] %s", compDiags[0].Code, compDiags[0].Msg)
	}
	return &burcProgram{gc: gc, fn: fn, shared: shared}
}

// run executes burc with argv on a fresh VM and returns its stdout.
func (p *burcProgram) run(t *testing.T, argv ...string) string {
	t.Helper()
	var buf bytes.Buffer
	vm := newVM(p.gc, p.shared)
	vm.out = &buf
	vm.args = argv
	if err := vm.run(p.fn); err != nil {
		t.Fatalf("burc %v: runtime error: %v\noutput so far:\n%s", argv, err, buf.String())
	}
	return buf.String()
}

// burCorpus is every .bur file in the repository: the examples (script and
// module form) plus burc's own sources, so the bootstrap stages always face
// themselves.
func burCorpus(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, pat := range []string{"examples/*.bur", "examples/*/*.bur", "examples/*/*/*.bur", "burc/*.bur", "testdata/*/*.bur"} {
		fs, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, fs...)
	}
	if len(files) == 0 {
		t.Fatal("empty corpus")
	}
	return files
}

func firstDiff(got, want string) string {
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gl) && i < len(wl); i++ {
		if gl[i] != wl[i] {
			return fmt.Sprintf("line %d:\nburc: %s\ngo:   %s", i+1, gl[i], wl[i])
		}
	}
	return fmt.Sprintf("line counts differ: burc %d, go %d", len(gl), len(wl))
}

// ---- AST dump (mirrors burc/dump.bur's dump_node) ----

func spanStr(sp Span) string { return fmt.Sprintf("%d:%d", sp.Start, sp.End) }

func strsAtom(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = dumpEscape(x)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func boolsAtom(xs []bool) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprintf("%t", x)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func spansAtom(xs []Span) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = spanStr(x)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// dumpNode renders one node per line — kind, atoms in struct field order,
// span last — children indented two spaces; nil prints as NoNode. Values are
// formatted through the VM's display() so both sides share one float format.
func dumpNode(b *strings.Builder, n any, pre string) {
	deeper := pre + "  "
	line := func(format string, args ...any) { fmt.Fprintf(b, "%s"+format+"\n", append([]any{pre}, args...)...) }
	if n == nil {
		line("NoNode")
		return
	}
	switch x := n.(type) {
	case *IntLit:
		line("IntLit %s %s", display(IntV(x.Val)), spanStr(x.Span))
	case *FloatLit:
		line("FloatLit %s %s", display(FloatV(x.Val)), spanStr(x.Span))
	case *StrLit:
		line("StrLit %s %s", dumpEscape(x.Val), spanStr(x.Span))
	case *BoolLit:
		line("BoolLit %t %s", x.Val, spanStr(x.Span))
	case *Ident:
		line("Ident %s %s", dumpEscape(x.Name), spanStr(x.Span))
	case *Unary:
		line("Unary %s %s", tokDumpNames[x.Op], spanStr(x.Span))
		dumpNode(b, x.Rhs, deeper)
	case *Binary:
		line("Binary %s %s", tokDumpNames[x.Op], spanStr(x.Span))
		dumpNode(b, x.Lhs, deeper)
		dumpNode(b, x.Rhs, deeper)
	case *Logical:
		line("Logical %s %s", tokDumpNames[x.Op], spanStr(x.Span))
		dumpNode(b, x.Lhs, deeper)
		dumpNode(b, x.Rhs, deeper)
	case *Call:
		line("Call %s", spanStr(x.Span))
		dumpNode(b, x.Callee, deeper)
		for _, a := range x.Args {
			dumpNode(b, a, deeper)
		}
	case *Index:
		line("Index %s", spanStr(x.Span))
		dumpNode(b, x.Target, deeper)
		dumpNode(b, x.Idx, deeper)
	case *ListLit:
		line("ListLit %s", spanStr(x.Span))
		for _, e := range x.Elems {
			dumpNode(b, e, deeper)
		}
	case *FnLit:
		line("FnLit %s params=%s muts=%s pspans=%s %s", dumpEscape(x.Name),
			strsAtom(x.Params), boolsAtom(x.ParamMuts), spansAtom(x.ParamSpans), spanStr(x.Span))
		dumpNode(b, x.Body, deeper)
	case *Block:
		line("Block %s", spanStr(x.Span))
		for _, s := range x.Stmts {
			dumpNode(b, s, deeper)
		}
	case *IfExpr:
		line("IfExpr %s", spanStr(x.Span))
		dumpNode(b, x.Cond, deeper)
		dumpNode(b, x.Then, deeper)
		if x.Else == nil {
			dumpNode(b, nil, deeper)
		} else {
			dumpNode(b, x.Else, deeper)
		}
	case *MatchExpr:
		line("MatchExpr %s", spanStr(x.Span))
		dumpNode(b, x.Scrut, deeper)
		for _, a := range x.Arms {
			fmt.Fprintf(b, "%sMatchArm %s\n", deeper, spanStr(a.Span))
			dumpNode(b, a.Pat, deeper+"  ")
			dumpNode(b, a.Body, deeper+"  ")
		}
	case *VariantAccess:
		line("VariantAccess %s %s %s", dumpEscape(x.EnumName), dumpEscape(x.Variant), spanStr(x.Span))
	case *PkgAccess:
		line("PkgAccess %s %s %s", dumpEscape(x.Pkg), dumpEscape(x.Name), spanStr(x.Span))
	case *QualVariantAccess:
		line("QualVariantAccess %s %s %s %s", dumpEscape(x.Pkg), dumpEscape(x.Enum), dumpEscape(x.Variant), spanStr(x.Span))
	case *TryExpr:
		line("TryExpr %s", spanStr(x.Span))
		dumpNode(b, x.Inner, deeper)
	case *RecvExpr:
		line("RecvExpr %s", spanStr(x.Span))
		dumpNode(b, x.Chan, deeper)
	case *PatWildcard:
		line("PatWildcard %s", spanStr(x.Span))
	case *PatLiteral:
		line("PatLiteral %s", spanStr(x.Span))
		dumpNode(b, x.Val, deeper)
	case *PatBinding:
		line("PatBinding %s %s", dumpEscape(x.Name), spanStr(x.Span))
	case *PatVariant:
		line("PatVariant %s %s %s %s", dumpEscape(x.Pkg), dumpEscape(x.EnumName), dumpEscape(x.Variant), spanStr(x.Span))
		for _, sub := range x.Binds {
			dumpNode(b, sub, deeper)
		}
	case *LetStmt:
		line("LetStmt %s %s mut=%t pub=%t %s", dumpEscape(x.Name), spanStr(x.NameSpan), x.Mut, x.Pub, spanStr(x.Span))
		dumpNode(b, x.Init, deeper)
	case *AssignStmt:
		line("AssignStmt %s", spanStr(x.Span))
		dumpNode(b, x.Target, deeper)
		dumpNode(b, x.Val, deeper)
	case *ExprStmt:
		line("ExprStmt %s", spanStr(x.Span))
		dumpNode(b, x.E, deeper)
	case *WhileStmt:
		line("WhileStmt %s", spanStr(x.Span))
		dumpNode(b, x.Cond, deeper)
		dumpNode(b, x.Body, deeper)
	case *ForStmt:
		line("ForStmt %s %s chan=%t %s", dumpEscape(x.Var), spanStr(x.VarSpan), x.IterIsChan, spanStr(x.Span))
		dumpNode(b, x.Iter, deeper)
		dumpNode(b, x.Body, deeper)
	case *ReturnStmt:
		line("ReturnStmt %s", spanStr(x.Span))
		if x.Val == nil {
			dumpNode(b, nil, deeper)
		} else {
			dumpNode(b, x.Val, deeper)
		}
	case *FnDecl:
		line("FnDecl %s %s pub=%t %s", dumpEscape(x.Name), spanStr(x.NameSpan), x.Pub, spanStr(x.Span))
		dumpNode(b, x.Fn, deeper)
	case *EnumDecl:
		line("EnumDecl %s params=%s pub=%t %s", dumpEscape(x.Name), strsAtom(x.Params), x.Pub, spanStr(x.Span))
		for _, v := range x.Variants {
			fmt.Fprintf(b, "%sEnumVariantDecl %s %d\n", deeper, dumpEscape(v.Name), v.Arity)
			for _, ty := range v.Types {
				dumpNode(b, ty, deeper+"  ")
			}
		}
	case *ImportDecl:
		line("ImportDecl %s %s %s %s %s", dumpEscape(x.Alias), spanStr(x.AliasSpan),
			dumpEscape(x.Path), spanStr(x.PathSpan), spanStr(x.Span))
	case *SpawnStmt:
		line("SpawnStmt %s", spanStr(x.Span))
		dumpNode(b, x.CallE, deeper)
	case *SelectStmt:
		line("SelectStmt default=%t %s %s", x.HasDefault, spanStr(x.DefaultSpan), spanStr(x.Span))
		for _, a := range x.Arms {
			fmt.Fprintf(b, "%sSelectArm send=%t %s %s %s\n", deeper, a.IsSend,
				dumpEscape(a.Bind), spanStr(a.BindSpan), spanStr(a.Span))
			dumpNode(b, a.Chan, deeper+"  ")
			if a.Val == nil {
				dumpNode(b, nil, deeper+"  ")
			} else {
				dumpNode(b, a.Val, deeper+"  ")
			}
			dumpNode(b, a.Body, deeper+"  ")
		}
		if x.Default == nil {
			dumpNode(b, nil, deeper)
		} else {
			dumpNode(b, x.Default, deeper)
		}
	case *SendStmt:
		line("SendStmt %s", spanStr(x.Span))
		dumpNode(b, x.Chan, deeper)
		dumpNode(b, x.Val, deeper)
	case *BreakStmt:
		line("BreakStmt %s", spanStr(x.Span))
	case *ContinueStmt:
		line("ContinueStmt %s", spanStr(x.Span))
	case *TEName:
		line("TEName %s %s %s", dumpEscape(x.Pkg), dumpEscape(x.Name), spanStr(x.Span))
		for _, a := range x.Args {
			dumpNode(b, a, deeper)
		}
	case *TEList:
		line("TEList %s", spanStr(x.Span))
		dumpNode(b, x.Elem, deeper)
	case *TEFn:
		line("TEFn %s", spanStr(x.Span))
		for _, p := range x.Params {
			dumpNode(b, p, deeper)
		}
		dumpNode(b, x.Ret, deeper)
	default:
		panic(fmt.Sprintf("dumpNode: unhandled node %T", n))
	}
}

// dumpGoParse mirrors burc's `parse` command: on lex errors dump only those
// (the pipeline stops before parsing), otherwise the AST plus parse diags.
func dumpGoParse(src string) string {
	toks, lexDiags := lex(src)
	var b strings.Builder
	if len(lexDiags) > 0 {
		dumpDiags(&b, lexDiags)
		return b.String()
	}
	stmts, parseDiags := parse(toks)
	for _, s := range stmts {
		dumpNode(&b, s, "")
	}
	dumpDiags(&b, parseDiags)
	return b.String()
}

// ---- check dump (mirrors burc's `check` command) ----

// dumpGoCheck runs lex/parse/typecheck with the same stop-at-first-failing-
// stage behavior as burc's cmd_check, dumping bind lines (top-level globals
// sorted by name) when error-free, then diagnostics.
func dumpGoCheck(src string) string {
	toks, lexDiags := lex(src)
	var b strings.Builder
	if len(lexDiags) > 0 {
		dumpDiags(&b, lexDiags)
		return b.String()
	}
	stmts, parseDiags := parse(toks)
	if len(parseDiags) > 0 {
		dumpDiags(&b, parseDiags)
		return b.String()
	}
	// replicate typecheck() but keep the checker for the bind dump
	c := newChecker()
	for _, s := range stmts {
		if st, ok := s.(*EnumDecl); ok {
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
	for _, s := range stmts {
		switch st := s.(type) {
		case *FnDecl:
			if st.Pub {
				c.errPubScript(st.Span)
			}
			c.level++
			ft := c.inferExpr(st.Fn)
			c.level--
			bind := c.lookup(st.Name)
			bind.scheme = c.generalize(ft)
			c.unifyAt(pre[st.Name], ft, st.NameSpan, "in this function")
		case *LetStmt:
			if st.Pub {
				c.errPubScript(st.Span)
			}
			ty := c.inferLetInit(st)
			c.unifyAt(pre[st.Name], ty, st.NameSpan, "in this binding")
		case *EnumDecl:
			if st.Pub {
				c.errPubScript(st.Span)
			}
		default:
			c.inferStmt(s)
		}
	}
	for name, bind := range c.scopes[0] {
		if bind.kind == "global" && !bind.used && !strings.HasPrefix(name, "_") {
			what := "variable"
			if bind.isFn {
				what = "function"
			}
			c.warnf(bind.span, "unused_variable",
				fmt.Sprintf("if this is intentional, prefix it with an underscore: `_%s`", name),
				"unused %s: `%s`", what, name)
		}
	}
	sort.SliceStable(c.diags, func(i, j int) bool {
		return c.diags[i].Span.Start < c.diags[j].Span.Start
	})

	hasErr := false
	for _, d := range c.diags {
		if d.IsErr {
			hasErr = true
		}
	}
	if !hasErr {
		var names []string
		for name, bind := range c.scopes[0] {
			if bind.kind == "global" {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "bind %s :: %s\n", name, pretty(c.scopes[0][name].scheme.t))
		}
	}
	dumpDiags(&b, c.diags)
	return b.String()
}

func TestBurcCheckParity(t *testing.T) {
	prog := loadBurc(t)
	for _, file := range burCorpus(t) {
		t.Run(file, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := dumpGoCheck(string(srcBytes))
			got := prog.run(t, "check", file)
			if got != want {
				t.Errorf("check dump mismatch (%d vs %d bytes)\n%s", len(got), len(want), firstDiff(got, want))
			}
		})
	}
}

// ---- dis dump (mirrors burc's `dis` command) ----

// captureStdout runs f with os.Stdout redirected into a buffer (debug.go's
// disassembler prints straight to stdout).
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()
	f()
	w.Close()
	os.Stdout = old
	return <-done
}

// dumpGoDis mirrors burc's cmd_dis: every diagnostic, then (when error-free)
// the disassembly of the compiled script.
func dumpGoDis(t *testing.T, src string) string {
	toks, lexDiags := lex(src)
	var b strings.Builder
	if len(lexDiags) > 0 {
		dumpDiags(&b, lexDiags)
		return b.String()
	}
	stmts, parseDiags := parse(toks)
	if len(parseDiags) > 0 {
		dumpDiags(&b, parseDiags)
		return b.String()
	}
	diags := typecheck(stmts)
	dumpDiags(&b, diags)
	for _, d := range diags {
		if d.IsErr {
			return b.String()
		}
	}
	gc := newGC()
	fn, shared, compDiags := compileProgram(gc, src, stmts)
	if len(compDiags) > 0 {
		dumpDiags(&b, compDiags)
		return b.String()
	}
	b.WriteString(captureStdout(t, func() { disasmAll(fn, shared) }))
	return b.String()
}

func TestBurcDisParity(t *testing.T) {
	prog := loadBurc(t)
	for _, file := range burCorpus(t) {
		t.Run(file, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := dumpGoDis(t, string(srcBytes))
			got := prog.run(t, "dis", file)
			if got != want {
				t.Errorf("disasm mismatch (%d vs %d bytes)\n%s", len(got), len(want), firstDiff(got, want))
			}
		})
	}
}

// ---- emitted C (mirrors burc's `emit-c` command) ----

// dumpGoEmitC mirrors burc's cmd_emit_c: every diagnostic, then (when
// error-free) genProgram's C translation unit.
func dumpGoEmitC(src string) string {
	toks, lexDiags := lex(src)
	var b strings.Builder
	if len(lexDiags) > 0 {
		dumpDiags(&b, lexDiags)
		return b.String()
	}
	stmts, parseDiags := parse(toks)
	if len(parseDiags) > 0 {
		dumpDiags(&b, parseDiags)
		return b.String()
	}
	diags := typecheck(stmts)
	dumpDiags(&b, diags)
	for _, d := range diags {
		if d.IsErr {
			return b.String()
		}
	}
	gc := newGC()
	fn, shared, compDiags := compileProgram(gc, src, stmts)
	if len(compDiags) > 0 {
		dumpDiags(&b, compDiags)
		return b.String()
	}
	csrc, err := genProgram(fn, shared)
	if err != nil {
		b.WriteString("cgen error: " + err.Error() + "\n")
		return b.String()
	}
	b.WriteString(csrc)
	return b.String()
}

func TestBurcEmitCParity(t *testing.T) {
	prog := loadBurc(t)
	for _, file := range burCorpus(t) {
		t.Run(file, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := dumpGoEmitC(string(srcBytes))
			got := prog.run(t, "emit-c", file)
			if got != want {
				t.Errorf("emitted C mismatch (%d vs %d bytes)\n%s", len(got), len(want), firstDiff(got, want))
			}
		})
	}
}

func TestBurcLexParity(t *testing.T) {
	prog := loadBurc(t)
	for _, file := range burCorpus(t) {
		t.Run(file, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := dumpGoLex(string(srcBytes))
			got := prog.run(t, "lex", file)
			if got != want {
				t.Errorf("token dump mismatch (%d vs %d bytes)\n%s", len(got), len(want), firstDiff(got, want))
			}
		})
	}
}

func TestBurcParseParity(t *testing.T) {
	prog := loadBurc(t)
	for _, file := range burCorpus(t) {
		t.Run(file, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := dumpGoParse(string(srcBytes))
			got := prog.run(t, "parse", file)
			if got != want {
				t.Errorf("AST dump mismatch (%d vs %d bytes)\n%s", len(got), len(want), firstDiff(got, want))
			}
		})
	}
}
