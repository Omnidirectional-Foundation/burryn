# GOALS — Burryn 路线与里程碑

> v0.6 · active · 2026-08-22
> 状态：前瞻规划 · 编号 `S<n>[.<m>]`
> 相关文档：[`SPEC.md`](SPEC.md) 已定设计 · [`NUMBERING.md`](NUMBERING.md) 旧编号对照 · [`grammar.md`](grammar.md) 表层语法 · [`../README.md`](../README.md)

> **注意：** 本文档管阶段、完成线、未开工项。语言/ABI/拒绝清单在 [`SPEC.md`](SPEC.md)；冲突以 SPEC 为准。
> 新的设计决策先问 owner，写入 SPEC，不堆在本文件当「已定」。

## 1. 编号

全项目单一编号 `S<n>[.<m>]`：`S<n>` 为阶段，`S<n>.<m>` 为阶段内可独立验收（自举 fixpoint）的模块。
旧 `v1/v2/v3/v4`、`L1/L2`、旧「S4 工具链」作废，对照 [`NUMBERING.md`](NUMBERING.md)。

状态标记：已完成 / 进行中 / 部分实现 / 未开工。

| 阶段 | 子项 | 状态 |
|------|------|------|
| **S1 语义内核** | S1.1 HM 全程序推导；S1.2 穷尽性检查；S1.3 GC；S1.4 CSP 基础 | 已完成 |
| **S2 C 后端与语言完备** | S2.1–S2.7：C 后端、模块、map、`select`/`close`、深 `mut`、`pub`、必要 stdlib | 已完成 |
| **S3 自举前端** | 编译器前端由 Burryn 写成并编译自己 | 已完成 |
| **S4 重写 VM** | VM 由 Burryn 重写，经 cc 编成原生 | 已完成 |
| **S5 删 Go** | CLI 用 Burryn 写；main 清零 Go；`archive/go-host` 留档 | 已完成 |
| **S6 生态工具链** | S6.1–S6.8：依赖、fmt、test、诊断、std/json、runtime IO、checker 债 | 已完成 |
| **S7 语言特性扩展** | S7.1–S7.8（S7.4 命名参数已否决，编号保留） | 已完成 |
| **S8 后端与重型类型** | S8.1 Linux ELF 单文件（完成条件见 SPEC §6.1）；S8.2 语法冻结 **已完成**；S8.3 row poly **已完成**；S8.4 封闭 record 按名合一 **已完成**；S8.5 PE（前提 = runtime Windows 移植）；S8.7 类型别名 **已完成** | 进行中 |
| **S9 LSP 与编辑器生态** | S9.1 核心服务器；S9.2 hover / go-to-def / completion / formatting / signature-help；S9.3 VSCode 扩展；S9.4 其他编辑器。前置 = S8.2 | 部分实现 |
| **S10 包生态** | 已有 std：`json`/`net`/`testing`/`cli`/`encoding`/`path`。待扩展：`log`/`datetime`/`regex`/`crypto`/`http`。S10.2 包模板；S10.3 `bur doc`；S10.4 包质量基础设施 | 未开工 |

S1–S5 为自举闭环：`bur` 由本语言写成、经 cc 逐字节重建自身。
stdlib 按「够自举用 + owner 真实脚本需求」生长。
触及 `ty_unify` / token 编号 / 自举链的改动，改完必验 fixpoint（gen1 == gen2）。

## 2. S8 剩余

S8.2 / S8.3 / S8.4 / S8.7 已完成。S8.1 与 S8.5 未完成。
完成条件、排除项、Mach-O 不接在 ELF 之后：见 SPEC §6.1。
内部顺序：先收 S8.1，再 S8.5。

## 3. S9 剩余

架构定案见 SPEC §6.2（`bur lsp`、full sync、薄客户端）。

已有：S9.1 传输 + 文档同步 + 诊断；S9.2 的 hover 与 go-to-definition；S9.3 VSCode 扩展。

未有：completion、formatting、signature-help；S9.4 JetBrains 与其他编辑器配置片段。

顺序：先收齐 S9.2 剩余三项，再 S9.4。S9 整体在 S8.1 完成线之后推进。

## 4. S10

原则：能纯 Burryn 就不加 native；每包 `bur.mod` + `*_test.bur`，随 std_embed 分发。
未开工。

## 5. 后端次序

主线：x86 Linux ELF（S8.1）→ runtime 平台抽象与工具链探测 → LLVM → Cranelift → WASM。
Mach-O / PE 的开工条件见 SPEC §6.1；值模型与 WASM CSP 见 SPEC §3。不在此复述。

## 6. 独立工作项（非语言语义）

跨进程 TCP idle-wait 是否优化：不改 SPEC §6.3。`poll(2)` 升 `epoll` 见本地 `reports/backlog-net-io-runtime.md`，不进本文件完成线。
