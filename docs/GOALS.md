# GOALS — Burryn 路线与里程碑

> v0.8 · active · 2026-09-07 ~ 09-24
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
| **S8 所有后端完工** | S8.2–S8.4 / S8.6 / S8.7 已实现；S8.1 / S8.5 因「x86 完成线收紧」余项标为部分实现（见 §3）；S8.8–S8.10 未实现 | 部分实现 |
| **S9 LSP 与编辑器生态** | S9.1 核心服务器；S9.2 语言特性（清单见 §4）；S9.3 VSCode 扩展；S9.4 其他编辑器。前置 = S8.2 | 部分实现 |
| **S10 包生态** | 已有 std：`json`/`net`/`testing`/`cli`/`encoding`/`path`/`log`/`crypto`/`regex`。待扩展：`datetime`/`http`。S10.2 包模板；S10.3 `bur doc`；S10.4 包质量基础设施 | 部分实现 |

S1–S5 为自举闭环：`bur` 由本语言写成、经 cc 逐字节重建自身。
stdlib 按「够自举用 + owner 真实脚本需求」生长。
触及 `ty_unify` / token 编号 / 自举链的改动，改完必验 fixpoint（gen1 == gen2）。

## 3. S8：所有后端完工

S8 = 所有后端完工，一条完成线：C 后端 + x86 后端（Linux / Windows / macOS 三目标，且含模块包）+ LLVM + Cranelift + WASM 全部可用；验收闸仍是 `.github/workflows/ci.yml` 的全部 job 持续全绿。子项编号只用来排工作顺序，不得用来缩小 S8 的范围。

