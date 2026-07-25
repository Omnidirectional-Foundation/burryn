# SPEC — Burryn 项目规范

> SPEC v0.4.0 | Last updated: 2026-07-25
> 状态:最高优先级约束(权威来源) · 编号体系:统一 `S<n>[.<m>]`(见 §5)
> 相关文档:[`NUMBERING.md`](NUMBERING.md) 旧编号历史映射 · [`../tutorial.md`](../tutorial.md) 语言教程 · [`../README.md`](../README.md) 项目概览

> **注意：** 本文档是本项目的最高优先级约束。
> 所有实现工作以此为准。
> 遇到本文档未覆盖的设计决策，**停下来问 owner**，不要自行拍板。
> **警告：** 文中「已定」条目不接受实现侧擅自更改；发现定案之间冲突时，同样停下上报。

## 1. 一句话定位

**静态推导、零标注、CSP 并发的实用工具语言；rustc 级诊断，Go 级简洁与编译速度，单二进制交付；以完全自举为核心里程碑。**

目标场景：运维脚本、CSP 风格并发管道、带静态保障的小工具。
终局是 owner 日常真实使用的工具语言，不是 DSL，不是玩具。

## 2. 语言设计定案

### 显式优先

语法糖允许存在，但任何语义不得隐藏在语法糖背后——读者看到代码即可推断全部行为，无需查阅"这个上下文里它其实还做了 X"。隐式转换、隐式 coercion、magic method、隐式控制流转移(异常)均在拒绝清单。`?` 是显式早退(看到 `?` 就知道这里可能返回)；`defer` 是显式清理(看到 `defer` 就知道退出时跑)；插值是显式拼接的糖(`{expr}` 内必须是 str，非 str 编译错而非隐式 `str()`)；record 字面量带 `record` 关键字(不与 block 混淆)。类型推导是唯一的"隐式"——但 S7.8 可选标注使显式标注随时可用。

### 类型系统

- 静态，Hindley-Milner 全程序推导；函数参数/返回值零标注，仅枚举字段声明类型
- **可选函数签名标注(S7.8)**：「零标注」从「不能标」收窄为「不必标」——不标注的程序语义与推导结果不变；显式标注为 opt-in，用于诊断锚定与包边界 API 冻结，推导须与标注 unify，冲突报错
- 参数多态函数 + 泛型枚举；运算符用 SML 式受约束类型变量(`num` / `addord`)
- **受约束标注类型参数**：小写类型参数可在函数签名标注与接口文件中写为 `a:addord`；同一签名后续出现的 `a` 复用该变量。允许现有 checker 的 `num`、`addord`、`key`、`int`、`float`、`str` 约束；重复标注按既有约束交集规则合并，空交集报静态错。该语法只表达既有 HM 约束，不增加运行时语义。
- 禁止一切隐式转换
- **多态运行时表示：统一装箱(uniform boxing)**。
  不做单态化(monomorphization)，该决策覆盖字节码 VM 与全部原生后端
- **`--dyn` 逃生门：砍掉**。
  语言只有一套语义(静态检查)，不维护动态模式

### 值与内存

- **无 null / nil**。
  可空值一律用 `Option` 枚举 + 穷尽 `match` 表达
- **数值类型只有 `i64` 与 `f64`**，不做定宽整数全家桶
- **整数溢出一律 trap(运行时 panic)**，不区分 debug/release，不静默回绕
- 内存管理：GC(mark-sweep)。
  明确不做所有权/借用检查
