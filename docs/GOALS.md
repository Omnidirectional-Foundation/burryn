# GOALS.md — 项目目标与设计定案

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

### 类型系统

- 静态，Hindley-Milner 全程序推导；函数参数/返回值零标注，仅枚举字段声明类型
- **可选函数签名标注(S7.8，owner 2026-07-10 定)**：「零标注」从「不能标」收窄为「不必标」——不标注的程序语义与推导结果不变；显式标注为 opt-in，用于诊断锚定与包边界 API 冻结，推导须与标注 unify，冲突报错
- 参数多态函数 + 泛型枚举；运算符用 SML 式受约束类型变量(`num` / `addord`)
- **受约束标注类型参数定案(owner 2026-07-15)**：小写类型参数可在函数签名标注与接口文件中写为 `a:addord`；同一签名后续出现的 `a` 复用该变量。允许现有 checker 的 `num`、`addord`、`key`、`int`、`float`、`str` 约束；重复标注按既有约束交集规则合并，空交集报静态错。该语法只表达既有 HM 约束，不增加运行时语义。
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
- **`mut` 为深语义、绑定级纪律(owner 2026-07-10 修订)**：经由 `let` 绑定名不可修改其值(含容器内容)；push/元素修改要求 `mut`。
  不可变性挂在**绑定**上而非值上——无借用检查器与 move 语义，别名可绕过(实测 `let mut b = a` 后改 `b` 可见于 `a`)，故**不承诺值级不可变**。
  补强定案(2026-07-11 owner 收窄采纳)：checker 增加流规则——`mut` 形参的实参与 `let mut` 的初始化来源须本身可变或为新鲜值(字面量/构造/调用返回值)，违者 error。
  **只对堆类型(list/map 及含其的类型)生效**：int/float/bool/str/unit 为拷贝语义、无别名危害，豁免；**chan 整体豁免、不论元素类型**(owner 2026-07-12 定：send/recv/close 在现行纪律里本就不要求 mut，chan 别名是 CSP 语义本体，如 examples/sieve.bur 的链式重绑)；if/match 作来源时递归看各臂尾表达式，皆新鲜则整体新鲜；来源类型未解时延迟判定(复用 S6.8 的组尾 flush 机制)。
  实现侧定案(owner 2026-07-12 四问)：检查点含 mut 绑定的**再赋值 RHS**(与初始化同规则，堵同源别名漏洞)；mut 形参实参检查走**旁路表**——绑定挂 mut-mask(init 为 FnLit 或包级 fn 声明时记录)，callee 为裸名/`pkg.name` 且解析到带 mask 的绑定才检查，经变量/参数的间接调用不查，fn 类型本身不携带 mut 标志；组尾 flush 时来源类型仍未解(已 generalize 的多态变量)**判 error**(宁滥勿缺)；错误码 **E0597**(与 E0596 直接改不可变绑定分码)。
  落地顺序：先迁移 burc 自身堆类型违例(约 10 处，摸底数字见 §6.5 S6.8 条目——注意该数字测于 S6.2/S6.4 落地前，开工须重新插桩摸底)再启用规则，否则新 checker 编不过自己
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
- **确定性承诺收窄(owner 2026-07-10 定)**：纯计算程序跨后端逐字节确定；含 IO 程序不承诺调度顺序(IO 完成时序来自外部世界，与真 IO 重叠原理上互斥)。
  提供 opt-in 确定性模式(环境变量 `BUR_DETERMINISTIC=1`，IO 全串行化、timer 唤醒按 deadline + fiber 创建序双键排序)，`bur test`(S6.4)默认启用

### 错误处理

- `Result` + `?`，无异常机制

### 模块与导出

- 模块系统为 S2.2 必做项(自举前提)；形态：目录即包，去中心化 import path(与工具链设计一致)，细节待定——**动工前先与 owner 对齐方案**
- **导出语法：`pub` 关键字**，不用首字母大写

### 语法

- 参考系：Rust(`let`/`mut`、`match`、带字段枚举、`?`、表达式导向、遮蔽)+ Go(`spawn`、channel 语法、自动分号插入)
- 补充定案：`defer`(资源清理，脚本场景刚需)——归 **S7.6**，倾向块作用域(表达式导向下比 Go 的函数作用域干净)，实现前细化(owner 2026-07-10 定)
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
- 原生运行时：GC 为 **shadow stack 精确扫描**(C 后端定案已落地，取代早期「保守式栈扫描」设想，2026-07-10 纠错)；单线程承诺使运行时无需线程同步

### 全自举终局(owner 2026-07-04 定)

- **终局 = 全自举，只留 C 底座**：工具链里非 Burryn 的只剩 C 运行时 + 目标平台 cc；**Go 整棵树最终清零**——编译器前端、VM、CLI driver 全用 Burryn 重写
- **改写本节旧定**：「VM(Go 宿主)为永久主力 / 零 C 依赖兜底」作废。
  VM 改由 Burryn 写、经 cc 编成原生；放弃"零 cc 工具链兜底"，cc 成**工具链构建**的硬依赖(站 Rust 侧，自觉代价)。
  注：VM 二进制建好后 `bur run` 运行期仍不需 cc；需 cc 的是构建工具链与 `bur build`。
  单二进制交付(§1)不变
