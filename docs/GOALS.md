# GOALS — Burryn 路线与里程碑

> v0.8 · active · 2026-09-07 ~ 09-13
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
| **S8 所有后端完工** | S8.2–S8.4 / S8.6 / S8.7 已实现；S8.1 / S8.5 因「x86 完成线收紧」标为部分实现（PIE / 节拆分 / 全 unwind 覆盖 / ELF unwind 待补，详见 §3）；S8.8–S8.10 未实现 | 部分实现 |
| **S9 LSP 与编辑器生态** | S9.1 核心服务器；S9.2 语言特性（清单见 §4）；S9.3 VSCode 扩展；S9.4 其他编辑器。前置 = S8.2 | 部分实现 |
| **S10 包生态** | 已有 std：`json`/`net`/`testing`/`cli`/`encoding`/`path`/`log`/`crypto`/`regex`。待扩展：`datetime`/`http`。S10.2 包模板；S10.3 `bur doc`；S10.4 包质量基础设施 | 部分实现 |

S1–S5 为自举闭环：`bur` 由本语言写成、经 cc 逐字节重建自身。
stdlib 按「够自举用 + owner 真实脚本需求」生长。
触及 `ty_unify` / token 编号 / 自举链的改动，改完必验 fixpoint（gen1 == gen2）。

## 3. S8：所有后端完工

S8 = 所有后端完工，一条完成线：C 后端 + x86 后端（Linux / Windows / macOS 三目标，且含模块包）+ LLVM + Cranelift + WASM 全部可用；验收闸仍是 `.github/workflows/ci.yml` 的全部 job 持续全绿。子项编号只用来排工作顺序，不得用来缩小 S8 的范围。

| 子项 | 内容 | 状态 |
|---|---|---|
| S8.1 | Linux ELF 单文件后端 | 已实现 |
| S8.2 / S8.3 / S8.4 / S8.7 | 语法冻结、row poly、封闭 record 按名合一、类型别名 | 已实现 |
| S8.5 | PE 与 Mach-O 序列化层，实心版 | 部分实现（见下方「x86 完成线收紧」） |
| S8.6 | x86 模块包 | 已实现 |
| S8.8 | LLVM 后端 | 未实现 |
| S8.9 | Cranelift 后端 | 未实现 |
| S8.10 | WASM 后端 | 未实现 |

内部顺序（2026-09-14 作者拍板，取代旧序）：**x86 后端彻底完工**（下方「x86 完成线收紧」全部条目清零）→ 多后端 S8.8 → S8.9 → S8.10 → **LSP（S9）彻底完工** → 基础生态。S10 新增包（`datetime`/`http` 等）暂缓，等前面几项收完再议。
> 本节曾有的 S8.5/S8.8/S8.9/S8.10 具体日期（09-28、10-12、10-26、11-09、11-23）是此前会话自排期，作者从未拍板，已删——这类工作节奏排期属 `reports/ROADMAP.md`（gitignored 本地文档）范畴，不该写进本文件冒充定案。

**x86 完成线收紧（2026-09-14 作者拍板）**：x86 后端（不分 S8.1/S8.5/S8.6 子项，统一一条线）须与 C 后端全面对齐，不留任何已知功能缺口，具体逐项：