- **`mut` 为深语义、绑定级纪律**：经由 `let` 绑定名不可修改其值(含容器内容)；push/元素修改要求 `mut`。
  不可变性挂在**绑定**上而非值上——无借用检查器与 move 语义，别名可绕过(实测 `let mut b = a` 后改 `b` 可见于 `a`)，故**不承诺值级不可变**。
  补强定案：checker 增加流规则——`mut` 形参的实参与 `let mut` 的初始化来源须本身可变或为新鲜值(字面量/构造/调用返回值)，违者 error。
  **只对堆类型(list/map 及含其的类型)生效**：int/float/bool/str/unit 为拷贝语义、无别名危害，豁免；**chan 整体豁免、不论元素类型**(send/recv/close 在现行纪律里本就不要求 mut，chan 别名是 CSP 语义本体，如 examples/concurrency/sieve.bur 的链式重绑)；if/match 作来源时递归看各臂尾表达式，皆新鲜则整体新鲜；来源类型未解时延迟判定(复用 S6.8 的组尾 flush 机制)。
  实现侧定案：检查点含 mut 绑定的**再赋值 RHS**(与初始化同规则，堵同源别名漏洞)；mut 形参实参检查走**旁路表**——绑定挂 mut-mask(init 为 FnLit 或包级 fn 声明时记录)，callee 为裸名/`pkg.name` 且解析到带 mask 的绑定才检查，经变量/参数的间接调用不查，fn 类型本身不携带 mut 标志；组尾 flush 时来源类型仍未解(已 generalize 的多态变量)**判 error**(宁滥勿缺)；错误码 **E0597**(与 E0596 直接改不可变绑定分码)。
  落地顺序：先迁移 burc 自身堆类型违例(约 10 处，见 §6.5 S6.8 条目，开工须重新插桩摸底)再启用规则，否则新 checker 编不过自己
- **参数默认不可变；`fn f(mut xs)` 声明可变参数**——已定，S2.5 实现(stdlib 原地操作与自举编译器的 emit 累积模式需要)。
  调用点无标记，与「无借用检查器 + GC」的定位一致，属 Go 式取舍

### 字符串

- **底层 UTF-8 字节序列**。
  `len` 与索引按字节；提供码点迭代器
- 后续字符串插值建立在字节语义之上

### 并发

- CSP：`spawn` + channel(`ch <- v` / `<-ch`)，死锁检测
- **`select` 与 `close(ch)` 为核心必做项**，无 select 的 CSP 视为残缺
- **执行模型长期承诺单 OS 线程**(并发 ≠ 并行，Node/Lua 路线)。
  纤程调度 + 时间片抢占，不做真并行——无借用检查器时真并行 + 深 mut 会引入数据竞争，且单线程使 GC 与原生运行时简化一个量级。
  此承诺覆盖全部后端
- **确定性承诺收窄**：纯计算程序跨后端逐字节确定；含 IO 程序不承诺调度顺序(IO 完成时序来自外部世界，与真 IO 重叠原理上互斥)。
  提供 opt-in 确定性模式(环境变量 `BUR_DETERMINISTIC=1`，IO 全串行化、timer 唤醒按 deadline + fiber 创建序双键排序)，`bur test`(S6.4)默认启用

### 错误处理

- `Result` + `?`，无异常机制

### 模块与导出

- 模块系统为 S2.2 必做项(自举前提)；形态：目录即包，去中心化 import path(与工具链设计一致)，细节待定——**动工前先与 owner 对齐方案**
- **导出语法：`pub` 关键字**，不用首字母大写

### 语法

- 参考系：Rust(`let`/`mut`、`match`、带字段枚举、`?`、表达式导向、遮蔽)+ Go(`spawn`、channel 语法、自动分号插入)
- 补充定案：`defer`(资源清理，脚本场景刚需)——归 **S7.6**，倾向块作用域(表达式导向下比 Go 的函数作用域干净)
- 语法当前未冻结；自举后冻结并产出正式 grammar 文件

### 明确拒绝清单(护住简洁，不接受重新提案)

- 宏 / 元编程
- trait / typeclass(S8 以后才可重新讨论)
- async/await(CSP 是唯一并发模型，不做第二套)
- 继承
- 异常
- 运算符重载
- 隐式类型转换
- null

## 3. 后端路线