- **分阶段**：S3 自举编译器前端 → S4 Burryn 重写 VM → S5 Burryn 写 CLI + 从 main 删 Go(先推留档分支 `archive/go-host`，重新接生靠 checkout 它)。
  每段自举判定 + parity 全绿才进下一段。
  **S3/S4/S5 已全部完成**：main 上 Go 整棵树已清零，`bur` 经 cc 逐字节重建自身；Go 种子归档于 `archive/go-host`
- **不改**：自举判定(输出 C 经 cc 落地 = 自举)、双后端互为测试参照、S8 手写 x86-64 + PE 才是 Go 级零工具链终局

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
| **S2 C 后端与语言完备** | S2.1 C 后端(顺序 + 并发)；S2.2 模块系统；S2.3 map；S2.4 `select` + `close`；S2.5 深 `mut` + `fn(mut xs)`；S2.6 `pub`；S2.7 必要 stdlib(os/exec、fs——json/net 当时未实现，2026-07-10 纠错移入 S6.6/S7.7) | 已完成 |
| **S3 自举前端** | 编译器前端由 Burryn 写成并编译自己 | 已完成 |
| **S4 重写 VM** | VM 由 Burryn 重写，经 cc 编成原生 | 已完成 |
| **S5 删 Go** | CLI driver 用 Burryn 写；main 清零 Go；`archive/go-host` 留档 | 已完成 |
| **S6 生态工具链** | S6.1 依赖解析 **已完成**(2026-07-18：MVS / `bur.sum` / 缓存包 import、interface declaration pipeline、disk interface cache/fallback、子包测试发现与 import-driven tidy)；S6.2 网络拉取 **已完成**(2026-07-10：mod_fetch + `bur mod` 家族 + `bur get`)；S6.3 `bur fmt` **已完成**(2026-07-10：全 AST + 注释重插 + 验证器 + 公开命令 + burc 全树已格式化)；S6.4 `bur test` **已完成**(2026-07-10：子进程隔离 + `--run`/`-v` + 死锁/trap 归为失败；2026-07-15 补齐 std/testing)；S6.5 诊断/DX 批 **已完成**(2026-07-19：cgen `#line` + `-g`、runtime/VM trap 带 span stack trace、公开命令 rustc 风格诊断渲染 + 多 span 标注 + 结构化修复建议 + loader 诊断接线)；S6.6 std/json + std/testing **已完成**(2026-07-15 核心实现；2026-07-18 CI regen+cmp)；S6.7 runtime IO **已完成**(2026-07-10：sleep/timer + 异步 exec + idle-wait + 确定性模式)；S6.8 checker 债批 **已完成**(2026-07-10：SCC 依赖序 + 枚举两遍注册 + `?` 延迟判定)；deep-mut checker 流规则 **已完成**(2026-07-15) | 已完成 |
| **S7 语言特性扩展** | S7.1 字符串插值 **已完成**(2026-07-19)；S7.2 管道 `\|>`；S7.3 match guard；S7.4 命名参数 + 默认值(**已否决** 2026-07-10，编号保留)；S7.5 编译期常量；S7.6 `defer`(倾向块作用域)；S7.7 net stdlib(依赖 S6.7 的 fd 感知调度讨论)；S7.8 可选函数签名标注(**已为 S6.1 提前完成**，2026-07-16) | 进行中 |
| **S8 后端与重型类型** | S8.1 手写 x86-64 **ELF** 后端；S8.2 语法冻结 + grammar 文件；S8.3 row polymorphism；S8.4 封闭 records；S8.5 PE 后端(前提 = runtime Windows 移植：ucontext 与 POSIX natives 全需替代) | 未开工 |

- S1–S5 为自举闭环，已全部达成：`bur` 由本语言写成、经 cc 逐字节重建自身，main 上 Go 整棵树已清零
- 自举判定标准：**编译器由本语言写成且能编译自己**；输出 C 再经 gcc/clang 落地，完全算自举
- stdlib 按「够自举用 + owner 真实脚本需求」逐个生长(os/exec、fs、json、net 优先)，不追大而全
- 编译速度是硬指标：任何特性提案先回答「是否显著拖慢编译」
- 触及 `ty_unify` / token 编号 / 自举链的改动(尤其 S6.8、S8.3/S8.4)，改完必验 fixpoint(gen1 == gen2 逐字节)
- **S8 内部推进顺序(owner 2026-07-10 定)**：S8.3 row poly → S8.4 封闭 records → S8.1 ELF 后端 → S8.5 PE。类型先行(日常效用高于第二后端)；手写后端是 owner 明确想做的目标，但不挡效用主线

## 6. 工程规范

- Conventional Commits 1.0.0:`<type>(<scope>): <description>`，subject ≤72 字符，祈使句，无句号无 emoji
- 分支：`main` 受保护仅 PR 合入(merge commit，不 squash)；开发期集成分支 `dev/<topic>`
- 测试：自举 parity + 示例 golden test 覆盖全链路；自举判定为一等验收(`bur build burc` 逐字节重建自身)；重构类改动必须先有测试安全网再动手
- 诊断质量是卖点本体：错误信息按 rustc 标准要求自己(精确 span、指出修法)

