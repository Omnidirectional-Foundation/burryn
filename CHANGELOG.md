# Changelog

All notable changes to this project are documented here.
本文件记录项目的所有重要变更。

Version numbers follow the `v0.N.0` tags, not the global two-day window. The
date range on each heading spans that version: from the day the window opened
until the next tag. Latest first.
版本号跟 `v0.N.0` tag 走，不走全局两日窗口。
条目标题上的日期是该版本的跨度：从开窗到下一枚 tag 之前。最新在前。

## v0.6 (2026-08-13 ~ 08-29)

**Recorded IPC clauses, then settled them; landed capture and name-based records;
retired `SPEC.md` and stopped restating live status across docs** —
architecture.md keeps decisions and the backend ABI, GOALS keeps milestones and
the rejection list, grammar keeps surface syntax, CHANGELOG keeps the version window,
README/tutorial follow observable semantics, CLAUDE.md keeps gates not a second
roadmap.
**先落地 IPC 条款再定案；补上 capture 与封闭 record 按名合一；废弃 `SPEC.md`，文档不再互相
复述「现在怎样」** —— architecture.md 管定案与后端 ABI，GOALS 管阶段与拒绝清单，grammar
管表层，CHANGELOG 管版本窗口，README/tutorial 跟可观察语义，CLAUDE.md 管闸门而不是第二
份路线图。

- **Removed:** `docs/SPEC.md`. Its content splits by job: language decisions,
  the backend matrix, the x86-64 ABI, LSP architecture and runtime IO go to
  `docs/architecture.md`; completion lines and the rejection list go to
  `docs/GOALS.md`. Pointers in both READMEs, both tutorials, `grammar.md`,
  `NUMBERING.md`, `CONTRIBUTING.md` and the feature-request template follow.
- **移除：** `docs/SPEC.md`。内容按职责拆分：语言定案、后端矩阵、x86-64 ABI、LSP
  架构与 runtime IO 迁入 `docs/architecture.md`；完成线与拒绝清单迁入
  `docs/GOALS.md`。两份 README、两份 tutorial、`grammar.md`、`NUMBERING.md`、
  `CONTRIBUTING.md` 与功能请求模板的指针同步更新。
- **Changed:** Both READMEs, both tutorials and `CLAUDE.md` describe the x86-64
  backend as emitting ELF, PE and Mach-O, matching the `--os linux|windows|darwin`
  the CLI already accepts; the tutorials list `--backend`, `--os`, `bur mod`,
  `bur get` and `bur lsp`.
- **变更：** 两份 README、两份 tutorial 与 `CLAUDE.md` 按 CLI 已接受的
  `--os linux|windows|darwin` 描述 x86-64 后端可发 ELF / PE / Mach-O；两份 tutorial
  补上 `--backend`、`--os`、`bur mod`、`bur get` 与 `bur lsp`。
- **Changed:** `GOALS.md` states S8.5 as PE together with Mach-O (no separate
  number), pins the completion gate for S8.1 and S8.5 to a continuously green
  `ci.yml`, and labels stages 已实现 / 部分实现 / 未实现 instead of phase words
  that go stale.
- **变更：** `GOALS.md` 把 S8.5 写为 PE 与 Mach-O 同项（不另开编号），把 S8.1 与
  S8.5 的验收闸钉在 `ci.yml` 持续全绿，阶段状态改用 已实现 / 部分实现 / 未实现。
- **Changed:** `CONTRIBUTING.md` records the long-lived `dev` branch instead of
  claiming `main` only.
- **变更：** `CONTRIBUTING.md` 改为记录常驻 `dev` 分支，不再声称只用 `main`。
- **Changed:** `CLAUDE.md` points at the README for the CLI table and keeps only
  this repository's verification entry points.
- **变更：** `CLAUDE.md` 的命令节指向 README，只保留本仓的验证入口。
- **Added:** `examples/README.md` indexes the eight runnable examples it omitted
  (`fileio`, `float_arith`, `readir`, `syscall`, `records`, `enum_mono`,
  `poly_hetero`, `select_recv`).
- **新增：** `examples/README.md` 补入此前漏列的八个可运行样例（`fileio`、
  `float_arith`、`readir`、`syscall`、`records`、`enum_mono`、`poly_hetero`、
  `select_recv`）。
- **Fixed:** The from-scratch bootstrap in both READMEs builds `bur-base` inside
  the archived Go host worktree, matching `ci.yml`; the obsolete `seed-base-1`
  bridge step is gone.
- **修复：** 两份 README 的从零自举改为在归档 Go 宿主 worktree 内构建 `bur-base`，
  与 `ci.yml` 一致；废弃的 `seed-base-1` 过桥步骤已移除。
- **Changed:** Release tags are `v0.N.0`; the `seed-base-N` anchors are retired and
  the CHANGELOG version note follows the new scheme.
- **变更：** 发布 tag 改用 `v0.N.0`，`seed-base-N` 基准 tag 退役，CHANGELOG 的版本号
  说明同步改写。
- **Changed:** Chinese prose in SPEC, GOALS, NUMBERING, README.zh-CN, CONTRIBUTING
  and SECURITY uses full-width punctuation.
- **变更：** SPEC、GOALS、NUMBERING、README.zh-CN、CONTRIBUTING 与 SECURITY 的中文
  正文改用全角标点。
- **Added:** `examples/README.md` is listed in the Documentation table of both
  READMEs.
- **新增：** 两份 README 的文档表补入 `examples/README.md`。
- **Changed:** Compiler sources split by concern under `frontend/`, `bytecode/`,
  `backends/{c,x86}/`, `module/`, `tooling/`, and `lib/`; CLI helpers move to
  `compiler/cli/` with a thin `main.bur` entry.
- **变更：** 编译器按职责拆到 `frontend/`、`bytecode/`、`backends/{c,x86}/`、
  `module/`、`tooling/`、`lib/`；CLI 辅助迁入 `compiler/cli/`，`main.bur` 只留入口。
- **Fixed:** x86 select arm typing no longer stacks every recv value type; slot-held
  closure calls resolve return types; unary neg/not replace the stack top; indirect
  higher-order calls skip aggregate param/`sig_specs` pollution.
- **修复：** x86 select 臂不再叠全部 recv 值类型；槽持闭包调用解析返回类型；一元
  neg/not 替换栈顶类型；间接高阶调用跳过聚合轮形参/`sig_specs` 污染。
- **Fixed:** x86 `compose`/`double_then_str` and `map_first(str, …)` — bind closure
  handles on `CLOSURE` (not a later `GET_LOCAL`), keep single-signature concrete
  param types, and select polymorphic variants on dynamic closure calls.