| 后端 | 角色 | 状态 |
|------|------|------|
| 字节码栈式 VM | 开发与测试基线 + 自举 oracle/种子；已由 Burryn 重写(自举) | 已完成(Go 种子归档于 `archive/go-host`) |
| **C 后端** | S2 主力：可移植性由目标平台 C 编译器兜底，自举走此路径 | 已有(顺序 + 并发) |
| 手写 x86-64 + PE | S8.1:核心目标之一，owner 明确想做；不借第三方工具链 | 未动工 |

- 自举判定标准：**编译器由本语言写成且能编译自己**；输出 C 再经 gcc/clang 落地，完全算自举
- 「任何架构都能跑」由 C 后端承担；手写后端只承诺 x86-64，其余架构不做手写
- 双后端互为测试参照：同一程序在 VM / C 后端 / 手写后端输出必须一致，纳入测试
- 原生运行时：GC 为 **shadow stack 精确扫描**；单线程承诺使运行时无需线程同步

### 全自举终局

- **终局 = 全自举，只留 C 底座**：工具链里非 Burryn 的只剩 C 运行时 + 目标平台 cc；**Go 整棵树最终清零**——编译器前端、VM、CLI driver 全用 Burryn 重写
- **VM 由 Burryn 重写、经 cc 编成原生**；cc 成**工具链构建**的硬依赖(站 Rust 侧，自觉代价)。放弃"零 cc 工具链兜底"。
  注：VM 二进制建好后 `bur run` 运行期仍不需 cc；需 cc 的是构建工具链与 `bur build`。
  单二进制交付(§1)不变
- **分阶段**：S3 自举编译器前端 → S4 Burryn 重写 VM → S5 Burryn 写 CLI + 从 main 删 Go(先推留档分支 `archive/go-host`，重新接生靠 checkout 它)。
  每段自举判定 + parity 全绿才进下一段。
  **S3/S4/S5 已全部完成**：main 上 Go 整棵树已清零，`bur` 经 cc 逐字节重建自身；Go 种子归档于 `archive/go-host`
- 自举判定(输出 C 经 cc 落地 = 自举)、双后端互为测试参照、S8 手写 x86-64 + PE 才是 Go 级零工具链终局——这些不变

## 4. 工具链设计(单一二进制，cargo 式一体化)

**内核学 go，工程功能与 UX 学 cargo：**

学 go(解析与分发内核)：

- **MVS(最小版本选择)** 版本解析——确定性、无求解器、可复现
- **去中心化**：import path 即来源，不运营中心 registry；proxy 仅为缓存
- **禁止 build 期执行任意代码**(不做 build.rs 等价物)——供应链安全红线

学 cargo(工程功能与 UX)：

- workspace
- profile:仅 `debug` / `release` 两档，不开放自定义
- feature flags:**只允许布尔、纯加法(additive)** feature；禁止互斥 feature、禁止 feature 改变 API 签名；不做 optional dependency 绑 feature。
  解析两阶段：先 MVS 定版本，再取全图 feature 并集
- 顶级 UX:一个命令、好报错、内建 `test` / `fmt` / `build`
- `fmt` 唯一官方格式，零配置

## 5. 阶段里程碑(统一 S 编号)

全项目单一编号体系为 `S<n>[.<m>]`：`S<n>` 为阶段，`S<n>.<m>` 为阶段内可独立开工、独立验收(自举 fixpoint)的模块。
旧 `v1/v2/v3/v4`、`L1/L2`、旧「S4 工具链」编号一律作废，历史对照见 [`NUMBERING.md`](NUMBERING.md)。
状态标记：已完成 / 进行中 / 未开工。

