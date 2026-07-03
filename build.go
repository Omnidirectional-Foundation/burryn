package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// findCC resolves the C compiler to drive: honor $CC, then look for the usual
// names. Returns "" when none is available.
func findCC() string {
	if cc := strings.TrimSpace(os.Getenv("CC")); cc != "" {
		return cc
	}
	for _, name := range []string{"cc", "gcc", "clang"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// buildFile compiles a source file or package directory through the front end
// and the C backend. With emitC it writes the generated C (to out, or stdout
// when out is ""); otherwise it invokes the C compiler to produce a binary.
func buildFile(path string, emitC bool, out string) {
	fn, shared := compileForBuild(path)
	csrc, err := genProgram(fn, shared)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitStatic)
	}

	if emitC {
		if out == "" || out == "-" {
			fmt.Print(csrc)
			return
		}
		if err := os.WriteFile(out, []byte(csrc), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(exitNoInput)
		}
		return
	}

	if out == "" {
		out = defaultOutName(path)
	}
	cc := findCC()
	if cc == "" {
		fmt.Fprintln(os.Stderr, "error: no C compiler found (set $CC or install cc/gcc/clang)")
		fmt.Fprintln(os.Stderr, "hint: run without a toolchain using `bur run`")
		os.Exit(exitStatic)
	}
	if err := compileC(cc, csrc, out); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitStatic)
	}
}

// defaultOutName derives the binary name from the input path's base name.
func defaultOutName(path string) string {
	base := filepath.Base(strings.TrimRight(path, string(os.PathSeparator)))
	return strings.TrimSuffix(base, ".bur")
}

// compileC writes the runtime headers and generated program into a temporary
// directory and runs the C compiler, placing the binary at out.
func compileC(cc, csrc, out string) error {
	dir, err := os.MkdirTemp("", "burc-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	for name, src := range runtimeSources() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			return err
		}
	}
	prog := filepath.Join(dir, "program.c")
	if err := os.WriteFile(prog, []byte(csrc), 0o644); err != nil {
		return err
	}

	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	cmd := exec.Command(cc, "-O2", "-o", absOut, "program.c", "-lm")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("C compiler failed: %v", err)
	}
	return nil
}

// compileForBuild runs the front end on a file or package directory and
// returns the entry function and shared program state, exiting with the
// appropriate code on any diagnostic error (mirroring runFile/runModule).
func compileForBuild(path string) (*OFunc, *Shared) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return compileModuleForBuild(path)
	}
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitNoInput)
	}
	src := string(srcBytes)
	srcs := map[string]string{path: src}
	render := func(diags []Diag) (errs, warns int) {
		stampFile(diags, path)
		return renderDiags(os.Stderr, diags, srcs)
	}
	toks, lexDiags := lex(src)
	if len(lexDiags) > 0 {
		errs, _ := render(lexDiags)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	stmts, parseDiags := parse(toks)
	if len(parseDiags) > 0 {
		errs, _ := render(parseDiags)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	if diags := typecheck(stmts); len(diags) > 0 {
		if errs, _ := render(diags); errs > 0 {
			fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
			os.Exit(exitStatic)
		}
	}
	gc := newGC()
	fn, shared, compDiags := compileProgram(gc, src, stmts)
	if len(compDiags) > 0 {
		errs, _ := render(compDiags)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	return fn, shared
}

func compileModuleForBuild(dir string) (*OFunc, *Shared) {
	m, loadDiags := loadModule(dir)
	if loadErrs, _ := renderDiags(os.Stderr, loadDiags, m.Srcs); loadErrs > 0 {
		if m.Path == "" {
			os.Exit(exitNoInput)
		}
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", loadErrs)
		os.Exit(exitStatic)
	}
	if diags := typecheckModule(m); len(diags) > 0 {
		if errs, _ := renderDiags(os.Stderr, diags, m.Srcs); errs > 0 {
			fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
			os.Exit(exitStatic)
		}
	}
	gc := newGC()
	fn, shared, compDiags := compileModule(gc, m)
	if len(compDiags) > 0 {
		errs, _ := renderDiags(os.Stderr, compDiags, m.Srcs)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	return fn, shared
}