- **修复：** x86 `compose`/`double_then_str` 与 `map_first(str, …)` — 在 `CLOSURE`
  时绑定闭包 handle（不拖到后续 `GET_LOCAL`），保留单签名具体实参类型，动态闭包
  调用按实参选多态变体。
- **Added:** SPEC `§6.3` formal clauses — `exec_start`/`exec_poll` pipe and
  collect-at-exit behavior (stdin inherited, no streaming), TCP loopback as the
  only bidirectional IPC with measured numbers (231 µs in-process, 1350–1550 µs
  cross-process, 65 MiB/s 512KiB round-trip), no built-in message framing.
- **新增：** SPEC `§6.3` 正式条款 —— `exec_start`/`exec_poll` 的管道与退出时收集行为
  （stdin 继承，无流式）；TCP loopback 是唯一的双向 IPC，附实测数据（进程内 231 µs，
  跨进程 1350–1550 µs，512KiB 往返 65 MiB/s）；不内建消息分帧。
- **Added:** SPEC `§6.1` known limitation — x86 `net_read`/`tcp_accept` are raw
  blocking syscalls that stall every fiber; the supervisor/worker concurrency
  model runs on the VM or the C backend only.
- **新增：** SPEC `§6.1` 已知局限 —— x86 的 `net_read`/`tcp_accept` 是裸阻塞系统调用，
  会卡住所有 fiber；supervisor/worker 并发模型只能跑在 VM 或 C 后端上。
- **Added:** SPEC `§7` four pending owner decisions — Unix domain socket
  primitive, built-in IPC message protocol, first-class process pool, x86
  fiber-aware IO (no conclusion preset).
- **新增：** SPEC `§7` 四项待定 owner 决策 —— Unix domain socket 原语、内建 IPC 消息协议、
  一等进程池、x86 fiber 感知 IO（不预设结论）。
- **Changed:** Version constants extended to 0.6.0 in lockstep with the new
  window (`compiler/main.bur` CLI version, `compiler/lib/defs.bur`
  toolchain_version, `compiler/tooling/lsp.bur` serverInfo); `docs/SPEC.md`
  header advanced to v0.6.
- **变更：** 版本常量随新窗口同步推到 0.6.0（`compiler/main.bur` 的 CLI 版本、
  `compiler/lib/defs.bur` 的 toolchain_version、`compiler/tooling/lsp.bur` 的 serverInfo）；
  `docs/SPEC.md` 头行推进到 v0.6。
- **Added:** `capture` / `capture(ref …)` clause — default copy-at-capture;
  `ref` shares a heap cell. Grammar keyword list, parser, and SPEC §6.1
  (grammar.md §10 revision after the S8.2 freeze).
- **新增：** `capture` / `capture(ref …)` 子句 —— 默认捕获时拷贝，`ref` 共享堆单元。
  涉及 grammar 关键字表、parser 与 SPEC `§6.1`（S8.2 冻结后对 grammar.md `§10` 的修订）。
- **Added:** S8.4 closed records unify by field name (`record { x, y }` is
  `record { y, x }`); RecordLit/RecordUpdate emit fields in lexicographic order.
- **新增：** S8.4 封闭 record 按字段名合一（`record { x, y }` 即 `record { y, x }`）；
  RecordLit/RecordUpdate 按字典序发射字段。
- **Changed:** SPEC §7 four IPC questions settled by owner — no AF_UNIX
  primitive, no built-in IPC framing, no first-class `std/procpool`, x86
  fiber-aware IO approved (`O_NONBLOCK` + fiber park + `poll(2)`; `net_nb` in
  the same design). The §6.1 "raw blocking net syscall" limitation is superseded
  by that decision.
- **变更：** SPEC `§7` 四项 IPC 问题由 owner 定案 —— 不做 AF_UNIX 原语、不内建 IPC 分帧、
  不做一等 `std/procpool`、批准 x86 fiber 感知 IO（`O_NONBLOCK` + fiber park + `poll(2)`；
  `net_nb` 属同一设计）。`§6.1` 的「裸阻塞 net 系统调用」局限由该定案取代。
- **Changed:** User-facing closure text in README and tutorial matches SPEC
  (copy-at-capture; `capture(ref)` for shared `mut`). grammar.md records
  shebang, labeled loops in `statement`, and `..` range in the expression EBNF.
- **变更：** README 与 tutorial 中面向用户的闭包表述与 SPEC 对齐（捕获时拷贝；共享 `mut`
  用 `capture(ref)`）。grammar.md 在 EBNF 中记入 shebang、`statement` 里的带标签循环与 `..` 区间。
- **Fixed:** C runtime defines `bur_start_ns` (monotonic origin used by `clock`).
- **修复：** C runtime 补上 `bur_start_ns` 定义（`clock` 用的单调时钟原点）。
- **Fixed:** x86 PE shim calls `[dispatcher slot]`, emits one import descriptor per
  DLL, keeps Win64 shadow space below saved registers, and fills GetStdHandle /
  WriteFile / QPC arguments in the Win64 registers.
- **修复：** x86 PE shim 经分派器槽间接调用；每个 DLL 一条导入描述符；Win64
  影子空间在保存寄存器之下；GetStdHandle / WriteFile / QPC 按 Win64 填参。
- **Fixed:** x86 pre-scan registers `net_nb` as `Result<str,str>`; GET_FIELD stamps
  payload types onto locals; link-slot seeding merges with enum payloads; an
  `enum_mono` sentinel is added; the `net_nb` would-block notice prints once.
- **修复：** x86 预扫描把 `net_nb` 记成 `Result<str,str>`；GET_FIELD payload
  类型盖到 local；链接槽种子与枚举 payload 合并；`enum_mono` 哨兵；`net_nb`
  的 would-block 只打一行。
- **Changed:** GitHub Actions L0 births `gen1` from `bur-base` cached by
  `archive/go-host` SHA at `CC_OPT=-O1`; seed skipped on hit; seed-era
  `runtime/*.h` overlay; testdata skips C three-gen fixpoint on push.
- **变更：** GitHub Actions L0 用按 `archive/go-host` SHA 缓存的 `bur-base`
  （`CC_OPT=-O1`）接生 `gen1`；命中则跳过 seed；覆盖 seed 世代 `runtime/*.h`；
  testdata 在 push 上跳过 C 三代定点。
