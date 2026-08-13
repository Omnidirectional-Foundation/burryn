# SPEC — Burryn 项目规范

> SPEC v0.5 | Last updated: 2026-08-07
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

语法糖允许存在，但任何语义不得隐藏在语法糖背后——读者看到代码即可推断全部行为，无需查阅"这个上下文里它其实还做了 X"。隐式转换、隐式 coercion、magic method、隐式控制流转移(异常)均在拒绝清单。`?` 是显式早退(看到 `?` 就知道这里可能返回)；`defer` 是显式清理(看到 `defer` 就知道退出时跑)；插值是显式拼接的糖(`{expr}` 内必须是 str，非 str 编译错而非隐式 `str()`)；record 字面量带 `record` 关键字(不与 block 混淆)。类型推导是唯一的"隐式"——但可选签名标注使显式标注随时可用。

### 类型系统

- 静态，Hindley-Milner 全程序推导；函数参数/返回值零标注，仅枚举字段声明类型
- **可选函数签名标注**：「零标注」从「不能标」收窄为「不必标」——不标注的程序语义与推导结果不变；显式标注为 opt-in，用于诊断锚定与包边界 API 冻结，推导须与标注 unify，冲突报错
- 参数多态函数 + 泛型枚举；运算符用 SML 式受约束类型变量(`num` / `addord`)
- **受约束标注类型参数**：小写类型参数可在函数签名标注中写为 `a:addord`；同一签名后续出现的 `a` 复用该变量。允许 `num`、`addord`、`key`、`int`、`float`、`str` 约束；重复标注按既有约束交集规则合并，空交集报静态错。该语法只表达既有 HM 约束，不增加运行时语义。
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
  **流规则**：`mut` 形参的实参与 `let mut` 的初始化来源须本身可变或为新鲜值(字面量/构造/调用返回值)，违者 error。
  **只对堆类型(list/map 及含其的类型)生效**：int/float/bool/str/unit 为拷贝语义、无别名危害，豁免；**chan 整体豁免、不论元素类型**(send/recv/close 在现行纪律里本就不要求 mut，chan 别名是 CSP 语义本体)；if/match 作来源时递归看各臂尾表达式，皆新鲜则整体新鲜；来源类型未解时延迟判 error(宁滥勿缺)。
- **参数默认不可变；`fn f(mut xs)` 声明可变参数**——调用点无标记，与「无借用检查器 + GC」的定位一致，属 Go 式取舍。

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
  提供 opt-in 确定性模式(环境变量 `BUR_DETERMINISTIC=1`，IO 全串行化、timer 唤醒按 deadline + fiber 创建序双键排序)，`bur test` 默认启用

### 错误处理

- `Result` + `?`，无异常机制

### 模块与导出

- 模块系统：目录即包，去中心化 import path(与工具链设计一致)
- **导出语法：`pub` 关键字**，不用首字母大写

### 语法

- 参考系：Rust(`let`/`mut`、`match`、带字段枚举、`?`、表达式导向、遮蔽)+ Go(`spawn`、channel 语法、自动分号插入)
- **`defer`**(资源清理，脚本场景刚需)：块作用域(表达式导向下比 Go 的函数作用域干净)
- **表层语法见 [`grammar.md`](grammar.md)**(与 `compiler/frontend/` 实现逐条对应)；语法变更走 grammar.md §10 修订流程。

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

### 后端矩阵