- **`scripts/multi-backend-verify.sh` 全 PASS**：SKIP 恒为 0（不接受"跳过不算"），当前 91 pass / 0 fail / 0 skip 已达标，往后每次改动都不能倒退。
- **PIE**（三目标）：现状 ELF 固定基址静态执行、PE 无 ASLR 标记、Mach-O 显式 `MH_PIE` 未置位——三个都是位置相关。根因：后端约 488 处用 `mov_ri64` 内嵌按固定 `img_base()`/`macho_base` 算出的绝对地址，仅 8 处用 `lea_rip_rel32` 相对寻址；`architecture.md` 目前没有任何 PIE 机制设计。落地前需要先定设计（大改现有绝对寻址成 RIP 相对，或保留绝对寻址但按平台各自的重定位表格式在装载时打补丁——ELF `.rela.dyn`/`R_X86_64_RELATIVE`、PE `.reloc` 基址重定位目录、Mach-O chained fixups 的 rebase 项，三种格式互不相通）。范围与风险目前最大的一项，值得单独开一轮设计讨论，不建议直接开工。
- **节/段拆分（R-X 代码与 R-W 数据分开）**（三目标）：现状三个后端都是单一 RWX 段/节（ELF `phdr_load` 注释明写"RWX: GC runtime 槽需写"，PE `.bub`、Mach-O `__TEXT` 同构），因为运行时/GC 状态与代码挤在同一块里。要拆开需要先把这些可写状态搬进独立的可写段，是与 PIE 强相关但可能可以先行的一步（先拆分、仍固定基址，PIE 单独后续跟上）。
- **崩溃可诊断性对齐 C 的覆盖面**（三目标）：C 运行时的纤程切换走 `ucontext`/Win32 Fiber API（`runtime/burrt_impl.c` 的 `getcontext`/`bur_switch_to_sched`），这类系统级协程切换本身对栈回溯就是不透明的——所以 PE/Mach-O 已落地的"只覆盖用户函数，纤程切换点（yield/调度器恢复）不覆盖"这条边界，其实已经对齐 C 的真实能力，不是缺口。**真正的缺口**是：(a) GC/调度器里*不做*纤程切换、只是普通 `call`/`ret` 的内部子程序（`gc_record` 9-push、`mark_addr` 7-push、`chan_schedule`/`wake_waiters`/`push_queue`/`remove_waiter`/`waitset_add`/`timer_add`）目前一条都没纳入 unwind 覆盖，而 C 编译同等函数会自动拿到完整栈回溯信息——这些是普通序言，补齐是已交付机制（`pe_unwind_info`/`macho_unwind_info`）的直接扩展，每种序言形态多算一份编码，不是新架构；(b) **ELF 完全没有 unwind 机制**（无 `.eh_frame`，无 CFI），PE/Mach-O 已经做了各自的机制，Linux 目标这块是从零开始，且是第三种、又不同的格式（DWARF CFI）。
- **`net_nb` / fiber 感知 IO 真机验收**：Windows 侧仍卡 R1/R2（`net_loopback` 双修复复验、`WSAPoll` vs `select` P/Invoke 对照），等工作站真机回写，非代码缺口，状态不变，见 `reports/ROADMAP.md`。
- **multi-backend 已知缺陷显式登记**：`testdata/pkg/{annotations,cached,constcycle,consts,deepmut,extimport,extmissing,pipeline,stdjson}` 九例三方一致拒绝，此前只记在一份过期的 gitignored 报告里，未按 S8.1 完成线原文"显式登记为语言级限制"的要求写进 `architecture.md`；且该报告本身已发现有过时内容（其记录的元组解构丢类型 bug 其实已修，报告未随之更新），这九例本身也需要重新逐条核实，不能直接采信旧报告的归类。

**x86 自举仍不在 S8 内**：用 x86 编 compiler 不是后端完工的必要条件——[`architecture.md`](architecture.md) §3.5 的自举判定已由 C 路径满足。

类型系统扩展的取舍见 §7；行多态与封闭 record 的实现定案见 [`architecture.md`](architecture.md) §2.1。

## 4. S9 剩余

架构定案见 [`architecture.md`](architecture.md) §6（`bur lsp`、full sync、薄客户端）。

已有：S9.1 传输 + 文档同步 + 诊断；S9.3 VSCode 扩展；S9.2 的 hover、go-to-definition、作用域感知 completion（含 `pkg::` 成员）、formatting、signature-help（含 native 内建）、references、documentSymbol、documentHighlight、rename（含 prepareRename）。

未有：S9.4 JetBrains 与其他编辑器配置片段。

**单文档索引是当前精度上限**：`lsp_check_document` 把每份文档单独送进 `typecheck_program`，而 `lsp_set_recording(true)` 每次都清空录制数组，因此 references/documentHighlight/rename 与 go-to-definition 只在当前文档内成立；跨文件要先换成多文档录制。

顺序（2026-09-14 作者拍板，取代旧序）：S9.2 语言特性已收齐，下一步 S9.4——但 S9 整体排在 x86 完成线收紧与 S8.8–S8.10 之后，见 §3「内部顺序」。

## 5. S10

原则：能纯 Burryn 就不加 native；每包 `bur.mod` + `*_test.bur`，随 std_embed 分发。

**新增包暂缓（2026-09-14 作者拍板）**：`datetime`/`http` 等新包排在 x86 完成线收紧、S8.8–S8.10、S9 之后，见 §3「内部顺序」；已有 std 包的维护不受影响。

## 6. 后端次序

主线（细节见 §3）：x86 Linux ELF（S8.1，已完成）→ x86 模块包（S8.6，已完成）→ PE 与 Mach-O 序列化层（S8.5，部分实现）→ **x86 完成线收紧**（PIE / 节拆分 / unwind 覆盖对齐 C / multi-backend 零缺口，§3 详列）→ runtime 平台抽象与工具链探测 → LLVM（S8.8）→ Cranelift（S8.9）→ WASM（S8.10）→ LSP（S9）彻底完工 → 基础生态（S10 新增包）。
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
