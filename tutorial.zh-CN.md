[English](tutorial.md) | 中文

# Burryn 教程

> 相关文档：[`README.zh-CN.md`](README.zh-CN.md) 项目概览 · [`docs/GOALS.md`](docs/GOALS.md) 设计定案与里程碑

Burryn 是一门静态推导、零标注、CSP 并发的实用工具语言：Rust 式的类型与
`match`，Go 式的并发与简洁，一个二进制交付。本教程只用**当前能跑**的特性，
每段都对着编译器验过。

---

## 0. 运行与构建

把代码存成 `.bur` 文件，用 VM 直接跑——**不需要任何 C 工具链**：

```sh
bur run hello.bur       # 类型检查并运行
bur hello.bur           # 同上
bur check hello.bur     # 只做类型检查
```

想要一个可独立分发的**原生二进制**，用 `bur build`（这一步需要系统的
`cc`/`gcc`/`clang`）：

```sh
bur build hello.bur -o hello   # 经由 C 编译成原生二进制
./hello                        # 独立运行，不再需要 bur 或 cc
bur build hello.bur --emit c   # 只吐出生成的 C
```

`bur run` 永远零依赖；`cc` 只在 `bur build` 那一下需要，编好的二进制拷到哪都能跑。

---

## 1. 你好，Burryn

```rust
// 这是注释。语句以换行结束——没有分号（Go 式自动分号）。
let name = "Burryn"
println("hello from", name)
```

`println` 接受任意多个参数，用空格分隔后打印并换行；`print` 不换行。脚本文件
可以直接在顶层写语句，从上到下执行。

因为换行即语句结束，`} else {` 这种要写在同一行（和 Go 一样）。

---

## 2. 变量：let、mut、遮蔽

`let` 绑定**默认不可变**，重新赋值是编译错误。要可变得写 `let mut`。

```rust
let x = 10
// x = 11            // 编译错误
let mut y = 10
y = y + 1            // ok

// 遮蔽：同名 let 重新绑定（Rust 式），旧值被盖住
let x = x * 2        // 这是一个新的 x = 20
```

**深 `mut`**：`let` 不仅冻结这个绑定，连它指向的容器内容都冻结。往列表 `push`、
改元素 `l[i] = v`、`pop`，都要求这个列表是 `let mut` 绑的。

```rust
let frozen = [1, 2, 3]
// push(frozen, 4)   // 编译错误：frozen 不是 mut
let mut xs = [1, 2, 3]
push(xs, 4)          // ok -> [1, 2, 3, 4]
```

### 编译期常量

`const` 声明一个**编译期求值**的绑定：表达式在编译时折叠成字面值，运行时零开销。
`const` 可出现在任何作用域（顶层、函数内）；跨包公开用 `pub const`。

```rust
const answer = 40 + 2            // 编译期折叠为 42
const greeting = "hello" + ", constants"

fn increment() {
    const next = answer + 1      // 函数内也行
    next
}
println(greeting)                        // hello, constants
println(str(answer), str(increment()))   // 42 43
```

`const` 的初始化表达式只能引用字面值、其它 `const`、以及纯内建运算；不能调用普通
函数或引用 `let` 变量。

---

## 3. 基本类型

数值只有两种：`int`（i64）和 `float`（f64）。还有 `bool`、`string`、以及 unit
`()`（"没有有意义的值"）。**禁止一切隐式转换**。

```rust
let a = 42            // int
let b = 3.14          // float
let c = true          // bool
let s = "text"        // string

let n = 7
// let bad = n + 1.5  // 编译错误：int 和 float 不混算
let ok = to_float(n) + 1.5   // 显式转
let m = trunc(3.9)           // float -> int（截断）= 3
let text = str(n)            // 任意值 -> string
```

**整数溢出一律 trap**（运行时 panic），不静默回绕；整数除零、取模零也 trap。这是
刻意的：默认安全，要就明说。

字符串是 **UTF-8 字节序列**，`len`/索引按**字节**算。取码点用 `char_at`、`ord`、
`chr`（见第 7 节）。

### 字符串插值

字符串里的 `{expr}` 会求值并拼进结果。表达式必须已经是 `str`；Burryn 不做隐式
转换，其他类型须显式调用 `str()`。字面左花括号写成 `{{`。

```rust
let name = "Burryn"
let jobs = 3
println("hello {name}, jobs={str(jobs)}")
println("literal brace: {{")
```

---

## 4. 表达式导向