## 6.5 S6 生态工具链(自举完成后，owner 2026-07 定)

自举闭环已成(S1–S5)，下一批目标是「让别人能日常用」的生态工具链，全部用 Burryn 自写，延续零 Go 依赖。
关键路径：**依赖管理 → `bur fmt` → `bur test`**，debugger 作 C 后端增强并行可选。

**S6.1 + S6.2 依赖管理(P0，MVS + fetch + lockfile)**

现状(2026-07-18)：离线 MVS、`bur.sum`、网络拉取、缓存定位与 require 闭包加载均已落地；跨模块 import 已能从 `$BURCACHE` 加载 required package 及其子包；interface reader、导出面 renderer 与 checker consumption pipeline 已落地；disk interface cache/fallback 已落地(`$BURCACHE/.interfaces/<toolchain>-<树哈希>/`，`.buri` + `.meta` channel-for sidecar，静默 miss、error 触发全量源码重跑、只写干净结果、原子替换；无 bur.sum 的模块零行为变化)；子包测试发现已落地(`bur test` 遍历全部子包，子包测试显示为 `<rel>/<fn>`，`bur dev run-test` 自任意包目录向上定位模块根)；`bur mod tidy` 按 import 增删 require 已落地(未用 require 删除、间接依赖被直接 import 时按 MVS 已选版本提升为直接 require、查无报错指向 `bur get`；`bur get` 改走 sum-sync，新加 require 不会被随手 tidy 掉)。
S6.1 无剩余项。

- S6.1 解析层(无网络)——**已落地**：`burc/lib/modgraph.bur` 构建依赖图跑 MVS(选满足约束的**最低**版本)；`bur.sum` lockfile(`path version hash`)；module loader 命中 require 图后从缓存加载跨模块 import；缓存目录 `$BURCACHE` 默认 `~/.burryn/pkg/<path>@<version>/`
- S6.2 网络拉取：倾向 shell-out `exec("git",["clone",...])` + `sha256sum` 校验(零新 native，延续 S5「simplify, no new natives」)；备选补最小 `http_get`/`sha256` native——**已落地(2026-07-10)**：shell-out 方案成立，零新 native；`bur mod init/tidy/download/verify` 与 `bur get` 全部接线；`bur get` 拉取失败回滚 bur.mod；树哈希输出已从 hex 纠正为定案的 `h1:<base64>`。**实现侧默认(2026-07-11 owner 追认为定案)**：clone URL = `https://<module path>`(Go 式「模块路径即仓库路径」，不支持子目录模块，真实需求出现再议 discovery)，环境变量 `$BURGITBASE` 可换 URL 前缀(镜像/离线测试)；`bur mod download` 在 bur.sum 存在时校验、缺失时写出
- **CLI 布局已定(owner 2026-07-10，照搬 Go 词汇表)**，随 S6.1/S6.2 落地：
  - `bur mod init <module-path>`：写 `bur.mod`，module path 显式给出、不从目录名猜
  - `bur mod tidy`：离线重算 MVS、重写 `bur.sum`(按 import 增删 require 已落地，2026-07-18)。**增加侧版本语义已定(owner 2026-07-17)**：新直接 require 的版本取自现有 MVS 依赖图(间接依赖被直接 import 时提升为直接 require、沿用已选版本)；依赖图查无该模块则报错并指向 `bur get <path>@<version>`——不猜版本、不上网
  - `bur mod download`：拉取 require 闭包进缓存并校验(S6.2)
  - `bur mod verify`：对照缓存树与 `bur.sum`(库函数 sum_check 已就位)
  - `bur get <path>@<version>`：写入/升级 require + download + tidy(S6.2)
  - `bur test [dir] [--run <substr>] [-v]`：顺序子进程测试，`--run` 子串过滤，`-v` 逐个打印；并行 `-j` 为后续加法
  - flag 风格沿用现状：长 flag(`--check`/`--emit c` 式) + 单字母短 flag(`-o`/`-v` 式)
