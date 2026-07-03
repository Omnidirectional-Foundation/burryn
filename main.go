package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "0.2.0"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(exitUsage)
	}
	switch args[0] {
	case "version", "-v", "--version":
		fmt.Printf("Burryn %s — the ring beneath Meyrin\n", version)
	case "run":
		if len(args) < 2 {
			usage()
			os.Exit(exitUsage)
		}
		runFile(args[1], modeRun, args[2:])
	case "check":
		if len(args) < 2 {
			usage()
			os.Exit(exitUsage)
		}
		runFile(args[1], modeCheck, nil)
	case "dis":
		if len(args) < 2 {
			usage()
			os.Exit(exitUsage)
		}
		runFile(args[1], modeDis, nil)
	case "build":
		buildCmd(args[1:])
	case "help", "-h", "--help":
		usage()
	default:
		runFile(args[0], modeRun, args[1:])
	}
}

const (
	modeRun = iota
	modeCheck
	modeDis
)

// exit codes: sequential, one per failure stage
const (
	exitStatic  = 1 // lex/parse/type/compile error
	exitUsage   = 2 // CLI misuse
	exitNoInput = 3 // source file unreadable
	exitRuntime = 4 // runtime trap
)

func usage() {
	fmt.Println(`Burryn ` + version + ` — a small language forged from Go and Rust

usage:
  bur run <file.bur>     typecheck and run a script
  bur run <dir>          run a module package (needs bur.mod, fn main)
  bur <file.bur|dir>     same as run
  bur check <file|dir>   typecheck only (rustc-style diagnostics)
  bur build <file|dir>   compile to a native binary via C
  bur dis <file|dir>     disassemble compiled bytecode
  bur version

bur build flags:
  -o <path>              output path (binary, or C file with --emit c)
  --emit c               emit generated C instead of a binary`)
}

// buildCmd parses the `bur build` flags and drives the C backend.
func buildCmd(args []string) {
	var input, out string
	emitC := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-o":
			if i+1 >= len(args) {
				usage()
				os.Exit(exitUsage)
			}
			i++
			out = args[i]
		case a == "--emit":
			if i+1 >= len(args) || args[i+1] != "c" {
				usage()
				os.Exit(exitUsage)
			}
			i++
			emitC = true
		case strings.HasPrefix(a, "-"):
			usage()
			os.Exit(exitUsage)
		default:
			if input != "" {
				usage()
				os.Exit(exitUsage)
			}
			input = a
		}
	}
	if input == "" {
		usage()
		os.Exit(exitUsage)
	}
	buildFile(input, emitC, out)
}

func runFile(path string, mode int, argv []string) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		runModule(path, mode, argv)
		return
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

	diags := typecheck(stmts)
	errs, warns := render(diags)
	if mode == modeCheck {
		if errs > 0 {
			fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s); %d warning(s)\n", errs, warns)
			os.Exit(exitStatic)
		}
		fmt.Fprintf(os.Stderr, "ok: 0 errors, %d warning(s)\n", warns)
		return
	}
	if errs > 0 {
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}

	gc := newGC()
	fn, shared, compDiags := compileProgram(gc, src, stmts)
	if len(compDiags) > 0 {
		errs, _ := render(compDiags)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	if mode == modeDis {
		disasmAll(fn, shared)
		return
	}
	vm := newVM(gc, shared)
	vm.args = argv
	if err := vm.run(fn); err != nil {
		if er, ok := err.(*exitRequest); ok {
			os.Exit(er.code)
		}
		if re, ok := err.(*runtimeErr); ok {
			render([]Diag{{IsErr: true, Msg: re.msg, Span: re.span}})
		} else {
			fmt.Fprintln(os.Stderr, err) // deadlock etc.: whole-program, no span
		}
		os.Exit(exitRuntime)
	}
}

// runModule drives the module pipeline for a package directory argument.
func runModule(dir string, mode int, argv []string) {
	m, loadDiags := loadModule(dir)
	loadErrs, loadWarns := renderDiags(os.Stderr, loadDiags, m.Srcs)
	if loadErrs > 0 {
		if m.Path == "" { // no usable bur.mod: an input problem, not a compile error
			os.Exit(exitNoInput)
		}
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", loadErrs)
		os.Exit(exitStatic)
	}

	diags := typecheckModule(m)
	errs, warns := renderDiags(os.Stderr, diags, m.Srcs)
	warns += loadWarns
	if mode == modeCheck {
		if errs > 0 {
			fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s); %d warning(s)\n", errs, warns)
			os.Exit(exitStatic)
		}
		fmt.Fprintf(os.Stderr, "ok: 0 errors, %d warning(s)\n", warns)
		return
	}
	if errs > 0 {
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}

	gc := newGC()
	fn, shared, compDiags := compileModule(gc, m)
	if len(compDiags) > 0 {
		errs, _ := renderDiags(os.Stderr, compDiags, m.Srcs)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	if mode == modeDis {
		disasmAll(fn, shared)
		return
	}
	vm := newVM(gc, shared)
	vm.args = argv
	if err := vm.run(fn); err != nil {
		if er, ok := err.(*exitRequest); ok {
			os.Exit(er.code)
		}
		if re, ok := err.(*runtimeErr); ok {
			renderDiags(os.Stderr, []Diag{{IsErr: true, Msg: re.msg, File: re.file, Span: re.span}}, m.Srcs)
		} else {
			fmt.Fprintln(os.Stderr, err) // deadlock etc.: whole-program, no span
		}
		os.Exit(exitRuntime)
	}
}