| 阶段 | 子项 | 状态 |
|------|------|------|
| **S1 语义内核** | S1.1 HM 全程序推导(occurs + level generalize + let-poly)；S1.2 穷尽性检查；S1.3 GC(mark-sweep 保守栈扫描)；S1.4 CSP 基础(spawn/channel/死锁检测) | 已完成 |
| **S2 C 后端与语言完备** | S2.1 C 后端(顺序 + 并发)；S2.2 模块系统；S2.3 map；S2.4 `select` + `close`；S2.5 深 `mut` + `fn(mut xs)`；S2.6 `pub`；S2.7 必要 stdlib(os/exec、fs) | 已完成 |
| **S3 自举前端** | 编译器前端由 Burryn 写成并编译自己 | 已完成 |
| **S4 重写 VM** | VM 由 Burryn 重写，经 cc 编成原生 | 已完成 |
| **S5 删 Go** | CLI driver 用 Burryn 写；main 清零 Go；`archive/go-host` 留档 | 已完成 |
| **S6 生态工具链** | S6.1 依赖解析 **已完成**(MVS / `bur.sum` / 缓存包 import、interface declaration pipeline、disk interface cache/fallback、子包测试发现与 import-driven tidy)；S6.2 网络拉取 **已完成**(mod_fetch + `bur mod` 家族 + `bur get`)；S6.3 `bur fmt` **已完成**(全 AST + 注释重插 + 验证器 + 公开命令 + burc 全树已格式化)；S6.4 `bur test` **已完成**(子进程隔离 + `--run`/`-v` + 死锁/trap 归为失败；std/testing)；S6.5 诊断/DX 批 **已完成**(cgen `#line` + `-g`、runtime/VM trap 带 span stack trace、公开命令 rustc 风格诊断渲染 + 多 span 标注 + 结构化修复建议 + loader 诊断接线)；S6.6 std/json + std/testing **已完成**(核心实现 + CI regen+cmp)；S6.7 runtime IO **已完成**(sleep/timer + 异步 exec + idle-wait + 确定性模式)；S6.8 checker 债批 **已完成**(SCC 依赖序 + 枚举两遍注册 + `?` 延迟判定)；deep-mut checker 流规则 **已完成** | 已完成 |
| **S7 语言特性扩展** | S7.1 字符串插值 **已完成**；S7.2 管道 `|>` **已完成**；S7.3 match guard **已完成**；S7.4 命名参数 + 默认值(**已否决**，编号保留)；S7.5 编译期常量 **已完成**；S7.6 `defer` **已完成**；S7.7 net stdlib **已完成**；S7.8 可选函数签名标注(**已为 S6.1 提前完成**) | 已完成 |
| **S8 后端与重型类型** | S8.1 手写 x86-64 后端：**ELF**(Linux) + **Mach-O**(macOS) + **PE**(Windows)；S8.2 语法冻结 + grammar 文件；S8.3 row polymorphism；S8.4 封闭 records；S8.5 PE 后端(前提 = runtime Windows 移植)；S8.7 类型别名。**全部子项由 LLM 实现**。**x86-64 ELF 后端开发中**（list/closure/global/str native 已完成，match/enum 进行中，详见 §6.7） | 进行中 |
| **S9 LSP 与编辑器生态** | S9.1 LSP 核心服务器(JSON-RPC + 文档同步 + 诊断推送)；S9.2 语言特性(hover / go-to-def / completion / formatting / signature-help)；S9.3 VSCode 薄扩展(TextMate grammar + LSP client)；S9.4 nvim/vim/emacs 配置片段。前置 = S8.2 语法冻结。**S9.1 + S9.2(hover) + S9.3(VSCode 扩展)已落地**；剩余：go-to-def、completion、signature-help、formatting | 部分实现 |
| **S10 包生态** | 已有 std 包：`json`/`net`/`testing`（S6.6/S7.7 落地）、`cli`/`encoding`/`path`。待扩展：`log`/`datetime`/`regex`/`crypto`/`http`。S10.2 包模板 + 示例包；S10.3 `bur doc`（导出签名 + 注释 → Markdown）；S10.4 包质量基础设施（CI 模板、测试约定、版本规范）。原则：能纯 Burryn 就不加 native；每包带 bur.mod + *_test.bur，随 std_embed 分发 | 未开工 |

