package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
	for _, pat := range []string{"examples/*.bur", "examples/*/*.bur", "examples/*/*/*.bur", "burc/*.bur"} {
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
