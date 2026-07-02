# Burryn

> **The ring beneath Meyrin.** A burrow, a ring — quiet work underground.

Burryn is a small programming language forged from Go and Rust, implemented
from scratch in ~3000 lines of Go: hand-written lexer, recursive-descent
parser, single-pass bytecode compiler, and a stack-based VM with its own
mark-sweep garbage collector and a green-thread scheduler.

The name: a **burrow** is where a gopher (Go) lives, and it puns on Rust's
*borrow* checker; a **burr** is what forging leaves on metal; and *burrin*
reads like **burin** — the engraver's tool for precise, quiet work.

```
bur run examples/sieve.bur
```

## What it takes from Rust

- **Immutable by default.** `let x = 1` cannot be reassigned — enforced at
  *compile time*. Mutation requires `let mut`.
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
        Rect(w, h) => float(w * h),
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

## Architecture

```
source ──lexer──▶ tokens ──parser──▶ AST ──compiler──▶ bytecode ──▶ BurrynVM
 (lexer.go)        (auto-semicolons)  (parser.go)      (compiler.go)   (vm.go)
                                                                        │
                                              fibers + channels + scheduler
                                              mark-sweep GC (gc.go)
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

```
bur run <file.bur>    run
bur <file.bur>        same
bur dis <file.bur>    disassemble the compiled bytecode
bur version
```

Build: `go build -o bur.exe .` &nbsp;•&nbsp; Test: `go test .` (48 tests)

## Honest limitations (v1)

Dynamically typed — the type system is the price paid for fitting in a day.
The v2 plan is Hindley-Milner type inference (full static checking with zero
annotations), which also unlocks `match` exhaustiveness checking. No maps,
no string interpolation, no modules yet. `mut` governs rebinding; list
contents are freely mutable through any reference.