- S1–S5 为自举闭环，已全部达成：`bur` 由本语言写成、经 cc 逐字节重建自身，main 上 Go 整棵树已清零
- 自举判定标准：**编译器由本语言写成且能编译自己**；输出 C 再经 gcc/clang 落地，完全算自举
- stdlib 按「够自举用 + owner 真实脚本需求」逐个生长(os/exec、fs、json、net 优先)，不追大而全
- 编译速度是硬指标：任何特性提案先回答「是否显著拖慢编译」
- 触及 `ty_unify` / token 编号 / 自举链的改动(尤其 S6.8、S8.3/S8.4)，改完必验 fixpoint(gen1 == gen2 逐字节)
- **S8 内部推进顺序**：S8.3 row poly → S8.4 封闭 records → S8.1 ELF 后端 → S8.5 PE。类型先行(日常效用高于第二后端)。**S8 全部子项由 LLM 实现**。S8 之后进 S9 LSP 与编辑器生态

## 6. 工程规范

- Conventional Commits 1.0.0:`<type>(<scope>): <description>`，subject ≤72 字符，祈使句，无句号无 emoji
- 分支：`main` 受保护仅 PR 合入(merge commit，不 squash)；开发期集成分支 `dev/<topic>`
- 测试：自举 parity + 示例 golden test 覆盖全链路；自举判定为一等验收(`bur build burc` 逐字节重建自身)；重构类改动必须先有测试安全网再动手
- 诊断质量是卖点本体：错误信息按 rustc 标准要求自己(精确 span、指出修法)

## 6.5 S6 生态工具链（全部完成）

- **S6.1 依赖管理**：离线 MVS + `bur.sum` lockfile + 网络拉取 + 缓存加载 + interface declaration pipeline + disk interface cache/fallback + 子包测试发现 + import-driven tidy。`bur mod init/tidy/download/verify` + `bur get`。`$BURCACHE` 默认 `~/.burryn/pkg/`。`std/` 随工具链内嵌分发。
- **S6.2 网络拉取**：shell-out `exec("git",["clone",...])` 零新 native。clone URL = `https://<module path>`，`$BURGITBASE` 换前缀。树哈希 `h1:<base64>`。
- **S6.3 `bur fmt`**：全 AST + 注释重插 + 验证器（reparse + `ast_eq` + 注释计数）。`bur fmt <file|dir|->`，`--check`。
- **S6.4 `bur test`**：子进程隔离（`bur dev run-test <dir> <fn>`，`BUR_DETERMINISTIC=1`）。`*_test.bur` + `fn test_*()` 自动发现。trap/死锁 = FAIL。`std/testing`（`assert_eq`/`assert_ok`/`assert_err`）。
- **S6.5 诊断/DX**：cgen `#line` + `-g`（gdb 映射 .bur 行）；runtime trap 带 span stack trace；rustc 风格诊断渲染 + 多 span 标注 + 结构化修复建议。
- **S6.6 std/json + std/testing**：`Json` enum + `parse`/`render`/`pretty`/`get`。内嵌表 `std_embed.bur` checked in，CI 有 regen+cmp。
- **S6.7 runtime IO**：`sleep(ms)`/`exec_start`/`exec_poll` 三 native。调度器 idle-wait（fd poll + timer deadline）。`BUR_DETERMINISTIC=1` 串行化。
- **S6.8 checker 债批**：SCC 依赖序推导 + 枚举两遍注册 + `?` 延迟判定。deep-mut 流规则。seed 定基 `seed-base-1`（= `bc8a40a`），三段链。

## 6.6 轻量语法/语义扩展（S7，全部完成）