| 子项 | 内容 | 状态 |
|---|---|---|
| S8.1 | Linux ELF 单文件后端 | 部分实现（见下方「x86 完成线收紧」） |
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
- **PE / Mach-O 功能对齐 Linux** ✅ 已完成（2026-09-24）：`scripts/xos-corpus.sh` 把 examples 与 `testdata/{basics,types,regression}` 的全部样例（70 例，另加带参 `args` 与文件重定向 `stdin` 两例）以 Linux x86 的 stdout 与退出码为基准，在真 Windows/macOS runner 上逐例对照，两端全过后转为 hard gate。摸出的缺口逐项补齐：`args`（Windows 经分派器合成号 1003 由 `CommandLineToArgvW` + UTF-8 转换合成启动向量；darwin 取 LC_MAIN 入口 `rsi-8`）、Windows 监听端口独占（`SO_REUSEADDR` 译成 `SO_EXCLUSIVEADDRUSE`）、Mach-O exec 族（`fork`/`execve`/`wait4`/`dup2`/`pipe2`/`nanosleep` 译 BSD 调用）、Windows exec（合成号 1004 走 `CreatePipe` + `CreateProcessA`，管道读端 `PeekNamedPipe` 先窥后读，`wait4` 走 `WaitForSingleObject` + `GetExitCodeProcess`），见 `architecture.md` §5.23。
- **运行时检查与错误文本对齐 C**（2026-09-24 登记）：此前的比对只看 stdout 与退出码、样例几乎不触发 trap，掩盖了 x86 与 C/VM 的大面积差异，实测：(a) x86 缺运行时安全检查——列表/元组下标越界、`char_at`/`substr`/`slice` 越界、空列表 `pop`、`ord("")`、非法 `chr`、整数加减乘与取负溢出均不报错（读到垃圾值、退出码 0），除零/取模零直接 SIGFPE，`assert` 失败静默退出 1，`exec_poll` 非法句柄段错误；C/VM 一律 `runtime error: <msg>` 加逐帧 `  at <fn> (<file>:<line>)` 回溯、退出码 4。(b) 已有 trap（通道关闭、`net_close`、死锁）文案不一致：缺 `runtime error: ` 前缀、换行与回溯，死锁不报剩余纤程数。(c) I/O 与网络的 `Err` 文本 x86 一律 `os error`，C 为 `strerror`/`gai_strerror` 文本并带 `tcp_dial:` 等前缀。(d) C 自身栈溢出时打印消息后段错误（退出码 139 而非 VM 的 4）。完成判据：同一程序在三平台 x86 上 stdout、stderr、退出码与同平台 C 完全一致；multi-backend 与跨平台语料改为比对 stderr，新增覆盖每类 trap 与每类 `Err` 文本的样例；跨平台语料并入 `testdata/pkg` 与 std 包测试。
  - **进展（2026-09-24）**：(a)(b)(d) 已完成——x86 共享 die 子程序 + 源码回溯表，全部检查项与 C 同文同回溯、退出码 4；死锁报剩余纤程数；调用深度上限与 C 一致（VM 原先差一层，一并改正）；C 栈溢出 trap 不再段错误；另补全局定义前读写的 `undefined variable`、`chan` 负容量、`net_nb` 非法操作码，exec 句柄改为与 C 相同的自 0 顺序编号，见 `architecture.md` §5.24。验证：新增 `testdata/traps/`（32 例，stdout/stderr/退出码逐字节），multi-backend 三方比对 stderr，123/0/0。
  - **值格式化与 exec（2026-09-24）**：x86 格式化重写为运行期构建器 + 按类型生成的子程序，容器内字符串转义、嵌套 payload、bool/unit 与 C 一致（引号规则统一为 VM 的 `go_quote`，C 同步改正），见 `architecture.md` §5.25；同步 `exec` 等待期间让出给其他 fiber（此前阻塞整个进程，输出次序与 C 不同）；空目录 `read_dir` 段错误已修。
  - **Err 文本与诊断列号（2026-09-26，WSL ZCode 值机）**：(c) 完成——x86 的 `tcp_listen`/`tcp_dial`/`tcp_accept`/`net_read`/`net_write` 失败路径改经 errtext 子程序拼 `<op>: ` + 平台 strerror 文本（与 C 的 `bur_net_err` 同构）；glibc/darwin/MSVC 三张 strerror 表逐字取自 CI「Dump C error texts」步骤的 dump，越界码格式逐平台（glibc `Unknown error N`、darwin `Unknown error: N`、MSVC 不带号码）；不可解析主机按平台 `gai_strerror(EAI_NONAME)` 文本应答。单文件 x86 诊断列号偏移已修（float_rt 注入基差在渲染前经 `emit_errors_base`/`shift_diag` 平移回用户坐标）。验证：三方探针（dial refused/DNS 文本、诊断列号 4:9）逐一对照 C，testdata 全门 + fixpoint gen1==gen2（`d4fb512`/`b88c745`）。**域名解析限制已解除（2026-09-26，`aef85ec`）**：x86 注入纯 Burryn 桩解析器 `compiler/lib/dns_rt.bur`（/etc/hosts → localhost 兜底 → resolv.conf nameserver 的 TCP/53 A 查询，RFC 1035 §4.2 长度帧），拨名与 C 同机成功，`.invalid` 负例与 C 的 gai 文本逐字一致；windows 目标暂不注入（该目标未验 hosts 语义，dial 路径与旧版逐字节一致）。`net_nb` 非 EAGAIN 的 Err 文本与 `exec`/`exec_start` 的 spawn 失败文本仍为 `os error`（C 的前缀口径待核后补）。
  - **stderr 比对与 pkg/std 并入语料（2026-09-26 核实翻新）**：早前登记的「尚未比对 stderr、未并入 pkg 与 std 测试」两条**已过时**——同平台 x86 vs C 步早已比对 stderr（Windows `$e1 -eq $e2` 先 CRLF 归一，macOS `cmp -s x-err.txt c-err.txt`，口径本就是同平台对照而非对 Linux 基线——strerror 文本天然逐平台）；`testdata/pkg` 早已并入语料（`0f80429`，`sccorder` 因印地址剔除），std 包测试并入（`4ad4215`，windows/darwin 语料各 89 例 BUILD_FAIL=0）。
  - **Err 文本残项（登记）**：上述 net_nb/exec spawn 的 `os error` 残项——C 口径已核实（2026-09-26）：POSIX 的 spawn/pipe/fork 同步失败打 `strerror(真 errno)` 无前缀，Windows 打固定文本 `cannot create pipe`/`cannot start process`；x86 的 spawn 同步失败路径当前把 errno 折叠成 `-1`（fail_tail/fork_fail/win_fail 统一 `mov -1`），直接换 `err_from_errno` 会报成 EPERM 文本，须先在失败链携带真 errno、Windows 臂改按固定串分型，此前 `d624c34` 的换法不达 C 文本故维持回退。map 函数值未印 `<fn name>` 一项已完成（2026-09-26，`b9330b3`：fn 名表 + closure 类型跟踪 + map 词表扩类，`testdata/types/fn_map` 三方一致）。
  - **堆模型差异（2026-09-26 登记，`reports/ROADMAP.md` §17）**：x86 目标堆为一次性 16MB 非回收 bump 区（`_start` mmap/VirtualAlloc），`gc_collect` 只紧缩分配记录表、**对象不移动、字节不复用、`r12` 永不回卷**；C 后端同路径 `realloc`+`free` 真复用。故 **x86 程序全生命周期累计分配不得超 16MB**：现有语料/标准测试全在预算内，但持续大分配负载（多轮大输出 `exec`、长跑服务）会写越 mmap 边界崩溃（0xC0000005/SIGSEGV）——byte-patch 判别实锤：压测 PE 堆立即数 16→256MB，崩溃轮从 2–3 线性后移至 43–48。补齐须在全部分配点引入回收（独立立项，建议单列排期）；本条不计入完成线勾销，登记为 x86 后端当前能力边界。
  - **macOS runner exec 小例间歇 SIGSEGV（2026-09-26 登记，§18，未决）**：`x86-macho-run` 语料对照步间歇崩 `examples_io_exec`/`exec_large`（rc 139，b9330b3–264b3af 时代 5 run 崩 3），二者总分配 <150KB、与上述堆模型无关；「peek/EOF 当 EOF 提前关管」旧假说已被 `pe.bur` read 臂代码证伪（kind==2 臂已区分真 EOF 与 -EAGAIN）；本机 Intel-win32 用当前编译器产物连跑 300 次全净。CI mac job 已加临时探针步（连跑 30+30 并解析 ReportCrash `.ips` 定位出错指令），根因定案前「三平台完全一致」对此项保持开放。