- 遵守 §4 红线：MVS 确定性、去中心化、禁 build 期执行任意代码
- 补充定案(owner 2026-07-10)：
  - `bur.sum` 行格式 `<path> <version> h1:<base64(树哈希)>`；树哈希 = 规范化目录哈希(路径排序 + 逐文件 sha256 汇总，Go dirhash 式)；版本 ↔ git tag 映射 `v<semver>`
  - S6.1 离线解析只读 `$BURCACHE`；cache miss 报错并提示 `bur mod download`(自动拉取归 S6.2)
  - **std 分发形式 = 随工具链捆绑**：保留 import 前缀 `std/`，版本跟工具链走，不经网络拉取；modgraph 解析须把 `std/` 特判为工具链内置，绝不落缓存/网络路径
  - **接口缓存已定(owner 2026-07-10，原必答题)**：对每个 `path@version` 首次编译后，把导出面序列化为接口文件缓存(enum 定义原样复制、fn 导出写成签名)；**接口文件语法 = S7.8 可选标注语法**(标注语法先在此定型，接口文件即自动生成的人类可读声明文件)；缓存 key = (工具链版本, 模块树哈希)，安全性由 bur.sum 锁定的哈希承担。**S7.8 语法形态已定(owner 2026-07-11，移出 §7 待定)**：参数 `name: type`、返回值 `-> type`，类型表达式复用 enum 字段既有语法(`[T]`、`fn(...) -> T`、`map(K, V)`、小写名即类型参数；受约束变量写 `a:addord`，规则见 §2)，标注可省略、语义不变。**落地状态(2026-07-16)**：参数/返回标注、普通泛型变量推导与 `a:addord` 的 parser/AST/formatter/module/checker 传播均已完成；interface reader、确定性导出面 renderer 与 checker consumption pipeline 已落地，可生成/读取完整 S7.8 类型表达式。disk cache、失败回退与 companion metadata 尚未落地。以下五条实现侧细则(随 2026-07-16 状态同步写入)owner 2026-07-17 追认为定案：
    - 导出函数写成无函数体的 `pub fn name(param: Type) -> Return`；导出值写成带类型的 `pub let name: Type`；`pub enum` 原样复制
    - exported signature 引用的 private enum 作为非 `pub` 支撑声明递归复制，维持现有可见性
    - 无函数体声明与 typed `let` 只允许 interface reader 读取，普通 `.bur` source parser 不接受
    - 缓存 identity = (toolchain version, module tree hash)；缺失或无效时回退 source checking 并重建
    - cached interface 只替换 checker input；`compile_program` 仍使用保留函数体的原始 source AST
  - S6.2 执行细节：clone 后进缓存前 strip `.git`(树哈希不含 git 元数据)；tag 缺失的报错指向对应 `require` 行

**S6.3 `bur fmt`(P0，文化基础设施)——已完成(2026-07-10)**

与包管理并列优先——越早冻结格式，后期生态一致性成本越低(§4「唯一官方格式，零配置」)。

- 复用 `lib.parse` 的 AST → 新 `burc/lib/format.bur` 写 pretty-printer；规则对齐 burc/lib 现有风格(换行即分号、块结构、表达式导向、4-space、`match` arm 对齐)
- 验收铁律(2026-07-10 补全为三条)：**幂等** `fmt(fmt(x))==fmt(x)`；**AST 不变**(执法机制 = 写回前强制 reparse + 忽略 span 的 `ast_eq` 结构比对)；**注释 trivia 不得丢失**(执法机制 = 注释计数一致检查)。任一失败拒绝写回并报 internal error
- 格式定案：顶层声明之间恰好一个空行，文件首不留空行；formatter 不做长行折行(折行如需要是独立后续 stage)
- 前置已就绪：lexer 现把注释作旁路 trivia 收集(`LexOut.Lexed` 第三字段，见 §6.6 前置)，`bur fmt` 可按 span 重插注释
- 命令：`bur fmt <file|dir>`(原地写回)、`--check`(CI，有 diff 则非 0 退出)、`-`(stdin→stdout，供 LSP format-on-save)
- 注释重插(stage 3)与验证器全绿**之后**才跑在 `burc/lib/` 自身统一风格；在此之前禁止对真实源码原地写回(2026-07-10 修订，早期「立即跑」措辞作废)
- stage 划分：stage 2 全 AST 节点覆盖 → stage 3 注释重插 → stage 4 `bur fmt` 公开命令 + burc 自格式化

**S6.4 `bur test`(P1)——已完成(2026-07-10；断言糖于 2026-07-15 补齐)**

现状：`bur test` 已是 first-class 测试命令；子包发现已随 S6.1 落地(2026-07-18)，`-j` 并行留在 backlog。

- 约定 `*_test.bur` + `fn test_*()` 自动发现；断言 API 归 stdlib(`assert_eq`/`assert`/`assert_ok`/`assert_err`，贴合 `Result`/`Option`)
- 并发语义：利用 fiber/channel 语义，把 VM 死锁检测(现为 exit 4)转成测试失败；`-j` 并行尚未实现，真实测试变慢后再补
- 报告：pass/fail 计数 + 失败 span 定位(复用 diag 渲染)
- **隔离模型已定(owner 2026-07-10，原必答题)：子进程隔离**——`bur test` 对每个 `test_*` `exec` 自身跑隐藏命令(`bur dev run-test <dir> <fn>`)收集 exit code 与输出；trap(exit 4)与死锁自然成为 test failure；默认 `BUR_DETERMINISTIC=1`；并行跑测试 = `exec_start` fan-out(S6.7 已解锁)
- 自身二进制路径经 shell-out `readlink /proc/self/exe` 获取(零新 native；不够用再议 `self_path` native)——**落地纠正(2026-07-10)**：子进程里 `/proc/self/exe` 指向子进程自身(readlink 二进制)，等价零 native 解 = `sh -c "readlink /proc/$PPID/exe"`($PPID = 发起 exec 的 bur 进程)
- `std/testing` 已提供 `assert_eq` / `assert_ok` / `assert_err`，随 S6.6 内嵌分发
- **落地情况(2026-07-10)**：约定 = 根包 `*_test.bur` 的零参 `fn test_*`(带参不发现；子包测试未纳入——owner 2026-07-12 定：随 S6.1 接线批顺手纳入)；`*_test.bur` 从此被普通 build/run/check 排除；无 `fn main` 的库包可测(测试入口合成)；死锁/trap(exit 4)自然记为 FAIL；并发测试模式与 `-j` 并行未做(exec_start fan-out 已解锁——owner 2026-07-12 定：归 backlog，真实测试变慢再做)