- **S7.1 字符串插值**：`{}` 内须为 str 值，非 str 编译错提示 `str()`。`{{` 转义。
- **S7.2 管道 `|>`**：Elixir 式 `lhs |> target` 脱糖为 lhs 作首参调用。`|>` 最低优先级、左结合。RHS = `path (“(“ args? “)”)?`（裸名/包成员/带参）。`?` 绑定紧于 `|>`。保留 Pipe AST 节点供 formatter 重建表层。
- **S7.3 match guard**：compiler 加条件跳转。
- **S7.4 命名参数**：**已否决**。
- **S7.5 编译期常量**：`const N = <可折叠式>`，包级（可 `pub`）与块级。初始化式限编译期可折叠（字面量、const 引用、算术/比较/bool、str 拼接）。折叠中溢出 = 编译错。不可 mut。
- **S7.6 defer**：挂包围函数，退出时 LIFO 执行。块是闭包、按闭包语义捕获。fiber 正常 return 执行；trap/死锁 = abort 不执行。
- **S7.7 net**：最小 TCP 面，6 native（`tcp_listen`/`tcp_accept`/`tcp_dial`/`net_read`/`net_write`/`net_close`），全部 fiber 级阻塞。调度器通用 fd 注册接口（socket/exec/timer 同入口，底层 poll）。
- **S7.8 可选签名标注**：参数 `name: type`、返回值 `-> type`。类型表达式复用 enum 字段语法。受约束变量 `a:addord`。标注可省略、语义不变。随 S6.1 接口缓存落地。

配套定案：调度器通用 fd 注册接口；DNS 解析(getaddrinfo)v1 同步阻塞整个调度器，标注为已知限制，真实需求出现再议异步 DNS；v1 不做 UDP/unix socket/TLS；`BUR_DETERMINISTIC=1` 下 net IO 与 exec 同策略串行化；测试用 loopback 端到端(listener + dialer 双 fiber)，不依赖外网；附带最小 `std/net` 包装包——`read_all(h) -> Result<str, str>`(读到 EOF)与 `write_line(h, s) -> Result<unit, str>`，随 std_embed 分发；burc 自身不调用 net native，vm.bur `do_native` 委托沿用 S6.7 两段提交纪律(先加 native 不消费、重建二进制后再接线)。

## 6.7 重型类型系统扩展评估(工程视角，对应 S8)

| 项 | 原评估 | 复评 | 理由 |
|---|---|---|---|
| Row Polymorphism | 高 / S8 | **S8.3** 首位，紧接封闭 record(S8.4) | 复用现有 var/generalize 加「行 var」，扁平 if-链撑得住；是结构化接口的公共地基，唯一值得投入的重型项 |
| Effects | S8+ | 明确排除 | 与现有 CSP(fiber/channel/select)竞争控制流转移；CSP 已覆盖 IO/并发大半；类型侧 effect row 还依赖 row poly |
| Refinement Types | 中 / 长期 backlog | 明确排除 | 无 constraint solver 地基，须从零造子系统；与轻标注工程气质冲突(Rust 未上) |
| GADTs | 暂不做 | 明确排除 | 通用工程价值最低，动 HM 最微妙处 |
| Linear(全局) | 永不 | 同意 | — |
| 局部 Affine(资源) | 未列 | 补进 backlog | file/socket/channel 的 use-after-close 检查，流敏感 lint 级，不碰 GC，能把 close-of-closed-channel 运行时 trap 提前为编译期错 |

**排除说明**：Refinement——无求解器地基，须从零造子系统，明确排除；Effects——与现有 CSP(fiber/channel/select)竞争控制流转移，CSP 已覆盖 IO/并发大半实用场景，边际价值与代价不成比例，明确排除。

**S8.1 大方向**：调用约定 = System V AMD64 ABI；GC 根扫描 = 沿用 C 后端的 shadow stack 精确 GC 语义(cgen 的根栈纪律照搬到手写代码生成)。该方向不阻塞 S6/S7 任何批次。**前置缺口**：语言目前无构造任意原始字节的能力——`chr(n)` 把 128–255 编为多字节 UTF-8，无法产出单字节 `0x80`–`0xFF`；ELF 发射(机器码、header、重定位表)需要全字节范围。修法 = 新增 `byte_chr(n: int) -> str`(0–255 → 单字节 str，越界 trap)，配合现有 `+` 拼接与 `write_file`(`fopen("wb")` + `fwrite`，已支持原始字节)即够。归 S8.1 前置小批，不触及类型系统核心。