`if`、`match`、块 `{}` 都是**表达式**——有值。函数返回它最后一个表达式。

```rust
let x = 10
let label = if x > 5 { "big" } else { "small" }   // if 作为值

let y = {                 // 块也有值
    let t = x * x
    t + 1                 // 块的值 = 最后一个表达式
}
```

用作值的 `if` 必须带 `else`（否则一条分支没有值）。

---

## 5. 函数与闭包

函数参数与返回值**零标注**（类型自动推导）。用 `return` 可提前返回，否则返回最后
一个表达式。

```rust
fn add(a, b) {
    a + b            // 隐式返回
}

fn classify(n) {
    if n < 0 { return "negative" }
    if n == 0 { return "zero" }
    "positive"
}
```

**闭包**：匿名函数 `fn() { ... }` 捕获它环境里的变量。捕获是按引用的——闭包能读写
外层的 `mut` 变量。

```rust
fn make_counter() {
    let mut n = 0
    fn() {               // 返回一个闭包
        n = n + 1
        n
    }
}
let tick = make_counter()
tick()                   // 1
tick()                   // 2
println(tick())          // 3
```

默认参数不可变；要让函数原地改传进来的容器，声明 `fn f(mut xs)`：

```rust
fn append_one(mut xs) {
    push(xs, 1)          // 需要 mut 参数
}
```

### 管道操作符

`x |> f` 把左侧的值作为第一个实参传给右侧调用，等价 `f(x)`；带参写 `x |> f(a)` =
`f(x, a)`。`|>` 优先级最低、左结合，所以 `x |> f |> g` 是 `g(f(x))`，
`1 + 2 |> str` 是 `str(1 + 2)`。右侧只接受函数名或 `pkg.name`（可带实参），
其它表达式是 parse error；`?` 结合更紧，传播错误写 `(x |> parse)?`。

```rust
fn double(x) { x * 2 }
fn clamp(x, hi) {
    if x > hi { hi } else { x }
}
let n = 3 |> double |> clamp(5)   // clamp(double(3), 5)
println(n)                        // 5
println(1 + 2 |> str)             // 3
```

### defer / 延迟清理

`defer { ... }` 把一个块挂到**包围函数**上，函数退出时按注册的逆序（LIFO）执行——
`return`、末尾表达式和 `?` 传播都算退出。块是闭包：按引用捕获环境，退出时读到的是
变量的最终值。trap 是进程级中止，不执行 defer。

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

脚本顶层也能写 `defer`——整个脚本就是一个函数，块在脚本结束时执行。

---

## 6. 列表与循环

列表用 `[...]` 字面量，`l[i]` 索引（越界 trap），`l[i] = v` 赋值（需 `mut`）。

```rust
let mut xs = [10, 20, 30]
println(xs[0])           // 10
xs[1] = 99               // [10, 99, 30]
push(xs, 40)             // [10, 99, 30, 40]
let last = pop(xs)       // 40，xs 变回长度 3
```

三种循环：`for x in list`、`for i in range(a, b)`（半开区间 `[a, b)`）、`while`。
`break` / `continue` 可用。

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

常用列表函数：`len`、`push`、`pop`、`slice(xs, start, end)`、`concat(a, b)`、
`contains(xs, v)`、`range(a, b)`。

---

## 7. 字符串

`+` 拼接字符串，`<` `>` 等按字节序比较。字符串是不可变的字节序列。

```rust
let greeting = "hello, " + name
let parts = split("a,b,c", ",")      // ["a", "b", "c"]
let joined = join(parts, " | ")      // "a | b | c"
let sub = substr("burryn", 0, 3)     // "bur"（起点, 长度）
let hit = str_contains("burryn", "rry")   // true
```

字符串速查：`len`（字节长）、`str_len`、`char_at(s, i)`、`ord(s)`（首码点 -> int）、
`chr(n)`（码点 -> 字符串）、`split`、`join`、`substr`、`trim`、`str_contains`、
`str_index_of`（-> `Option<int>`）、`parse_int`/`parse_float`（-> `Option`）。

---

## 8. Map / 映射

map 用**纯函数 API**，不开 `m[k]` 语法糖（零标注推导下，`容器[键]` 在未标注参数里
分不清是列表还是 map）。键只能是 `int` 或 `str`，迭代按**插入顺序**。

```rust
let mut counts = map()           // 新建空 map
put(counts, "the", 3)
put(counts, "fox", 2)

match get(counts, "the") {       // get -> Option
    Some(n) => println("the:", n),
    None    => println("missing"),
}

for k in keys(counts) {          // keys -> 键列表
    println(k, "->", get(counts, k))
}
delete(counts, "fox")
println(len(counts))             // map 里的条目数
```

