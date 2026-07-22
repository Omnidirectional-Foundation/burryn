English | [中文](tutorial.zh-CN.md)

# The Burryn Tutorial

> See also: [`README.md`](README.md) project overview · [`docs/GOALS.md`](docs/GOALS.md) design authority and milestones

Burryn is a small, statically-inferred, zero-annotation language with CSP
concurrency: Rust-style types and `match`, Go-style concurrency and
simplicity, shipped as a single binary. This tutorial uses only features that
work **today**, every snippet checked against the compiler.

---

## 0. Running and building

Save code as a `.bur` file and run it on the VM — **no C toolchain needed**:

```sh
bur run hello.bur       # typecheck and run
bur hello.bur           # same thing
bur check hello.bur     # typecheck only
```

For a standalone **native binary**, use `bur build` (this step needs a system
`cc`/`gcc`/`clang`):

```sh
bur build hello.bur -o hello   # compile to a native binary via C
./hello                        # runs on its own
bur build hello.bur --emit c   # dump the generated C instead
```

`bur run` is always dependency-free; `cc` is needed only at `bur build` time,
and the resulting binary runs anywhere.

---

## 1. Hello, Burryn

```rust
// A comment. Statements end at newlines — no semicolons (Go-style).
let name = "Burryn"
println("hello from", name)
```

`println` takes any number of arguments, prints them space-separated with a
trailing newline; `print` omits the newline. A script may put statements at the
top level and runs them top to bottom.

Because a newline ends a statement, `} else {` must sit on one line (like Go).

---

## 2. Variables: let, mut, shadowing

A `let` binding is **immutable by default**; reassigning is a compile error.
Use `let mut` for mutability.

```rust
let x = 10
// x = 11            // compile error
let mut y = 10
y = y + 1            // ok

// Shadowing: a same-name let rebinds (Rust style)
let x = x * 2        // a new x = 20
```

**Deep `mut`**: a plain `let` freezes not just the binding but the contents of
the container it points to. `push`, `l[i] = v` and `pop` all require the list to
be bound with `let mut`.

```rust
let frozen = [1, 2, 3]
// push(frozen, 4)   // compile error
let mut xs = [1, 2, 3]
push(xs, 4)          // ok -> [1, 2, 3, 4]
```

### Compile-time constants

A `const` declares a binding whose value is **folded at compile time** — zero
runtime cost. It works at any scope (top level or inside a function); use
`pub const` to export across packages.

```rust
const answer = 40 + 2            // folded to 42 at compile time
const greeting = "hello" + ", constants"

fn increment() {
    const next = answer + 1      // works inside functions too
    next
}
println(greeting)                        // hello, constants
println(str(answer), str(increment()))   // 42 43
```

The initializer may only reference literals, other `const` values, and pure
built-in operations — no ordinary function calls or `let` variables.

---

## 3. Primitive types

There are exactly two number types: `int` (i64) and `float` (f64). Plus `bool`,
`string`, and unit `()` ("no meaningful value"). **No implicit conversions,
ever.**

```rust
let a = 42            // int
let b = 3.14          // float
let c = true          // bool
let s = "text"        // string

let n = 7
// let bad = n + 1.5  // compile error: int and float don't mix
let ok = to_float(n) + 1.5   // convert explicitly
let m = trunc(3.9)           // float -> int (truncate) = 3
let text = str(n)            // any value -> string
```

**Integer overflow always traps** (runtime panic) — it never wraps silently;
integer divide/modulo by zero also traps. Deliberate: safe by default.

Strings are **UTF-8 byte sequences**; `len` and indexing count **bytes**. Use
`char_at`, `ord`, `chr` for code points (section 7).

### String interpolation

`{expr}` inside a string evaluates the expression and appends its result. The
expression must already be `str`; Burryn performs no implicit conversion, so
other types require an explicit `str()`. Write `{{` for a literal opening
brace.

```rust
let name = "Burryn"
let jobs = 3
println("hello {name}, jobs={str(jobs)}")
println("literal brace: {{")
```

---

## 4. Expression orientation

`if`, `match` and blocks `{}` are **expressions** — they have values. A function
returns its last expression.

```rust
let x = 10
let label = if x > 5 { "big" } else { "small" }   // if as a value

let y = {                 // a block has a value
    let t = x * x
    t + 1                 // value = last expression
}
```

An `if` used as a value must have an `else` (otherwise one branch has no value).

---

## 5. Functions and closures

Function parameters and return types are **unannotated** (types are inferred).
Use `return` to return early; otherwise the last expression is returned.