- **Changed:** `exec_large` redirects `yes` stderr.
- **变更：** `exec_large` 重定向 `yes` 的 stderr。
- **Changed:** Split `docs/GOALS.md` (milestones and remaining completion lines)
  from `docs/SPEC.md` (retrospective decisions). Stage state no longer lives in
  SPEC §5; that section points at GOALS.
- **变更：** 把 `docs/GOALS.md`（里程碑与剩余完成线）从 `docs/SPEC.md`（回顾性定案）中拆出。
  阶段状态不再留在 SPEC `§5`，该节改为指向 GOALS。
- **Changed:** User-facing docs match the current toolchain: `bur lsp` in the
  command list; tutorial `net_nb` is `(op, h, data, max)`; CONTRIBUTING uses
  `bur test [dir]`.
- **变更：** 面向用户的文档与当前工具链对齐：命令表补 `bur lsp`；tutorial 的 `net_nb`
  改为 `(op, h, data, max)`；CONTRIBUTING 改用 `bur test [dir]`。
- **Changed:** README follows the CLI skeleton (Prerequisites, Features,
  Examples, Documentation before Honest Limitations) with `$` on command
  examples. Tutorial headings drop emoji.
- **变更：** README 按 CLI 骨架排列（Prerequisites、Features、Examples、Documentation 在
  Honest Limitations 之前），命令示例统一加 `$`。tutorial 标题去掉 emoji。

## v0.5 (2026-08-03 ~ 08-04)

**Landed S8.2 syntax freeze** — the language syntax is frozen; the formal
grammar lives in `docs/grammar.md` and future changes go through its revision
process.
**落地 S8.2 语法冻结** — 语言表层语法冻结；正式 grammar 见 `docs/grammar.md`，此后变更须走修订流程。

- **Added:** `::` path qualifier (packages, enum variants) distinct from `.`
  member access (fields, methods).
- **新增：** `::` 路径限定符（包、枚举变体），与 `.` 成员访问（字段、方法）区分。
- **Added:** Compound assignment `+= -= *= /= %=`; `let`/`const` type
  annotations; indexed `for i, x in xs`; or-patterns `A | B`; labeled loops
  `label:` with `break label`/`continue label`; receiver methods
  `fn (s: Type) name()`; record patterns `record { x, y }`; tuples `(a, b)`
  with destructuring `let (a, b) = t`.
- **新增：** 复合赋值 `+= -= *= /= %=`；`let`/`const` 类型标注；带索引的 `for i, x in xs`；
  或模式 `A | B`；带标签循环 `label:` 配 `break label`/`continue label`；接收者方法
  `fn (s: Type) name()`；record 模式 `record { x, y }`；元组 `(a, b)` 及解构 `let (a, b) = t`。
