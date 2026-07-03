package main

import (
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