**S6.5 诊断/DX 批(owner 2026-07-12 扩容：debugger + 诊断深度)**

- **前置(owner 2026-07-17)**：seed 定基完成后才动工——本批的 cgen 与 runtime 改动会破坏归档 seed 的 gen1 == gen2 判定(见 S6.8 条目定基修订)。**前置已满足(2026-07-18)**：tag `seed-base-1` 已推送，CI 三段链全绿
- cgen 生成 C 时插 `#line <n> "<file>"`(cgen 已知每 Node span)→ 原生二进制直接 `gdb`/`lldb` 映射回 `.bur`
- runtime trap 打印带 source span 的 stack trace(复用 diag + line_starts)
- 符合「终局只留 C 底座」，属后端增强非新工具
- 扩容(owner 2026-07-12)：诊断深度 backlog 里最值钱的**多 span 标注 + 结构化修复建议**并入本批同做；`--explain`、JSON/彩色输出、compile 首错即停等其余项仍按需不排期
- **落地状态(2026-07-19，全批完成)**：cgen 每函数头与每条指令前发射 `#line`(只在行变化处发射会被 cc 递增行号错标)、`bur build` 加 `-g`，gdb 三帧精确映射 `.bur` 行；runtime trap 打印 `at <fn> (<file>:<line>)` 逐帧 trace(burrt.h per-fiber trace 栈 + VM 侧 `fb_fr_callip` 对等实现，两侧输出逐字节一致，无源文件的合成帧抑制)；DiagT 增 `DiagX(..., [LabelT], [FixT])` 变体，`Lab(start, end, label)` 同文件次级 span、`Fix(start, end, replacement, desc)` 结构化建议；公开命令(`check`/`run`/`build`)走 `render.bur` rustc 风格渲染(file:line:col + 源行摘录 + caret + help/fix，源码不可读时降级 header-only)，dev parity dump 保持旧裸格式逐字节不动；loader 诊断(unused_import 等)接入公开命令渲染(此前被静默丢弃，`run_test_target` 有意不接防测试子进程刷屏)；判定沿用 5a 修正(gen2 == gen3)，首次实证 gen1 != gen2(cgen 发射变更所致，符合预期)

**S6.6 std/json + std/testing(纯 Burryn，零新 native)——核心实现已完成(2026-07-15)**

- 前提：std 分发形式已定(随工具链捆绑，`std/` 保留前缀，见 S6.1 补充定案)
- **捆绑机制已定(owner 2026-07-10)：内嵌进二进制**——std 源码构建时转为字符串常量编进 burc，`import "std/..."` 从内嵌表取源码；与 §1 单二进制交付一致，无安装布局探测。「禁 build 期执行任意代码」红线针对第三方包，工具链自建生成物不在其列
- json 解析/序列化全用 Burryn 实现，作为 std 首个成员与捆绑机制一起落地
- 来历：S2.7 曾把 json/net 标为已完成，2026-07-10 核实均未实现，json 移入本项、net 移入 S7.7
- **API 定案(owner 2026-07-11，原 §7 四问全部关闭)**：
  - 值表示 = `pub enum Json { JNull, JBool(bool), JInt(int), JFloat(float), JStr(str), JArr([Json]), JObj([str], [Json]) }`；对象用**保序平行列表**(round-trip 稳定、序列化确定，贴合确定性承诺)，配 `get(keys, vals, k) -> Option<Json>` 式帮手；数字双变体：字面量无小数点无指数且在 i64 范围 → `JInt`，否则 `JFloat`
  - 函数名走包前缀裸名：`parse(s) -> Result<Json, str>`(错误消息带字节偏移)、`render(v) -> str`(紧凑)、`pretty(v, indent) -> str`
  - 源码布局 = repo 根 `std/json/`，带 `bur.mod`(`module std/json`)：开发期直接 `bur check`/`bur test` 走本地 loader，发布走内嵌表，同一份源码
  - 生成器 = 隐藏命令 `bur dev embed-std` 扫 `std/` 生成 `burc/lib/std_embed.bur`(字符串常量表，**checked in**——seed 编 burc 也需要它)；CI 加「重新生成 + cmp」一步防手改漂移
  - `std/testing`(`assert_eq`/`assert_ok`/`assert_err`)与 json 同批、同机制落地，顺带清 S6.4 的断言糖债
- **落地状态(2026-07-15)**：`std/json` 的 `parse` / `render` / `pretty` / `get`、`std/testing`、内嵌表生成器与 module loader 接线均已完成；仅剩 CI 的 `bur dev embed-std` regen+cmp

**S6.7 runtime IO 工作包——已完成(2026-07-10)**