```rust
fn add(a, b) {
    a + b            // implicit return
}

fn classify(n) {
    if n < 0 { return "negative" }
    if n == 0 { return "zero" }
    "positive"
}
```

**Closures**: an anonymous function `fn() { ... }` captures variables from its
environment, by reference — a closure can read and write an enclosing `mut`.

```rust
fn make_counter() {
    let mut n = 0
    fn() {               // returns a closure
        n = n + 1
        n
    }
}
let tick = make_counter()
tick()                   // 1
tick()                   // 2
println(tick())          // 3
```

Parameters are immutable by default; to mutate a passed-in container in place,
declare `fn f(mut xs)`:

```rust
fn append_one(mut xs) {
    push(xs, 1)          // needs the mut parameter
}
```

### The pipe operator

`x |> f` feeds the left value as the first argument of the call on the right,
equivalent to `f(x)`; with arguments, `x |> f(a)` is `f(x, a)`. `|>` has the
lowest precedence and associates left, so `x |> f |> g` is `g(f(x))` and
`1 + 2 |> str` is `str(1 + 2)`. The right side only accepts a function name or
`pkg.name`, optionally with arguments; any other expression is a parse error.
`?` binds tighter, so error propagation is written `(x |> parse)?`.

```rust
fn double(x) { x * 2 }
fn clamp(x, hi) {
    if x > hi { hi } else { x }
}
let n = 3 |> double |> clamp(5)   // clamp(double(3), 5)
println(n)                        // 5
println(1 + 2 |> str)             // 3
```

### defer

`defer { ... }` attaches a block to the **enclosing function**; the blocks run
in reverse registration order (LIFO) when the function exits — via `return`,
the tail expression, or `?` propagation. The block is a closure: it captures
its environment by reference and sees each variable's final value at exit. A
trap aborts the process and runs no defers.

```rust
fn process(path) {
    let mut lines = 0
    defer {
        println("processed " + str(lines) + " line(s)")
    }
    let text = read_file(path)?
    lines = len(split(text, "\n"))
    Ok(lines)
}
```

`defer` works at script top level too — the whole script is one function, so
the blocks run when the script ends.

---

## 6. Lists and loops

Lists use `[...]` literals, `l[i]` to index (out-of-bounds traps), `l[i] = v` to
assign (needs `mut`).

```rust
let mut xs = [10, 20, 30]
println(xs[0])           // 10
xs[1] = 99               // [10, 99, 30]
push(xs, 40)             // [10, 99, 30, 40]
let last = pop(xs)       // 40, xs back to length 3
```

Three loops: `for x in list`, `for i in range(a, b)` (half-open `[a, b)`), and
`while`. `break` / `continue` are available.

```rust
let mut sum = 0
for i in range(1, 101) {         // 1..100
    sum = sum + i
}
println("1..100 =", sum)         // 5050

let words = ["forge", "burrow", "ring"]
for w in words {
    print(w + " ")
}
println()

let mut i = 0
while i < len(words) {
    i = i + 1
}
```

Common list functions: `len`, `push`, `pop`, `slice(xs, start, end)`,
`concat(a, b)`, `contains(xs, v)`, `range(a, b)`.

---

## 7. Strings

`+` concatenates strings; `<` `>` etc. compare bytewise. Strings are immutable
byte sequences.

```rust
let greeting = "hello, " + name
let parts = split("a,b,c", ",")      // ["a", "b", "c"]
let joined = join(parts, " | ")      // "a | b | c"
let sub = substr("burryn", 0, 3)     // "bur" (start, length)
let hit = str_contains("burryn", "rry")   // true
```

Cheat sheet: `len` (byte length), `str_len`, `char_at(s, i)`, `ord(s)` (first
code point -> int), `chr(n)` (code point -> string), `split`, `join`, `substr`,
`trim`, `str_contains`, `str_index_of` (-> `Option<int>`), `parse_int`/
`parse_float` (-> `Option`).

---

## 8. Maps

Maps use a **function API**, with no `m[k]` sugar (under zero-annotation
inference, `container[key]` can't tell a list from a map in an unannotated
parameter). Keys are `int` or `str`; iteration is in **insertion order**.

```rust
let mut counts = map()           // a fresh empty map
put(counts, "the", 3)
put(counts, "fox", 2)

match get(counts, "the") {       // get -> Option
    Some(n) => println("the:", n),
    None    => println("missing"),
}

for k in keys(counts) {          // keys -> list of keys
    println(k, "->", get(counts, k))
}
delete(counts, "fox")
println(len(counts))             // number of entries
```