- **PIE**（三目标）✅ 已完成（2026-09-24）：代码取址一律 RIP 相对、数据不存绝对指针，镜像零重定位项——以探针基址与真实基址各装配一遍，逐字节差分定位 `mov imm64` 地址站点并原地改写成 `lea rip`，未收口的绝对地址报内部错误；跳转表存相对偏移，地址槽由 `_start` 运行期写入。ELF `ET_DYN`（static-pie，`.eh_frame` 改 pcrel）、PE `DYNAMIC_BASE | HIGH_ENTROPY_VA | NX_COMPAT` 加空重定位目录、Mach-O `MH_PIE`，见 `architecture.md` §5.22。
- **节/段拆分（R-X 代码与 R-W 数据分开）** ✅ 已完成（2026-09-23）：三目标一致把可写 runtime 区（GC/调度器槽；Windows 另含分派器 thunk 槽、fd 表、WSA 区）迁到镜像基址 + 16MB（`DATA_VA_OFF`）的独立 R-W 段——ELF 第二条 PT_LOAD、PE `.data`（头区随之抬到 1024B，另以只占 VA 的 `.bss` 填洞节使节 VA 相邻）、Mach-O `__DATA`（先行落地）；代码域收成 R-X——ELF 首条 PT_LOAD、PE `.text`、Mach-O `__TEXT`。定案与验证见 `architecture.md` §5.21；其后 PIE 落地见 §5.22。
- **崩溃可诊断性对齐 C 的覆盖面** ✅ 已完成（2026-09-14）：C 运行时的纤程切换走 `ucontext`/Win32 Fiber API（`runtime/burrt_impl.c` 的 `getcontext`/`bur_switch_to_sched`），这类系统级协程切换本身对栈回溯就是不透明的——所以 PE/Mach-O/ELF 落地的"只覆盖用户函数与内部子程序，纤程切换点（yield/调度器恢复）不覆盖"这条边界，其实已经对齐 C 的真实能力，不是缺口。(a) **ELF `.eh_frame`（DWARF CFI）**：单份共享 CIE + 每函数一条 FDE，用真实工具链逐字节核对编码，再用编译器实际产出的 ELF 在 gdb 下跑深度非尾递归到真实栈溢出崩溃验证回溯正确，详见 `architecture.md` §5.19。(b) **内部子程序 unwind 覆盖扩展**：GC/调度器里纯 `call`/`ret` 的内部子程序（`gc_record` 9-push、`gc_collect`/`mark_addr` 7-push、`chan_schedule`/`wake_waiters`/`push_queue`×3/`remove_waiter`/`waitset_add`/`timer_add` 共 11 个代码实例）三个格式（PE/Mach-O/ELF）都已纳入 unwind 覆盖——序言形态逐个核对 `runtime.bur` 源码分三类（零序言/9-push/7-push），ELF 用真实 gdb 断点+GC 压力测试验证回溯正确，PE/Mach-O 逐字节核对真实构建产物的二进制内容，详见 `architecture.md` §5.20。
- **`net_nb` / fiber 感知 IO 真机验收** ✅ 已完成（2026-09-24）：`x86-pe-run` 在 Windows runner 上以看门狗 hard gate 跑 `net_loopback`/`net_nb`，对 golden 一致；等 fd 由分派器译成 `WSAPoll`，见 `architecture.md` §5.7。
- **multi-backend 已知缺陷显式登记** ✅ 已完成（2026-09-14）：`testdata/pkg/{annotations,cached,constcycle,consts,deepmut,extimport,extmissing,pipeline,stdjson}` 九例逐条重新核实后，原"三方一致拒绝"的归类本身是错的——4 例（`annotations`/`consts`/`pipeline`/`stdjson`）是 fixture 用 `.` 代替 `::` 做跨包访问的语法笔误，改正后三方一致成功；3 例（`deepmut`/`constcycle`/`extmissing`）是三方一致的正确诊断拒绝（deep-mut 安全检查/const 环检测/缺失 `require`），不是缺陷；2 例（`cached`/`extimport`）依赖文档占位域名，环境缺依赖导致三方一致拒绝，用假缓存验证过依赖可用时三方一致成功。**零 x86 缺陷、零语言级限制**，以"已知缺陷清零"满足本条完成线，详见 `architecture.md` §5.18。

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

主线（细节见 §3）：x86 Linux ELF（S8.1，已完成）→ x86 模块包（S8.6，已完成）→ PE 与 Mach-O 序列化层（S8.5，部分实现）→ **x86 完成线收紧**（PIE / 节拆分 / unwind 覆盖对齐 C / multi-backend 零缺口 / PE 与 Mach-O 功能对齐 / 运行时检查与错误文本对齐，§3 详列）→ runtime 平台抽象与工具链探测 → LLVM（S8.8）→ Cranelift（S8.9）→ WASM（S8.10）→ LSP（S9）彻底完工 → 基础生态（S10 新增包）。
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
