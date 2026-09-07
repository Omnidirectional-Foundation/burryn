# GOALS — Burryn 路线与里程碑

> v0.8 · active · 2026-09-07
> 状态：前瞻规划 · 编号 `S<n>[.<m>]`
> 相关文档：[`architecture.md`](architecture.md) 实现权威 · [`NUMBERING.md`](NUMBERING.md) 旧编号对照 · [`grammar.md`](grammar.md) 表层语法 · [`../README.md`](../README.md)

> **注意：** 本文档管阶段、完成线、未开工项与明确排除项。语言定案、ABI 与实现架构在 [`architecture.md`](architecture.md)；冲突以 architecture.md 为准。
> 新的设计决策先问 owner，写入 architecture.md，不堆在本文件当「已定」。

## 1. 编号

全项目单一编号 `S<n>[.<m>]`：`S<n>` 为阶段，`S<n>.<m>` 为阶段内可独立验收（自举 fixpoint）的模块。
旧 `v1/v2/v3/v4`、`L1/L2`、旧「S4 工具链」作废，对照 [`NUMBERING.md`](NUMBERING.md)。

状态取值：已实现 / 部分实现 / 未实现。

## 2. 阶段表

| 阶段 | 子项 | 状态 |
|------|------|------|
| **S1 语义内核** | S1.1 HM 全程序推导；S1.2 穷尽性检查；S1.3 GC；S1.4 CSP 基础 | 已实现 |
| **S2 C 后端与语言完备** | S2.1–S2.7：C 后端、模块、map、`select`/`close`、深 `mut`、`pub`、必要 stdlib | 已实现 |
| **S3 自举前端** | 编译器前端由 Burryn 写成并编译自己 | 已实现 |
| **S4 重写 VM** | VM 由 Burryn 重写，经 cc 编成原生 | 已实现 |
| **S5 删 Go** | CLI 用 Burryn 写；main 清零 Go；`seed/go-host` 留档 | 已实现 |
| **S6 生态工具链** | S6.1–S6.8：依赖、fmt、test、诊断、std/json、runtime IO、checker 债 | 已实现 |
| **S7 语言特性扩展** | S7.1–S7.8（S7.4 命名参数已否决，编号保留） | 已实现 |
| **S8 所有后端完工** | S8.1–S8.4 / S8.7 已实现；S8.5 / S8.6 / S8.8–S8.10 未实现（排期见 §3） | 部分实现 |
| **S9 LSP 与编辑器生态** | S9.1 核心服务器；S9.2 hover / go-to-def / completion / formatting / signature-help；S9.3 VSCode 扩展；S9.4 其他编辑器。前置 = S8.2 | 部分实现 |
| **S10 包生态** | 已有 std：`json`/`net`/`testing`/`cli`/`encoding`/`path`。待扩展：`log`/`datetime`/`regex`/`crypto`/`http`。S10.2 包模板；S10.3 `bur doc`；S10.4 包质量基础设施 | 未实现 |

S1–S5 为自举闭环：`bur` 由本语言写成、经 cc 逐字节重建自身。
stdlib 按「够自举用 + owner 真实脚本需求」生长。
触及 `ty_unify` / token 编号 / 自举链的改动，改完必验 fixpoint（gen1 == gen2）。

## 3. S8：所有后端完工

S8 = 所有后端完工，一条完成线：C 后端 + x86 后端（Linux / Windows / macOS 三目标，且含模块包）+ LLVM + Cranelift + WASM 全部可用；验收闸仍是 `.github/workflows/ci.yml` 的全部 job 持续全绿。子项编号只用来排工作顺序，不得用来缩小 S8 的范围。

| 子项 | 内容 | 状态 |
|---|---|---|
| S8.1 | Linux ELF 单文件后端 | 已实现 |
| S8.2 / S8.3 / S8.4 / S8.7 | 语法冻结、row poly、封闭 record 按名合一、类型别名 | 已实现 |
| S8.5 | PE 与 Mach-O 序列化层，实心版 | 未实现，死线 2026-09-28 |
| S8.6 | x86 模块包 | 未实现，死线 2026-10-12 |
| S8.8 | LLVM 后端 | 未实现，死线 2026-10-26 |
| S8.9 | Cranelift 后端 | 未实现，死线 2026-11-09 |
| S8.10 | WASM 后端 | 未实现，死线 2026-11-23 |

S8 全阶段 2026-11-23 收线。内部顺序：先收 S8.1，再 S8.5，再 S8.6，然后 S8.8 → S8.9 → S8.10。

**S8.1 完成线（定案，已实现）**：Linux ELF 单文件程序后端（`bur build --backend x86 <file.bur>`）。
完成条件 = fiber 感知 IO + `net_nb` 落地 + multi-backend 已知缺陷清零或显式登记为语言级限制。
自举判定维持 [`architecture.md`](architecture.md) §3.5：编译器由本语言写成且能编译自己；输出 C 再经 cc 落地，完全算自举。