`[]` is always list-only; use `char_at` on strings and `get` on maps.

---

## 9. Enums and match

Enums are algebraic data types with typed fields — the **only place you write
types**. Variants may have zero or more fields.

```rust
enum Shape {
    Circle(float),        // one float field
    Rect(int, int),       // two int fields
    Point,                // no fields (a singleton)
}

let shapes = [Circle(2.0), Rect(3, 4), Point]
```

`match` destructures by variant, supports literal arms, bindings and `_`, and
must be **exhaustive**. It is an expression, usable anywhere.

```rust
fn area(s) {
    match s {
        Circle(r)  => 3.14159 * r * r,
        Rect(w, h) => to_float(w * h),
        Point      => 0.0,
    }
}

// literals + binding + wildcard
let grade = 87
let letter = match grade / 10 {
    10 => "S",
    9  => "A",
    8  => "B",
    other => "F (" + str(other) + ")",   // binds the rest
}
```

A match arm may add an `if` guard after its pattern. The guard runs after
pattern binding, so it can use newly bound names, and its result must be
`bool`. A guarded arm does not count toward exhaustiveness because its guard
may reject the value, so an unguarded fallback is still required.

```rust
let jobs = Some(3)
let status = match jobs {
    Some(n) if n > 0 => "active: " + str(n),
    Some(_) => "idle",
    None => "missing",
}
println(status)
```

When a variant name is shared across enums, qualify it as `Enum.Variant`, e.g.
`Shape.Circle(r)`.

---

## 10. No null: Option, Result, and ?

**There is no null.** A possibly-absent value is the built-in `Option`
(`Some(v)` / `None`); a possibly-failing one is `Result` (`Ok(v)` / `Err(e)`).
You are forced to handle both cases with `match`.

```rust
fn find(xs, want) {
    let mut i = 0
    while i < len(xs) {
        if xs[i] == want { return Some(i) }
        i = i + 1
    }
    None
}
```

The `?` operator unwraps `Ok`/`Some`, or **immediately returns** the `Err`/`None`
to the caller — that is error propagation, with no exceptions.

```rust
fn safe_div(a, b) {
    if b == 0 { return Err("division by zero") }
    Ok(a / b)
}

fn average(a, b, c, d) {
    let x = safe_div(a, b)?      // short-circuits on Err
    let y = safe_div(c, d)?
    Ok((x + y) / 2)
}

println(average(10, 2, 30, 3))   // Ok(7)
println(average(10, 0, 30, 3))   // Err("division by zero")
```

Stdlib convention: filesystem/process calls return `Result<T, str>`, failing
with `Err(message)`. `exec` uses the built-in `Output(int, str, str)` enum to
carry exit code, stdout and stderr.

```rust
match read_file("notes.txt") {
    Ok(text) => println("read", len(text), "bytes"),
    Err(msg) => println("failed:", msg),
}
```

---

## 11. Concurrency: spawn, channel, select

CSP model: `spawn` starts a **fiber** (green thread); fibers talk over
**channels**. Execution is **always single-threaded** — cooperative scheduling
plus a 10k-instruction time slice — so single-threaded interleaving means **no
data races by construction**.

```rust
fn producer(ch) {
    for i in range(0, 5) {
        ch <- i * i          // send
    }
    close(ch)                // signal end-of-stream
}

let ch = chan(2)             // buffered channel (cap 2); chan(0)/chan() rendezvous
spawn producer(ch)           // start a fiber

for v in ch {                // for-in ends when the channel closes and drains
    println("got", v)
}
```

`ch <- v` sends, `<-ch` receives (may block). `recv(ch)` returns an `Option`:
`Some(v)` when a value is available, `None` once closed and drained. Sending on a
closed channel, or closing twice, traps. When all fibers are blocked it is
reported as a **deadlock**. The program ends when the main fiber returns (Go
semantics).

`select` picks one ready channel operation; it takes the first ready arm in
declaration order, with an optional `default` to make it non-blocking.

```rust
select {
    x = <-a => { println("from a:", x) },   // receive arm
    y = <-b => { println("from b:", y) },
    out <- 1 => { println("sent to out") }, // send arm
    default => { println("nothing ready") },// at most one
}
```

> Concurrency behaves identically on both paths: `bur run` (the VM) and
> `bur build` (a native binary via C) produce byte-identical output for the
> same program, fibers and channels included — enforced by tests.

---

## 12. Networking

