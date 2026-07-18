[English](README.md) | 中文

# Burryn

[![License](https://img.shields.io/badge/license-Apache--2.0-lightgrey?style=flat-square)](LICENSE)
[![Bootstrapping](https://img.shields.io/badge/Bootstrapping--4a4a4a?style=flat-square)](docs/GOALS.md)

> **Meyrin 之下的环。** 一座洞穴，一枚戒指——在地下安静生长。

Burryn 是一门借鉴 Go 与 Rust 的小型编程语言。
它拥有手写词法分析器、递归下降解析器、零标注的 Hindley-Milner 类型推断、单遍字节码编译器、自带 mark-sweep 垃圾回收器与绿色线程调度器的栈式虚拟机，以及能把程序编译成独立原生二进制文件的 C 后端。

**编译器是自举的。**
`burc/` 用 Burryn 自身重实现了整条编译管线——词法、语法、类型检查、字节码编译、C 代码生成——约 7000 行。
`bur` 先把自身编译为 C，`cc` 再将其编译为原生二进制，而这个原生 `bur` 又能把同一份源码编译出逐字节相同的输出。
一个封闭的自举定点。
用于启动自举的原始 Go 实现已归档在 `archive/go-host` 分支。

## 名字

名字溯源：**burrow**（洞穴）是 gopher（Go 的吉祥物）的居所，又谐音 Rust 的 *borrow* checker。
**burr** 是锻造在金属上留下的毛刺。
*burrin* 则近似 **burin**——雕版师精细而安静的刻刀。

## 取自 Rust

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
  `enum Shape { Circle(r), Rect(w, h), Point }`。

## 取自 Go

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

## 语言速览

```sh
bur run examples/sieve.bur
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

## 示例

| 文件 | 说明 |
| ------ | ------ |
| `examples/hello.bur` | 基础、闭包、循环 |
| `examples/shapes.bur` | 枚举与 match |
| `examples/errors.bur` | Result、Option 与 `?` |
| `examples/sieve.bur` | 经典 CSP 素数筛：每个素数一个 fiber |
| `examples/pipeline.bur` | 带缓冲 channel 的生产者/消费者 |
| `examples/gc_stress.bur` | 观察垃圾回收器工作 |
| `examples/fib.bur` | 递归微基准 |
| `examples/brainfuck.bur` | 用 Burryn 写的 Brainfuck 解释器 |
| `examples/multiplex.bur` | 多 channel 的 `select` |
| `examples/streaming.bur` | channel 关闭与 for-in 排空 |
| `examples/textproc.bur` | 文件、exec、argv：小型文本工具 |
| `examples/wordcount.bur` | map 与字符串函数 |
| `examples/geometry/` | 多包模块示例（`bur.mod`、`import`、`pub`） |
| `burc/` | 自举编译器与 `bur` CLI——现存最大的 Burryn 程序 |

## 架构

```text
source --lexer--> tokens --parser--> AST --checker--> typed --compiler--> bytecode
 (lexer.bur)        (auto-semicolons)  (parser.bur) (types.bur, HM)   (compiler.bur)
||
                                         +--------------------------------+
|  |
                                         v                                v
                                   BurrynVM (vm.bur)           C backend (cgen.bur)
                                   fibers + channels            --> .c --cc--> native
                                   scheduler, GC                runtime/burrt*.h

Burryn is self-hosted: the whole pipeline lives in `burc/lib/` (token/lexer/
parser/types/compiler/cgen/module/vm .bur), and the `bur` CLI (`burc/main.bur`)
drives it. `bur` compiles itself to C, `cc` turns that into a native binary,
and the result rebuilds itself byte for byte.
```

- **编译器：** 单遍编译，clox 风格的局部变量/upvalue，配合临时值追踪，使 `match`/块表达式（会声明局部变量）可出现在任意表达式深度。
- **闭包：** upvalue 在打开时引用 `(fiber, slot)`，作用域退出时关闭，在 fiber 切换与栈增长时保持安全。
- **Match：** 编译为变体测试（基于枚举标识与 tag 的 `TEST_VARIANT`）和字段提取，无需哈希，无需反射。
- **GC：** 每个语言对象都在一个侵入式列表中；根包括全局变量、每个 fiber 的栈/帧/开放 upvalue/待发送的 channel 值。
- **调度器：** FIFO 就绪队列；fiber 在 channel 等待队列上 park；接收方/发送方直接交接值。

## 命令

```sh
bur run <file|dir>    类型检查并在 VM 上运行
bur <file|dir>        同上
bur check <file|dir>  仅类型检查（rustc 风格诊断）
bur build <file|dir>  经 C 编译为原生二进制（需要 cc/gcc/clang）
bur dis <file|dir>    反编译字节码
bur version           打印版本号
```

构建（自举）：原生 `bur` 用 `bur build burc -o bur` 构建自身。
要从零自举，请切出归档的 Go 宿主，由它编译出第一个 `bur`：

```sh
git checkout archive/go-host
go build -o bur.exe .          # 临时 Go 种子
./bur.exe build burc -o bur    # 种子编译自举 CLI
git checkout main
./bur build burc -o bur        # 此后 bur 自行重建
```

> **注意：** 从归档的 Go 宿主自举需要 **Go 1.26+**。
> C 后端与 `bur build` 需要 C99 编译器，如 `gcc` 或 `clang`。

## 安全

Burryn 是自举编译器与运行时。
安全敏感面包括：

- **编译器与生成 C**：`bur build` 会写入 `program.c` 并调用系统 C 编译器。
  运行不可信来源时请审计生成的 C 代码。
- **进程创建**：`exec` 原语调用 `fork` + `execvp`，不经过 shell。
- **持久化状态**：工具链会在工作目录写入构建产物（`program.c`、输出二进制与临时文件）。
- **网络端点**：`bur mod download` 与 `bur get` 通过网络执行 `git clone` 拉取依赖。
- **信任边界**：编译后的原生二进制与来自不可信来源的 `.bur` 源码都应视为不可信代码。

完整策略与私密报告方式见 [`SECURITY.md`](SECURITY.md)。

## 诚实局限

S1–S6、S7.1 与 S7.3 已完成。
覆盖范围包括语义内核、C 后端、模块、map、`select`/`close`、`mut` 参数、依赖工具、诊断、字符串插值、match guard，以及移除 Go 宿主后的完全自举编译器。
两个后端对整门语言（含并发）都能产生逐字节一致的输出。

仍缺失：records/structs（可用单变体枚举建模乘积类型）、管道、编译期常量、`defer` 与 net stdlib。
深 `mut` 是绑定级别的规则，不是借用检查器：两个 `mut` 绑定仍可能别名同一个列表。

## 文档

| 文档 | 用途 |
| ----------------- | ---------------- |
| [`tutorial.md`](tutorial.md) | 用户 —— 语言实践导览 |
| [`docs/GOALS.md`](docs/GOALS.md) | 设计权威 —— 语言设计、路线图与分阶段里程碑 |
| [`docs/NUMBERING.md`](docs/NUMBERING.md) | 贡献者 —— 旧 `v`/`L` 标签到新 `S` 编号的历史对照 |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | 贡献者 —— 分支策略、提交规范与自举定点要求 |
| [`SECURITY.md`](SECURITY.md) | 私下报告安全漏洞 |
| [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) | 社区行为准则 |
| [`CHANGELOG.md`](CHANGELOG.md) | 重要变更，最新优先 |
| [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md) | 拉取请求模板 |

## 许可与免责

本项目以 [Apache License 2.0](LICENSE) 授权。

本项目由个人贡献者在自愿、非商业基础上开发和维护。

本软件按“原样”提供，不附带任何形式的担保。
作者不对因使用本软件而产生的任何损害承担责任。
完整条款（包括免责声明与责任限制）见 [LICENSE](LICENSE) 文件。

任何使用本软件的商业实体，须自行负责遵守适用的法律法规，包括但不限于欧盟《网络弹性法案》(CRA) 及任何其他区域性要求。