**S8.5 实心版（定案）**：PE 与 Mach-O 序列化层，与 S8.1 同一后端的另外两个目标（`bur build --backend x86 --os windows|darwin`）；Mach-O 不另开编号，与 PE 同属 S8.5。
完成条件 = 两目标可编 + spawn / sleep / fs / net / exec 语料在 macOS 与 Windows 真机 job 全绿 + 共用验收闸持续全绿；hello 级全绿即收的 hollow 版不算完成。
**不含**加固填字段（UUID、DllCharacteristics flag 位不阻塞收线）；PIE 与节拆分属序列化层本职，在完成线之内。

**S8.6 x86 模块包（定案）**：模块包并入 S8，不另开编号。`compile_to_x86` 的 `is_dir` 拒收去掉，三目标共用这条路径。

**x86 自举仍不在 S8 内**：用 x86 编 compiler 不是后端完工的必要条件——[`architecture.md`](architecture.md) §3.5 的自举判定已由 C 路径满足。

类型系统扩展的取舍见 §7；行多态与封闭 record 的实现定案见 [`architecture.md`](architecture.md) §2.1。

## 4. S9 剩余

架构定案见 [`architecture.md`](architecture.md) §6（`bur lsp`、full sync、薄客户端）。

已有：S9.1 传输 + 文档同步 + 诊断；S9.2 的 hover 与 go-to-definition；S9.3 VSCode 扩展。

未有：completion、formatting、signature-help；S9.4 JetBrains 与其他编辑器配置片段。

顺序：先收齐 S9.2 剩余三项，再 S9.4。S9 整体在 S8.1 完成线之后推进。

## 5. S10

原则：能纯 Burryn 就不加 native；每包 `bur.mod` + `*_test.bur`，随 std_embed 分发。

## 6. 后端次序

主线（编号与排期见 §3）：x86 Linux ELF（S8.1，已完成）→ PE 与 Mach-O 序列化层（S8.5）→ x86 模块包（S8.6）→ runtime 平台抽象与工具链探测 → LLVM（S8.8）→ Cranelift（S8.9）→ WASM（S8.10）。
后端矩阵、工具链探测与值模型见 [`architecture.md`](architecture.md) §3，不在此复述。

## 7. 明确排除（不接受重新提案）

### 7.1 语言层

护住简洁，下列项不接受重新提案：

- 宏 / 元编程
- trait / typeclass（S8 以后才可重新讨论）
- async/await（CSP 是唯一并发模型，不做第二套）
- 继承
- 异常
- 运算符重载
- 隐式类型转换
- null

### 7.2 类型系统重型项

S8 工程评估的取舍：**纳入** row polymorphism（S8.3，首位）→ 封闭 record（S8.4）。
复用现有 var/generalize 加「行 var」，扁平 if-链撑得住；这是结构化接口的公共地基，唯一值得投入的重型项。其余排除：

- **Effects**：与现有 CSP（fiber/channel/select）竞争控制流转移，CSP 已覆盖 IO/并发大半实用场景，边际价值与代价不成比例
- **Refinement Types**：无 constraint solver 地基，须从零造子系统；与轻标注工程气质冲突（Rust 未上）
- **GADTs**：动 HM 最微妙处，通用工程价值最低
- **Linear（全局）**：永不
- **backlog · 局部 Affine（资源）**：file/socket/channel 的 use-after-close 检查，流敏感 lint 级，不碰 GC，能把 close-of-closed-channel 运行时 trap 提前为编译期错误

### 7.3 运行时与 IPC

- **Unix domain socket 原语：不做**。TCP loopback 保持本机双向流式通信的唯一原语。跨进程延迟主导在调度器 idle-wait，加 AF_UNIX 不解决该瓶颈，还多一套 native 并与 S8.5 的 Windows 移植冲突。fiber 感知 IO 落地且 waitset 成为瓶颈后再议
- **内置 IPC 消息协议：不做**。维持 `std/json` + 使用者自行分帧。类 Erlang 内置消息格式等于第二套运行时协议，与「显式优先」冲突
- **`std/procpool` 一等能力：不做**。supervisor/worker 维持「模式可用，语言不提供额外支持」。一等化会倒逼流式 exec stdin，与 [`architecture.md`](architecture.md) §7.1 的 exec 收尾式、非流式定案冲突
- **x86 后端 fiber 感知 IO：做**。约束见 [`architecture.md`](architecture.md) §5.7，完成线见 §3

## 8. 独立工作项（非语言语义）

跨进程 TCP idle-wait 是否优化：不改 [`architecture.md`](architecture.md) §7.2 的语义。
`poll(2)` 升 `epoll` 见本地 `reports/backlog-net-io-runtime.md`，不进本文件完成线。