Burryn has built-in TCP support: six natives provide non-blocking TCP handle
management, and the scheduler automatically parks/wakes fibers waiting on
network IO — as natural as channels.

Core primitives:

- `tcp_listen(host, port) -> Result<int, str>` — bind and listen
- `tcp_accept(h) -> Result<int, str>` — accept a connection (blocks until a peer arrives)
- `tcp_dial(host, port) -> Result<int, str>` — initiate a connection
- `net_read(h, max) -> Result<str, str>` — read up to max bytes; empty string means EOF
- `net_write(h, s) -> Result<(), str>` — write all bytes of s
- `net_close(h)` — close handle (invalid handle traps)

```rust
fn server(lh) {
    match tcp_accept(lh) {
        Ok(conn) => {
            match net_read(conn, 1024) {
                Ok(data) => {
                    let _ = net_write(conn, "echo:" + data)
                    net_close(conn)
                },
                Err(_) => {},
            }
        },
        Err(_) => {},
    }
}

fn client() {
    let lh = tcp_listen("127.0.0.1", 19876)?
    spawn server(lh)

    let conn = tcp_dial("127.0.0.1", 19876)?
    let _ = net_write(conn, "hello")
    let reply = net_read(conn, 1024)?
    println(reply)                    // echo:hello
    net_close(conn)
    net_close(lh)
    Ok({})
}

match client() {
    Ok(_) => {},
    Err(e) => eprintln("failed: " + e),
}
```

`std/net` offers two convenience functions: `read_all(h)` reads until EOF and
returns the full string; `write_line(h, s)` writes a line (appends `\n`).

**Known limitations**: DNS resolution blocks the scheduler synchronously; UDP,
Unix sockets and TLS are not provided — those are for later versions.

---

## 13. Modules

**A directory is a package.** A module root holds a `bur.mod` declaring its
import path. Files in the same directory share a top-level scope; `pub` and
`import` matter only across packages.

`bur.mod`:

```text
module example.com/hello
```

Package files are declarations only (no bare top-level statements; top-level
`let` must be constant); the entry package starts at `fn main`:

```rust
// main.bur
import "example.com/hello/shapes"          // import a subpackage
// import s "example.com/hello/shapes"     // alias it as s

fn main() {
    let c = shapes.Shape.Circle(2.0)       // access via pkg.name
    println(shapes.describe(c))
}
```

```rust
// shapes/shapes.bur
pub fn describe(s) {                        // pub is cross-package visible
    match s {
        Shape.Circle(r) => "circle r=" + str(r),
        _ => "other",
    }
}
```

Run or build a package with `bur run <dir>` / `bur build <dir>`.

### Dependency management

External dependencies live in the `require` block of `bur.mod`; versions follow
semver and resolution uses MVS (minimal version selection). Common commands:

```sh
bur mod init example.com/myapp    # initialize bur.mod
bur get example.com/lib@v1.2.0    # add or upgrade a dependency
bur mod tidy                      # sync require with actual imports
bur mod download                  # fetch all deps to local cache
bur mod verify                    # verify cache against bur.sum
```

Dependencies cache under `$BURCACHE` (default `~/.cache/bur`); fetching is a
shallow `git clone` of the `v<semver>` tag.

---

## 14. Standard library cheat sheet

Every built-in function today, grouped. `-> Option` / `-> Result` means it
returns that enum.

**Output**
- `print(...)`, `println(...)` — print args space-separated
- `eprintln(...)` — same, but to stderr

**Lists**
- `len(x)`, `push(mut xs, v)`, `pop(xs)`, `slice(xs, start, end)`,
  `concat(a, b)`, `contains(xs, v)`, `range(a, b)`

**Map**
- `map()`, `get(m, k) -> Option`, `put(m, k, v)`, `delete(m, k)`, `keys(m)`

**Strings**
- `str_len(s)`, `char_at(s, i)`, `split(s, sep)`, `join(xs, sep)`,
  `substr(s, start, n)`, `trim(s)`, `str_contains(s, sub)`,
  `str_index_of(s, sub) -> Option`, `ord(s)`, `chr(n)`

**Conversions**
- `str(v)`, `to_float(i)`, `trunc(f)`, `float_bits(f)`,
  `parse_int(s) -> Option`, `parse_float(s) -> Option`, `type_of(v)`

**Filesystem**
- `read_file(p) -> Result`, `write_file(p, s) -> Result`, `file_exists(p)`,
  `read_dir(p) -> Result`

