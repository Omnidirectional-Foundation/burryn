package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This file checks the Burryn-hosted VM (burc/vm.bur, S2) against the Go VM:
// `burc run <example>` must reproduce the Go VM's stdout byte for byte
// (timings scrubbed) over every script example, sequential and concurrent.

func exampleScripts(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("examples", "*.bur"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	return files
}

// TestBurcRunParity interprets every example on the Burryn VM hosted by the
// Go VM (double interpretation) and compares stdout with the Go VM's.
func TestBurcRunParity(t *testing.T) {
	prog := loadBurc(t)
	for _, file := range exampleScripts(t) {
		name := strings.TrimSuffix(filepath.Base(file), ".bur")
		t.Run(name, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			vmOut, cerr, rerr := interpret(string(srcBytes))
			if cerr != nil || rerr != nil {
				t.Fatalf("Go VM failed: compile=%v runtime=%v", cerr, rerr)
			}
			got := prog.run(t, "run", file)
			if scrubTimings(got) != scrubTimings(vmOut) {
				t.Errorf("stdout mismatch\n--- burc run ---\n%s\n--- Go VM ---\n%s", got, vmOut)
			}
		})
	}
}

// TestBurcSelfHostRun compiles burc (with its VM) to a native binary through
// the C backend and reruns the whole example suite on it: the S2 acceptance
// shape — a Burryn-written VM running as a native program against the Go VM.
func TestBurcSelfHostRun(t *testing.T) {
	if findCC() == "" {
		t.Skip("no C compiler available")
	}
	m, diags := loadModule("burc")
	if err := diagsToErr(diags); err != nil {
		t.Fatal(err)
	}
	if err := diagsToErr(typecheckModule(m)); err != nil {
		t.Fatal(err)
	}
	gc := newGC()
	fn, shared, compDiags := compileModule(gc, m)
	if err := diagsToErr(compDiags); err != nil {
		t.Fatal(err)
	}
	csrc, err := genProgram(fn, shared)
	if err != nil {
		t.Fatal(err)
	}
	bin := buildBinary(t, csrc)
	for _, file := range exampleScripts(t) {
		name := strings.TrimSuffix(filepath.Base(file), ".bur")
		t.Run(name, func(t *testing.T) {
			srcBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			vmOut, cerr, rerr := interpret(string(srcBytes))
			if cerr != nil || rerr != nil {
				t.Fatalf("Go VM failed: compile=%v runtime=%v", cerr, rerr)
			}
			var out bytes.Buffer
			cmd := exec.Command(bin, "run", file)
			cmd.Stdout = &out
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("native burc run failed: %v", err)
			}
			if got := out.String(); scrubTimings(got) != scrubTimings(vmOut) {
				t.Errorf("stdout mismatch\n--- native burc run ---\n%s\n--- Go VM ---\n%s", got, vmOut)
			}
		})
	}
}
