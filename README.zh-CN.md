[English](README.md) | 中文

[![License](https://img.shields.io/badge/license-Apache--2.0-lightgrey?style=flat-square)](LICENSE)
[![Bootstrapping](https://img.shields.io/badge/Bootstrapping--4a4a4a?style=flat-square)](docs/SPEC.md)

# Burryn

> 一座洞穴，一枚戒指——在地下安静生长。

## 简介

Burryn 是一门借鉴 Go 与 Rust 的小型编程语言。
它拥有手写词法分析器、递归下降解析器、零标注的 Hindley-Milner 类型推断（需要时可选签名标注）、单遍字节码编译器、自带 mark-sweep 垃圾回收器与绿色线程调度器的栈式虚拟机、可移植的 C 后端，以及手写 x86-64 Linux ELF 后端（单文件程序；剩余工作见 `docs/GOALS.md`）。

**编译器是自举的。**
`compiler/` 用 Burryn 自身重实现了整条编译管线——词法、语法、类型检查、字节码编译、C 代码生成、虚拟机、模块加载、工具链与 `bur` CLI。
`bur` 先把自身编译为 C，`cc` 再将其编译为原生二进制，而这个原生 `bur` 又能把同一份源码编译出逐字节相同的输出。
一个封闭的自举定点。
用于启动自举的原始 Go 实现已归档在 `archive/go-host` 分支。

## 名字

名字溯源：**burrow**（洞穴）是 gopher（Go 的吉祥物）的居所，又谐音 Rust 的 *borrow* checker。
**burr** 是锻造在金属上留下的毛刺。
*burrin* 则近似 **burin**——雕版师精细而安静的刻刀。

## 环境要求

- 一份原生 `bur` 二进制，或用 **Go 1.26+** 从 `archive/go-host` 自举
- C99 编译器（`gcc` 或 `clang`），用于 `bur build` 以及重建 `bur`
- Linux，用于 x86-64 ELF 后端（`bur build --backend x86`）

## 特性

### 取自 Rust

- **默认不可变。**
  `let x = 1` 不可重新赋值——在*编译期*强制执行。
  要可变必须用 `let mut`。
  深度生效：普通 `let` 会冻结列表内容，`push`、`pop`、`l[i] = v` 都需要 `mut` 绑定。
- **没有 null。**
  缺失用 `Option`（`Some(v)` / `None`），失败用 `Result`（`Ok(v)` / `Err(e)`）。
  两者都是内建枚举。
- **`?` 运算符。**
  自动展开 `Ok`/`Some`，否则立即把 `Err`/`None` 返回给调用方。
- **`match` 表达式。**
  支持枚举解构、字面量分支、绑定与 `_`，可在任何需要表达式的位置使用。
- **表达式导向。**
  `if`、`match` 与 `{}` 块都有值。
  函数返回其最后一个表达式。
- **遮蔽（Shadowing）。**
  `let x = x + 1` 重新绑定，与 Rust 一致。
- **代数数据类型。**
  `enum Shape { Circle(float), Rect(int, int), Point }`。

### 取自 Go

- **GC 替代借用检查。**
  手写 mark-sweep 直接回收虚拟机自身堆。
  可用 `gc()`、`heap_objects()`、`gc_cycles()` 观察。
- **绿色线程。**
  `spawn worker(ch)` 在虚拟机调度器上启动一个 fiber——协作式调度，外加 1 万条指令的时间片，防止自旋 fiber 饿死其他 fiber。
  单线程交错意味着**从结构上避免数据竞争**。
- **Go 风格的 channel。**
  `ch <- v` 发送，`<-ch` 接收。
  无缓冲 channel 直接会面；`chan(n)` 带缓冲。
  当所有 fiber 都阻塞时会被检测并报为死锁。
  主 fiber 返回时程序结束。
- **无分号。**
  换行结束语句（Go 风格的自动分号插入），所以 `} else` 要写在同一行。

## 示例

```sh
$ bur run examples/concurrency/sieve.bur
```

```text
enum Shape { Circle(float), Rect(int, int) }

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

// capture(ref) 与 mut 局部共享；默认是 copy-at-capture
fn make_counter() {
    let mut n = 0
    fn() capture(ref n) { n = n + 1
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

| 文件 | 说明 |
| ------ | ------ |
| `examples/basics/hello.bur` | 基础、闭包、循环 |
| `examples/basics/fib.bur` | 递归微基准 |
| `examples/basics/numeric.bur` | 数值转换：trunc、to_float、parse_float、float_bits |
| `examples/basics/args.bur` | 命令行参数 |
| `examples/basics/textproc.bur` | 字符串与列表原语：split、trim、join、slice、concat |
| `examples/basics/interpolation.bur` | 字符串插值 `{expr}` |
| `examples/basics/cleanup.bur` | defer：LIFO 清理、闭包捕获 |
| `examples/types/shapes.bur` | 枚举与 match |
| `examples/types/errors.bur` | Result、Option 与 `?` |
| `examples/types/constants.bur` | 编译期 const 折叠 |
| `examples/types/match_guard.bur` | 带 `if` guard 的 match 臂 |
| `examples/types/pipeline_op.bur` | 管道操作符 `\|>` |
| `examples/concurrency/sieve.bur` | 经典 CSP 素数筛：每个素数一个 fiber |
| `examples/concurrency/pipeline.bur` | 带缓冲 channel 的生产者/消费者 |
| `examples/concurrency/multiplex.bur` | 多 channel 的 `select` |
| `examples/concurrency/streaming.bur` | channel 关闭与 for-in 排空 |
| `examples/concurrency/yield.bur` | fiber 间显式协作让出 |
| `examples/net/net_loopback.bur` | TCP 监听 + 拨号回显交换 |
| `examples/net/net_nb.bur` | socket 非阻塞 accept/read/write |
| `examples/io/fs.bur` | read_file/write_file/file_exists/read_dir 与错误路径 |
| `examples/io/exec.bur` | 同步 exec：Output、退出码 |
| `examples/programs/brainfuck.bur` | 用 Burryn 写的 Brainfuck 解释器 |
| `examples/programs/wordcount.bur` | map 与字符串函数 |
| `examples/programs/gc_stress.bur` | 观察垃圾回收器工作 |
| `examples/programs/geometry/` | 多包模块示例（`bur.mod`、`import`、`pub`） |
| `compiler/` | 自举编译器与 `bur` CLI——现存最大的 Burryn 程序 |

## 架构

```text
source --lexer--> tokens --parser--> AST --checker--> typed --compiler--> bytecode
 (lexer.bur)        (auto-semicolons)  (parser.bur) (types.bur, HM)   (compiler.bur)
||
                                         +-------------------+-------------------+
                                         |                   |
                                         v                   v
                                   BurrynVM               原生后端
                                   (vm.bur)               - C backend (backends/c/cgen.bur)
                                   fibers + GC            - x86-64 ELF (backends/x86/)

Burryn 已完成自举：整条管线位于 `compiler/` 下（`frontend/`、`bytecode/`、
`backends/`、`module/`、`tooling/`），`bur` CLI 位于 `compiler/main.bur`。
`bur` 先把自身编译为 C，`cc` 再将其编译为原生二进制，而该结果又能逐字节重建自己。
```

- **编译器：** 单遍编译，clox 风格的局部变量/upvalue，配合临时值追踪，使 `match`/块表达式（会声明局部变量）可出现在任意表达式深度。
- **闭包：** 捕获语义见 `docs/SPEC.md` §6.1。
- **Match：** 编译为变体测试（基于枚举标识与 tag 的 `TEST_VARIANT`）和字段提取，无需哈希，无需反射。
- **GC：** 每个语言对象都在一个侵入式列表中；根包括全局变量、每个 fiber 的栈/帧/闭包 cell/待发送的 channel 值。
- **调度器：** FIFO 就绪队列；fiber 在 channel 等待队列上 park；接收方/发送方直接交接值。

## 命令

```sh
$ bur run <file|dir>    类型检查并在 VM 上运行
$ bur <file|dir>        同上
$ bur check <file|dir>  仅类型检查（rustc 风格诊断）
$ bur build <file|dir>  经 C 编译为原生二进制（需要 cc/gcc/clang）
$ bur build --backend x86 <file.bur> -o <out>  编译为 x86-64 ELF 原生二进制
$ bur fmt <file|dir|->  按官方格式重写源码
$ bur dis <file|dir>    反编译字节码
$ bur test [dir]        运行 *_test.bur 中的 test_* 函数
$ bur mod <subcommand>  模块管理（init、tidy、download、verify）
$ bur get <path>@<ver>  添加或升级模块依赖
$ bur lsp               language server（stdin/stdout JSON-RPC）
$ bur version           打印版本号
```

原生 `bur` 重建自身：

```sh
$ bur build compiler -o bur
```

从零自举（归档 Go 宿主）：

```sh
$ git worktree add ../go-host archive/go-host
$ (cd ../go-host && go build -o ../bur-seed .)
$ (cd ../go-host && ../bur-seed build burc -o ../bur-base)
$ ./bur-base build compiler -o bur
```

## 安全

敏感面：生成的 C、`exec`、构建产物、模块的 `git clone`、不可信 `.bur`。
策略与私密报告：[`SECURITY.md`](SECURITY.md)。

## 文档

| 文档 | 用途 |
| ----------------- | ---------------- |
| [`tutorial.zh-CN.md`](tutorial.zh-CN.md) | 用户 —— 语言实践导览（[English](tutorial.md)） |
| [`docs/grammar.md`](docs/grammar.md) | 已冻结的表层 grammar |
| [`docs/SPEC.md`](docs/SPEC.md) | 设计权威 —— 语言定案与拒绝清单 |
| [`docs/GOALS.md`](docs/GOALS.md) | 路线 —— 分阶段里程碑（S1–S10）与剩余完成线 |
| [`docs/NUMBERING.md`](docs/NUMBERING.md) | 贡献者 —— 旧 `v`/`L` 标签到新 `S` 编号的历史对照 |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | 贡献者 —— 分支策略、提交规范与自举定点要求 |
| [`SECURITY.md`](SECURITY.md) | 私下报告安全漏洞 |
| [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | 社区行为准则 |
| [`CHANGELOG.md`](CHANGELOG.md) | 重要变更，最新优先 |
| [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) | 拉取请求模板 |
| 本地 `reports/`（gitignore） | 工作笔记 —— 不是权威 |

## 诚实局限

缺口见 [`docs/GOALS.md`](docs/GOALS.md)。当前可观察：x86-64 ELF 仅 Linux、仅单文件；std 没有 `sort`、`getenv`、regex。

## 许可与免责

本项目以 [Apache License 2.0](LICENSE) 授权。

本项目由个人贡献者在自愿、非商业基础上开发和维护。

本软件按“原样”提供，不附带任何形式的担保。
作者不对因使用本软件而产生的任何损害承担责任。
完整条款（包括免责声明与责任限制）见 [LICENSE](LICENSE) 文件。

任何使用本软件的商业实体，须自行负责遵守适用的法律法规，包括但不限于欧盟《网络弹性法案》(CRA) 及任何其他区域性要求。