**S8 分工**：S8 全部子项(S8.1–S8.5)由 LLM 实现，owner 审查设计与验收。

## 6.8 LSP 与编辑器生态(工程视角，对应 S9)

**架构定案**：LSP 服务器用 Burryn 写(延续自举原则)，作为 `bur lsp` 子命令，stdin/stdout 走 JSON-RPC 2.0(LSP 3.17 规范)。所有语言智能在服务器端；编辑器插件是**薄客户端**——只转发 LSP 消息 + 渲染 UI，不含语言逻辑。新增编辑器支持 = 实现 LSP client 协议，零服务器改动。

**传输层**：Content-Length 帧分割 + JSON-RPC 消息解析/路由；消息体 JSON 序列化/反序列化走 `std/json`。

**文档同步**：Full sync 模式(didOpen/didChange/didClose 全量内容)，服务器维护内存文档表，checker 读内存覆盖磁盘。增量 sync(delta)为后续优化，v1 不做。

**诊断推送**：didOpen 与 didChange(防抖)后重跑 check 管线(lex → parse → check)，DiagT/DiagX 转 LSP Diagnostic 推送。需要 check 管线支持从内存源码运行(现只从磁盘读文件)——这是 LSP 的核心工程改造点。

**语言特性(S9.2)**：
- hover：显示推导类型/签名(复用 checker 推导结果)
- go-to-definition：从名字使用处跳到绑定处(需 AST span → 定义 span 映射，跨文件走 module loader)
- completion：作用域感知名字补全(局部变量、函数名、包成员 `pkg.`)
- formatting：调 `bur fmt -`(stdin→stdout，已就绪)
- signature-help：函数调用时显示参数信息

**编辑器客户端**：

| 编辑器 | 技术栈 | 备注 |
|--------|--------|------|
| VSCode | TypeScript + vscode-languageclient | 首个客户端；bundled `bur` 二进制或 PATH 探测；TextMate grammar 语法高亮；Marketplace + VSIX 分发 |
| JetBrains | Kotlin + lsp4j | 单插件兼容 IDEA/CLion/PyCharm 等；IntelliJ 2023.2+ 内建 LSP API，旧版走 LSP4IJ |
| Neovim | 用户配置 | 0.8+ 原生 LSP，提供 `bur lsp` 配置片段即可 |
| Emacs | 用户配置 | lsp-mode 或 eglot 配置片段 |
| 其他 | — | Helix / Zed / Sublime 等 LSP-capable 编辑器直连 |

**实现顺序**：S9.1 核心服务器(JSON-RPC 传输 + 文档同步 + 诊断推送)→ S9.2 语言特性(hover / go-to-def / completion / formatting / signature-help)→ S9.3 VSCode 扩展(首个客户端，验证服务器)→ S9.4 JetBrains 插件 + 其他编辑器配置。

**前置与依赖**：S9 前置 = S8.2 语法冻结(语法不再变，LSP 不追语法债)。S9.1 的 check 管线改造(内存源码)可与 S8.3/S8.4 并行，但 S9 整体排在 S8 之后。`std/json` 已就绪(S6.6)，JSON-RPC 层无需新 native。**前置缺口**：语言目前无 stdin 读取能力——全部 native 中无 `read_line`/`read_stdin`/`input` 等，LSP 服务器的 JSON-RPC Content-Length 帧需要精确读 N 字节。修法 = 新增 `read_stdin(max: int) -> str`(读至多 max 字节，EOF 返回 `""`，fiber 级阻塞)或更通用的 fd 读取接口；配合现有 `print`(stdout 写)即构成 LSP 传输层。归 S9.1 前置小批。

## 7. 当前待定项(动工前必须先问 owner)

(暂无。新出现的待定设计决策须登记于此并先问 owner，规矩不变。)
