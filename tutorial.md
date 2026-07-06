# Burryn 教程 / The Burryn Tutorial

> 相关文档 / See also:[`README.md`](README.md) 项目概览 · [`docs/GOALS.md`](docs/GOALS.md) 设计定案与里程碑
>
> 每节先中文讲解,后英文。代码、关键字与命令保持原样。
> Each section gives the Chinese explanation first, then English. Code, keywords and commands stay as-is.

Burryn 是一门静态推导、零标注、CSP 并发的实用工具语言:Rust 式的类型与
`match`,Go 式的并发与简洁,一个二进制交付。本教程只用**当前能跑**的特性,
每段都对着编译器验过。

Burryn is a small, statically-inferred, zero-annotation language with CSP
concurrency: Rust-style types and `match`, Go-style concurrency and
simplicity, shipped as a single binary. This tutorial uses only features that
work **today**, every snippet checked against the compiler.

---

## 0. 运行与构建 / Running and building

把代码存成 `.bur` 文件,用 VM 直接跑——**不需要任何 C 工具链**:

Save code as a `.bur` file and run it on the VM — **no C toolchain needed**:

```sh
bur run hello.bur       # 类型检查并运行 / typecheck and run
bur hello.bur           # 同上 / same thing
bur check hello.bur     # 只做类型检查 / typecheck only
```

想要一个可独立分发的**原生二进制**,用 `bur build`(这一步需要系统的
`cc`/`gcc`/`clang`):

For a standalone **native binary**, use `bur build` (this step needs a system
`cc`/`gcc`/`clang`):

```sh
bur build hello.bur -o hello   # 经由 C 编译成原生二进制 / compile to a native binary via C
./hello                        # 独立运行,不再需要 bur 或 cc / runs on its own
bur build hello.bur --emit c   # 只吐出生成的 C / dump the generated C instead
```

`bur run` 永远零依赖;`cc` 只在 `bur build` 那一下需要,编好的二进制拷到哪都能跑。

`bur run` is always dependency-free; `cc` is needed only at `bur build` time,
and the resulting binary runs anywhere.

---

## 1. 你好,Burryn / Hello, Burryn

```rust
// 这是注释。语句以换行结束——没有分号(Go 式自动分号)。
// A comment. Statements end at newlines — no semicolons (Go-style).
let name = "Burryn"
println("hello from", name)
```

`println` 接受任意多个参数,用空格分隔后打印并换行;`print` 不换行。脚本文件
可以直接在顶层写语句,从上到下执行。

`println` takes any number of arguments, prints them space-separated with a
trailing newline; `print` omits the newline. A script may put statements at the
top level and runs them top to bottom.

因为换行即语句结束,`} else {` 这种要写在同一行(和 Go 一样)。

Because a newline ends a statement, `} else {` must sit on one line (like Go).

---

## 2. 变量:let、mut、遮蔽 / Variables: let, mut, shadowing

`let` 绑定**默认不可变**,重新赋值是编译错误。要可变得写 `let mut`。

A `let` binding is **immutable by default**; reassigning is a compile error.
Use `let mut` for mutability.

```rust
let x = 10
// x = 11            // 编译错误 / compile error
let mut y = 10
y = y + 1            // ok

// 遮蔽:同名 let 重新绑定(Rust 式),旧值被盖住
// Shadowing: a same-name let rebinds (Rust style)
let x = x * 2        // 这是一个新的 x = 20 / a new x = 20
```

**深 `mut`**:`let` 不仅冻结这个绑定,连它指向的容器内容都冻结。往列表 `push`、
改元素 `l[i] = v`、`pop`,都要求这个列表是 `let mut` 绑的。

**Deep `mut`**: a plain `let` freezes not just the binding but the contents of
the container it points to. `push`, `l[i] = v` and `pop` all require the list to
be bound with `let mut`.

```rust
let frozen = [1, 2, 3]
// push(frozen, 4)   // 编译错误:frozen 不是 mut / compile error
let mut xs = [1, 2, 3]
push(xs, 4)          // ok -> [1, 2, 3, 4]
```

---

## 3. 基本类型 / Primitive types

数值只有两种:`int`(i64)和 `float`(f64)。还有 `bool`、`string`、以及 unit
`()`(“没有有意义的值”)。**禁止一切隐式转换**。

There are exactly two number types: `int` (i64) and `float` (f64). Plus `bool`,
`string`, and unit `()` ("no meaningful value"). **No implicit conversions,
ever.**

