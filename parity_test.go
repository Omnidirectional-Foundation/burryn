package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// concurrentExamples still rely on the fiber scheduler, which the C backend
// grows in the concurrency core; they are excluded from sequential parity.
var concurrentExamples = map[string]bool{
	"sieve": true, "brainfuck": true, "multiplex": true,
	"pipeline": true, "streaming": true,
}

// compileScriptToC runs the front end on a script source and returns its
// generated C, without the process-exiting error handling of the CLI path.
func compileScriptToC(src string) (string, error) {
	toks, lexDiags := lex(src)
	if err := diagsToErr(lexDiags); err != nil {
		return "", err
	}
	stmts, parseDiags := parse(toks)
	if err := diagsToErr(parseDiags); err != nil {
		return "", err
	}
	if err := diagsToErr(typecheck(stmts)); err != nil {
		return "", err
	}
	gc := newGC()
	fn, shared, compDiags := compileProgram(gc, src, stmts)
	if err := diagsToErr(compDiags); err != nil {
		return "", err
	}
	return genProgram(fn, shared)
}

// compileModuleToC runs the module pipeline on a package directory and
// returns its generated C.
func compileModuleToC(dir string) (string, error) {
	m, loadDiags := loadModule(dir)
	if err := diagsToErr(loadDiags); err != nil {
		return "", err
	}
	if err := diagsToErr(typecheckModule(m)); err != nil {
		return "", err
	}
	gc := newGC()
	fn, shared, compDiags := compileModule(gc, m)
	if err := diagsToErr(compDiags); err != nil {
		return "", err
	}
	return genProgram(fn, shared)
}

// buildBinary compiles C source to an executable in a temp dir and returns
// its path.
func buildBinary(t *testing.T, csrc string) string {
	t.Helper()
	cc := findCC()
	if cc == "" {
		t.Skip("no C compiler available")
	}
	bin := filepath.Join(t.TempDir(), "prog")
	if err := compileC(cc, csrc, bin); err != nil {
		t.Fatalf("C compilation failed: %v", err)
	}
	return bin
}

// runBinary executes bin and returns its stdout and exit code.
func runBinary(t *testing.T, bin string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	cmd := exec.Command(bin)
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", bin, err)
	}
	return out.String(), code
}

// TestParityScripts compiles every sequential examples/*.bur through the C
// backend and asserts its stdout matches the VM's byte for byte (timings
// scrubbed). Trap text and GC-internal counters are out of the parity
// contract, so examples never rely on them.
func TestParityScripts(t *testing.T) {
	if findCC() == "" {
		t.Skip("no C compiler available")
	}
	files, err := filepath.Glob(filepath.Join("examples", "*.bur"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".bur")
		if concurrentExamples[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			src := string(srcBytes)
			vmOut, cerr, rerr := interpret(src)
			if cerr != nil || rerr != nil {
				t.Fatalf("VM failed: compile=%v runtime=%v", cerr, rerr)
			}
			csrc, err := compileScriptToC(src)
			if err != nil {
				t.Fatalf("C backend failed: %v", err)
			}
			bin := buildBinary(t, csrc)
			cOut, code := runBinary(t, bin)
			if code != 0 {
				t.Fatalf("C binary exited %d", code)
			}
			if got, want := scrubTimings(cOut), scrubTimings(vmOut); got != want {
				t.Errorf("stdout mismatch\n--- C backend ---\n%s\n--- VM ---\n%s", got, want)
			}
		})
	}
}

// TestParityModules does the same for sequential module examples.
func TestParityModules(t *testing.T) {
	if findCC() == "" {
		t.Skip("no C compiler available")
	}
	mods, err := filepath.Glob(filepath.Join("examples", "*", "bur.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, modFile := range mods {
		dir := filepath.Dir(modFile)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			m, diags := loadModule(dir)
			diags = append(diags, typecheckModule(m)...)
			if err := diagsToErr(diags); err != nil {
				t.Fatalf("module not clean: %v", err)
			}
			gc := newGC()
			fn, shared, compDiags := compileModule(gc, m)
			if err := diagsToErr(compDiags); err != nil {
				t.Fatalf("compile error: %v", err)
			}
			var buf bytes.Buffer
			vm := newVM(gc, shared)
			vm.out = &buf
			if err := vm.run(fn); err != nil {
				t.Fatalf("VM runtime error: %v", err)
			}
			csrc, err := compileModuleToC(dir)
			if err != nil {
				t.Fatalf("C backend failed: %v", err)
			}
			bin := buildBinary(t, csrc)
			cOut, code := runBinary(t, bin)
			if code != 0 {
				t.Fatalf("C binary exited %d", code)
			}
			if got, want := scrubTimings(cOut), scrubTimings(buf.String()); got != want {
				t.Errorf("stdout mismatch\n--- C backend ---\n%s\n--- VM ---\n%s", got, want)
			}
		})
	}
}