- 落地前基线：全部 IO native 同步阻塞整个调度器(实测两个 `exec sleep 0.5` 串行跑 1.008s)；CSP 只能交错纯计算，对运维脚本的 exec/net 并发场景空转
- 落地方案 = 最小异步 exec + timer，不做通用 async IO(现无 socket，epoll 无对象；通用化等 S7.7 net 一并议)：
  - 已新增 native：`sleep(ms)`(CLOCK_MONOTONIC)、`exec_start(cmd, args) -> Result<int, str>`(handle 为 int)、`exec_poll(handle) -> Option<Result<Output, str>>`(未完成 None；命令不存在等 fork 后错误经此浮出；收割后 handle 失效，poll 无效 handle 为 trap)
  - 三个原语均为公开 native，可供用户手写 fan-out
  - 既有 `exec` 语义不变，降级为 **fiber 级阻塞**(内部 start/poll/yield 循环；独跑时直接阻塞 poll 自身 fd)，调度器级不再阻塞
  - 调度器(burrt.h 与 vm.bur **两份**，parity 铁律)增加 idle-wait：无 runnable fiber 时 poll(等待 fd 集，timeout = 最近 timer deadline)；死锁检测把 IO/timer 等待者视为活跃
  - 确定性模式(`BUR_DETERMINISTIC=1`)：IO 串行化 + timer 唤醒按 deadline + fiber 创建序双键排序
- 每个新 native 照五处约定走，加完必重跑自举定点
- **重新接生缺口(2026-07-16 实测)**：`archive/go-host` 归档 seed 早于本批的 `sleep` / `exec_start` / `exec_poll` native 声明；用该 seed 执行 `bur-seed build burc -o gen1` 会在 `burc/lib/vm.bur` 报 7 个 E0425，gen1 未生成。runtime IO 实现本身仍已完成，但 CI 的 seed→gen1 路径须在继续 S6.1 前修复；修复前不得把当前 main 记为可重新接生或 CI 全绿。**已修复(2026-07-17，archive/go-host f239cd3)**：seed 补三 native 的 checker 声明、Go VM 实现(sleep 阻塞调度器、exec_start 同步跑完子进程——seed 无 timer/IO parking，重生链只用 `bur-seed build`，不经这些路径)与 native 名表(yield 之后，序同 burc)；seed 内嵌的 `runtime/burrt.h` + `burrt_natives.h` 与 main 逐字节同步(头文件内容编进二进制，是 gen1 == gen2 的必要条件)。本地 worktree 链与纯 `git archive` 快照复现均达 fixpoint(gen1 == gen2 == 当前 bur)；归档分支 Go 测试套件的既有环境性失败集经改动前后对照零新增。CI 真绿以 owner push 后的实际运行为准

**S6.8 checker 债批——已完成(2026-07-10)**