`[]` 恒为列表；字符串取字符用 `char_at`，map 取值用 `get`。

---

## 9. 枚举与 match

枚举是带类型字段的代数数据类型——**唯一需要写类型的地方**。变体可以有 0 个或多个
字段。

```rust
enum Shape {
    Circle(float),        // 一个 float 字段
    Rect(int, int),       // 两个 int 字段
    Point,                // 无字段（单例）
}

let shapes = [Circle(2.0), Rect(3, 4), Point]
```

`match` 按变体解构，支持字面量臂、绑定、`_` 通配，且必须**穷尽**。它是表达式，能用在
任何地方。

```rust
fn area(s) {
    match s {
        Circle(r)  => 3.14159 * r * r,
        Rect(w, h) => to_float(w * h),
        Point      => 0.0,
    }
}

// 字面量 + 绑定 + 通配
let grade = 87
let letter = match grade / 10 {
    10 => "S",
    9  => "A",
    8  => "B",
    other => "F (" + str(other) + ")",   // other 绑定剩余值
}
```

match 臂可在 pattern 后加 `if` guard。guard 在 pattern 绑定之后求值，所以能使用
刚绑定的名字；结果必须是 `bool`。带 guard 的臂不算穷尽覆盖，因为 guard 可能为假，
因此仍须保留无 guard 的 fallback。

```rust
let jobs = Some(3)
let status = match jobs {
    Some(n) if n > 0 => "active: " + str(n),
    Some(_) => "idle",
    None => "missing",
}
println(status)
```

同名变体属于多个枚举时，用 `Enum.Variant` 限定，如 `Shape.Circle(r)`。

---

## 10. 没有 null：Option、Result 与 ?

**没有 null**。可能缺失的值用内建枚举 `Option`（`Some(v)` / `None`）；可能失败的用
`Result`（`Ok(v)` / `Err(e)`）。你被迫用 `match` 处理两种情况。

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

`?` 运算符解开 `Ok`/`Some`，否则把 `Err`/`None` **立刻返回**给调用者——这就是错误
传播，没有异常。

```rust
fn safe_div(a, b) {
    if b == 0 { return Err("division by zero") }
    Ok(a / b)
}

fn average(a, b, c, d) {
    let x = safe_div(a, b)?      // ? 短路：失败就直接返回 Err
    let y = safe_div(c, d)?
    Ok((x + y) / 2)
}

println(average(10, 2, 30, 3))   // Ok(7)
println(average(10, 0, 30, 3))   // Err("division by zero")
```

标准库约定：文件、进程等操作返回 `Result<T, str>`，失败给 `Err(消息)`。`exec` 用
内建的 `Output(int, str, str)` 枚举带出退出码、stdout、stderr。

```rust
match read_file("notes.txt") {
    Ok(text) => println("read", len(text), "bytes"),
    Err(msg) => println("failed:", msg),
}
```

---

## 11. 并发：spawn、channel、select

CSP 模型：`spawn` 起一个**纤程**（green thread），纤程间用 **channel** 通信。执行
**始终单 OS 线程**——协作调度 + 10k 指令时间片，单线程交织意味着**天然无数据竞争**。

```rust
fn producer(ch) {
    for i in range(0, 5) {
        ch <- i * i          // 发送
    }
    close(ch)                // 关闭：通知"流结束"
}

let ch = chan(2)             // 容量 2 的缓冲 channel；chan(0)/chan() 无缓冲会合
spawn producer(ch)           // 起一个纤程

for v in ch {                // for-in 收到 channel 关闭且排空自然结束
    println("got", v)
}
```

`ch <- v` 发送，`<-ch` 接收（可能阻塞）。`recv(ch)` 返回 `Option`：有值 `Some(v)`，
关闭且排空 `None`。向已关闭 channel 发送、或重复 `close` 会 trap。所有纤程都阻塞时
报**死锁**。主纤程返回时整个程序结束（Go 语义）。

`select` 从多个准备好的 channel 操作里挑一个；按声明顺序取第一个就绪的，可选
`default` 让它变非阻塞。

```rust
select {
    x = <-a => { println("from a:", x) },   // 接收臂
    y = <-b => { println("from b:", y) },
    out <- 1 => { println("sent to out") }, // 发送臂
    default => { println("nothing ready") },// 至多一个 default
}
```

