# Burryn

> **The ring beneath Meyrin.** A burrow, a ring — quiet work underground.

Burryn is a small programming language forged from Go and Rust: hand-written
lexer, recursive-descent parser, Hindley-Milner type inference with zero
annotations, single-pass bytecode compiler, a stack-based VM with its own
mark-sweep garbage collector and green-thread scheduler, and a C backend that
turns programs into standalone native binaries.

**The compiler is self-hosted.** `burc/` reimplements the whole pipeline —
lexer, parser, type checker, bytecode compiler, C code generator — in Burryn
itself (~7000 lines). It compiles itself to C byte-identical to the Go
toolchain's output, and the resulting native `burc` compiles the same source
to the same bytes again: a closed bootstrap fixpoint, checked in CI
(`TestBurcSelfHost`). The Go implementation (~11k lines) stays as the
reference oracle.

The name: a **burrow** is where a gopher (Go) lives, and it puns on Rust's
*borrow* checker; a **burr** is what forging leaves on metal; and *burrin*
reads like **burin** — the engraver's tool for precise, quiet work.

```sh
bur run examples/sieve.bur
```

New to the language? Start with the tutorial: [`docs/tutorial.md`](docs/tutorial.md).

## What it takes from Rust

- **Immutable by default.** `let x = 1` cannot be reassigned — enforced at
  *compile time*. Mutation requires `let mut`, and it runs deep: a plain
  `let` freezes list contents too — `push`, `pop` and `l[i] = v` all demand
  a `mut` binding.
- **No null.** Absence is `Option` (`Some(v)` / `None`), failure is `Result`
  (`Ok(v)` / `Err(e)`). Both are built-in enums.
- **The `?` operator.** Unwraps `Ok`/`Some`, or returns the `Err`/`None` to
  the caller immediately.
- **`match` expressions** with enum destructuring, literal arms, bindings and
  `_`, usable anywhere an expression fits.
- **Expression orientation.** `if`, `match` and blocks `{}` have values; a
  function returns its last expression.
- **Shadowing.** `let x = x + 1` rebinds, Rust style.
- **Algebraic data types.** `enum Shape { Circle(r), Rect(w, h), Point }`.

## What it takes from Go

- **A garbage collector instead of a borrow checker.** Hand-written
  mark-sweep over the VM's own heap; inspect it with `gc()`,
  `heap_objects()`, `gc_cycles()`.
- **Green threads.** `spawn worker(ch)` starts a fiber on the VM's
  scheduler — cooperative, plus a 10k-instruction time slice so a spinning
  fiber cannot starve the rest. Single-threaded interleaving means **no data
  races by construction**.
- **Channels, with Go's arrow.** `ch <- v` sends, `<-ch` receives.
  Unbuffered channels rendezvous; `chan(n)` buffers. All-fibers-blocked is
  detected and reported as a deadlock. The program exits when the main fiber
  returns.
- **No semicolons.** Newlines end statements (Go-style automatic insertion),
  so `} else` belongs on one line.

## Tour

```rust
enum Shape { Circle(r), Rect(w, h) }

fn area(s) {
    match s {
        Circle(r) => 3.14159 * r * r,
        Rect(w, h) => to_float(w * h),
    }
}

fn safe_div(a, b) {
    if b == 0 { return Err("division by zero") }
    Ok(a / b)
}

fn ratio(a, b) {
    let x = safe_div(a, b)?     // ? propagates the Err
    Ok(x * 100)
}

// closures capture by reference
fn make_counter() {
    let mut n = 0
    fn() { n = n + 1
           n }
}

// fibers and channels
fn producer(ch) {
    for i in range(0, 5) { ch <- i * i }
}
let ch = chan(2)
spawn producer(ch)
let mut sum = 0
for _i in range(0, 5) { sum = sum + <-ch }
```

## Examples

| file | shows |
|------|-------|
| `examples/hello.bur` | basics, closures, loops |
| `examples/shapes.bur` | enums + match |
| `examples/errors.bur` | Result/Option and `?` |
| `examples/sieve.bur` | the classic CSP prime sieve: one fiber per prime |
| `examples/pipeline.bur` | buffered-channel producer/consumer |
| `examples/gc_stress.bur` | watch the collector work |
| `examples/fib.bur` | recursion micro-benchmark |
| `examples/brainfuck.bur` | a Brainfuck interpreter written in Burryn |
| `examples/multiplex.bur` | `select` over several channels |
| `examples/streaming.bur` | channel close / for-in draining |
| `examples/textproc.bur` | files, exec, argv: a small text tool |
| `examples/wordcount.bur` | maps and string functions |
| `examples/geometry/` | a multi-package module (`bur.mod`, `import`, `pub`) |
| `burc/` | the self-hosted compiler — the biggest Burryn program there is |

## Architecture

```markdown
source ──lexer──▶ tokens ──parser──▶ AST ──checker──▶ typed ──compiler──▶ bytecode
 (lexer.go)        (auto-semicolons)  (parser.go)  (types.go, HM)   (compiler.go)
                                                                        │
                                        ┌───────────────────────────────┤
                                        ▼                               ▼
                                  BurrynVM (vm.go)            C backend (cbackend.go)
                                  fibers + channels           ──▶ .c ──cc──▶ native
                                  scheduler, GC (gc.go)       runtime/burrt*.h

burc/ mirrors the same pipeline in Burryn (token/lexer/parser/types/compiler/
cgen/module/vm .bur): it reproduces the C output byte for byte and interprets
the same bytecode on its own VM (`burc run <file>` / `burc run-dir <dir>`),
matching the Go VM's output byte for byte — sequential and concurrent alike.
```

- **Compiler**: single pass, clox-style locals/upvalues, with a
  temp-tracking scheme that lets `match`/block expressions (which declare
  locals) appear at any expression depth.
- **Closures**: upvalues reference `(fiber, slot)` while open and are closed
  on scope exit — safe across fiber switches and stack growth.
- **Match**: compiles to variant tests (`TEST_VARIANT` on enum identity +
  tag) and field extraction, no hashing, no reflection.
- **GC**: every language object lives in an intrusive list; roots are
  globals, every fiber's stack/frames/open upvalues/pending channel sends.
- **Scheduler**: FIFO ready queue; fibers park on channel wait queues; the
  receiver/sender hands values across directly.

## Commands

```sh
bur run <file|dir>    typecheck and run on the VM
bur <file|dir>        same
bur check <file|dir>  typecheck only (rustc-style diagnostics)
bur build <file|dir>  compile to a native binary via C (needs cc/gcc/clang)
bur dis <file|dir>    disassemble the compiled bytecode
bur version
```

Build: `go build -o bur.exe .` &nbsp;•&nbsp; Test: `go test .` (includes a
golden test for every example)

## Honest limitations

The v3 milestone (C backend, modules, maps, `select`/`close`, `mut`
parameters, a fully self-hosted compiler) is done; both backends produce
byte-identical output for the whole language, concurrency included. Still
missing: records/structs (model product types with single-variant enums),
`defer`, string interpolation, and third-party dependency fetching (only
local packages resolve). The self-hosted `burc` handles single, import-free
packages; multi-package builds still go through the Go toolchain. Deep `mut`
is a binding-level discipline, not a borrow checker: two `mut` bindings may
still alias the same list.
