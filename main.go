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
		os.Exit(64)
	}
	dyn := false
	var rest []string
	for _, a := range args {
		if a == "--dyn" {
			dyn = true
		} else {
			rest = append(rest, a)
		}
	}
	args = rest
	if len(args) == 0 {
		usage()
		os.Exit(64)
	}
	switch args[0] {
	case "version", "-v", "--version":
		fmt.Printf("Burryn %s — the ring beneath Meyrin\n", version)
	case "run":
		if len(args) < 2 {
			usage()
			os.Exit(64)
		}
		runFile(args[1], modeRun, dyn)
	case "check":
		if len(args) < 2 {
			usage()
			os.Exit(64)
		}
		runFile(args[1], modeCheck, false)
	case "dis":
		if len(args) < 2 {
			usage()
			os.Exit(64)
		}
		runFile(args[1], modeDis, dyn)
	case "help", "-h", "--help":
		usage()
	default:
		runFile(args[0], modeRun, dyn)
	}
}

const (
	modeRun = iota
	modeCheck
	modeDis
)

func usage() {
	fmt.Println(`Burryn ` + version + ` — a small language forged from Go and Rust

usage:
  bur run <file.bur>     typecheck and run a program
  bur <file.bur>         same as run
  bur check <file.bur>   typecheck only (rustc-style diagnostics)
  bur dis <file.bur>     disassemble compiled bytecode
  bur version

flags:
  --dyn                  skip the type checker (v1 dynamic mode)`)
}

func runFile(path string, mode int, dyn bool) {
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(66)
	}
	src := string(srcBytes)
	toks, err := lex(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(65)
	}
	stmts, err := parse(toks)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(65)
	}

	if !dyn {
		diags := typecheck(stmts)
		errs, warns := renderDiags(os.Stderr, diags, path, src)
		if mode == modeCheck {
			if errs > 0 {
				fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s); %d warning(s)\n", errs, warns)
				os.Exit(65)
			}
			fmt.Fprintf(os.Stderr, "ok: 0 errors, %d warning(s)\n", warns)
			return
		}
		if errs > 0 {
			fmt.Fprintf(os.Stderr, "error: could not compile due to %d previous error(s)\n", errs)
			os.Exit(65)
		}
	} else if mode == modeCheck {
		fmt.Fprintln(os.Stderr, "check does not support --dyn")
		os.Exit(64)
	}

	gc := newGC()
	fn, shared, err := compileProgram(gc, stmts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(65)
	}
	if mode == modeDis {
		disasmAll(fn)
		return
	}
	vm := newVM(gc, shared)
	if err := vm.run(fn); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(70)
	}
}