> 并发在两条路径上行为一致：`bur run`（VM）与 `bur build`（经 C 出原生二进制）
> 对同一程序——包括纤程与 channel——输出逐字节相同，这一点由测试保证。

---

## 12. 网络

Burryn 内建 TCP 支持：六个 native 提供非阻塞的 TCP 连接管理，调度器在等待网络
IO 时自动 park/唤醒 fiber——和 channel 一样自然。

核心原语：

- `tcp_listen(host, port) -> Result<int, str>` — 绑定并监听
- `tcp_accept(h) -> Result<int, str>` — 接受一个连接（阻塞直到有对端）
- `tcp_dial(host, port) -> Result<int, str>` — 发起连接
- `net_read(h, max) -> Result<str, str>` — 读至多 max 字节；空串表示 EOF
- `net_write(h, s) -> Result<(), str>` — 写完 s 的全部字节
- `net_close(h)` — 关闭句柄（无效句柄 trap）

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

`std/net` 提供两个便利函数：`read_all(h)` 读到 EOF 并返回完整字符串，
`write_line(h, s)` 写一行（自动追加 `\n`）。

**已知限制**：DNS 解析同步阻塞调度器；不提供 UDP、Unix socket 或 TLS——这些是
后续版本的事。

---

## 13. 模块

**目录即包**。一个模块的根放一个 `bur.mod` 声明它的 import path。同一目录的文件共享
顶层作用域；跨包才需要 `pub` 和 `import`。

`bur.mod`：

```text
module example.com/hello
```

包文件必须以声明为主（顶层不能有裸语句，`let` 限常量），入口包从 `fn main` 开始：

```rust
// main.bur
import "example.com/hello/shapes"          // 导入子包
// import s "example.com/hello/shapes"     // 起别名

fn main() {
    let c = shapes.Shape.Circle(2.0)       // 用 pkg.name 访问
    println(shapes.describe(c))
}
```

```rust
// shapes/shapes.bur
pub fn describe(s) {                        // pub 才跨包可见
    match s {
        Shape.Circle(r) => "circle r=" + str(r),
        _ => "other",
    }
}
```

用 `bur run <目录>` / `bur build <目录>` 跑一个包。

### 依赖管理

外部依赖声明在 `bur.mod` 的 `require` 块里，版本走 semver、解析用 MVS（最小版本
选择）。常用命令：

```sh
bur mod init example.com/myapp    # 初始化 bur.mod
bur get example.com/lib@v1.2.0    # 添加/升级依赖
bur mod tidy                      # 按实际 import 增删 require
bur mod download                  # 拉取全部依赖到本地缓存
bur mod verify                    # 校验缓存与 bur.sum 一致
```

依赖缓存在 `$BURCACHE`（默认 `~/.cache/bur`）；拉取走 `git clone` 浅克隆
`v<semver>` tag。

---

## 14. 标准库速查

现有的全部内建函数（按域分组）。`-> Option` / `-> Result` 表示返回相应枚举。

**输出**
- `print(...)`、`println(...)` — 按空格连接打印
- `eprintln(...)` — 同上，写到 stderr

**列表**
- `len(x)`、`push(mut xs, v)`、`pop(xs)`、`slice(xs, start, end)`、
  `concat(a, b)`、`contains(xs, v)`、`range(a, b)`

**Map**
- `map()`、`get(m, k) -> Option`、`put(m, k, v)`、`delete(m, k)`、`keys(m)`

**字符串**
- `str_len(s)`、`char_at(s, i)`、`split(s, sep)`、`join(xs, sep)`、
  `substr(s, start, n)`、`trim(s)`、`str_contains(s, sub)`、
  `str_index_of(s, sub) -> Option`、`ord(s)`、`chr(n)`

**转换**
- `str(v)`、`to_float(i)`、`trunc(f)`、`float_bits(f)`、
  `parse_int(s) -> Option`、`parse_float(s) -> Option`、`type_of(v)`

**文件系统**
- `read_file(p) -> Result`、`write_file(p, s) -> Result`、`file_exists(p)`、
  `read_dir(p) -> Result`

**进程**
- `exec(cmd, args) -> Result<Output, str>` — 同步运行子进程
- `exec_start(cmd, args) -> Result<int, str>` — 异步启动，返回 pid
- `exec_poll(pid) -> Option<Result<Output, str>>` — 轮询子进程；未完成 `None`
- `sleep(ms)` — 当前 fiber 挂起指定毫秒
- `args()`、`exit(code)`