| 后端 | 值表示 | GC / CSP | 工具链依赖 | 平台覆盖 | 发布形态 |
|---|---|---|---|---|---|
| 字节码栈式 VM | 16B tagged Value(开发/测试基线、自举 oracle) | 已完整(burrt.h:mark-sweep + ucontext) | 自举期 cc(构建 `bur` 本身) | 天然跨平台(POSIX,两处 #ifdef) | 并入核心 `bur` |
| **C 后端** | 16B tagged Value | 已完整(同 VM,链接 burrt.c) | `cc`/`gcc`/`clang`(任一) | 天然跨平台 | 并入核心 `bur`,`bur build` 运行期探测 cc |
| **手写 x86-64** | 8B raw int64,无 tag | 独立实现(shadow stack 编译期类型跟踪;并发 opcode 待设计) | **无**(硬约束) | Linux ELF(已做);macOS Mach-O / Windows PE 待序列化层 | 并入核心 `bur` |
| **LLVM 后端** | 复用 C runtime 16B tagged | 链接 burrt.c,免费获得 | `clang`(必须是 clang) | 三平台(随 clang target) | 并入核心 `bur`,运行期探测 clang |
| **Cranelift 后端** | 复用 C runtime 16B tagged | 链接 burrt.c | 构建期 `cargo`;运行期 `cranelift-driver` + 系统链接器(`cc`/`ld`)落地 `.o` | 三平台(随 driver 编译目标) | codegen 在核心 `bur`;**独立 `cranelift-driver` 二进制** |
| **WASM 后端** | 16B tagged(仅 GC 子集) | GC 链接 wasm32 版 burrt.c;**CSP 不能链接 ucontext**,须独立状态机或等 WASM Stack Switching 提案 | `clang --target=wasm32` + `wasm-ld` | 平台无关(wasm 本身) | 并入核心 `bur`,运行期探测 clang |

**单二进制交付(§1)精化**:`bur` 一个二进制打包 VM + C + x86 + WASM(codegen) + LLVM(codegen) 五种后端的 codegen 逻辑。真正"零外部工具链"的只有 x86;C/WASM/LLVM 三个后端在运行期需要机器上有 clang 才能产出可执行文件,Cranelift 还需独立 `cranelift-driver` 二进制 + 系统链接器。简言之:**发行的编译器本身自包含,非所有后端都自包含**。

### 后端次序

主线先 x86(Linux ELF → macOS Mach-O → Windows PE),其后并行铺多后端公共基础(runtime 平台抽象、工具链探测),再开 LLVM → Cranelift → WASM。**次序原因**:
- x86 是唯一真正零工具链依赖的后端,先做完校验"无 cc 也能产出可执行"这个路线核心假设。
- 后续三原生后端(LLVM/Cranelift/WASM)在 CSP 上都间接依赖平台抽象层,平台抽象比 x86 序列化层更优先。
- WASM 的 CSP 不能靠链接现有代码得到,工作量被低估,单独排期(见下)。

### 工具链探测:按工具聚类,判定独立

四个非 x86 后端共享底层工具(主要 clang),但**判定逻辑不合并**——机器只装 `gcc` 没装 `clang` 时若合并探测,LLVM/WASM 会被误判可跑,运行时才报错比探测期 skip 更差:

- 探测原语(`find_binary`、`clang_supports_target` 等)共享实现,放 `shared/toolchain-probe.bur`,避免四份重复样板
- 每后端在自己模块内声明依赖组合(C 要 `cc/gcc/clang` 任一;LLVM 要 clang;WASM 要 clang + `wasm32` target;Cranelift 要 `cranelift-driver` + `cc`/`ld`),不合并成统一布尔
- CI 保持每后端独立 job(出现"有 clang 无 wasm32 target"时,LLVM 跑、WASM skip,合并 job 无法表达)。

### Runtime 平台抽象

调度器平台差异从 `runtime/burrt.c` 拆出独立模块(`#ifdef` 分支):
- POSIX(Linux/macOS)继续走 ucontext
- Windows 改走 Fibers API(`CreateFiber`/`SwitchToFiber`/`DeleteFiber`)
- WASM **不复用此层**——CSP 不能链接 ucontext,见下

平台抽象先做,优先于 x86 三平台序列化层。理由:它直接决定 LLVM/Cranelift 后端能不能在 Windows 上跑,而 `pe.bur`(x86 的 Windows 文件格式层)与调度器互不阻塞,可并行推进。

### WASM CSP 修正

WASM 后端"链接 wasm32 版 burrt.c 获得 CSP"在架构上不成立——burrt.c 的 CSP 调用 `ucontext.h`(POSIX 系统调用级),wasm32 freestanding target 下没有这套东西,编译期直接失败,不是链接期报错。

修正:WASM 第一版只能链接 GC 子集(`bur_alloc`/`bur_gc_collect`/`bur_mark_value`/`bur_gc_trace`,这些不碰 ucontext);CSP(spawn/send/recv/select)要么编译期状态机转换重写,要么等 WASM Stack Switching 提案成熟。**不存在"链接现有代码就免费拿到 CSP"这条路**,这点上 WASM 与 x86 的 CSP 处境类似(都要独立造轮子),各后端的 CSP 路径需独立设计。

### 自举判定与原生运行时

- 自举判定标准:**编译器由本语言写成且能编译自己**;输出 C 再经 gcc/clang 落地,完全算自举
- 「任何架构都能跑」由 C 后端承担;手写后端只承诺 x86-64,其余架构不做手写
- 双后端互为测试参照:同一程序在 VM / C 后端 / 手写后端输出必须一致,纳入测试
- 原生运行时:GC 为 **shadow stack 精确扫描**;单线程承诺使运行时无需线程同步

### Release 产物

**命名**:`burryn-<version>-<os>-<arch>.<ext>` 综合包;`burryn-<backend>-<version>-<os>-<arch>.<ext>` 单后端产物。
- 架构命名统一 `amd64`/`arm64`(对齐 GHA runner、Docker tag),不用 Rust 的 `x86_64`/`aarch64`
- 归档格式按平台走:Windows `.zip`、Linux/macOS `.tar.gz`
- 版本号进文件名(脱离 release 页上下文仍可辨识)
- `cranelift-driver` 随 Cranelift 后端产物打包,不独立发

**预编译矩阵(6 cell + 1 跨系统 wasm)**:

| 系统 \ 架构 | amd64 | arm64 |
|---|---|---|
| Linux | 5 后端(x86/C/LLVM/Cranelift/WASM) | 4 后端(无 x86 自写) |
| macOS | 5 后端 | 4 后端(无 x86 自写) |
| Windows | 5 后端 | 4 后端(无 x86 自写,Windows arm64 条件成熟再上) |
| WASM | 跨系统共享,独立 zip | — |

→ **7 个综合包** + 各后端单产物(22 个,扣除不可能的 cell)。

**Release 页组织**:正文手写 Markdown 表,行 = 后端(含 Full Bundle 一行),列 = 6 平台架构,每格直链具体 asset。原生 Assets 面板留作自动化抓取入口 + `SHA256SUMS.txt` 落脚点。

**`SHA256SUMS.txt`**:每次发布生成,汇总所有产物哈希,脚本化校验单一入口。

**冷门架构与移动平台**:riscv64/ppc64le/i386/armv7 等 32 位架构、iOS/Android 等移动平台**不进预编译矩阵**——32 位/冷门架构走 `docs/build-exotic.md` 自行产出,移动平台走 `docs/mobile-future.md`(未规划)。

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
| **S6 生态工具链** | S6.1 依赖解析；S6.2 网络拉取；S6.3 `bur fmt`；S6.4 `bur test`；S6.5 诊断/DX；S6.6 std/json + std/testing；S6.7 runtime IO；S6.8 checker 债批 | 已完成 |
| **S7 语言特性扩展** | S7.1 字符串插值；S7.2 管道 `|>`；S7.3 match guard；S7.4 命名参数(**已否决**，编号保留)；S7.5 编译期常量；S7.6 `defer`；S7.7 net stdlib；S7.8 可选函数签名标注 | 已完成 |
| **S8 后端与重型类型** | S8.1 手写 x86-64 后端(**ELF**/Linux + **Mach-O**/macOS + **PE**/Windows)；S8.2 语法冻结 + [`grammar.md`](grammar.md) **已完成**；S8.3 row polymorphism；S8.4 封闭 records；S8.5 PE 后端(前提 = runtime Windows 移植)；S8.7 类型别名。**全部子项由 LLM 实现** | 进行中 |
| **S9 LSP 与编辑器生态** | S9.1 LSP 核心服务器(JSON-RPC + 文档同步 + 诊断推送)；S9.2 语言特性(hover / go-to-def / completion / formatting / signature-help)；S9.3 VSCode 薄扩展(TextMate grammar + LSP client)；S9.4 nvim/vim/emacs 配置片段。前置 = S8.2 语法冻结 | 部分实现 |
| **S10 包生态** | 已有 std 包：`json`/`net`/`testing`/`cli`/`encoding`/`path`。待扩展：`log`/`datetime`/`regex`/`crypto`/`http`。S10.2 包模板 + 示例包；S10.3 `bur doc`(导出签名 + 注释 → Markdown)；S10.4 包质量基础设施(CI 模板、测试约定、版本规范)。原则：能纯 Burryn 就不加 native；每包带 bur.mod + `*_test.bur`，随 std_embed 分发 | 未开工 |

- S1–S5 为自举闭环：`bur` 由本语言写成、经 cc 逐字节重建自身，main 上 Go 整棵树已清零，Go 种子归档 `archive/go-host`
- stdlib 按「够自举用 + owner 真实脚本需求」逐个生长，不追大而全
- 编译速度是硬指标：任何特性提案先回答「是否显著拖慢编译」
- 触及 `ty_unify` / token 编号 / 自举链的改动(尤其 S8.3/S8.4)，改完必验 fixpoint(gen1 == gen2 逐字节)
- **S8 内部推进顺序**：S8.3 row poly → S8.4 封闭 records → S8.1 ELF → S8.5 PE。类型先行(日常效用高于第二后端)。S8 之后进 S9

## 6. 工程规范

- Conventional Commits 1.0.0:`<type>(<scope>): <description>`，subject ≤72 字符，祈使句，无句号无 emoji
- 分支：`main` 受保护仅 PR 合入(merge commit，不 squash)；开发期集成分支 `dev/<topic>`
- 测试：自举 parity + 示例 golden test 覆盖全链路；自举判定为一等验收(`bur build burc` 逐字节重建自身)；重构类改动必须先有测试安全网再动手
- 诊断质量是卖点本体：错误信息按 rustc 标准要求自己(精确 span、指出修法)

## 6.1 S8 后端与类型系统扩展

**类型系统扩展决策**(S8 工程评估):

- **纳入**:row polymorphism(S8.3,首位) → 封闭 record(S8.4)。复用现有 var/generalize 加「行 var」,扁平 if-链撑得住;是结构化接口的公共地基,唯一值得投入的重型项
- **S8.3 实现定案**:行变量复用类型变量句柄（`tn="row"` 标记），开放 record 编码为 `@rec|field|?rowvar`——`?` 前缀 = 行变量槽，置于字段名列表末尾，对应句柄置于 `ta` 末尾。`ty_unify` record 开放分支：开放侧已知字段按名与另一侧配对（须为子集），行变量绑定另一侧剩余字段的（开放/封闭）record；行变量独立出现时绑定整个 record 或另一行变量。generalize/occurs/adjust 经 `ta` 递归自动覆盖行变量。标注解析经 `collect_annotation_vars` 预建行变量句柄
- **排除**:Effects——与现有 CSP(fiber/channel/select)竞争控制流转移,CSP 已覆盖 IO/并发大半实用场景,边际价值与代价不成比例
- **排除**:Refinement Types——无 constraint solver 地基,须从零造子系统;与轻标注工程气质冲突(Rust 未上)
- **排除**:GADTs——动 HM 最微妙处,通用工程价值最低
- **排除**:Linear(全局)——永不
- **backlog**:局部 Affine(资源)——file/socket/channel 的 use-after-close 检查,流敏感 lint 级,不碰 GC,能把 close-of-closed-channel 运行时 trap 提前为编译期错

### x86-64 后端架构（S8.1，开发中）

单文件 `compiler/backends/x86.bur` + ELF64 发射 `compiler/backends/elf.bur`。无 cc 依赖,手写 ELF。调用约定 = System V AMD64 ABI;GC 根扫描沿用 C 后端 shadow stack 精确扫描语义(cgen 的根栈纪律照搬到手写代码生成)。

#### 寄存器约定

| 寄存器 | 角色 |
|--------|------|
| r15 | 值栈顶（向上增长） |
| r14 | 帧基（当前函数） |
| r13 | 跳转表基址 |
| r12 | 堆 bump 指针 |
| rbx | 全局变量表基址 |
| rbp | 保存调用者 r14 |

**不得修改寄存器约定**——这是 ABI 级别的约束，改了全盘崩。

#### 值表示

Raw int64，8 字节/槽，**无 tag**。字符串是指针 → `[8B len][content bytes]`（data section 或堆）。**float 装箱(boxed)**：值栈/字段/列表元素/闭包 upval 里存 8B **指针** → 指向堆上 `[8B bits]` 盒（IEEE-754 double 位模式）；算术/比较/取负走 SSE（`addsd`/`subsd`/`mulsd`/`divsd`/`cmpsd`），`to_float`/`trunc`/`float_bits`/`parse_float` 在盒与 int/str 间转换。

#### 内存布局

```text
ELF header (64B) | phdr (56B) | str_data | funcs | jump_table | _start
                                 ^base_addr+120
```

- 堆：16MB via mmap(MAP_ANONYMOUS)，零填充，bump-allocated via r12
- 值栈：1MB below rsp，向上增长
- 全局变量：堆首 N*8 字节（rbx = base）

#### Shadow stack（编译期类型跟踪）

每个值 push 记录一个 shadow 条目（编译期 `[]` 字符串数组，不进入输出二进制）：
- `""` = int/unknown
- `"str"` = string pointer
- `"float"` = 装箱 float（盒指针）
- `"list"` = list pointer
- 函数名 = 用于 call dispatch
（另有扩展 `type_shadow` 多态编码如 `list-<elem>` / `tup:` / `record:` / `map` / `enum:`，驱动格式化分派与字段类型推断）

#### 堆对象布局

**String**：`[8B len][content bytes]`（data section 或堆）

**List**（header 不移动）：`[8B len][8B cap][8B elements_ptr]`（24 字节）。elements 在 elements_ptr 处，连续 8 字节槽。初始 inline（elements_ptr = header+24）。push 增长：分配新 elements 数组（cap*2, min 4），复制旧数据，更新 header 的 elements_ptr 和 cap。

**Closure**：`[8B fn_index][8B upval0][8B upval1]...`（8*(1+N) 字节）。fn_index 通过跳转表解析：`shl rax, 3; add rax, r13; mov rax, [rax]; call rax`。Slot 0 of callee frame holds closure ptr; op_get_upval reads `[closure + 8 + 8*idx]`。默认值捕获（copy-at-capture，非共享 cell）；`capture(ref n)` 引用捕获的槽存** cell 指针**（堆 8B，恒在堆上，无 open/close 概念），读写须二次解引用。

**Enum instance**（堆，由 constructor 调用创建）：
```text
[8B: eh][8B: vi][8B: field0][8B: field1]...[8B: fieldN-1]
```
- eh = enum type index；vi = variant index；fields 紧随

**Enum type object**（data section，CEnumType 常量）：`[8B: eh]`

**Singleton**（data section，CSingleton 常量，即 0 字段 enum instance）：`[8B: eh][8B: vi]`

**Constructor**（data section，CCtor 常量，callable，非 instance）：
```text
[8B: 0xFFFFFFFFFFFFFFFF][8B: eh][8B: vi]
```
- Sentinel `-1` at offset 0（永远不可能是有效 fn_index）。op_call 检测 `[callee] == -1` 时分流到 ctor path。

**Float box**（堆，8 字节）：`[8B bits]`——IEEE-754 double 位模式。值栈/字段/元素/upval 存盒指针，算术比较走 SSE。

**Tuple**（堆）：`[8B n][8B elem0]...[8B elemN-1]`（8+8n 字节），构造时从值栈 rep movsb 复制元素。

**Record**（堆）：`[8B n_fields][8B cidx][8B field0]...[8B fieldN-1]`,与 op_record 配套字段名编码查表。

**不加 type tag 的理由**：类型检查器保证 op_test_variant 和 op_get_field 只收到 enum instance。运行时类型区分只在 op_call（closure vs constructor）需要，sentinel 处理。

#### 调用约定

Caller pushes: `[callee/closure-ptr][arg0]...[argN]`, then calls。
Callee prologue: `push rbp; mov rbp, r14; lea r14, [r15 - 8*(arity+1)]`。
Callee epilogue: peek return value, collapse to r14, restore rbp, ret。
op_set_global 和 op_set_local PEEK（不 pop）——匹配 VM 语义。

### Phase 3: CSP opcode（设计定案 2026-08-11）

x86 后端无 libc、无 ucontext——fiber 调度器、阻塞、park/wake、上下文切换全部自写。值表示是 8B raw int64（无 tag），与 C runtime 的 16B tagged Value 不兼容，**不能链接 burrt.c 的 CSP**。以下为定案设计，全部子项照此实现：

- **op_spawn (44)**: 2 bytes `[op][argc:1B]`。需 fiber struct + 独立栈 + 上下文切换。
- **op_send (45)**: 2 bytes。需 channel struct + sendq/recvq + park/wake。
- **op_recv (46)**: 2 bytes。同 send。
- **op_chan_next (47)**: 3 bytes `[op][cidx:2B]`。channel 迭代。
- **op_select (48)**: variable。多路 select,最复杂 CSP opcode。
- **op_defer (49)**: 1 byte。需 per-frame defer stack(可与 CSP 并发独立实现,但仍需 defer 栈管理)。
- **close_upvalue**: 跳过——默认值捕获不产生 open upvalues；ref 捕获用恒开 cell（见下节）。

#### 闭包捕获语义：默认值捕获 + 显式 ref（设计定案 2026-08-12）

- **默认捕获 = 值拷贝**（copy-at-capture）：闭包槽存捕获时快照。x86 现状即此语义；VM/C runtime 的 `op_closure` 默认分支改为直接压入栈值（不再建 open upvalue cell），两后端语义统一
- **显式 `capture(ref n)` = 引用捕获**：闭包槽存 cell 指针，读写经 cell 与外部变量共享存储
  - 语法：闭包块前可选捕获子句，`capture` 为保留字，`ref` 仅 capture 列表内特判；未被 `ref` 声明的捕获项即默认值捕获（显式列出与省略同义）
- **x86 cell 实现**：被 ref 捕获的 mut 局部**提升为 cell 指针槽**——所在函数预扫描本函数全体闭包的 ref 声明，被声明的变量其栈槽存 `&cell`（声明时堆分配 8B），该变量的 get/set_local 全部经 cell 解引用；闭包 upval 槽存同一 cell 指针。GC：cell 为普通堆对象，mark 跟随内容
  - **与 VM open-upvalue 的可观察等价性**：open 期读外部栈槽 = 读 cell 当前值（外部帧存活期 cell 只由提升变量写入，两者同值）；close 发生于外部作用域退出后，彼时外部变量已死、cell 无后续写入，闭包读 cell = 固话值。故 x86 无需 open/close 机制
- **约束**：`ref` 只能作用于局部（含参数），顶层全局拒绝（全局天然共享）；`ref` 捕获不可变变量允许（等价值拷贝，无害）；`ref` 捕获参数允许（参数即栈槽）
- **字节码**：`op_closure` 描述符 `[1B islocal]` 扩展为标志字节 bit0=islocal、bit1=ref（旧值 1/0 仍合法：旧描述符即默认值捕获）

#### 上下文切换：手写 8 槽 ctx（定案）

唯一可行方案（无 ucontext、无 setjmp 可依赖）。关键洞察：**切换不需要保存指令指针**——切换子程序由生成代码用 `call` 调用，返回地址已在机器栈上，随 rsp 一起保存/恢复。

- fiber ctx = 8 槽内存 `[rsp][rbx][rbp][r12][r13][r14][r15]`
- 切换 = 保存 8 槽到当前 fiber ctx → 读下一个 fiber ctx → 恢复 → `ret` 继续执行
- 正确性论证：生成代码无机器栈局部变量（全部在值栈，r14 相对寻址）；临时值要么在值栈（r15 以下）要么在 callee-saved（全保存）→ 任意指令点挂起都安全
- spawn 首次 resume：trampoline 地址压新栈，ctx.rsp 指向它，trampoline 弹参数调入口函数
- 切换子程序内**不允许分配**——GC 只在生成代码分配点触发，切换中间无 GC 窗口

#### 栈布局：每 fiber 一套机器栈 + 值栈（定案）

- 新 fiber mmap **1MiB**（对齐 C runtime `BUR_STACK_SIZE`），栈底加 **guard page**（不可读写，防机器栈/值栈越界静默损坏；越界 = SIGSEGV trap）
- rsp 与 r15 一起换；`rt_stack_base` 从全局槽改为 **per-fiber 字段**，GC mark 遍历 fiber 表扫每 fiber 的 `[stack_base, top)`
- 主 fiber 保持现状（OS 栈 + `rsp - 1MiB` 值栈）

#### 时间片：callee 入口插桩（定案）

- 插桩点 = **函数 prologue 后一处**（budget 减一 + 条件 call yield 子程序：schedule 自己到队尾 + 切换），比 C runtime 的调用点插桩省开销
- **已知限制（写入限制清单）**：紧循环不可抢占——budget 只在函数边界递减，纯计算紧循环内无调度点，会独占 CPU 直至函数返回或阻塞；语义上仍是确定性协作式，不违反单线程承诺

#### channel / send / recv / close（定案）

- 堆对象布局照 C runtime `OChannel`：bounded FIFO buf + sendq/recvq/waiters + closed
- send/recv/close 三路分支照 VM 语义（`vm.bur` op_send/op_recv）逐条翻译
- **操作数留在值栈直到操作完成**——单 send 不用 fiber.sendVal 字段，park 期间值栈整体保留，比 C runtime 更简单
- **元素类型传播（设计定案 2026-08-13，方案 ②）**：x86 后端不消费 checker 输出，bytecode 的 op_send/op_recv 无类型操作数，故 channel 元素类型由 x86 后端自维护的栈类型影子（`type_shadow`/`pos_types`）传播：
  - `chan(n)` 构造推类型 `"chan"`（元素未知）；`ch <- v` 处若 v 的类型影子已知（非空），把 ch 持有槽的类型升级为 `"chan:<v>"`（经 op_get_local 推入的 `"slot:N"` 影子定位槽位，写回 `pos_types[N]`）
  - `<-ch` / `for x in ch` / select recv 臂的结果类型从 ch 的类型影子取元素：`"chan:X"` → 结果类型 `X`；`"chan"`（未知）→ 空/`int` 兜底
  - **跨函数传播**：复用已实现的 `fn_param_types` 调用点聚合（sig_specs 机制）——spawn/fn 调用实参为 `"chan:X"` 时，被调函数参数槽类型经 prologue 填充拿到 `"chan:X"`，函数体内 recv 即正确
  - **边界**：send 值类型未知 → 保持 `"chan"` 兜底（recv 后 print 退化为 int 转换，与既有启发式一致）；同一 chan 被不同类型 send（类型错误程序）→ 以最后一次 send 的元素类型为准，行为不定
  - **未来评估**：接入 LLVM/Cranelift 后端、或语言层面需要真正多态（泛型单态化/约束）时，重新评估 bytecode 类型标注方案，届时与 checker 输出接入 bytecode 一并设计

#### select：重试循环 + 顺序选臂（定案）

- 照 VM 语义：顺序扫臂找第一个 ready → 命中执行 + 跳臂目标；default → 跳 default；全不 ready → 注册到全部 chan 的 waiters + park(FBLOCKED_SELECT) + 醒来回循环头重试；send 臂的 val 在值栈上，park 期间自然保留
- **公平性设计选择（刻意对齐 VM，非遗漏）**：多臂同时 ready 时确定性选**第一个**，不做随机化/round-robin——确定性优先于公平，与 VM 行为逐字节一致是验收前提

#### GC 覆盖与 park 一致性（定案）

- fiber 表 = 全局 root；chan 的 buf、sendq→fiber→sendVal、waiters 全链路可达
- **测试硬性要求（第一批单测必含）**：fiber A park 在 send 上时触发 GC，断言待发送值仍存活——防 top 指针漂移导致漏扫
- mark 时当前 fiber 扫值栈 `[stack_base, r15)`，其余 fiber 扫其 ctx 槽 + 值栈（保守扫，地址范围检查排除 raw int 误判）

#### 死锁检测（定案）

- 调度器主循环：ready 空 && 存在非 done fiber → fatal deadlock（exit 4，文本照 C runtime）
- x86 无 timer/io 阻塞（net_nb 等仍未实现 int3），检测条件比 C runtime 简单

#### defer（定案，独立于 CSP 先行实现）

- per-fiber defer 栈（数组存 closure 指针）+ 帧进入记 watermark（照 C runtime `bur_run_defers` 的 dbase 语义）
- op_defer 压入；函数 epilogue 前按 watermark LIFO 执行，返回值先 peek 保留在栈上再执行 defer（防 defer 内分配回收）
- **实现顺序（从独立到耦合）**：defer → fiber 调度器（切换 + spawn + 死锁检测）→ channel（send/recv/close）→ chan_next → select

## 6.2 LSP 与编辑器生态(工程视角，对应 S9)

**架构定案**：LSP 服务器用 Burryn 写(延续自举原则)，作为 `bur lsp` 子命令，stdin/stdout 走 JSON-RPC 2.0(LSP 3.17 规范)。所有语言智能在服务器端；编辑器插件是**薄客户端**——只转发 LSP 消息 + 渲染 UI，不含语言逻辑。新增编辑器支持 = 实现 LSP client 协议，零服务器改动。

**传输层**：Content-Length 帧分割 + JSON-RPC 消息解析/路由；消息体 JSON 序列化/反序列化走 `std/json`。

**文档同步**：Full sync 模式(didOpen/didChange/didClose 全量内容)，服务器维护内存文档表，checker 读内存覆盖磁盘。增量 sync(delta)为后续优化，v1 不做。

**诊断推送**：didOpen 与 didChange(防抖)后重跑 check 管线(lex → parse → check)，DiagT/DiagX 转 LSP Diagnostic 推送。check 管线须支持从内存源码运行(LSP 核心工程改造点)。

**语言特性(S9.2)**：
- hover：显示推导类型/签名(复用 checker 推导结果)
- go-to-definition：从名字使用处跳到绑定处(需 AST span → 定义 span 映射，跨文件走 module loader)
- completion：作用域感知名字补全(局部变量、函数名、包成员 `pkg.`)
- formatting：调 `bur fmt -`(stdin→stdout)
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