- 包级值推导改 SCC 依赖序，根除文件字母序语义(quirk #9)——已落地：作用域感知自由名扫描 + Tarjan，组内共享推导 level、组尾统一 generalize；自递归/相互递归函数也不再被 pin 成单态
- 枚举注册改两遍(先收名字再验字段类型)，根除跨文件枚举只能「向前」引用(quirk #2)——已落地
- `?` 在相互递归函数组内可用(`Result + ?` 是唯一错误机制，必须处处可用)——已落地：操作数类型未解时延迟到推导组尾再判 Option/Result，仍未解才报 E0277
- 三项都触及推导核心，同批做、一次验自举定点——已验(gen1 == gen2 逐字节)
- **seed 兼容注意**：CI 从 `archive/go-host` 的 Go seed 重建全链，seed 的 checker 仍是旧规则，故 **burc 自身源码继续遵守旧纪律**(文件字母序、bounce 惯用法)直至重新定基 seed。**定基时机已修订(owner 2026-07-17，取代 2026-07-12「S6 收尾发布」定案)：S6.5 诊断/DX 批动工前**，即 S6.1 接口缓存、子包测试发现、import-driven tidy 与 CI embed regen+cmp 完成后立即定基——S6.5 的 cgen `#line` 发射与 runtime trap-trace 改动会结构性破坏 seed 链(gen1 出自归档 cbackend.go、gen2 出自 main cgen.bur，逐字节 cmp 要求两代 C 与内嵌 runtime 头文件完全一致，2026-07-17 修复归档 seed 时实证过一次)，提前定基使归档 seed 在 S6.5 前退出接生链；**定基机制已定(owner 2026-07-17)：tag 基准 commit**——在 main 上打 tag 标记基准，重生链改三段(归档 Go seed → 构建基准 commit 的 burc → 构建当前 burc；**判定修正 2026-07-18**：gen1 出自冻结的基准 cgen 发射，当前 cgen 发射一变 gen1 就合理地 != gen2，故定点判定 = 再自举一代比 **gen2 == gen3**——gen1/gen2 内含同一份当前编译器逻辑，发射的 C 必须一致；cgen 不变时 gen1 == gen2 == gen3，严格向后兼容)，基准 commit 的源码永久遵守旧纪律、当前源码自定基起解禁，纯源码可重建、不引入二进制工件信任；定基后 S6.5 与 S7 从新 seed 起步；**v0.3 发布判据 = S6 全部收尾不变，与定基解绑**。**定基已完成(2026-07-18)**：ci.yml 三段链(基准 tag 名 = `seed-base-1`，`env.SEED_BASE_TAG`)，owner 已打 tag `seed-base-1`(= `bc8a40a`)并推送，CI 三段链实跑全绿(run 29637257373，bootstrap-fixpoint 12m30s)；**burc 源码自此解禁三条旧纪律**(文件字母序、bounce 惯用法、跨文件枚举向前引用)，基准 commit `bc8a40a` 的源码永久遵守旧纪律
- deep-mut 流规则(§2)——**已完成(2026-07-15)**：local/global `let mut`、再赋值 RHS 与静态可知 mut 形参调用均已覆盖；list/map 递归检查、chan/标量豁免、fresh 来源与 pattern binding 规则均已落地并通过 fixpoint

**探查结论**：`exec git clone` 可行性已确认(shell-out 可行，无需新 native)；lexer 注释保留已完成(见 §6.6 前置)。

**当前推进顺序(2026-07-19 更新；S6.1–S6.8 全部完成)**：**S6 收尾 = v0.3 发布** → S7。

## 6.6 轻量语法/语义扩展评估(工程视角，对应 S7)

前置：Burryn 是真 HM(occurs check + level generalize + let-poly)，unify 是 con/fn/var 三 kind 扁平 if-链，无 typeclass/constraint solver，运行时字段按整数下标访问，已有 CSP 并发。
以下按触及类型系统的深度排序。

| 项 | 成本 | 触及范围 | 备注 |
|---|---|---|---|
| 字符串插值(S7.1) | 低 | 纯 lexer+parser 脱糖为 `join`/`+` | 不碰类型系统；lexer 需在串内切回表达式模式，`{{` 转义；`{}` 内须为 str 值，非 str 编译错、提示显式 `str()`(owner 2026-07-11 定，无隐式转换的语言气质) |
| 管道 `\|>`(S7.2) | 极低 | 纯 parser 脱糖 | — |
| Match Guard(S7.3) | 低 | compiler 加条件跳转 | — |
| 命名参数+默认值(S7.4) | 中 | checker(按名重排实参+arity) + compiler(默认值字节码) | **已否决(2026-07-10)**：名字是否进 fn 类型参与 unification 无良解(进则函数值传递变脆，不进则语义两张皮)；若重提仅限「直接调用的纯语法糖」形态 |
| 编译期常量(S7.5) | 中 | `const` 声明 + 常量折叠阶段 | 形态已定(owner 2026-07-12)：`const N = <可折叠式>`，包级(可 `pub`)与块级均可；初始化式限编译期可折叠——字面量、const 引用、算术/比较/bool 运算、str 拼接；折叠中溢出 = 编译错(语义与运行期 trap 一致、只是提前)；类型照常推导不另标注；const 名不可 mut、不可重赋值 |
| 封闭 Records(→ **S8.4**) | 中高 | 改 `ty_unify` 核心(+tk==3 逐字段配对) + 新 TRecord kind + cgen 字段名→下标 | 因改 unify + 自举风险，归 S8 与 row poly 同段，不属 S7；原评估明显偏低 |

**S7 落地顺序**：S7.8 可选签名标注随 S6.1 接口缓存前置完成；S7 主批按字符串插值(最先，不阻塞任何事)→ 管道 / match guard(顺手)→ 编译期常量 → defer(S7.6) → net(S7.7)。
封闭 records 移出 S7，归 **S8.4**(改 unify + 自举风险与 row poly 叠加，同段一次性验 fixpoint)。

S7.6 `defer` 语义已定(owner 2026-07-12，移出 §7 待定)：`defer { ... }` 挂**包围函数**，函数退出时 LIFO 执行；块是闭包、捕获按闭包语义(无参数求值时机问题)；fiber 正常 return 执行 defer；trap/死锁 = 进程级 abort，defer 不执行。

S7.7 调度器升级已定(owner 2026-07-12，移出 §7 待定)：net natives 落地时把 idle-wait 抽成**通用 fd 注册接口**(socket/exec/timer 同一入口)，底层维持 poll；epoll/kqueue 只在 fd 规模成为瓶颈时换实现、接口不变；两份调度器(burrt.h + vm.bur)parity 铁律照旧。

**S7.1 落地状态(2026-07-19)**：lexer 在字符串与表达式模式间切换，`{{` 产生字面左花括号，嵌套 block/match/插值按 brace depth 配平；parser 脱糖为现有 `+` AST，非 `str` 表达式沿用类型检查并提示显式 `str()`；formatter 重建插值表层语法并保留含注释片段；旧 seed 与当前 compiler 通过 `chr(123)` 字面 brace 迁移保持三段自举定点。

## 6.7 重型类型系统扩展评估(工程视角，对应 S8)

| 项 | 原评估 | 复评 | 理由 |
|---|---|---|---|
| Row Polymorphism | 高 / S8 | **S8.3** 首位，紧接封闭 record(S8.4) | 复用现有 var/generalize 加「行 var」，扁平 if-链撑得住；是结构化接口的公共地基，唯一值得投入的重型项 |
| Effects | S8+ | 明确排除 | 与现有 CSP(fiber/channel/select)竞争控制流转移；CSP 已覆盖 IO/并发大半；类型侧 effect row 还依赖 row poly |
| Refinement Types | 中 / 长期 backlog | 明确排除 | 无 constraint solver 地基，须从零造子系统；与轻标注工程气质冲突(Rust 未上) |
| GADTs | 暂不做 | 明确排除 | 通用工程价值最低，动 HM 最微妙处 |
| Linear(全局) | 永不 | 同意 | — |
| 局部 Affine(资源) | 未列 | 补进 backlog | file/socket/channel 的 use-after-close 检查，流敏感 lint 级，不碰 GC，能把 close-of-closed-channel 运行时 trap 提前为编译期错 |

**与原路线两大分歧**：(1) Refinement 成本被低估——无求解器地基，实为从零造子系统，明确排除；(2) Effects 价值被高估——CSP 已覆盖其大半实用场景，边际价值与代价不成比例，明确排除。

**S8.1 大方向已定(owner 2026-07-12，作默认假设，S8 开工探查批可推翻)**：调用约定 = System V AMD64 ABI；GC 根扫描 = 沿用 C 后端的 shadow stack 精确 GC 语义(cgen 的根栈纪律照搬到手写代码生成)。该方向不阻塞 S6/S7 任何批次。

**S8 分工意向(owner 2026-07-12 口述)**：S8.3/S8.4 类型批仍由 LLM 实现；**S8.1/S8.5 手写后端本体由 owner 亲手写**(时间不定，数月至一年后、玩透语言且有空为前提)，届时 LLM 转为对齐语法与逻辑的辅导角色、不代写实现。LLM 推进到 S8 时以此为准，不要主动动手写后端。

## 7. 当前待定项(动工前必须先问 owner)

(暂无——2026-07-12 第二轮决策批清空本列表。新出现的待定设计决策须登记于此并先问 owner，规矩不变。)

已探查结论(移出待定)：lexer comment/trivia 保留已完成(S6.3 前置就绪，见 §6.6)；`exec` shell-out `git clone` 够用已确认(S6.2 无需新 native，见 §6.5)；模块系统具体形态已全部落地、无悬空(import 语法 S2.2、`pub` 可见性 S2.6、`bur.mod`/semver S6.1–S6.2)；`select` 语义已实现即规格(default 分支已在 parser 且至多一个，公平性 = 按声明顺序取首个就绪的确定序，见 tutorial.md §11)；S8 调用约定与 GC 根扫描已定大方向(见 §6.7)。

2026-07-10 设计审查定案(全文散见对应章节)：深 mut 降级为绑定级纪律 + checker 流规则；确定性承诺收窄 + `BUR_DETERMINISTIC` 模式；可选签名标注归 S7.8；S7.4 命名参数否决；S8 重排(类型先行、ELF 先于 PE)；std 捆绑分发；`bur.sum` 树哈希；fmt 验收三条铁律；json/net 从 S2.7 纠错移入 S6.6/S7.7；新增 S6.7 runtime IO 与 S6.8 checker 债批。

2026-07-11 决策批(五项，全文散见对应章节)：deep-mut 流规则收窄到堆类型后采纳为 error(§2)；S6.2 实现侧默认追认(clone URL 与 download 语义，§6.5)；S6.6 json API 四问关闭(值表示/函数名/目录布局/内嵌生成器，§6.5，std/testing 同批)；S7.8 标注语法定型(`name: type` + `-> type`，复用类型表达式，§6.5 接口缓存条目，S6.1 解锁)；S7.1 插值非 str 为编译错(§6.6 表)。

2026-07-12 决策批(五问，全文见 §2)：deep-mut 流规则实现侧——再赋值 RHS 一并检查；mut 形参实参走旁路表(只查静态可知被调方)；多态残余判 error；错误码 E0597；chan 整体豁免(不论元素类型，CSP 别名是语义本体)。另记：§2 摸底数字测于 S6.2/S6.4 落地前，开工须重新插桩摸底。

2026-07-17 决策批(四项，全文散见对应章节)：seed 定基提前至 S6.5 动工前——S6.5 的 cgen `#line` 与 runtime 改动会破坏归档 seed 的 gen1 == gen2 判定，v0.3 发布判据(S6 全部收尾)不变、与定基解绑(§6.5 S6.8 条目与 S6.5 前置)；接口缓存五条实现侧细则(2026-07-16 随状态同步写入)追认为定案(§6.5 接口缓存条目)；`bur mod tidy` 增加侧版本 = MVS 依赖图推导、查无报错指向 `bur get`(§6.5 CLI 布局条目)；定基机制 = tag 基准 commit、三段重生链(§6.5 S6.8 条目)。

2026-07-12 决策批(第二轮，路线清仓，全文散见对应章节)：v0.3 判据 = S6 全部收尾，seed 定基随 v0.3 发布(§6.5 S6.8 条目，**定基时点已被 2026-07-17 决策批修订**)；无 cc 兜底维持现状、议题关闭(VM 兜底 + 优雅降级，不引入 tcc)；bur test 子包发现归 S6.1 批、`-j` 并行归 backlog(§6.5 S6.4 条目)；诊断深度的多 span + 结构化建议并入 S6.5 成诊断/DX 批(§6.5 S6.5 条目)；S7.6 defer = 函数级 + 闭包块 + trap 不跑(§6.6)；S7.7 = 通用 fd 注册接口、poll 实现(§6.6)；S7.5 = `const` 声明(包级 + 块级，初始化限可折叠式，§6.6 表)；S8.1 大方向 = System V AMD64 ABI + shadow stack 精确 GC(§6.7，探查批可推翻)；§7 待定项清空(模块系统与 select 两条系已落地的过时项，移入已探查结论)。