**并发**
- `chan(cap?)`、`close(ch)`、`recv(ch) -> Option`、`yield()`

**网络（TCP）**
- `tcp_listen(host, port) -> Result<int, str>` — 监听，返回句柄
- `tcp_accept(h) -> Result<int, str>` — 接受连接
- `tcp_dial(host, port) -> Result<int, str>` — 发起连接
- `net_read(h, max) -> Result<str, str>` — 读至多 max 字节；空串 = EOF
- `net_write(h, s) -> Result<(), str>` — 写完全部字节
- `net_close(h)` — 关闭句柄；无效句柄 trap
- `net_nb(h, timeout_ms, host, port) -> Result<str, str>` — 非阻塞内部原语

**标准库模块**（`import "std/..."`）
- `std/net` — `read_all(h) -> Result<str, str>`（读到 EOF）、`write_line(h, s)`（写一行）
- `std/json` — `parse(s) -> Result`、`render(v)`、`pretty(v, indent)`、`get(keys, vals, key) -> Option`
- `std/testing` — `assert_eq(got, want)`、`assert_ok(r)`、`assert_err(r)`（用于 `bur test`）

`std/json` 用法（模块内）：

```rust
import "std/json"

fn main() {
    // {{ 是字面左花括号（{ 触发插值）
    match json.parse("{{\"name\":\"bur\"}") {
        Ok(v) => println(json.render(v)),   // {"name":"bur"}
        Err(e) => eprintln(e),
    }
}
```

**其它**
- `clock()`、`assert(cond, msg)`、`gc()`、`heap_objects()`、`gc_cycles()`

---

## 15. 命令行

```sh
bur run <file|dir>       类型检查并在 VM 上运行
bur <file|dir>           同 run
bur check <file|dir>     只类型检查（rustc 式诊断）
bur build <file|dir>     经由 C 编译成原生二进制
bur fmt <file|dir|->     格式化源码（--check 只检查不写回）
bur test [dir]           运行测试（--run <substr> 过滤，-v 详细）
bur dis <file|dir>       反汇编字节码
bur version
```

`bur build` 选项：`-o <path>` 输出路径；`--emit c` 吐出 C 而非二进制。找
编译器顺序：`$CC` -> `cc` -> `gcc` -> `clang`；都没有则报错并提示改用 `bur run`。

`bur fmt` 格式化整棵 AST 并重新插入注释；`--check` 模式只报告是否需要格式化而不
写回文件，适合 CI。`bur fmt -` 从 stdin 读、写到 stdout。

退出码：`0` 成功、`1` 静态错误、`2` 用法错误、`3` 读不到输入、`4` 运行时 trap。

**两套后端**：VM（`bur run`，零依赖）与 C 后端（`bur build`，经系统 cc 出原生
二进制）。判定标准是 **stdout 逐字节 + 退出码一致**，整门语言（含并发）两条路径
已一致，由测试持续保证。

---

## 16. 测试

`bur test` 自动发现当前包（及子包）里 `*_test.bur` 文件中所有零参 `fn test_*`
函数，每个测试跑在独立子进程里。trap 或死锁 = FAIL。

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
bur test              # 跑全部
bur test --run add    # 只跑名字含 "add" 的
bur test -v           # 详细输出
```

`*_test.bur` 文件被普通 `bur run`/`bur build`/`bur check` 排除，只在 `bur test`
时参与编译。`std/testing` 提供 `assert_eq(got, want)`、`assert_ok(r)`、
`assert_err(r)` 三个断言 helper。

---

## 17. 当前限制（诚实）

语言层还没有的东西：

- **记录/结构体（records）**：目前只能用单变体枚举（如 `Output(int, str, str)`）
  凑，字段按位置访问。
- 还没有 `sort`、`getenv`、`math`（sqrt/floor…）、regex 等标准库；按需生长中。

---

*v3 里程碑「编译器完全自举」已达成，并已收尾：整条编译管线与 VM 都用 Burryn 写成
（`burc/lib/` 下的 lexer/parser/types/compiler/cgen/vm .bur），`bur` CLI
（`burc/main.bur`）驱动它。`bur` 把自己编译成 C、经 cc 落地成原生二进制，再由该
二进制逐字节重建自身。曾作为参照的 Go 宿主已归档到 `archive/go-host` 分支。想看一个
"真实规模"的 Burryn 程序，`burc/` 就是最好的读物。语法尚未冻结；冻结与正式 grammar
是 v4 的事。*
