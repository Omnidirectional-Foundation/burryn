package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite examples/*.golden from current output")

// fib.bur prints a wall-clock benchmark; scrub durations so goldens stay stable.
var timingRE = regexp.MustCompile(`\d+(\.\d+)? ms`)

func scrubTimings(s string) string { return timingRE.ReplaceAllString(s, "<time> ms") }

// TestExampleModules runs every examples/<dir> containing a bur.mod through
// the module pipeline and compares stdout against examples/<dir>.golden.
// Module examples must stay diagnostic-clean: zero errors, zero warnings.
// Regenerate goldens with: go test -run TestExampleModules -update .
func TestExampleModules(t *testing.T) {
	mods, err := filepath.Glob(filepath.Join("examples", "*", "bur.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) == 0 {
		t.Fatal("no module examples found under examples/")
	}
	for _, modFile := range mods {
		dir := filepath.Dir(modFile)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			m, diags := loadModule(dir)
			diags = append(diags, typecheckModule(m)...)
			for _, d := range diags {
				t.Errorf("example is not diagnostic-clean: [%s] %s", d.Code, d.Msg)
			}
			if t.Failed() {
				return
			}
			gc := newGC()
			fn, shared, compDiags := compileModule(gc, m)
			if len(compDiags) > 0 {
				t.Fatalf("compile error: [%s] %s", compDiags[0].Code, compDiags[0].Msg)
			}
			var buf bytes.Buffer
			vm := newVM(gc, shared)
			vm.out = &buf
			if err := vm.run(fn); err != nil {
				t.Fatalf("runtime error: %v\noutput so far:\n%s", err, buf.String())
			}
			got := scrubTimings(buf.String())
			goldenPath := dir + ".golden"
			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden file (regenerate with: go test -run TestExampleModules -update .): %v", err)
			}
			if want := string(wantBytes); got != want {
				t.Errorf("output does not match golden\n--- got ---\n%s\n--- want (%s) ---\n%s", got, goldenPath, want)
			}
		})
	}
}

// TestExamples runs every examples/*.bur through the full pipeline and
// compares stdout against the .golden file next to it. Examples must also
// stay diagnostic-clean: zero errors and zero warnings.
// Regenerate goldens with: go test -run TestExamples -update .
func TestExamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("examples", "*.bur"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no examples found under examples/")
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".bur")
		t.Run(name, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			src := string(srcBytes)
			for _, d := range checkDiags(t, src) {
				t.Errorf("example is not diagnostic-clean: [%s] %s", d.Code, d.Msg)
			}
			if t.Failed() {
				return
			}
			out, cerr, rerr := interpret(src)
			if cerr != nil {
				t.Fatalf("compile error: %v", cerr)
			}
			if rerr != nil {
				t.Fatalf("runtime error: %v\noutput so far:\n%s", rerr, out)
			}
			got := scrubTimings(out)
			goldenPath := strings.TrimSuffix(file, ".bur") + ".golden"
			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden file (regenerate with: go test -run TestExamples -update .): %v", err)
			}
			if want := string(wantBytes); got != want {
				t.Errorf("output does not match golden\n--- got ---\n%s\n--- want (%s) ---\n%s", got, goldenPath, want)
			}
		})
	}
}