```rust
let a = 42            // int
let b = 3.14          // float
let c = true          // bool
let s = "text"        // string

let n = 7
// let bad = n + 1.5  // 编译错误:int 和 float 不混算 / int and float don't mix
let ok = to_float(n) + 1.5   // 显式转 / convert explicitly
let m = trunc(3.9)           // float -> int(截断)= 3
let text = str(n)            // 任意值 -> string / any value -> string
```

**整数溢出一律 trap**(运行时 panic),不静默回绕;整数除零、取模零也 trap。这是
刻意的:默认安全,要就明说。

**Integer overflow always traps** (runtime panic) — it never wraps silently;
integer divide/modulo by zero also traps. Deliberate: safe by default.

字符串是 **UTF-8 字节序列**,`len`/索引按**字节**算。取码点用 `char_at`、`ord`、
`chr`(见第 7 节)。

Strings are **UTF-8 byte sequences**; `len` and indexing count **bytes**. Use
`char_at`, `ord`, `chr` for code points (section 7).

---

## 4. 表达式导向 / Expression orientation

`if`、`match`、块 `{}` 都是**表达式**——有值。函数返回它最后一个表达式。

`if`, `match` and blocks `{}` are **expressions** — they have values. A function
returns its last expression.

```rust
let x = 10
let label = if x > 5 { "big" } else { "small" }   // if 作为值 / if as a value

let y = {                 // 块也有值 / a block has a value
    let t = x * x
    t + 1                 // 块的值 = 最后一个表达式 / value = last expression
}
```

用作值的 `if` 必须带 `else`(否则一条分支没有值)。

An `if` used as a value must have an `else` (otherwise one branch has no value).

---

## 5. 函数与闭包 / Functions and closures

函数参数与返回值**零标注**(类型自动推导)。用 `return` 可提前返回,否则返回最后
一个表达式。

Function parameters and return types are **unannotated** (types are inferred).
Use `return` to return early; otherwise the last expression is returned.

```rust
fn add(a, b) {
    a + b            // 隐式返回 / implicit return
}

fn classify(n) {
    if n < 0 { return "negative" }
    if n == 0 { return "zero" }
    "positive"
}
```

**闭包**:匿名函数 `fn() { ... }` 捕获它环境里的变量。捕获是按引用的——闭包能读写
外层的 `mut` 变量。

**Closures**: an anonymous function `fn() { ... }` captures variables from its
environment, by reference — a closure can read and write an enclosing `mut`.

```rust
fn make_counter() {
    let mut n = 0
    fn() {               // 返回一个闭包 / returns a closure
        n = n + 1
        n
    }
}
let tick = make_counter()
tick()                   // 1
tick()                   // 2
println(tick())          // 3
```

默认参数不可变;要让函数原地改传进来的容器,声明 `fn f(mut xs)`:

Parameters are immutable by default; to mutate a passed-in container in place,
declare `fn f(mut xs)`:

```rust
fn append_one(mut xs) {
    push(xs, 1)          // 需要 mut 参数 / needs the mut parameter
}
```

---

## 6. 列表与循环 / Lists and loops

列表用 `[...]` 字面量,`l[i]` 索引(越界 trap),`l[i] = v` 赋值(需 `mut`)。

Lists use `[...]` literals, `l[i]` to index (out-of-bounds traps), `l[i] = v` to
assign (needs `mut`).

```rust
let mut xs = [10, 20, 30]
println(xs[0])           // 10
xs[1] = 99               // [10, 99, 30]
push(xs, 40)             // [10, 99, 30, 40]
let last = pop(xs)       // 40,xs 变回长度 3 / xs back to length 3
```

三种循环:`for x in list`、`for i in range(a, b)`(半开区间 `[a, b)`)、`while`。
`break` / `continue` 可用。

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

常用列表函数:`len`、`push`、`pop`、`slice(xs, start, end)`、`concat(a, b)`、
`contains(xs, v)`、`range(a, b)`。

Common list functions: `len`, `push`, `pop`, `slice(xs, start, end)`,
`concat(a, b)`, `contains(xs, v)`, `range(a, b)`.

---

## 7. 字符串 / Strings

`+` 拼接字符串,`<` `>` 等按字节序比较。字符串是不可变的字节序列。

`+` concatenates strings; `<` `>` etc. compare bytewise. Strings are immutable
byte sequences.

```rust
let greeting = "hello, " + name
let parts = split("a,b,c", ",")      // ["a", "b", "c"]
let joined = join(parts, " | ")      // "a | b | c"
let sub = substr("burryn", 0, 3)     // "bur"(起点, 长度)/ (start, length)
let hit = str_contains("burryn", "rry")   // true
```

