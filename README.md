# Burryn

<!-- Badges / 徽章 -->
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Self-hosted](https://img.shields.io/badge/compiler-self--hosted-brightgreen.svg)](docs/GOALS.md)
[![Status](https://img.shields.io/badge/status-active-brightgreen.svg)]()
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

> **The ring beneath Meyrin.** A burrow, a ring — quiet work underground.
>
> **Meyrin 之下的环。** 一座洞穴，一枚戒指——在地下安静生长。

Burryn is a small programming language forged from Go and Rust: hand-written
lexer, recursive-descent parser, Hindley-Milner type inference with zero
annotations, single-pass bytecode compiler, a stack-based VM with its own
mark-sweep garbage collector and green-thread scheduler, and a C backend that
turns programs into standalone native binaries.

Burryn 是一门借鉴 Go 与 Rust 的小型编程语言：手写词法分析器、递归下降解析器、
零标注的 Hindley-Milner 类型推断、单遍字节码编译器、自带 mark-sweep 垃圾回收器
与绿色线程调度器的栈式虚拟机，以及能把程序编译成独立原生二进制文件的 C 后端。

**The compiler is self-hosted.** `burc/` reimplements the whole pipeline —
lexer, parser, type checker, bytecode compiler, C code generator — in Burryn
itself (~7000 lines). `bur` compiles itself to C, `cc` turns that into a
native binary, and the resulting native `bur` compiles the same source to the
same bytes again: a closed bootstrap fixpoint. The original Go implementation
that seeded the bootstrap is archived on the `archive/go-host` branch.

**编译器是自举的。** `burc/` 用 Burryn 自身重实现了整条编译管线——词法、
语法、类型检查、字节码编译、C 代码生成——约 7000 行。`bur` 先把自身编译为 C，
`cc` 再将其编译为原生二进制，而这个原生 `bur` 又能把同一份源码编译出逐字节
相同的输出：一个封闭的自举定点。用于启动自举的原始 Go 实现已归档在
`archive/go-host` 分支。

The name: a **burrow** is where a gopher (Go) lives, and it puns on Rust's
*borrow* checker; a **burr** is what forging leaves on metal; and *burrin*
reads like **burin** — the engraver's tool for precise, quiet work.

名字溯源：**burrow**（洞穴）是 gopher（Go 的吉祥物）的居所，又谐音 Rust 的
*borrow* checker；**burr** 是锻造在金属上留下的毛刺；*burrin* 则近似 **burin**
——雕版师精细而安静的刻刀。

```sh
bur run examples/sieve.bur
```

## 🦀 What it takes from Rust / 取自 Rust

- **Immutable by default.** `let x = 1` cannot be reassigned — enforced at
  *compile time*. Mutation requires `let mut`, and it runs deep: a plain
  `let` freezes list contents too — `push`, `pop` and `l[i] = v` all demand
  a `mut` binding.

  **默认不可变。** `let x = 1` 不可重新赋值——在*编译期*强制执行。
  要可变必须用 `let mut`，且深度生效：普通 `let` 会冻结列表内容，
  `push`、`pop`、`l[i] = v` 都需要 `mut` 绑定。
- **No null.** Absence is `Option` (`Some(v)` / `None`), failure is `Result`
  (`Ok(v)` / `Err(e)`). Both are built-in enums.

  **没有 null。** 缺失用 `Option`（`Some(v)` / `None`），失败用 `Result`
  （`Ok(v)` / `Err(e)`）。两者都是内建枚举。
- **The `?` operator.** Unwraps `Ok`/`Some`, or returns the `Err`/`None` to
  the caller immediately.

  **`?` 运算符。** 自动展开 `Ok`/`Some`，否则立即把 `Err`/`None` 返回给调用方。
- **`match` expressions** with enum destructuring, literal arms, bindings and
  `_`, usable anywhere an expression fits.

  **`match` 表达式。** 支持枚举解构、字面量分支、绑定与 `_`，可在任何需要表达式的位置使用。
- **Expression orientation.** `if`, `match` and blocks `{}` have values; a
  function returns its last expression.

  **表达式导向。** `if`、`match` 与 `{}` 块都有值；函数返回其最后一个表达式。
- **Shadowing.** `let x = x + 1` rebinds, Rust style.

  **遮蔽（Shadowing）。** `let x = x + 1` 重新绑定，与 Rust 一致。
- **Algebraic data types.** `enum Shape { Circle(r), Rect(w, h), Point }`.

  **代数数据类型。** `enum Shape { Circle(r), Rect(w, h), Point }`。

## 🐹 What it takes from Go / 取自 Go

- **A garbage collector instead of a borrow checker.** Hand-written
  mark-sweep over the VM's own heap; inspect it with `gc()`,
  `heap_objects()`, `gc_cycles()`.

  **GC 替代借用检查。** 手写 mark-sweep 直接回收虚拟机自身堆；可用 `gc()`、
  `heap_objects()`、`gc_cycles()` 观察。
- **Green threads.** `spawn worker(ch)` starts a fiber on the VM's
  scheduler — cooperative, plus a 10k-instruction time slice so a spinning
  fiber cannot starve the rest. Single-threaded interleaving means **no data
  races by construction**.

  **绿色线程。** `spawn worker(ch)` 在虚拟机调度器上启动一个 fiber——协作式调度，
  外加 1 万条指令的时间片，防止自旋 fiber 饿死其他 fiber。单线程交错意味着
  **从结构上避免数据竞争**。
- **Channels, with Go's arrow.** `ch <- v` sends, `<-ch` receives.
  Unbuffered channels rendezvous; `chan(n)` buffers. All-fibers-blocked is
  detected and reported as a deadlock. The program exits when the main fiber
  returns.

  **Go 风格的 channel。** `ch <- v` 发送，`<-ch` 接收。无缓冲 channel 直接会面；
  `chan(n)` 带缓冲。当所有 fiber 都阻塞时会被检测并报为死锁。主 fiber 返回时程序结束。
- **No semicolons.** Newlines end statements (Go-style automatic insertion),
  so `} else` belongs on one line.

  **无分号。** 换行结束语句（Go 风格的自动分号插入），所以 `} else` 要写在同一行。

## 🚀 Tour / 语言速览

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

## 📂 Examples / 示例

| file | shows / 说明 |
|------|--------------|
| `examples/hello.bur` | basics, closures, loops / 基础、闭包、循环 |
| `examples/shapes.bur` | enums + match / 枚举与 match |
| `examples/errors.bur` | Result/Option and `?` / Result、Option 与 `?` |
| `examples/sieve.bur` | the classic CSP prime sieve: one fiber per prime / 经典 CSP 素数筛：每个素数一个 fiber |
| `examples/pipeline.bur` | buffered-channel producer/consumer / 带缓冲 channel 的生产者/消费者 |
| `examples/gc_stress.bur` | watch the collector work / 观察垃圾回收器工作 |
| `examples/fib.bur` | recursion micro-benchmark / 递归微基准 |
| `examples/brainfuck.bur` | a Brainfuck interpreter written in Burryn / 用 Burryn 写的 Brainfuck 解释器 |
| `examples/multiplex.bur` | `select` over several channels / 多 channel 的 `select` |
| `examples/streaming.bur` | channel close / for-in draining / channel 关闭与 for-in 排空 |
| `examples/textproc.bur` | files, exec, argv: a small text tool / 文件、exec、argv：小型文本工具 |
| `examples/wordcount.bur` | maps and string functions / map 与字符串函数 |
| `examples/geometry/` | a multi-package module (`bur.mod`, `import`, `pub`) / 多包模块示例 |
| `burc/` | the self-hosted compiler and `bur` CLI — the biggest Burryn program there is / 自举编译器与 `bur` CLI——现存最大的 Burryn 程序 |

## 🏗 Architecture / 架构

```markdown
source ──lexer──▶ tokens ──parser──▶ AST ──checker──▶ typed ──compiler──▶ bytecode
 (lexer.bur)       (auto-semicolons)  (parser.bur) (types.bur, HM)  (compiler.bur)
                                                                        │
                                        ┌───────────────────────────────┤
                                        ▼                               ▼
                                  BurrynVM (vm.bur)           C backend (cgen.bur)
                                  fibers + channels           ──▶ .c ──cc──▶ native
                                  scheduler, GC               runtime/burrt*.h

Burryn is self-hosted: the whole pipeline lives in `burc/lib/` (token/lexer/
parser/types/compiler/cgen/module/vm .bur), and the `bur` CLI (`burc/main.bur`)
drives it. `bur` compiles itself to C, `cc` turns that into a native binary,
and the result rebuilds itself byte for byte.
```

- **Compiler**: single pass, clox-style locals/upvalues, with a
  temp-tracking scheme that lets `match`/block expressions (which declare
  locals) appear at any expression depth.

  **编译器：** 单遍编译，clox 风格的局部变量/upvalue，配合临时值追踪，
  使 `match`/块表达式（会声明局部变量）可出现在任意表达式深度。
- **Closures**: upvalues reference `(fiber, slot)` while open and are closed
  on scope exit — safe across fiber switches and stack growth.

  **闭包：** upvalue 在打开时引用 `(fiber, slot)`，作用域退出时关闭，
  在 fiber 切换与栈增长时保持安全。
- **Match**: compiles to variant tests (`TEST_VARIANT` on enum identity +
  tag) and field extraction, no hashing, no reflection.

  **Match：** 编译为变体测试（基于枚举标识与 tag 的 `TEST_VARIANT`）
  和字段提取，无需哈希，无需反射。
- **GC**: every language object lives in an intrusive list; roots are
  globals, every fiber's stack/frames/open upvalues/pending channel sends.

  **GC：** 每个语言对象都在一个侵入式列表中；根包括全局变量、
  每个 fiber 的栈/帧/开放 upvalue/待发送的 channel 值。
- **Scheduler**: FIFO ready queue; fibers park on channel wait queues; the
  receiver/sender hands values across directly.

  **调度器：** FIFO 就绪队列；fiber 在 channel 等待队列上 park；
  接收方/发送方直接交接值。

## ⌨️ Commands / 命令

```sh
bur run <file|dir>    typecheck and run on the VM / 类型检查并在 VM 上运行
bur <file|dir>        same / 同上
bur check <file|dir>  typecheck only (rustc-style diagnostics) / 仅类型检查（rustc 风格诊断）
bur build <file|dir>  compile to a native binary via C (needs cc/gcc/clang) / 经 C 编译为原生二进制（需要 cc/gcc/clang）
bur dis <file|dir>    disassemble the compiled bytecode / 反编译字节码
bur version           print the version / 打印版本号
```

Build (self-hosting): a native `bur` builds itself with
`bur build burc -o bur`. To bootstrap from scratch, check out the archived Go
host and let it compile the first `bur`:

构建（自举）：原生 `bur` 用 `bur build burc -o bur` 构建自身。
要从零自举，请切出归档的 Go 宿主，由它编译出第一个 `bur`：

```sh
git checkout archive/go-host
go build -o bur.exe .          # temporary Go seed / 临时 Go 种子
./bur.exe build burc -o bur    # seed compiles the self-hosted CLI / 种子编译自举 CLI
git checkout main
./bur build burc -o bur        # from here on, bur rebuilds itself / 此后 bur 自行重建
```

## ⚠️ Honest limitations / 诚实局限

Stages S1–S5 (semantic core, C backend, modules, maps, `select`/`close`,
`mut` parameters, a fully self-hosted compiler with the Go host removed) are
done; both backends produce byte-identical output for the whole language,
concurrency included. Still missing: records/structs (model product types
with single-variant enums), `defer`, string interpolation, and third-party
dependency fetching (only local packages resolve). Deep `mut` is a
binding-level discipline, not a borrow checker: two `mut` bindings may still
alias the same list.

S1–S5 阶段（语义内核、C 后端、模块、map、`select`/`close`、
`mut` 参数、移除 Go 宿主后的完全自举编译器）已完成；两个后端对整门语言
（含并发）都能产生逐字节一致的输出。仍缺失：records/structs（可用单变体枚举
建模乘积类型）、`defer`、字符串插值、第三方依赖拉取（目前仅解析本地包）。
深 `mut` 是绑定级别的规则，不是借用检查器：两个 `mut` 绑定仍可能别名同一个列表。

## 📚 Documentation / 文档

| Document / 文档 | Purpose / 用途 |
|-----------------|----------------|
| [`tutorial.md`](tutorial.md) | users / 用户 — a hands-on tour of the language / 语言实践导览 |
| [`docs/GOALS.md`](docs/GOALS.md) | design authority / 设计权威 — the language design, roadmap, and staged milestones (S1–S8) / 语言设计、路线图与分阶段里程碑 |
| [`docs/NUMBERING.md`](docs/NUMBERING.md) | contributors / 贡献者 — historical map from old `v`/`L` labels to the unified `S` scheme / 旧 `v`/`L` 标签到新 `S` 编号的历史对照 |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | contributors / 贡献者 — branching, commit rules, and the bootstrap-fixpoint requirement / 分支策略、提交规范与自举定点要求 |
| [`SECURITY.md`](SECURITY.md) | reporting vulnerabilities privately / 私下报告安全漏洞 |
| [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | community conduct standards / 社区行为准则 |
| [`CHANGELOG.md`](CHANGELOG.md) | notable changes, latest first / 重要变更，最新优先 |
| [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) | PR template / 拉取请求模板 |

## ⚖️ License & Disclaimer / 许可与免责

This project is licensed under the [Apache License 2.0](LICENSE).

This project is developed and maintained by individual contributors on a voluntary, non-commercial basis.

This software is provided **"as is"**, without warranty of any kind. The author(s) accept no liability for any damages arising from the use of this software. See the [LICENSE](LICENSE) file for the full terms, including the disclaimer of warranty and limitation of liability.

Any commercial entity using this software is solely responsible for their own compliance with applicable laws and regulations, including but not limited to the EU Cyber Resilience Act (CRA) and any other regional requirements.
