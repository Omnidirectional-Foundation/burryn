English | [中文](README.zh-CN.md)

# Burryn

[![License](https://img.shields.io/badge/license-Apache--2.0-lightgrey?style=flat-square)](LICENSE)
[![Bootstrapping](https://img.shields.io/badge/Bootstrapping--4a4a4a?style=flat-square)](docs/SPEC.md)

> A burrow, a ring — quiet work underground.

Burryn is a small programming language forged from Go and Rust — a practical tool language for ops scripts, CSP-style concurrent pipelines, and small tools with static guarantees.
It has a hand-written lexer, recursive-descent parser, Hindley-Milner type inference (zero annotations by default, optional signature annotations when you want them), a single-pass bytecode compiler, a stack-based VM with its own mark-sweep garbage collector and green-thread scheduler, a portable C backend that turns programs into standalone native binaries, and an in-progress hand-written x86-64 backend.

**The compiler is self-hosted.**
`compiler/` reimplements the whole pipeline — lexer, parser, type checker, bytecode compiler, C code generator, VM, module loader, tooling, and the `bur` CLI — in Burryn itself.
`bur` compiles itself to C, `cc` turns that into a native binary, and the resulting native `bur` compiles the same source to the same bytes again.
A closed bootstrap fixpoint.
The original Go implementation that seeded the bootstrap is archived on the `archive/go-host` branch.

## Name

The name: a **burrow** is where a gopher (Go) lives, and it puns on Rust's *borrow* checker.
A **burr** is what forging leaves on metal.
*Burrin* reads like **burin** — the engraver's tool for precise, quiet work.

## What It Takes from Rust

- **Immutable by default.**
  `let x = 1` cannot be reassigned — enforced at *compile time*.
  Mutation requires `let mut`.
  It runs deep: a plain `let` freezes list contents too — `push`, `pop` and `l[i] = v` all demand a `mut` binding.
- **No null.**
  Absence is `Option` (`Some(v)` / `None`), failure is `Result` (`Ok(v)` / `Err(e)`).
  Both are built-in enums.
- **The `?` operator.**
  Unwraps `Ok`/`Some`, or returns the `Err`/`None` to the caller immediately.
- **`match` expressions** with enum destructuring, literal arms, bindings and `_`, usable anywhere an expression fits.
- **Expression orientation.**
  `if`, `match` and blocks `{}` have values.
  A function returns its last expression.
- **Shadowing.**
  `let x = x + 1` rebinds, Rust style.
- **Algebraic data types.**
  `enum Shape { Circle(r), Rect(w, h), Point }`.

## What It Takes from Go

- **A garbage collector instead of a borrow checker.**
  Hand-written mark-sweep over the VM's own heap.
  Inspect it with `gc()`, `heap_objects()`, `gc_cycles()`.
- **Green threads.**
  `spawn worker(ch)` starts a fiber on the VM's scheduler — cooperative, plus a 10k-instruction time slice so a spinning fiber cannot starve the rest.
  Single-threaded interleaving means **no data races by construction**.
- **Channels, with Go's arrow.**
  `ch <- v` sends, `<-ch` receives.
  Unbuffered channels rendezvous; `chan(n)` buffers.
  All-fibers-blocked is detected and reported as a deadlock.
  The program exits when the main fiber returns.
- **No semicolons.**
  Newlines end statements (Go-style automatic insertion), so `} else` belongs on one line.

## Tour

```sh
bur run examples/concurrency/sieve.bur
```

```text
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

| file | description |
| ------ | ------------- |
| `examples/basics/hello.bur` | basics, closures, loops |
| `examples/basics/fib.bur` | recursion micro-benchmark |
| `examples/basics/textproc.bur` | string and list natives: split, trim, join, slice, concat |
| `examples/basics/interpolation.bur` | string interpolation with `{expr}` |
| `examples/basics/cleanup.bur` | defer: LIFO cleanup, closure capture |
| `examples/types/shapes.bur` | enums + match |
| `examples/types/errors.bur` | Result/Option and `?` |
| `examples/types/constants.bur` | compile-time const folding |
| `examples/types/match_guard.bur` | match arms with `if` guards |
| `examples/types/pipeline_op.bur` | the `\|>` pipe operator |
| `examples/concurrency/sieve.bur` | the classic CSP prime sieve: one fiber per prime |
| `examples/concurrency/pipeline.bur` | buffered-channel producer/consumer |
| `examples/concurrency/multiplex.bur` | `select` over several channels |
| `examples/concurrency/streaming.bur` | channel close + for-in drain |
| `examples/net/net_loopback.bur` | TCP listener + dialer echo exchange |
| `examples/io/fs.bur` | read_file/write_file round-trip and error paths |
| `examples/io/exec.bur` | synchronous exec: Output, exit codes |
| `examples/programs/brainfuck.bur` | a Brainfuck interpreter written in Burryn |
| `examples/programs/wordcount.bur` | maps and string functions |
| `examples/programs/gc_stress.bur` | watch the collector work |
| `examples/programs/geometry/` | a multi-package module (`bur.mod`, `import`, `pub`) |
| `compiler/` | the self-hosted compiler and `bur` CLI — the biggest Burryn program there is |

## Architecture

```text
source --lexer--> tokens --parser--> AST --checker--> typed --compiler--> bytecode
 (lexer.bur)        (auto-semicolons)  (parser.bur) (types.bur, HM)   (compiler.bur)
||
                                         +-------------------+-------------------+
                                         |                   |
                                         v                   v
                                   BurrynVM            Native backends
                                   (vm.bur)            - C backend (cgen.bur)
                                   fibers + GC         - x86-64 ELF (x86.bur + elf.bur)

Burryn is self-hosted: the whole pipeline lives under `compiler/`
(`frontend/`, `bytecode/`, `backends/`, `module/`, `tooling/`), and the `bur`
CLI is `compiler/main.bur`. `bur` compiles itself to C, `cc` turns that into a
native binary, and the result rebuilds itself byte for byte.
```

- **Compiler**: single pass, clox-style locals/upvalues, with a temp-tracking scheme that lets `match`/block expressions (which declare locals) appear at any expression depth.
- **Closures**: upvalues reference `(fiber, slot)` while open and are closed on scope exit — safe across fiber switches and stack growth.
- **Match**: compiles to variant tests (`TEST_VARIANT` on enum identity + tag) and field extraction, no hashing, no reflection.
- **GC**: every language object lives in an intrusive list; roots are globals, every fiber's stack/frames/open upvalues/pending channel sends.
- **Scheduler**: FIFO ready queue; fibers park on channel wait queues; the receiver/sender hands values across directly.

## Commands

```sh
bur run <file|dir>    typecheck and run on the VM
bur <file|dir>        same
bur check <file|dir>  typecheck only (rustc-style diagnostics)
bur build <file|dir>  compile to a native binary via C (needs cc/gcc/clang)
bur build --backend x86 <file.bur> -o <out>  compile to a native x86-64 ELF binary
bur fmt <file|dir|->  rewrite source in the official format
bur dis <file|dir>    disassemble the compiled bytecode
bur test [dir]        run test_* functions from *_test.bur files
bur mod <subcommand>  module management (init, tidy, download, verify)
bur get <path>@<ver>  add or upgrade a module dependency
bur version           print the version
```

Build (self-hosting): a native `bur` builds itself with `bur build compiler -o bur`.
To bootstrap from scratch, use the archived Go host plus the frozen `seed-base-1` bridge:

```sh
git worktree add ../go-host archive/go-host
(cd ../go-host && go build -o ../bur-seed .)
git worktree add ../seed-base seed-base-1
(cd ../seed-base && ../bur-seed build burc -o ../bur-base)
./bur-base build compiler -o bur
```

> **Note:** Bootstrapping from the archived Go host requires **Go 1.26+**.
> The C backend and `bur build` require a C99 compiler such as `gcc` or `clang`.

## Security

Burryn is a self-hosted compiler and runtime.
Security-sensitive surfaces include:

- **Compiler and emitted C**: `bur build` writes `program.c` and invokes the system C compiler.
  Audit generated C when running untrusted sources.
- **Process spawning**: the `exec` native calls `fork` + `execvp` without a shell.
- **Persistent state**: the toolchain writes build artifacts (`program.c`, the output binary, and temporary files) to the working directory.
- **Network endpoints**: `bur mod download` and `bur get` fetch dependencies with `git clone` over the network.
- **Trust boundary**: treat compiled native binaries and `.bur` source from untrusted origins as untrusted code.

See [`SECURITY.md`](SECURITY.md) for the full policy and private reporting instructions.

## Honest Limitations

Stages S1-S7 are done.
They cover the semantic core, the portable C backend, modules, maps, `select`/`close`, dependency tooling, diagnostics, string interpolation, the `|>` pipe operator, match guards, compile-time constants (`const`), `defer`, TCP networking, and a fully self-hosted compiler with the Go host removed.

Current work is in S8 and S9.
The syntax is frozen (S8.2): the formal grammar lives in `docs/grammar.md`. The freeze covers `::` vs `.` (paths vs members), compound assignment, indexed `for` loops, or-patterns, labeled loops, record patterns, receiver methods, tuples with destructuring, hex/binary/exponent literals, multiline strings, and `..` ranges — syntax changes now go through the grammar revision process.
The hand-written x86-64 ELF backend is in progress on Linux and currently trails the C backend in feature coverage.
Row polymorphism, closed records, type aliases, and the remaining editor features are still in progress.
Deep `mut` is a binding-level discipline, not a borrow checker: two `mut` bindings may still alias the same list.

## Documentation

| Document | Purpose |
| ----------------- | ---------------- |
| [`tutorial.md`](tutorial.md) | users — a hands-on tour of the language ([中文](tutorial.zh-CN.md)) |
| [`docs/grammar.md`](docs/grammar.md) | the frozen formal grammar (S8.2) — lexical and syntax reference |
| [`docs/SPEC.md`](docs/SPEC.md) | design authority — the language design, roadmap, and staged milestones (S1-S10) |
| [`docs/NUMBERING.md`](docs/NUMBERING.md) | contributors — historical map from old `v`/`L` labels to the unified `S` scheme |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | contributors — branching, commit rules, and the bootstrap-fixpoint requirement |
| [`SECURITY.md`](SECURITY.md) | reporting vulnerabilities privately |
| [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | community conduct standards |
| [`CHANGELOG.md`](CHANGELOG.md) | notable changes, latest first |
| [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) | PR template |

## License & Disclaimer

This project is licensed under the [Apache License 2.0](LICENSE).

This project is developed and maintained by individual contributors on a voluntary, non-commercial basis.

This software is provided **"as is"**, without warranty of any kind.
The author(s) accept no liability for any damages arising from the use of this software.
See the [LICENSE](LICENSE) file for the full terms, including the disclaimer of warranty and limitation of liability.

Any commercial entity using this software is solely responsible for its own compliance with applicable laws and regulations, including but not limited to the EU Cyber Resilience Act (CRA) and any other regional requirements.