字符串速查 / string cheat sheet:`len`(字节长)、`str_len`、`char_at(s, i)`、
`ord(s)`(首码点 -> int)、`chr(n)`(码点 -> 字符串)、`split`、`join`、`substr`、
`trim`、`str_contains`、`str_index_of`(-> `Option<int>`)、`parse_int`/
`parse_float`(-> `Option`)。

Cheat sheet: `len` (byte length), `str_len`, `char_at(s, i)`, `ord(s)` (first
code point -> int), `chr(n)` (code point -> string), `split`, `join`, `substr`,
`trim`, `str_contains`, `str_index_of` (-> `Option<int>`), `parse_int`/
`parse_float` (-> `Option`).

---

## 8. Map / 映射

map 用**纯函数 API**,不开 `m[k]` 语法糖(零标注推导下,`容器[键]` 在未标注参数里
分不清是列表还是 map)。键只能是 `int` 或 `str`,迭代按**插入顺序**。

Maps use a **function API**, with no `m[k]` sugar (under zero-annotation
inference, `container[key]` can't tell a list from a map in an unannotated
parameter). Keys are `int` or `str`; iteration is in **insertion order**.

```rust
let mut counts = map()           // 新建空 map / a fresh empty map
put(counts, "the", 3)
put(counts, "fox", 2)

match get(counts, "the") {       // get -> Option / 查询返回 Option
    Some(n) => println("the:", n),
    None    => println("missing"),
}

for k in keys(counts) {          // keys -> 键列表 / list of keys
    println(k, "->", get(counts, k))
}
delete(counts, "fox")
println(len(counts))             // map 里的条目数 / number of entries
```

`[]` 恒为列表;字符串取字符用 `char_at`,map 取值用 `get`。

`[]` is always list-only; use `char_at` on strings and `get` on maps.

---

## 9. 枚举与 match / Enums and match

枚举是带类型字段的代数数据类型——**唯一需要写类型的地方**。变体可以有 0 个或多个
字段。

Enums are algebraic data types with typed fields — the **only place you write
types**. Variants may have zero or more fields.

```rust
enum Shape {
    Circle(float),        // 一个 float 字段 / one float field
    Rect(int, int),       // 两个 int 字段 / two int fields
    Point,                // 无字段(单例)/ no fields (a singleton)
}

let shapes = [Circle(2.0), Rect(3, 4), Point]
```

`match` 按变体解构,支持字面量臂、绑定、`_` 通配,且必须**穷尽**。它是表达式,能用在
任何地方。

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

// 字面量 + 绑定 + 通配 / literals + binding + wildcard
let grade = 87
let letter = match grade / 10 {
    10 => "S",
    9  => "A",
    8  => "B",
    other => "F (" + str(other) + ")",   // other 绑定剩余值 / binds the rest
}
```

同名变体属于多个枚举时,用 `Enum.Variant` 限定,如 `Shape.Circle(r)`。

When a variant name is shared across enums, qualify it as `Enum.Variant`, e.g.
`Shape.Circle(r)`.

---

## 10. 没有 null:Option、Result 与 ? / No null: Option, Result, and ?

**没有 null**。可能缺失的值用内建枚举 `Option`(`Some(v)` / `None`);可能失败的用
`Result`(`Ok(v)` / `Err(e)`)。你被迫用 `match` 处理两种情况。

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

`?` 运算符解开 `Ok`/`Some`,否则把 `Err`/`None` **立刻返回**给调用者——这就是错误
传播,没有异常。

The `?` operator unwraps `Ok`/`Some`, or **immediately returns** the `Err`/`None`
to the caller — that is error propagation, with no exceptions.

```rust
fn safe_div(a, b) {
    if b == 0 { return Err("division by zero") }
    Ok(a / b)
}

fn average(a, b, c, d) {
    let x = safe_div(a, b)?      // ? 短路:失败就直接返回 Err / short-circuits on Err
    let y = safe_div(c, d)?
    Ok((x + y) / 2)
}

println(average(10, 2, 30, 3))   // Ok(7)
println(average(10, 0, 30, 3))   // Err("division by zero")
```

标准库约定:文件、进程等操作返回 `Result<T, str>`,失败给 `Err(消息)`。`exec` 用
内建的 `Output(int, str, str)` 枚举带出退出码、stdout、stderr。

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

## 11. 并发:spawn、channel、select / Concurrency

CSP 模型:`spawn` 起一个**纤程**(green thread),纤程间用 **channel** 通信。执行
**始终单 OS 线程**——协作调度 + 10k 指令时间片,单线程交织意味着**天然无数据竞争**。

CSP model: `spawn` starts a **fiber** (green thread); fibers talk over
**channels**. Execution is **always single-threaded** — cooperative scheduling
plus a 10k-instruction time slice — so single-threaded interleaving means **no
data races by construction**.

```rust
fn producer(ch) {
    for i in range(0, 5) {
        ch <- i * i          // 发送 / send
    }
    close(ch)                // 关闭:通知“流结束” / signal end-of-stream
}