**Process**
- `exec(cmd, args) -> Result<Output, str>` — run a child synchronously
- `exec_start(cmd, args) -> Result<int, str>` — start async, returns pid
- `exec_poll(pid) -> Option<Result<Output, str>>` — poll; `None` if still running
- `sleep(ms)` — suspend the current fiber
- `args()`, `exit(code)`

**Concurrency**
- `chan(cap?)`, `close(ch)`, `recv(ch) -> Option`, `yield()`

**Networking (TCP)**
- `tcp_listen(host, port) -> Result<int, str>` — listen, returns handle
- `tcp_accept(h) -> Result<int, str>` — accept a connection
- `tcp_dial(host, port) -> Result<int, str>` — initiate a connection
- `net_read(h, max) -> Result<str, str>` — read up to max bytes; empty = EOF
- `net_write(h, s) -> Result<(), str>` — write all bytes
- `net_close(h)` — close; invalid handle traps
- `net_nb(h, timeout_ms, host, port) -> Result<str, str>` — non-blocking internal primitive

**Standard library modules** (`import "std/..."`)
- `std/net` — `read_all(h) -> Result<str, str>` (read to EOF), `write_line(h, s)` (write a line)
- `std/json` — `parse(s) -> Result`, `render(v)`, `pretty(v, indent)`, `get(keys, vals, key) -> Option`
- `std/testing` — `assert_eq(got, want)`, `assert_ok(r)`, `assert_err(r)` (for `bur test`)

`std/json` usage (inside a module):

```rust
import "std/json"

fn main() {
    // {{ is a literal opening brace ({ triggers interpolation)
    match json.parse("{{\"name\":\"bur\"}") {
        Ok(v) => println(json.render(v)),   // {"name":"bur"}
        Err(e) => eprintln(e),
    }
}
```

**Misc**
- `clock()`, `assert(cond, msg)`, `gc()`, `heap_objects()`, `gc_cycles()`

---

## 15. The CLI

```sh
bur run <file|dir>       typecheck and run on the VM
bur <file|dir>           same as run
bur check <file|dir>     typecheck only (rustc-style diagnostics)
bur build <file|dir>     compile to a native binary via C
bur fmt <file|dir|->     format source (--check reports without writing)
bur test [dir]           run tests (--run <substr> filter, -v verbose)
bur dis <file|dir>       disassemble the bytecode
bur version
```

`bur build` flags: `-o <path>` output path; `--emit c` emits C instead of a
binary. Compiler search: `$CC` -> `cc` -> `gcc` -> `clang`; if none, it errors
and suggests `bur run`.

`bur fmt` formats the full AST and reinserts comments; `--check` reports whether
formatting is needed without writing, suitable for CI. `bur fmt -` reads stdin
and writes stdout.

Exit codes: `0` success, `1` static error, `2` usage error, `3` input unreadable,
`4` runtime trap.

**Two backends**: the VM (`bur run`, dependency-free) and the C backend
(`bur build`, native binary via the system cc). The contract is **byte-identical
stdout + matching exit code**; the whole language, concurrency included, matches
on both paths, enforced by tests.

---

## 16. Testing

`bur test` discovers all zero-argument `fn test_*` functions in `*_test.bur`
files across the package (and subpackages). Each test runs in its own
subprocess; a trap or deadlock counts as FAIL.

```rust
// math_test.bur
import "std/testing"

fn test_add() {
    testing.assert_eq(1 + 1, 2)
}

fn test_div_by_zero() {
    testing.assert_err(safe_div(1, 0))
}
```

```sh
bur test              # run all
bur test --run add    # filter by substring
bur test -v           # verbose
```

`*_test.bur` files are excluded from normal `bur run`/`bur build`/`bur check`;
they compile only under `bur test`. `std/testing` provides three assertion
helpers: `assert_eq(got, want)`, `assert_ok(r)`, `assert_err(r)`.

---

## 17. Honest limitations

Not in the language yet:

- No **records/structs**: model product types with single-variant enums for
  now, fields positional.
- No `sort`, `getenv`, `math`, regex yet; growing as needed.

---

*The v3 milestone — a fully self-hosted compiler — is done and wrapped up: the
whole pipeline and VM are written in Burryn (`burc/lib/`:
lexer/parser/types/compiler/cgen/vm .bur), driven by the `bur` CLI
(`burc/main.bur`). `bur` compiles itself to C, `cc` turns that into a native
binary, and the binary rebuilds itself byte for byte. The Go host that once
served as the reference is archived on the `archive/go-host` branch. For a
real-sized Burryn program to read, `burc/` is the best material. The grammar is
not frozen yet; freezing it is a v4 matter.*
