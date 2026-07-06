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
- 参数多态函数 + 泛型枚举；运算符用 SML 式受约束类型变量(`num` / `addord`)
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
- **`mut` 为深语义**：`let` 绑定的容器连内容都不可变；push/元素修改要求 `mut`。
  默认不可变必须名实相符
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

### 错误处理

- `Result` + `?`，无异常机制

### 模块与导出

- 模块系统为 S2.2 必做项(自举前提)；形态：目录即包，去中心化 import path(与工具链设计一致)，细节待定——**动工前先与 owner 对齐方案**
- **导出语法：`pub` 关键字**，不用首字母大写

### 语法

- 参考系：Rust(`let`/`mut`、`match`、带字段枚举、`?`、表达式导向、遮蔽)+ Go(`spawn`、channel 语法、自动分号插入)
- 补充定案：`defer`(资源清理，脚本场景刚需)
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
- 原生运行时：GC 起步用保守式栈扫描，省掉 stack map；单线程承诺使运行时无需线程同步

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
| **S2 C 后端与语言完备** | S2.1 C 后端(顺序 + 并发)；S2.2 模块系统；S2.3 map；S2.4 `select` + `close`；S2.5 深 `mut` + `fn(mut xs)`；S2.6 `pub`；S2.7 必要 stdlib(os/exec、fs、json、net) | 已完成 |
| **S3 自举前端** | 编译器前端由 Burryn 写成并编译自己 | 已完成 |
| **S4 重写 VM** | VM 由 Burryn 重写，经 cc 编成原生 | 已完成 |
| **S5 删 Go** | CLI driver 用 Burryn 写；main 清零 Go；`archive/go-host` 留档 | 已完成 |
| **S6 生态工具链** | S6.1 依赖解析(MVS + `bur.sum` + 放开 module.bur:538)；S6.2 网络拉取(`git clone` + `sha256sum`)；S6.3 `bur fmt`；S6.4 `bur test`；S6.5 debugger | 进行中 |
| **S7 语言特性扩展** | S7.1 字符串插值；S7.2 管道 `\|>`；S7.3 match guard；S7.4 命名参数 + 默认值；S7.5 编译期常量 | 未开工 |
| **S8 后端与重型类型** | S8.1 手写 x86-64 + PE 后端；S8.2 语法冻结 + grammar 文件；S8.3 row polymorphism；S8.4 封闭 records | 未开工 |

- S1–S5 为自举闭环，已全部达成：`bur` 由本语言写成、经 cc 逐字节重建自身，main 上 Go 整棵树已清零
- 自举判定标准：**编译器由本语言写成且能编译自己**；输出 C 再经 gcc/clang 落地，完全算自举
- stdlib 按「够自举用 + owner 真实脚本需求」逐个生长(os/exec、fs、json、net 优先)，不追大而全
- 编译速度是硬指标：任何特性提案先回答「是否显著拖慢编译」
- 触及 `ty_unify` / token 编号 / 自举链的改动(尤其 S8.3/S8.4)，改完必验 fixpoint(gen1 == gen2 逐字节)

## 6. 工程规范

- Conventional Commits 1.0.0:`<type>(<scope>): <description>`，subject ≤72 字符，祈使句，无句号无 emoji
- 分支：`main` 受保护仅 PR 合入(merge commit，不 squash)；开发期集成分支 `dev/<topic>`
- 测试：自举 parity + 示例 golden test 覆盖全链路；自举判定为一等验收(`bur build burc` 逐字节重建自身)；重构类改动必须先有测试安全网再动手
- 诊断质量是卖点本体：错误信息按 rustc 标准要求自己(精确 span、指出修法)

## 6.5 S6 生态工具链(自举完成后，owner 2026-07 定)

自举闭环已成(S1–S5)，下一批目标是「让别人能日常用」的生态工具链，全部用 Burryn 自写，延续零 Go 依赖。
关键路径：**依赖管理 → `bur fmt` → `bur test`**，debugger 作 C 后端增强并行可选。

**S6.1 + S6.2 依赖管理(P0，MVS + fetch + lockfile)**

现状：`bur.mod` 已解析 `module` + `require <path> <version>`(module.bur:117，require 校验但不解析)；`valid_import_path` 已在；跨模块 import 现被 E0432 拒(module.bur:538)。
骨架在，缺解析 + 拉取 + 定位。

- S6.1 解析层(无网络)：新 `burc/lib/modgraph.bur` 构建依赖图跑 MVS(选满足约束的**最低**版本)；新增 `bur.sum` lockfile(`path version hash`)；放开 module.bur:538 跨模块限制(命中 require 图则放行)；缓存目录 `$BURCACHE` 默认 `~/.burryn/pkg/<path>@<version>/`
- S6.2 网络拉取：倾向 shell-out `exec("git",["clone",...])` + `sha256sum` 校验(零新 native，延续 S5「simplify, no new natives」)；备选补最小 `http_get`/`sha256` native
- 命令面(随 S6.1/S6.2 落地)：`bur mod init` / `bur mod tidy` / `bur mod download` / `bur get <path>@<version>`
- 遵守 §4 红线：MVS 确定性、去中心化、禁 build 期执行任意代码