let ch = chan(2)             // 容量 2 的缓冲 channel;chan(0)/chan() 无缓冲会合
spawn producer(ch)           // 起一个纤程 / start a fiber

for v in ch {                // for-in 收到 channel 关闭且排空自然结束
    println("got", v)
}
```

`ch <- v` 发送,`<-ch` 接收(可能阻塞)。`recv(ch)` 返回 `Option`:有值 `Some(v)`,
关闭且排空 `None`。向已关闭 channel 发送、或重复 `close` 会 trap。所有纤程都阻塞时
报**死锁**。主纤程返回时整个程序结束(Go 语义)。

`ch <- v` sends, `<-ch` receives (may block). `recv(ch)` returns an `Option`:
`Some(v)` when a value is available, `None` once closed and drained. Sending on a
closed channel, or closing twice, traps. When all fibers are blocked it is
reported as a **deadlock**. The program ends when the main fiber returns (Go
semantics).

`select` 从多个准备好的 channel 操作里挑一个;按声明顺序取第一个就绪的,可选
`default` 让它变非阻塞。

`select` picks one ready channel operation; it takes the first ready arm in
declaration order, with an optional `default` to make it non-blocking.

```rust
select {
    x = <-a => { println("from a:", x) },   // 接收臂 / receive arm
    y = <-b => { println("from b:", y) },
    out <- 1 => { println("sent to out") }, // 发送臂 / send arm
    default => { println("nothing ready") },// 至多一个 default / at most one
}
```

> 并发在两条路径上行为一致:`bur run`(VM)与 `bur build`(经 C 出原生二进制)
> 对同一程序——包括纤程与 channel——输出逐字节相同,这一点由测试保证。
>
> Concurrency behaves identically on both paths: `bur run` (the VM) and
> `bur build` (a native binary via C) produce byte-identical output for the
> same program, fibers and channels included — enforced by tests.

---

## 12. 模块 / Modules

**目录即包**。一个模块的根放一个 `bur.mod` 声明它的 import path。同一目录的文件共享
顶层作用域;跨包才需要 `pub` 和 `import`。

**A directory is a package.** A module root holds a `bur.mod` declaring its
import path. Files in the same directory share a top-level scope; `pub` and
`import` matter only across packages.

`bur.mod`:

```text
module example.com/hello
```

包文件必须以声明为主(顶层不能有裸语句,`let` 限常量),入口包从 `fn main` 开始:

Package files are declarations only (no bare top-level statements; top-level
`let` must be constant); the entry package starts at `fn main`:

```rust
// main.bur
import "example.com/hello/shapes"          // 导入子包 / import a subpackage
// import s "example.com/hello/shapes"     // 起别名 / alias it as s