- **Added:** Literal forms — hex `0xFF`, binary `0b1010`, digit separators,
  float exponents, `\r` escape, shebang, multiline strings `"""` with `\`
  content markers, range literals `1..10`.
- **新增：** 字面量形式 —— 十六进制 `0xFF`、二进制 `0b1010`、数字分隔符、浮点指数、`\r`
  转义、shebang、`"""` 多行字符串（配 `\` 内容标记）、区间字面量 `1..10`。
- **Added:** grammar.md covers keywords, tokens, EBNF, record, `<-`
  disambiguation, and interface-file syntax.
- **新增：** grammar.md 覆盖关键字、token、EBNF、record、`<-` 消歧与接口文件语法。
- **Added:** Examples cover the previously undocumented natives — numeric
  conversions (`trunc`/`float_bits`/`parse_float`), `file_exists`/`read_dir`,
  `read_stdin`/`stdin_nb`, `byte_chr`, `yield`, `net_nb` — each with a `.golden`.
- **新增：** 示例补齐此前无文档的 native —— 数值转换（`trunc`/`float_bits`/`parse_float`）、
  `file_exists`/`read_dir`、`read_stdin`/`stdin_nb`、`byte_chr`、`yield`、`net_nb` —— 每个都配 `.golden`。
- **Changed:** `testdata/` regrouped by topic (`basics/`, `types/`,
  `regression/`) like `examples/`; the frozen-grammar samples moved out of the
  monolithic `syntax/` directory and `modules/` joined `pkg/`.
- **变更：** `testdata/` 比照 `examples/` 按主题重组（`basics/`、`types/`、`regression/`）；
  冻结语法样本移出单体的 `syntax/` 目录，`modules/` 并入 `pkg/`。
- **Changed:** Verification scripts merged — `scripts/examples-verify.sh` and
  `scripts/testdata-verify.sh` (with the bootstrap fixpoint) replace
  `fixpoint.sh` and `syntax-verify.sh`; `golden-verify.sh` gained multi-directory
  and trap-exit-4 handling.
- **变更：** 验证脚本合并 —— `scripts/examples-verify.sh` 与 `scripts/testdata-verify.sh`
  （含自举定点）取代 `fixpoint.sh` 和 `syntax-verify.sh`；`golden-verify.sh` 支持多目录与 trap exit 4。
- **Fixed:** x86 backend `chr` now emits UTF-8 for multi-byte code points
  (was single-byte only, silently wrong for non-ASCII).
- **修复：** x86 后端 `chr` 对多字节码点发射 UTF-8（此前只出单字节，非 ASCII 时静默出错）。
- **Changed:** Tutorial reorganized — the syntax-freeze features are folded
  into their topic chapters (literals, tuples, records, methods, indexed and
  labeled loops, multiline strings, or-patterns) and the standalone section is
  dropped; EN and ZH stay in sync.
- **变更：** tutorial 重组 —— 语法冻结的特性折进各自主题章节（字面量、元组、record、方法、
  带索引与带标签循环、多行字符串、或模式），独立章节删除；中英保持同步。
- **Fixed:** README Tour enum sample used nonexistent field names; the
  examples table gained the new samples; the SPEC x86 backend status and
  progress were refreshed.
- **修复：** README Tour 的枚举样例用了不存在的字段名；示例表补入新样本；SPEC 的 x86
  后端状态与进度刷新。

## v0.4 (2026-07-21 ~ 07-22)

**Landed S7.6 `defer`** — `defer { ... }` registers a block on the enclosing
function; deferred blocks run LIFO when the function exits normally.
**落地 S7.6 `defer`** — `defer { ... }` 将块挂到包围函数上，函数正常退出时 LIFO 执行。

- **Added:** `defer` keyword, DeferStmt across the full pipeline
  (lexer/parser/checker/compiler/formatter/disassembler/constant folder).
- **新增：** `defer` 关键字与 DeferStmt 贯通整条管线（lexer/parser/checker/compiler/
  formatter/disassembler/常量折叠）。
- **Added:** Blocks compile as zero-parameter closures; new opcode `DEFER`(49)
  pushes onto a per-frame defer stack; `return`, tail expressions, and `?`
  early exits run deferred blocks LIFO; traps do not run defers.
- **新增：** 块编译为零参闭包；新增操作码 `DEFER`(49) 压入每帧的 defer 栈；`return`、
  尾表达式与 `?` 早退按 LIFO 执行延迟块；trap 不执行 defer。
- **Added:** VM frame-level defer stack with exit-value stash and C runtime
  fiber-level defer array with dbase watermark; both GC roots covered.
- **新增：** VM 帧级 defer 栈带退出值暂存，C 运行时 fiber 级 defer 数组带 dbase 水位；
  两者的 GC 根都已覆盖。
- **Added:** Checker validates defer blocks as closure bodies; discarded
  Result/Option in defer blocks reports `unused_must_use`; missing block
  produces E1119.
- **新增：** checker 按闭包体校验 defer 块；块内丢弃的 Result/Option 报 `unused_must_use`；
  缺块报 E1119。

**Landed S7.7 net** — minimal TCP networking with six natives and a `std/net`
helper package; fiber-level blocking keeps the scheduler responsive.
**落地 S7.7 net** — 最小 TCP 网络面，六个 native 加 `std/net` 辅助包；fiber 级阻塞保持调度器响应。

- **Added:** `tcp_listen`, `tcp_accept`, `tcp_dial`, `net_read`, `net_write`,
  `net_close` natives with non-blocking sockets and fiber-level IO parking in
  both the C runtime scheduler and the VM.
- **新增：** `tcp_listen`、`tcp_accept`、`tcp_dial`、`net_read`、`net_write`、`net_close`
  六个 native，使用非阻塞套接字，在 C 运行时调度器与 VM 中均做 fiber 级 IO 挂起。
- **Added:** `net_nb` non-blocking multiplexed native for the VM scheduler's
  park-and-retry loop (accept/read/write with `__eagain` sentinel).
- **新增：** `net_nb` 非阻塞复用 native，供 VM 调度器的挂起重试循环使用（accept/read/write，
  以 `__eagain` 作哨兵）。
- **Added:** `std/net` package: `read_all` (read until EOF) and `write_line`
  (write with trailing newline), distributed via `std_embed`.
- **新增：** `std/net` 包：`read_all`（读到 EOF）与 `write_line`（写入并补换行），经
  `std_embed` 分发。
- **Added:** Go seed SCC-based package-level function inference, fixing
  polymorphic forward references (e.g. `concat_lists` used with both `[[str]]`
  and `[[int]]` before its definition).
- **新增：** Go seed 基于 SCC 的包级函数推导，修正多态前向引用（如 `concat_lists` 在定义前
  同时以 `[[str]]` 和 `[[int]]` 使用）。
- **Added:** Loopback and error-path golden examples (port-in-use,
  connection-refused, read-EOF) covering VM/native parity and
  `BUR_DETERMINISTIC=1`.
- **新增：** loopback 与错误路径的 golden 示例（端口占用、连接被拒、读到 EOF），覆盖
  VM/native 一致性与 `BUR_DETERMINISTIC=1`。
- **Other:** DNS resolution (`getaddrinfo`) is synchronous and
  blocks the entire scheduler; UDP, Unix sockets, and TLS are out of scope.
- **其他：** DNS 解析（`getaddrinfo`）是同步的，会阻塞整个调度器；UDP、Unix socket 与 TLS
  不在范围内。

## v0.3 (2026-07-19 ~ 07-20)

**Landed S7.5 compile-time constants** — `const` declarations fold supported
expressions before type checking in scripts, blocks, and packages.
**落地 S7.5 编译期常量** — `const` 声明在脚本、block 与 package 中于类型检查前
折叠受支持的初始化式。

- **Added:** Constant folding for literals, constant references, arithmetic,
  comparisons, boolean operators, and string concatenation; package constants
  support forward references across files and exported references across
  packages.
- **新增：** 字面量、常量引用、算术、比较、布尔运算与字符串拼接的常量折叠；包级常量支持跨文件
  前向引用与跨包导出引用。
- **Added:** E0015 for non-constant initializers, E0080 for fold-time traps,
  and E0391 for constant cycles; short-circuit operators retain their dead
  side's type and shape checks without evaluating dead traps.
- **新增：** 非常量初始化式报 E0015，折叠期 trap 报 E0080，常量环报 E0391；短路运算符保留死支
  的类型与形状检查，但不求值死 trap。
- **Changed:** `const` declarations lower to immutable `let` bindings, so
  existing type inference and reassignment diagnostics remain authoritative.
- **变更：** `const` 声明降级为不可变 `let` 绑定，因此既有的类型推导与重赋值诊断仍是权威。
- **Fixed:** VM and C runtime ordered comparisons now compare two integers as
  exact `int64` values instead of converting them to `double`.
- **修复：** VM 与 C 运行时的有序比较按精确 `int64` 比较两个整数，不再转成 `double`。
- **Added:** Constant examples, formatter fixtures, package and cycle fixtures,
  fold diagnostic fixtures, and test-runner coverage.
- **新增：** 常量示例、formatter fixture、包与环 fixture、折叠诊断 fixture，以及 test-runner 覆盖。

**Landed the S7.2 pipe operator** — `x |> f(a)` calls `f(x, a)`; pipes are
lowest-precedence and left-associative, so `x |> f |> g` reads `g(f(x))`.
**落地 S7.2 管道操作符** — `x |> f(a)` 即 `f(x, a)`；`|>` 优先级最低、左结合，`x |> f |> g` 即 `g(f(x))`。

- **Added:** `|>` lexing, a dedicated pipe-target grammar (`f`, `pkg.f`,
  optional arguments), and a pre-checker lowering pass from `Pipe` nodes to
  plain calls.
- **新增：** `|>` 词法、专用的管道目标文法（`f`、`pkg.f`、可选实参），以及从 `Pipe` 节点降级为
  普通调用的前置检查 pass。
- **Added:** Formatter rendering that keeps `|>` chains and explicit empty
  parentheses intact across `bur fmt`.
- **新增：** formatter 渲染在 `bur fmt` 前后保持 `|>` 链与显式空括号不变。
- **Added:** Pipe examples plus parser, module, and formatter regression
  fixtures.
- **新增：** 管道示例，以及 parser、module 与 formatter 回归 fixture。

**Landed S7.3 match guards** — a match arm can add `if <bool>` after its
pattern, with pattern bindings visible to the guard.
**落地 S7.3 match guard** — match 臂可在 pattern 后添加 `if <bool>`，guard 可访问 pattern 绑定。

- **Added:** Guard parsing, type checking, formatting, AST dumps, and bytecode
  generation for `pattern if guard => body` arms.
- **新增：** 为 `pattern if guard => body` 臂实现 guard 的解析、类型检查、格式化、AST dump
  与字节码生成。
- **Changed:** Guarded arms no longer count toward exhaustiveness because their
  condition may reject an otherwise matching value.
- **变更：** 带 guard 的臂不再计入穷尽性，因为其条件可能拒绝一个本来匹配的值。
- **Added:** VM/native parity examples plus guard type, parser, and
  exhaustiveness regression fixtures.
- **新增：** VM/native 一致性示例，以及 guard 类型、parser 与穷尽性回归 fixture。

**Landed S7.1 string interpolation** — strings can splice `str` expressions
with `{expr}` and escape literal opening braces as `{{`.
**落地 S7.1 字符串插值** — 字符串可用 `{expr}` 拼接 `str` 表达式，字面左花括号写成 `{{`。

- **Added:** Lexer mode switching for interpolation segments, nested brace
  balancing, and parser lowering to the existing string-concatenation AST.
- **新增：** 插值段的 lexer 模式切换、嵌套花括号配平，以及 parser 降级到既有的字符串拼接 AST。
- **Changed:** Non-`str` interpolation expressions now produce a compile error
  whose help suggests an explicit `str()` conversion.
- **变更：** 非 `str` 的插值表达式现在报编译错误，help 建议显式 `str()` 转换。
- **Added:** Formatter reconstruction, interpolation examples, and malformed
  interpolation regression fixtures.
- **新增：** formatter 重建、插值示例，以及畸形插值的回归 fixture。

**S6 (ecosystem toolchain) is complete** — this version closes every S6
work package: dependency management with a disk interface cache,
sub-package testing, embedded std, a rebased bootstrap seed, and the
diagnostics/DX batch.
**S6（生态工具链）全部收尾** — 本版本关闭 S6 全部工作包：带磁盘接口缓存的
依赖管理、子包测试、内嵌 std、重新定基的自举种子、诊断/DX 批。

**Landed the S6.5 diagnostics/DX batch** — native debugging, runtime
stack traces, and human-readable diagnostics.
**落地 S6.5 诊断/DX 批** — 原生调试、runtime stack trace、人类可读诊断。

- **Added:** The C backend now emits `#line <n> "<file>"` before every function
  header and every instruction, and `bur build` compiles with `-g`:
  `gdb`/`lldb` on a produced binary maps frames straight back to `.bur`
  source lines.
- **新增：** C 后端在每个函数头与每条指令前发射 `#line <n> "<file>"`，`bur build` 带 `-g`
  编译：对产出的二进制用 `gdb`/`lldb` 可把栈帧直接映射回 `.bur` 源码行。
- **Added:** Runtime traps now print a span stack trace (`  at <fn> (<file>:<line>)`
  per frame) in both the C runtime and the VM, byte-identical across the
  two; frames without a source file are suppressed.
- **新增：** runtime trap 在 C 运行时与 VM 中都打印 span 栈回溯（每帧 `  at <fn> (<file>:<line>)`），
  两者逐字节一致；无源文件的帧被抑制。
- **Changed:** Public commands (`check`/`run`/`build`) render diagnostics
  rustc-style: `error[CODE]: msg` with `file:line:col`, the source line,
  a caret underline, and `= help:` / `= fix:` trailers. The hidden
  `bur dev` parity dumps keep the old raw format byte-for-byte.
- **变更：** 公开命令（`check`/`run`/`build`）按 rustc 风格渲染诊断：`error[CODE]: msg` 配
  `file:line:col`、源码行、脱字号下划线与 `= help:` / `= fix:` 尾注。隐藏的 `bur dev` 对照
  dump 保持旧的裸格式逐字节不变。
- **Added:** Multi-span labels and structured fixes to the diagnostic
  carrier (`DiagX` with `Lab(start, end, label)` and
  `Fix(start, end, replacement, desc)`): duplicate definitions point at
  the first declaration; unused variables suggest the `_` prefix.
- **新增：** 诊断载体加入多 span 标签与结构化修复建议（`DiagX` 含 `Lab(start, end, label)`
  与 `Fix(start, end, replacement, desc)`）：重复定义指向首次声明；未用变量建议加 `_` 前缀。
- **Fixed:** Module-loader diagnostics (e.g. `unused_import`) now reach the public
  commands; previously they were silently dropped.
- **修复：** 模块加载器的诊断（如 `unused_import`）现在能到达公开命令；此前被静默丢弃。
- **Changed:** CI's fixpoint judgment moved from `gen1 == gen2` to `gen2 == gen3`:
  gen1 is built by the frozen base compiler, so a cgen emission change
  legitimately alters it; gen2 and gen3 embed the same current compiler
  and must emit identical C.
- **变更：** CI 的定点判据从 `gen1 == gen2` 改为 `gen2 == gen3`：gen1 由冻结的基准编译器构建，
  cgen 发射变更会合法地改变它；gen2 与 gen3 内嵌同一份当前编译器，必须发射相同的 C。

**Rebased the bootstrap seed** — the rebirth chain is now three stages.
**重新定基自举种子** — 重生链改为三段。

- **Added:** An internal bootstrap anchor commit (not a release):
  the archived Go seed builds that commit's burc, which then builds the
  current burc. CI runs the full chain on every push.
- **新增：** 一个内部自举基准 commit（非发布）：归档 Go seed 构建该 commit 的 burc，再由它
  构建当前 burc。CI 每次 push 跑完整链条。
- **Changed:** burc's own sources are freed from the three legacy checker
  disciplines (file-order inference, bounce idioms, forward-only enum
  references); the anchored commit keeps them forever.
- **变更：** burc 自身源码摆脱三条遗留 checker 纪律（文件序推导、bounce 惯用法、仅前向枚举
  引用）；基准 commit 永久保留它们。

**Landed S6.1 end to end** — dependency management now spans loading,
interfaces, caching, and tidy.
**落地 S6.1 全链** — 依赖管理贯通加载、接口、缓存与 tidy。

- **Added:** The module loader follows the `require` closure through `$BURCACHE`,
  with MVS version selection and `bur.sum` tree-hash verification.
- **新增：** 模块加载器经 `$BURCACHE` 跟随 `require` 闭包，配 MVS 版本选择与 `bur.sum` 树哈希校验。
- **Added:** The interface pipeline — a deterministic exported-declaration
  renderer, an interface-only parser, and checker consumption of
  bodiless declarations.
- **新增：** 接口管线 —— 确定性的导出声明渲染器、仅接口 parser，以及 checker 对无体声明的消费。
- **Added:** The disk interface cache
  (`$BURCACHE/.interfaces/<toolchain>-<tree hash>/`): a warm hit
  replaces a dependency's checker input with its interface; any read,
  parse, or metadata failure falls back to full source silently, and an
  error diagnostic forces a full-source re-run so a stale cache can
  never fabricate errors.
- **新增：** 磁盘接口缓存（`$BURCACHE/.interfaces/<toolchain>-<tree hash>/`）：命中时用接口替换
  依赖的 checker 输入；任何读取、解析或元数据失败都静默回退到全量源码，出现 error 诊断则强制
  全量重跑，因此陈旧缓存不可能凭空造出错误。
- **Added:** `bur test` discovers sub-package tests (shown as `<rel>/<fn>`),
  and `bur mod tidy` adds and removes `require` lines from actual
  imports across the whole module, promoting indirectly-required
  versions picked by MVS.
- **新增：** `bur test` 发现子包测试（显示为 `<rel>/<fn>`），`bur mod tidy` 依据整个模块的实际
  import 增删 `require` 行，并把 MVS 选中的间接依赖版本提升为直接依赖。
- **Added:** CI regenerates `burc/lib/std_embed.bur` and fails on drift.
- **新增：** CI 重新生成 `burc/lib/std_embed.bur`，漂移即失败。

**Landed S7.8 optional signature annotations early** — plus constrained
type variables.
**提前落地 S7.8 可选签名标注** — 并带受约束类型变量。

- **Added:** Function parameters and returns accept optional `name: type` /
  `-> type` annotations, reusing the type-expression grammar.
- **新增：** 函数形参与返回值接受可选的 `name: type` / `-> type` 标注，复用类型表达式语法。
- **Added:** Type variables can carry constraints (`a:addord`), checked by
  constraint intersection across parser, formatter, and checker.
- **新增：** 类型变量可携带约束（`a:addord`），由 parser、formatter 与 checker 按约束交集校验。

**Enforced the deep-`mut` flow rule** — the v0.2 plan became an error.
**落实 deep-`mut` 流规则** — v0.2 的计划升级为 error。

- **Changed:** A `mut` binding or `mut` argument must come from a fresh heap source
  (list/map); scalars are exempt; an if/match source is fresh when every
  arm's tail is fresh.
- **变更：** `mut` 绑定或 `mut` 实参必须来自新鲜的堆来源（list/map）；标量豁免；if/match 来源
  在每个臂的尾表达式都新鲜时才算新鲜。

**Landed S6.6 std/json and std/testing** — the first embedded std
members.
**落地 S6.6 std/json 与 std/testing** — 首批内嵌 std 成员。

- **Added:** `std/json` ships `parse` / `render` / `pretty` / `get` over the
  seven-variant `Json` enum with ordered parallel-list objects.
- **新增：** `std/json` 提供 `parse` / `render` / `pretty` / `get`，基于七变体 `Json` 枚举，
  对象用有序平行列表表示。
- **Added:** `std/testing` ships `assert_eq` / `assert_ok` / `assert_err`, closing
  the S6.4 assertion-sugar debt.
- **新增：** `std/testing` 提供 `assert_eq` / `assert_ok` / `assert_err`，结清 S6.4 的断言语法糖欠债。

## v0.2 (2026-07-10 ~ 07-11)

**Recorded the 2026-07-10 design-review decisions in `docs/SPEC.md`** — a
full audit of settled decisions, with corrections where the doc had drifted
from the implementation.
**在 `docs/SPEC.md` 记录 2026-07-10 设计审查定案** — 对已定决策的全面
审计，并纠正文档与实现漂移之处。

- **Changed:** Narrowed the determinism promise: pure-compute programs stay byte-for-byte
  deterministic across backends; IO-concurrent programs no longer promise
  scheduling order. Added the opt-in `BUR_DETERMINISTIC=1` mode (IO
  serialized) as the future `bur test` default.
- **变更：** 收窄确定性承诺：纯计算程序在各后端间仍逐字节确定；IO 并发程序不再承诺调度
  顺序。新增可选的 `BUR_DETERMINISTIC=1` 模式（IO 串行化）作为将来 `bur test` 的默认值。
- **Changed:** Downgraded deep `mut` from a value-level guarantee to a binding-level
  discipline (aliasing can bypass it), and added a planned checker flow rule
  for `mut` argument sources.
- **变更：** 深 `mut` 从值级保证降为绑定级纪律（别名可绕过），并规划了针对 `mut` 实参来源
  的 checker 流规则。
- **Fixed:** Corrected S2.7: json and net were never implemented; json moves to new
  S6.6 (bundled `std/`), net to new S7.7.
- **修复：** 订正 S2.7：json 与 net 从未实现；json 移入新的 S6.6（随工具链捆绑的 `std/`），
  net 移入新的 S7.7。
- **Added:** S6.7 (runtime IO work package: `sleep`, `exec_start`/`exec_poll`,
  scheduler idle-wait, deterministic mode) and S6.8 (checker-debt batch: SCC
  inference order, two-pass enum registration, `?` in mutual recursion).
- **新增：** S6.7（runtime IO 工作包：`sleep`、`exec_start`/`exec_poll`、调度器空闲等待、
  确定性模式）与 S6.8（checker 债批：SCC 推导序、两遍枚举注册、互递归中的 `?`）。
- **Changed:** Rejected S7.4 named arguments + defaults; slotted S7.6 `defer`
  (block-scope leaning), S7.7 net, S7.8 optional signature annotations.
- **变更：** 否决 S7.4 具名实参与默认值；排入 S7.6 `defer`（倾向块作用域）、S7.7 net、
  S7.8 可选签名标注。
- **Changed:** Reordered S8: types first (S8.3 row poly, S8.4 records), then the
  hand-written x86-64 ELF backend; split PE into S8.5 with its Windows
  runtime prerequisite spelled out.
- **变更：** 重排 S8：类型先行（S8.3 行多态、S8.4 record），再做手写 x86-64 ELF 后端；
  PE 拆为 S8.5 并写明其 Windows 运行时前置。
- **Fixed:** The stale native-GC line (shadow-stack precise GC, not conservative
  stack scanning) and hardened the `bur fmt` acceptance rules to three:
  idempotent, AST-invariant (enforced via reparse + `ast_eq`), and no
  comment loss.
- **修复：** 订正过时的原生 GC 表述（shadow-stack 精确 GC，而非保守栈扫描），并把 `bur fmt`
  的验收规则收紧为三条：幂等、AST 不变（经重解析 + `ast_eq` 强制）、不丢注释。
- **Changed:** Decided std distribution (bundled with the toolchain, reserved `std/`
  prefix) and the `bur.sum` line format
  `<path> <version> h1:<base64(tree hash)>`.
- **变更：** 定下 std 分发方式（随工具链捆绑，保留 `std/` 前缀）与 `bur.sum` 行格式
  `<path> <version> h1:<base64(tree hash)>`。

**Landed S6.7 runtime IO and the complete `bur fmt` (S6.3)** — the same
window's implementation batch.
**落地 S6.7 runtime IO 与完整的 `bur fmt`（S6.3）** — 同窗口的实现批次。

- **Added:** The `sleep(ms)` native with timer-aware scheduling in both the C
  runtime and the VM: idle schedulers sleep to the nearest deadline
  instead of spinning or deadlocking.
- **新增：** `sleep(ms)` native，在 C 运行时与 VM 中都带定时器感知调度：空闲调度器睡到最近
  的截止时刻，而非空转或死锁。
- **Changed:** Made `exec` fiber-blocking instead of scheduler-blocking, and added the
  `exec_start`/`exec_poll` natives (two `exec sleep 0.5` fibers now finish
  in 0.5s, previously 1.0s); `BUR_DETERMINISTIC=1` serializes children for
  reproducible runs.
- **变更：** `exec` 改为 fiber 级阻塞而非阻塞调度器，并新增 `exec_start`/`exec_poll` native
  （两个 `exec sleep 0.5` 的 fiber 现在 0.5 秒完成，此前 1.0 秒）；`BUR_DETERMINISTIC=1`
  串行化子进程以便复现。
- **Added:** Finished the formatter: full AST coverage, comment reinsertion from
  lexer trivia, and a verifier that rejects output unless it reparses to a
  structurally equal AST (`ast_eq`) with every comment intact.
- **新增：** formatter 完工：全 AST 覆盖、从 lexer trivia 重新插入注释，以及一个校验器 ——
  输出必须能重解析成结构相等的 AST（`ast_eq`）且注释无一丢失，否则拒绝。
- **Added:** The public `bur fmt <file|dir|->` command with `--check` and stdin
  modes, and formatted the whole `burc/` tree with it once.
- **新增：** 公开命令 `bur fmt <file|dir|->`，带 `--check` 与 stdin 模式，并用它把整棵
  `burc/` 树格式化了一遍。
- **Fixed:** `EnumVariantDecl` gained a real span (was `Sp(0, 0)`).
- **修复：** `EnumVariantDecl` 补上真实 span（此前是 `Sp(0, 0)`）。
- **Added:** `burc/lib/modgraph.bur`: offline S6.1 groundwork — bur.mod
  parsing, semver ordering, MVS over `$BURCACHE`, canonical tree hashes,
  bur.sum rendering/checking, and the hidden `bur dev mod-graph` command.
- **新增：** `burc/lib/modgraph.bur`：S6.1 的离线铺垫 —— bur.mod 解析、semver 排序、基于
  `$BURCACHE` 的 MVS、规范树哈希、bur.sum 渲染与校验，以及隐藏命令 `bur dev mod-graph`。
- **Changed:** Settled the remaining S6 design questions: interface files in the
  future optional-annotation syntax as the module cache (key: toolchain
  version + tree hash), subprocess isolation for `bur test`, std embedded
  into the binary, and the S6 order `S6.8 -> S6.2 -> S6.6 -> S6.4 ->
  S6.1 wiring`.
- **变更：** 敲定 S6 余下的设计问题：以将来的可选标注语法写成的接口文件充当模块缓存
  （key：工具链版本 + 树哈希）、`bur test` 的子进程隔离、std 内嵌进二进制，以及 S6 顺序
  `S6.8 -> S6.2 -> S6.6 -> S6.4 -> S6.1 接线`。
- **Changed:** Settled the S6 CLI layout after Go's vocabulary: `bur mod
  init/tidy/download/verify`, `bur get <path>@<version>`, and
  `bur test [dir] [--run <substr>] [-v]`; the CLI-naming item leaves the
  pending list.
- **变更：** 按 Go 的词汇敲定 S6 的 CLI 形态：`bur mod init/tidy/download/verify`、
  `bur get <path>@<version>`、`bur test [dir] [--run <substr>] [-v]`；CLI 命名一项移出待定列表。

**Landed S6.8, the checker-debt batch** — the checker sheds its
file-order semantics.
**落地 S6.8 checker 债批** — checker 摆脱文件字母序语义。

- **Changed:** Enum registration is two passes per package (collect every name,
  then validate field types), so enum fields may reference enums from any
  file in any order.
- **变更：** 枚举注册改为每包两遍（先收集全部名字，再校验字段类型），因此枚举字段可以任意
  顺序引用任意文件中的枚举。
- **Changed:** Package-level fns are inferred in SCC dependency order (Tarjan over a
  scope-aware free-name scan): a fn defined in a later file — and a self-
  or mutually-recursive fn — now stays polymorphic at every use site.
- **变更：** 包级函数按 SCC 依赖序推导（在作用域感知的自由名扫描上跑 Tarjan）：定义在靠后
  文件中的函数、以及自递归或互递归的函数，现在在每个使用点都保持多态。
- **Fixed:** `?` works inside mutually-recursive fn groups: an operand whose
  type is still unresolved defers the Option/Result decision to the end
  of the inference group; E0277 is reported only if it never resolves.
- **修复：** `?` 现在可用于互递归函数组：类型尚未定下的操作数把 Option/Result 判定推迟到
  推导组结束；只有始终无法定下时才报 E0277。
- **Other:** Surveyed the burc tree for the planned deep-`mut` flow rule (GOALS §2):
  32 violating sites out of ~1,230 checked; adopting or narrowing the
  rule stays an owner decision.
- **其他：** 为规划中的深 `mut` 流规则（GOALS §2）普查 burc 树：约 1,230 个检查点中有 32 处
  违规；采纳还是收窄该规则仍由 owner 决定。
- **Other:** burc's own sources keep the old file-order discipline: CI rebuilds the
  chain from the archived Go seed, whose checker still infers in file
  order.
- **其他：** burc 自身源码保留旧的文件序纪律：CI 从归档的 Go seed 重建整条链，而那个 checker
  仍按文件序推导。

**Landed S6.2 network fetch with the `bur mod` and `bur get` commands** —
dependency management is now end to end: require, fetch, lock, verify.
**落地 S6.2 网络拉取与 `bur mod` / `bur get` 命令** — 依赖管理全链打通：
require、拉取、锁定、校验。

- **Added:** `mod_fetch`: a shallow `git clone` of the `v<semver>` tag into
  `$BURCACHE`, with `.git` stripped before the tree enters the cache; a
  missing tag reports the offending `require` line. Clone URLs default to
  `https://<module path>`; `$BURGITBASE` overrides the prefix.
- **新增：** `mod_fetch`：对 `v<semver>` tag 做浅 `git clone` 落入 `$BURCACHE`，树进入缓存前
  剥掉 `.git`；tag 缺失时报出出错的 `require` 行。clone URL 默认 `https://<module path>`，
  `$BURGITBASE` 可覆盖前缀。
- **Added:** Wired `bur mod init <path>`, `bur mod tidy [dir]`, `bur mod download
  [dir]`, `bur mod verify [dir]`, and `bur get <path>@<version>` (which
  restores the previous bur.mod if the fetch fails).
- **新增：** 接线 `bur mod init <path>`、`bur mod tidy [dir]`、`bur mod download [dir]`、
  `bur mod verify [dir]` 与 `bur get <path>@<version>`（拉取失败时回滚原 bur.mod）。
- **Fixed:** Corrected the tree-hash encoding from hex to the settled
  `h1:<base64(sha256)>` format.
- **修复：** 树哈希编码从 hex 订正为定案的 `h1:<base64(sha256)>` 格式。

**Landed S6.4 `bur test` with subprocess isolation** — the first
first-class test runner; S6.6 std/json waits on an owner API decision, so
S6.4 landed first.
**落地 S6.4 `bur test`（子进程隔离）** — 首个一等测试跑器；S6.6 std/json
卡在 owner 的 API 决策上，故 S6.4 先行。

- **Added:** Discovers zero-parameter `fn test_*` in the root package's `*_test.bur`
  files; those files are now excluded from every normal build, run, and
  check.
- **新增：** 发现根包 `*_test.bur` 中的零参 `fn test_*`；这些文件此后从所有常规 build、run
  与 check 中排除。
- **Added:** Runs each test as its own subprocess (hidden `bur dev run-test`) with
  `BUR_DETERMINISTIC=1`; traps and deadlocks (exit 4) count as failures.
- **新增：** 每个测试以独立子进程运行（隐藏命令 `bur dev run-test`）并带 `BUR_DETERMINISTIC=1`；
  trap 与死锁（exit 4）计为失败。
- **Added:** Supports `--run <substr>` filtering and `-v`; main-less library packages
  are testable via a synthetic entry point.
- **新增：** 支持 `--run <substr>` 过滤与 `-v`；无 main 的库包可经合成入口点测试。
- **Fixed:** Corrected the self-path detail: a child's `/proc/self/exe` names the
  child, so the binary resolves itself via `sh -c "readlink
  /proc/$PPID/exe"`.
- **修复：** 订正自身路径的取法：子进程的 `/proc/self/exe` 指向子进程本身，故改用
  `sh -c "readlink /proc/$PPID/exe"` 让二进制定位自己。

**Settled the 2026-07-11 decision batch** — five pending items closed,
unblocking every remaining S6 line.
**敲定 2026-07-11 决策批** — 关闭五个待定项，S6 剩余主线全部解锁。

- **Changed:** Narrowed and adopted the deep-`mut` flow rule as an error: heap-typed
  (list/map) sources only, scalars exempt; an if/match source is fresh
  when every arm's tail is fresh; burc migrates its own violations before
  the rule lands.
- **变更：** 收窄并采纳深 `mut` 流规则为 error：仅限堆类型（list/map）来源，标量豁免；
  if/match 来源在每个臂的尾表达式都新鲜时才算新鲜；规则落地前 burc 先迁移自身的违规处。
- **Changed:** Ratified the S6.2 implementation defaults: `https://<module path>`
  clone URLs with the `$BURGITBASE` override, and `bur mod download`
  verifying bur.sum when present, writing it when absent.
- **变更：** 追认 S6.2 的实现侧默认值：clone URL 为 `https://<module path>` 并可由
  `$BURGITBASE` 覆盖，`bur mod download` 在 bur.sum 存在时校验、缺失时写出。
- **Changed:** Settled the std/json API: a seven-variant `Json` enum (`JInt`/`JFloat`
  split; objects as ordered parallel lists), bare `parse`/`render`/
  `pretty` behind the package prefix, sources at `std/json/` with a
  bur.mod, and a checked-in `burc/lib/std_embed.bur` generated by the
  hidden `bur dev embed-std`; `std/testing` lands in the same batch.
- **变更：** 敲定 std/json API：七变体 `Json` 枚举（`JInt`/`JFloat` 分开；对象用有序平行列表）、
  包前缀下的裸 `parse`/`render`/`pretty`、源码置于 `std/json/` 并带 bur.mod，以及由隐藏命令
  `bur dev embed-std` 生成并入库的 `burc/lib/std_embed.bur`；`std/testing` 同批落地。
- **Changed:** Settled the S7.8 annotation syntax — `name: type` parameters, `-> type`
  returns, reusing the existing type-expression grammar — which unblocks
  the S6.1 interface cache.
- **变更：** 敲定 S7.8 标注语法 —— `name: type` 形参、`-> type` 返回值，复用既有类型表达式
  语法 —— 从而解锁 S6.1 接口缓存。
- **Changed:** Settled S7.1 interpolation: a non-str value inside `{}` is a compile
  error suggesting an explicit `str()`.
- **变更：** 敲定 S7.1 插值：`{}` 内的非 str 值报编译错误，并建议显式 `str()`。

## v0.1 (2026-07-05 ~ 07-06)

**Initial community scaffolding added** — set up the open-source contribution
infrastructure and rounded out the README.
**新增初始社区脚手架** — 搭建开源贡献基础设施，补全 README。

- **Added:** `CONTRIBUTING.md`, `SECURITY.md`, and `CODE_OF_CONDUCT.md`, adapted to
  the `main`-only workflow and the bootstrap-fixpoint verification rule.
- **新增：** `CONTRIBUTING.md`、`SECURITY.md` 与 `CODE_OF_CONDUCT.md`，适配 `main` 单分支
  工作流与自举定点校验规则。
- **Added:** Apache-2.0 and self-hosted badges plus a "License & Disclaimer" section
  to `README.md`.
- **新增：** `README.md` 加入 Apache-2.0 与自举徽章，以及「License & Disclaimer」一节。
