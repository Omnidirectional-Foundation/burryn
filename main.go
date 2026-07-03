package main

import (
	"fmt"
	"os"
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
		runFile(args[1], modeRun)
	case "check":
		if len(args) < 2 {
			usage()
			os.Exit(exitUsage)
		}
		runFile(args[1], modeCheck)
	case "dis":
		if len(args) < 2 {
			usage()
			os.Exit(exitUsage)
		}
		runFile(args[1], modeDis)
	case "help", "-h", "--help":
		usage()
	default:
		runFile(args[0], modeRun)
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
  bur run <file.bur>     typecheck and run a program
  bur <file.bur>         same as run
  bur check <file.bur>   typecheck only (rustc-style diagnostics)
  bur dis <file.bur>     disassemble compiled bytecode
  bur version`)
}

func runFile(path string, mode int) {
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitNoInput)
	}
	src := string(srcBytes)
	toks, lexDiags := lex(src)
	if len(lexDiags) > 0 {
		errs, _ := renderDiags(os.Stderr, lexDiags, path, src)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	stmts, parseDiags := parse(toks)
	if len(parseDiags) > 0 {
		errs, _ := renderDiags(os.Stderr, parseDiags, path, src)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}

	diags := typecheck(stmts)
	errs, warns := renderDiags(os.Stderr, diags, path, src)
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
		errs, _ := renderDiags(os.Stderr, compDiags, path, src)
		fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
		os.Exit(exitStatic)
	}
	if mode == modeDis {
		disasmAll(fn, shared.lines)
		return
	}
	vm := newVM(gc, shared)
	if err := vm.run(fn); err != nil {
		if re, ok := err.(*runtimeErr); ok {
			renderDiags(os.Stderr, []Diag{{IsErr: true, Msg: re.msg, Span: re.span}}, path, src)
		} else {
			fmt.Fprintln(os.Stderr, err) // deadlock etc.: whole-program, no span
		}
		os.Exit(exitRuntime)
	}
}