fn main() {
    let c = shapes.Shape.Circle(2.0)       // 用 pkg.name 访问 / access via pkg.name
    println(shapes.describe(c))
}
```

```rust
// shapes/shapes.bur
pub fn describe(s) {                        // pub 才跨包可见 / pub is cross-package visible
    match s {
        Shape.Circle(r) => "circle r=" + str(r),
        _ => "other",
    }
}
```

用 `bur run <目录>` / `bur build <目录>` 跑一个包。

Run or build a package with `bur run <dir>` / `bur build <dir>`.

---

## 13. 标准库速查 / Standard library cheat sheet

现有的全部内建函数(按域分组)。`-> Option` / `-> Result` 表示返回相应枚举。

Every built-in function today, grouped. `-> Option` / `-> Result` means it
returns that enum.

**输出 / Output**
- `print(...)`, `println(...)` — 按空格连接打印 / print args space-separated

**列表 / Lists**
- `len(x)`, `push(mut xs, v)`, `pop(xs)`, `slice(xs, start, end)`,
  `concat(a, b)`, `contains(xs, v)`, `range(a, b)`

**Map**
- `map()`, `get(m, k) -> Option`, `put(m, k, v)`, `delete(m, k)`, `keys(m)`

**字符串 / Strings**
- `str_len(s)`, `char_at(s, i)`, `split(s, sep)`, `join(xs, sep)`,
  `substr(s, start, n)`, `trim(s)`, `str_contains(s, sub)`,
  `str_index_of(s, sub) -> Option`, `ord(s)`, `chr(n)`

**转换 / Conversions**
- `str(v)`, `to_float(i)`, `trunc(f)`, `float_bits(f)`,
  `parse_int(s) -> Option`, `parse_float(s) -> Option`, `type_of(v)`

**文件系统 / Filesystem**
- `read_file(p) -> Result`, `write_file(p, s) -> Result`, `file_exists(p)`,
  `read_dir(p) -> Result`

**进程 / Process**
- `exec(cmd, args) -> Result<Output, str>`, `args()`, `exit(code)`

**并发 / Concurrency**
- `chan(cap?)`, `close(ch)`, `recv(ch) -> Option`, `yield()`

**其它 / Misc**
- `clock()`, `assert(cond, msg)`, `gc()`, `heap_objects()`, `gc_cycles()`

---

## 14. 命令行 / The CLI

```sh
bur run <file|dir>       类型检查并在 VM 上运行 / typecheck and run on the VM
bur <file|dir>           同 run / same as run
bur check <file|dir>     只类型检查(rustc 式诊断)/ typecheck only
bur build <file|dir>     经由 C 编译成原生二进制 / compile to a native binary via C
bur dis <file|dir>       反汇编字节码 / disassemble the bytecode
bur version
```

`bur build` 选项 / flags:`-o <path>` 输出路径;`--emit c` 吐出 C 而非二进制。找
编译器顺序:`$CC` -> `cc` -> `gcc` -> `clang`;都没有则报错并提示改用 `bur run`。

`bur build` flags: `-o <path>` output path; `--emit c` emits C instead of a
binary. Compiler search: `$CC` -> `cc` -> `gcc` -> `clang`; if none, it errors
and suggests `bur run`.

退出码 / exit codes:`0` 成功、`1` 静态错误、`2` 用法错误、`3` 读不到输入、
`4` 运行时 trap。

Exit codes: `0` success, `1` static error, `2` usage error, `3` input unreadable,
`4` runtime trap.

**两套后端** / **Two backends**:VM(`bur run`,零依赖)与 C 后端(`bur build`,
经系统 cc 出原生二进制)。判定标准是 **stdout 逐字节 + 退出码一致**,整门语言
(含并发)两条路径已一致,由测试持续保证。

The VM (`bur run`, dependency-free) and the C backend (`bur build`, native
binary via the system cc). The contract is **byte-identical stdout + matching
exit code**; the whole language, concurrency included, matches on both paths,
enforced by tests.

---

## 15. 当前限制(诚实) / Honest limitations

语言层还没有的东西 / not in the language yet:

- **记录/结构体(records)**:目前只能用单变体枚举(如 `Output(int, str, str)`)
  凑,字段按位置访问。
  No **records/structs**: model them with single-variant enums for now, fields
  positional.
- **`defer`**、**字符串插值**:计划中,还没实现;拼字符串暂时靠 `+ str(x) +`。
  **`defer`** and **string interpolation** are planned but not built; build
  strings with `+ str(x) +` for now.
- 还没有 `sort`、`getenv`、`math`(sqrt/floor…)、`json`、`net`、regex 等标准库;
  按需生长中。
  No `sort`, `getenv`, `math`, `json`, `net`, regex yet; growing as needed.

工具链 / toolchain:

- **依赖拉取(MVS + 网络)** 还没做——目前只能用本地包,拉不了第三方库。
  **Dependency fetching** isn't in yet — only local packages work; third-party
  libraries can't be pulled.
- `fmt`(官方格式化)、`test` 命令:规划中。
  `fmt` and a `test` command are planned.


---

*v3 里程碑「编译器完全自举」已达成,并已收尾:整条编译管线与 VM 都用 Burryn 写成
(`burc/lib/` 下的 lexer/parser/types/compiler/cgen/vm .bur),`bur` CLI
(`burc/main.bur`)驱动它。`bur` 把自己编译成 C、经 cc 落地成原生二进制,再由该
二进制逐字节重建自身。曾作为参照的 Go 宿主已归档到 `archive/go-host` 分支。想看一个
"真实规模"的 Burryn 程序,`burc/` 就是最好的读物。语法尚未冻结;冻结与正式 grammar
是 v4 的事。*

*The v3 milestone — a fully self-hosted compiler — is done and wrapped up: the
whole pipeline and VM are written in Burryn (`burc/lib/`:
lexer/parser/types/compiler/cgen/vm .bur), driven by the `bur` CLI
(`burc/main.bur`). `bur` compiles itself to C, `cc` turns that into a native
binary, and the binary rebuilds itself byte for byte. The Go host that once
served as the reference is archived on the `archive/go-host` branch. For a
real-sized Burryn program to read, `burc/` is the best material. The grammar is
not frozen yet; freezing it is a v4 matter.*