**S6.3 `bur fmt`(P0，文化基础设施)**

与包管理并列优先——越早冻结格式，后期生态一致性成本越低(§4「唯一官方格式，零配置」)。

- 复用 `lib.parse` 的 AST → 新 `burc/lib/format.bur` 写 pretty-printer；规则对齐 burc/lib 现有风格(换行即分号、块结构、表达式导向、4-space、`match` arm 对齐)
- 验收铁律：**幂等** `fmt(fmt(x))==fmt(x)` 且 `fmt` 前后 AST 不变
- 前置已就绪：lexer 现把注释作旁路 trivia 收集(`LexOut.Lexed` 第三字段，见 §6.6 前置)，`bur fmt` 可按 span 重插注释
- 命令：`bur fmt <file|dir>`(原地写回)、`--check`(CI，有 diff 则非 0 退出)、`-`(stdin→stdout，供 LSP format-on-save)
- 立即跑在 `burc/lib/` 自身统一自举代码风格

**S6.4 `bur test`(P1)**

现状测试 = 自举 parity + golden example，无 first-class 框架。

- 约定 `*_test.bur` + `fn test_*()` 自动发现；断言 API 归 stdlib(`assert_eq`/`assert`/`assert_ok`/`assert_err`，贴合 `Result`/`Option`)
- 并发特色：利用 fiber/channel 语义，把 VM 死锁检测(现为 exit 4)转成测试失败；支持并发测试模式
- 报告：pass/fail 计数 + 失败 span 定位(复用 diag 渲染)

**S6.5 debugger(可选，C 后端增强，优先级最低)**

- cgen 生成 C 时插 `#line <n> "<file>"`(cgen 已知每 Node span)→ 原生二进制直接 `gdb`/`lldb` 映射回 `.bur`
- runtime trap 打印带 source span 的 stack trace(复用 diag + line_starts)
- 符合「终局只留 C 底座」，属后端增强非新工具

**探查结论**：`exec git clone` 可行性已确认(shell-out 可行，无需新 native)；lexer 注释保留已完成(见 §6.6 前置)。

**推进顺序**：`bur fmt`(S6.3)先落地(小、无网络、立刻自举验证、解锁 LSP)+ 依赖解析(S6.1)并行 → 网络拉取(S6.2)→ `bur test`(S6.4)→ debugger(S6.5)。

## 6.6 轻量语法/语义扩展评估(工程视角，对应 S7)

前置：Burryn 是真 HM(occurs check + level generalize + let-poly)，unify 是 con/fn/var 三 kind 扁平 if-链，无 typeclass/constraint solver，运行时字段按整数下标访问，已有 CSP 并发。
以下按触及类型系统的深度排序。

| 项 | 成本 | 触及范围 | 备注 |
|---|---|---|---|
| 字符串插值(S7.1) | 低 | 纯 lexer+parser 脱糖为 `join`/`+` | 不碰类型系统；lexer 需在串内切回表达式模式，`{{` 转义 |
| 管道 `\|>`(S7.2) | 极低 | 纯 parser 脱糖 | — |
| Match Guard(S7.3) | 低 | compiler 加条件跳转 | — |
| 命名参数+默认值(S7.4) | 中 | checker(按名重排实参+arity) + compiler(默认值字节码) | 原评估偏低；不碰 unify 但碰调用点 infer |
| 编译期常量(S7.5) | 中 | 新常量折叠阶段 | — |
| 封闭 Records(→ **S8.4**) | 中高 | 改 `ty_unify` 核心(+tk==3 逐字段配对) + 新 TRecord kind + cgen 字段名→下标 | 因改 unify + 自举风险，归 S8 与 row poly 同段，不属 S7；原评估明显偏低 |

**S7 落地顺序**：字符串插值(最先，不阻塞任何事)→ 管道 / match guard(顺手)→ 命名参数 → 编译期常量。
封闭 records 移出 S7，归 **S8.4**(改 unify + 自举风险与 row poly 叠加，同段一次性验 fixpoint)。

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

## 7. 当前待定项(动工前必须先问 owner)

- 模块系统具体形态(import 语法、包内可见性细节、版本声明文件格式)
- `select` 语义细节(default 分支？公平性？)
- 工具链命令命名与 CLI 布局
- 手写后端的调用约定与 GC 根扫描策略(S8 前定)
- S6 各命令 CLI 布局与命名(`bur mod`/`bur get`/`bur fmt`/`bur test` 子命令形态)

已探查结论(移出待定)：lexer comment/trivia 保留已完成(S6.3 前置就绪，见 §6.6)；`exec` shell-out `git clone` 够用已确认(S6.2 无需新 native，见 §6.5)。
