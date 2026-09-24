# Architecture — Burryn 实现架构

> v0.7 · active · 2026-09-24
> 状态：实现权威 · 编号 `S<n>[.<m>]`（阶段表见 [`GOALS.md`](GOALS.md)）
> 相关文档：[`GOALS.md`](GOALS.md) 路线与完成线 · [`grammar.md`](grammar.md) 表层语法 · [`NUMBERING.md`](NUMBERING.md) 旧编号 · [`../README.md`](../README.md)

> **注意：** 本文件是实现层的最高约束，所有实现工作以此为准。
> 遇到本文件未覆盖的设计决策，停下来问 owner，不要自行拍板。
> **警告：** 文中「定案」条目不接受实现侧擅自更改；发现定案之间冲突时，同样停下上报。

## 1. 定位

**静态推导、零标注、CSP 并发的实用工具语言；rustc 级诊断，Go 级简洁与编译速度，单二进制交付；以完全自举为核心里程碑。**

目标场景：运维脚本、CSP 风格并发管道、带静态保障的小工具。
终局是 owner 日常真实使用的工具语言，不是 DSL，不是玩具。

三条贯穿全项目的基调：

- **显式优先。** 语法糖允许存在，但任何语义不得隐藏在语法糖背后——读者看到代码即可推断全部行为，无需查阅「这个上下文里它其实还做了 X」。隐式转换、隐式 coercion、magic method、隐式控制流转移（异常）均在排除之列（见 [`GOALS.md`](GOALS.md) §7）。`?` 是显式早退（看到 `?` 就知道这里可能返回）；`defer` 是显式清理（看到 `defer` 就知道退出时跑）；插值是显式拼接的糖（`{expr}` 内必须是 str，非 str 编译错而非隐式 `str()`）；record 字面量带 `record` 关键字（不与 block 混淆）。类型推导是唯一的「隐式」——但可选签名标注使显式标注随时可用。
- **诊断质量是卖点本体。** 错误信息按 rustc 标准要求自己：精确 span、指出修法。
- **编译速度是硬指标。** 任何特性提案先回答「是否显著拖慢编译」。

## 2. 语言设计定案

### 2.1 类型系统

- 静态，Hindley-Milner 全程序推导；函数参数/返回值零标注，仅枚举字段声明类型
- **可选函数签名标注**：「零标注」从「不能标」收窄为「不必标」——不标注的程序语义与推导结果不变；显式标注为 opt-in，用于诊断锚定与包边界 API 冻结，推导须与标注 unify，冲突报错
- 参数多态函数 + 泛型枚举；运算符用 SML 式受约束类型变量（`num` / `addord`）
- **受约束标注类型参数**：小写类型参数可在函数签名标注中写为 `a:addord`；同一签名后续出现的 `a` 复用该变量。允许 `num`、`addord`、`key`、`int`、`float`、`str` 约束；重复标注按既有约束交集规则合并，空交集报静态错。该语法只表达既有 HM 约束，不增加运行时语义。
- 禁止一切隐式转换
- **多态运行时表示：统一装箱（uniform boxing）**。
  不做单态化（monomorphization），该决策覆盖字节码 VM 与全部原生后端
- **`--dyn` 逃生门：砍掉**。
  语言只有一套语义（静态检查），不维护动态模式

**行多态与封闭 record（定案）**：

- **行变量**复用类型变量句柄（`tn="row"` 标记），开放 record 编码为 `@rec|field|?rowvar`——`?` 前缀 = 行变量槽，置于字段名列表末尾，对应句柄置于 `ta` 末尾。
  `ty_unify` record 开放分支：开放侧已知字段按名与另一侧配对（须为子集），行变量绑定另一侧剩余字段的（开放/封闭）record；行变量独立出现时绑定整个 record 或另一行变量。
  generalize/occurs/adjust 经 `ta` 递归自动覆盖行变量。
  标注解析经 `collect_annotation_vars` 预建行变量句柄。
- **封闭 record 是结构类型**。合一按字段名配对（与开放分支同一套），字段书写顺序不进入类型身份——`record { x: int, y: int }` 与 `record { y: int, x: int }` 是同一类型。
  封闭额外要求字段集合相等（无剩余、无缺失）；开放侧仍按行变量规则。
  相等与字段偏移按名，不按插入顺序下标。
  完成条件（类型与运行时缺一不可）：`take_xy(record { y: 2, x: 1 })` 在标注 `record { x: int, y: int }` 下通过类型检查且三后端求值均为 `3`；`record { x: 1, y: 2 } == record { y: 2, x: 1 }` 三后端均为 true。

### 2.2 值与内存

- **无 null / nil**。
  可空值一律用 `Option` 枚举 + 穷尽 `match` 表达
- **数值类型只有 `i64` 与 `f64`**，不做定宽整数全家桶
- **整数溢出一律 trap（运行时 panic）**，不区分 debug/release，不静默回绕
- 内存管理：GC（mark-sweep）。
  明确不做所有权/借用检查
- **`mut` 为深语义、绑定级纪律**：经由 `let` 绑定名不可修改其值（含容器内容）；push/元素修改要求 `mut`。
  不可变性挂在**绑定**上而非值上——无借用检查器与 move 语义，别名可绕过（`let mut b = a` 后改 `b` 可见于 `a`），故**不承诺值级不可变**。
  **流规则**：`mut` 形参的实参与 `let mut` 的初始化来源须本身可变或为新鲜值（字面量/构造/调用返回值），违者 error。
  **只对堆类型（list/map 及含其的类型）生效**：int/float/bool/str/unit 为拷贝语义、无别名危害，豁免；**chan 整体豁免、不论元素类型**（send/recv/close 在现行纪律里本就不要求 mut，chan 别名是 CSP 语义本体）；if/match 作来源时递归看各臂尾表达式，皆新鲜则整体新鲜；来源类型未解时延迟判 error（宁滥勿缺）。
- **参数默认不可变；`fn f(mut xs)` 声明可变参数**——调用点无标记，与「无借用检查器 + GC」的定位一致，属 Go 式取舍。

### 2.3 字符串

- **底层 UTF-8 字节序列**。
  `len` 与索引按字节；提供码点迭代器
- 字符串插值建立在字节语义之上

### 2.4 并发

- CSP：`spawn` + channel（`ch <- v` / `<-ch`），死锁检测
- **`select` 与 `close(ch)` 为核心必做项**，无 select 的 CSP 视为残缺
- **执行模型长期承诺单 OS 线程**（并发 ≠ 并行，Node/Lua 路线）。
  纤程调度 + 时间片抢占，不做真并行——无借用检查器时真并行 + 深 mut 会引入数据竞争，且单线程使 GC 与原生运行时简化一个量级。
  此承诺覆盖全部后端
- **确定性承诺收窄**：纯计算程序跨后端逐字节确定；含 IO 程序不承诺调度顺序（IO 完成时序来自外部世界，与真 IO 重叠原理上互斥）。
  提供 opt-in 确定性模式（环境变量 `BUR_DETERMINISTIC=1`，IO 全串行化、timer 唤醒按 deadline + fiber 创建序双键排序），`bur test` 默认启用

### 2.5 错误处理

- `Result` + `?`，无异常机制

### 2.6 模块与导出

- 模块系统：目录即包，去中心化 import path（与 §4 工具链设计一致）
- **导出语法：`pub` 关键字**，不用首字母大写

### 2.7 语法参考系

- 参考系：Rust（`let`/`mut`、`match`、带字段枚举、`?`、表达式导向、遮蔽）+ Go（`spawn`、channel 语法、自动分号插入）
- **`defer`**（资源清理，脚本场景刚需）：块作用域（表达式导向下比 Go 的函数作用域干净）
- **表层语法见 [`grammar.md`](grammar.md)**（与 `compiler/frontend/` 实现逐条对应）；语法变更走 grammar.md §10 修订流程。

### 2.8 方法与 `impl` 块（定案）

- **声明结构：方法收进 `impl` 块、挂在类型上**。类型是这门语言唯一的命名空间边界
  （无 trait/interface），方法的归属必须语法可见；游离顶层写法是历史遗留（`E0801`/
  `E1114` 两处诊断的根源），已废弃，语法层直接拒绝（`E1121`）。
- **最小变体**：接收者语法原样进块，不引入 `self` 关键字，调用侧 `p.dist()` 不变：
  ```bur
  impl Point {
      fn (p: Point) dist(other: Point) -> float { ... }
  }
  ```
- **可见性**：`impl` 块内的方法在包内/script 内可见，含跨包（一个方法能否被另一个
  包的代码调用，只取决于接收者类型本身能否被引用到，不看方法是否 `pub`——方法目前
  没有独立于类型的导出粒度）；`impl` 块内的方法本身不接受 `pub` 修饰。
- **接收者类型必须与 `impl` 声明一致**：方法自带的 `(recv: T)` 中 `T` 若与所在
  `impl` 块的类型名不一致，报 `E0610`；校验后一律按 `impl` 声明的类型注册（挂在
  类型上是权威，不一致已经报过错）。
- **类型检查的保守调用图**：方法调用 `obj.mname()` 的接收者类型要等 `obj` 推断完
  才知道，构建 SCC/mutual-recursion 依赖图（推断之前）时静态拿不到。故
  `.mname()` 一律记为依赖**全部**同名方法（不分接收者类型），作为调用图的保守
  上界——不影响调用解析本身（`infer_call` 仍按推断出的接收者类型精确查表），
  只是让恰好同名的不同类型方法被并入同一 SCC，牺牲一点泛化粒度换正确性；
  真出现跨层互递归导致的粒度问题再收紧。
- **`impl` 块内整体预声明**：与 `FnDecl` 同阶段，在任何函数体或方法体开始推断前
  完成——方法调用（无论调用方是函数还是另一方法）不再对声明顺序敏感。
- **跨包方法解析靠绑定句柄，不靠按名字重查**：类型检查按包处理，每包处理完就
  `pop_scope()`；`method_tbl` 存的是绑定句柄本身（`declare()` 的返回值），而非
  类型或按名字重新 `lookup()`——后者在声明方法的包已经出栈后必然查不到（跨包普通
  函数走独立的 `pkg_vals` 句柄表避开了这个问题，方法调用点复用同一个道理）。

## 3. 后端路线

### 3.1 后端矩阵

| 后端 | 值表示 | GC / CSP | 工具链依赖 | 平台覆盖 | 发布形态 |
|---|---|---|---|---|---|
| 字节码栈式 VM | 16B tagged Value（开发/测试基线、自举 oracle） | 已完整（burrt.h：mark-sweep + ucontext） | 自举期 cc（构建 `bur` 本身） | 天然跨平台（POSIX，两处 #ifdef） | 并入核心 `bur` |
| **C 后端** | 16B tagged Value | 已完整（同 VM，链接 burrt.c） | `cc`/`gcc`/`clang`（任一） | 天然跨平台 | 并入核心 `bur`，`bur build` 运行期探测 cc |
| **手写 x86-64** | 8B raw int64，无 tag | 独立实现（shadow stack；CSP 不链 burrt.c，见 §5） | **无**（硬约束） | ELF / PE / Mach-O 三平台序列化层（`--os linux\|windows\|darwin`） | 并入核心 `bur` |
| **LLVM 后端** | 复用 C runtime 16B tagged | 链接 burrt.c，免费获得 | `clang`（必须是 clang） | 三平台（随 clang target） | 并入核心 `bur`，运行期探测 clang |
| **Cranelift 后端** | 复用 C runtime 16B tagged | 链接 burrt.c | 构建期 `cargo`；运行期 `cranelift-driver` + 系统链接器（`cc`/`ld`）落地 `.o` | 三平台（随 driver 编译目标） | codegen 在核心 `bur`；**独立 `cranelift-driver` 二进制** |
| **WASM 后端** | 16B tagged（仅 GC 子集） | GC 链接 wasm32 版 burrt.c；**CSP 不能链接 ucontext**，须独立状态机或等 WASM Stack Switching 提案 | `clang --target=wasm32` + `wasm-ld` | 平台无关（wasm 本身） | 并入核心 `bur`，运行期探测 clang |

**单二进制交付（§1）精化**：`bur` 一个二进制打包 VM + C + x86 + WASM（codegen）+ LLVM（codegen）五种后端的 codegen 逻辑。
真正「零外部工具链」的只有 x86；C/WASM/LLVM 三个后端在运行期需要机器上有 clang 才能产出可执行文件，Cranelift 还需独立 `cranelift-driver` 二进制 + 系统链接器。
简言之：**发行的编译器本身自包含，非所有后端都自包含**。

后端开工次序见 [`GOALS.md`](GOALS.md) §6。下列条款是次序背后的约束，不是进度。

### 3.2 工具链探测：按工具聚类，判定独立

四个非 x86 后端共享底层工具（主要 clang），但**判定逻辑不合并**——机器只装 `gcc` 没装 `clang` 时若合并探测，LLVM/WASM 会被误判可跑，运行时才报错比探测期 skip 更差：

- 探测原语（`find_binary`、`clang_supports_target` 等）共享实现，放 `shared/toolchain-probe.bur`，避免四份重复样板
- 每后端在自己模块内声明依赖组合（C 要 `cc/gcc/clang` 任一；LLVM 要 clang；WASM 要 clang + `wasm32` target；Cranelift 要 `cranelift-driver` + `cc`/`ld`），不合并成统一布尔
- CI 保持每后端独立 job（出现「有 clang 无 wasm32 target」时，LLVM 跑、WASM skip，合并 job 无法表达）

### 3.3 Runtime 平台抽象

调度器平台差异从 `runtime/burrt.c` 拆出独立模块（`#ifdef` 分支）：

- POSIX（Linux/macOS）继续走 ucontext
- Windows 改走 Fibers API（`CreateFiber`/`SwitchToFiber`/`DeleteFiber`）
- WASM **不复用此层**——CSP 不能链接 ucontext，见 §3.4

平台抽象先做，优先于 x86 三平台序列化层。
理由：它直接决定 LLVM/Cranelift 后端能不能在 Windows 上跑，而 `pe.bur`（x86 的 Windows 文件格式层）与调度器互不阻塞，可并行推进。

### 3.4 WASM CSP 修正

WASM 后端「链接 wasm32 版 burrt.c 获得 CSP」在架构上不成立——burrt.c 的 CSP 调用 `ucontext.h`（POSIX 系统调用级），wasm32 freestanding target 下没有这套东西，编译期直接失败，不是链接期报错。

修正：WASM 第一版只能链接 GC 子集（`bur_alloc`/`bur_gc_collect`/`bur_mark_value`/`bur_gc_trace`，这些不碰 ucontext）；CSP（spawn/send/recv/select）要么编译期状态机转换重写，要么等 WASM Stack Switching 提案成熟。
**不存在「链接现有代码就免费拿到 CSP」这条路**，这点上 WASM 与 x86 的 CSP 处境类似（都要独立造轮子），各后端的 CSP 路径需独立设计。

### 3.5 自举判定与原生运行时

- 自举判定标准：**编译器由本语言写成且能编译自己**；输出 C 再经 gcc/clang 落地，完全算自举
- 「任何架构都能跑」由 C 后端承担；手写后端只承诺 x86-64，其余架构不做手写
- 双后端互为测试参照：同一程序在 VM / C 后端 / 手写后端输出必须一致，纳入测试
- 原生运行时：GC 为 **shadow stack 精确扫描**；单线程承诺使运行时无需线程同步

### 3.6 Release 产物

**命名**：`burryn-<version>-<os>-<arch>.<ext>` 综合包；`burryn-<backend>-<version>-<os>-<arch>.<ext>` 单后端产物。

- 架构命名统一 `amd64`/`arm64`（对齐 GHA runner、Docker tag），不用 Rust 的 `x86_64`/`aarch64`
- 归档格式按平台走：Windows `.zip`、Linux/macOS `.tar.gz`
- 版本号进文件名（脱离 release 页上下文仍可辨识）
- `cranelift-driver` 随 Cranelift 后端产物打包，不独立发

**预编译矩阵（6 cell + 1 跨系统 wasm）**：

| 系统 \ 架构 | amd64 | arm64 |
|---|---|---|
| Linux | 5 后端（x86/C/LLVM/Cranelift/WASM） | 4 后端（无 x86 自写） |
| macOS | 5 后端 | 4 后端（无 x86 自写） |
| Windows | 5 后端 | 4 后端（无 x86 自写，Windows arm64 条件成熟再上） |
| WASM | 跨系统共享，独立 zip | — |

合计 **7 个综合包** + 各后端单产物（22 个，扣除不可能的 cell）。

**Release 页组织**：正文手写 Markdown 表，行 = 后端（含 Full Bundle 一行），列 = 6 平台架构，每格直链具体 asset。
原生 Assets 面板留作自动化抓取入口 + `SHA256SUMS.txt` 落脚点。

**`SHA256SUMS.txt`**：每次发布生成，汇总所有产物哈希，脚本化校验单一入口。

**冷门架构与移动平台**：riscv64/ppc64le/i386/armv7 等 32 位架构、iOS/Android 等移动平台**不进预编译矩阵**——32 位/冷门架构走 `docs/build-exotic.md` 自行产出，移动平台走 `docs/mobile-future.md`（未规划）。

## 4. 工具链设计（单一二进制，cargo 式一体化）

**内核学 go，工程功能与 UX 学 cargo。**

学 go（解析与分发内核）：

- **MVS（最小版本选择）** 版本解析——确定性、无求解器、可复现
- **去中心化**：import path 即来源，不运营中心 registry；proxy 仅为缓存
- **禁止 build 期执行任意代码**（不做 build.rs 等价物）——供应链安全红线

学 cargo（工程功能与 UX）：

- workspace
- profile：仅 `debug` / `release` 两档，不开放自定义
- feature flags：**只允许布尔、纯加法（additive）** feature；禁止互斥 feature、禁止 feature 改变 API 签名；不做 optional dependency 绑 feature。
  解析两阶段：先 MVS 定版本，再取全图 feature 并集
- 顶级 UX：一个命令、好报错、内建 `test` / `fmt` / `build`
- `fmt` 唯一官方格式，零配置

## 5. x86-64 后端

实现：`compiler/backends/x86/x86.bur` + 序列化层 `elf.bur` / `pe.bur` / `macho.bur`。
无 cc 依赖，手写目标文件格式。
调用约定 = System V AMD64 ABI；GC 根扫描沿用 C 后端 shadow stack 精确扫描语义（cgen 的根栈纪律照搬到手写代码生成）。

### 5.1 寄存器约定

| 寄存器 | 角色 |
|--------|------|
| r15 | 值栈顶（向上增长） |
| r14 | 帧基（当前函数） |
| r13 | 跳转表基址（表项存入口相对表基址的偏移，`jt_load` 加回） |
| r12 | 堆 bump 指针 |
| rbx | 全局变量表基址 |
| rbp | 保存调用者 r14 |

**不得修改寄存器约定**——这是 ABI 级别的约束，改了全盘崩。

### 5.2 值表示

Raw int64，8 字节/槽，**无 tag**。
字符串是指针 → `[8B len][content bytes]`（data section 或堆）。
**float 装箱（boxed）**：值栈/字段/列表元素/闭包 upval 里存 8B **指针** → 指向堆上 `[8B bits]` 盒（IEEE-754 double 位模式）；算术/比较/取负走 SSE（`addsd`/`subsd`/`mulsd`/`divsd`/`cmpsd`），`to_float`/`trunc`/`float_bits`/`parse_float` 在盒与 int/str 间转换。

### 5.3 内存布局

```text
代码域（R-X）: headers | consts | funcs | jump_table | _start | .eh_frame
                ^img_base = ELF base+176 / PE base+4096 / Mach-O base+0x1000
数据域（R-W）: runtime slots [+ windows: thunk 槽 | fd 表 | WSA 区]
                ^data_origin = 镜像基址 + DATA_VA_OFF (16MB)
```

- 可写 runtime 区独立成 R-W 段，与代码域之间留 VA 空洞（ELF/Mach-O 不映射，PE 由只占 VA 的 `.bss` 填洞节补齐），定案见 §5.21
- 三目标均为 PIE：上图基址是链接基址，实际装载基址由内核/加载器随机选定，代码全程 RIP 相对，定案见 §5.22
- 堆：16MB via mmap(MAP_ANONYMOUS)，零填充，bump-allocated via r12
- 值栈：1MB below rsp，向上增长
- 全局变量：堆首 N*8 字节（rbx = base）

### 5.4 Shadow stack（编译期类型跟踪）

每个值 push 记录一个 shadow 条目（编译期 `[]` 字符串数组，不进入输出二进制）：

- `""` = int/unknown
- `"str"` = string pointer
- `"float"` = 装箱 float（盒指针）
- `"list"` = list pointer
- 函数名 = 用于 call dispatch

另有扩展 `type_shadow` 多态编码如 `list-<elem>` / `tup:` / `record:` / `map` / `enum:`，驱动格式化分派与字段类型推断。

### 5.5 堆对象布局

**String**：`[8B len][content bytes]`（data section 或堆）

**List**（header 不移动）：`[8B len][8B cap][8B elements_ptr]`（24 字节）。
elements 在 elements_ptr 处，连续 8 字节槽。
初始 inline（elements_ptr = header+24）。
push 增长：分配新 elements 数组（cap*2，min 4），复制旧数据，更新 header 的 elements_ptr 和 cap。

**Closure**：`[8B fn_index][8B upval0][8B upval1]...`（8*(1+N) 字节）。
fn_index 通过跳转表解析：`shl rax, 3; add rax, r13; mov rax, [rax]; call rax`。
Slot 0 of callee frame holds closure ptr；op_get_upval reads `[closure + 8 + 8*idx]`。
默认值捕获（copy-at-capture，非共享 cell）；`capture(ref n)` 引用捕获的槽存 **cell 指针**（堆 8B，恒在堆上，无 open/close 概念），读写须二次解引用。

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

**Float box**（堆，8 字节）：`[8B bits]`——IEEE-754 double 位模式。
值栈/字段/元素/upval 存盒指针，算术比较走 SSE。

**Tuple**（堆）：`[8B n][8B elem0]...[8B elemN-1]`（8+8n 字节），构造时从值栈 rep movsb 复制元素。

**Record**（堆）：`[8B n_fields][8B cidx][8B field0]...[8B fieldN-1]`，与 op_record 配套字段名编码查表。

**不加 type tag 的理由**：类型检查器保证 op_test_variant 和 op_get_field 只收到 enum instance。
运行时类型区分只在 op_call（closure vs constructor）需要，sentinel 处理。

### 5.6 调用约定

Caller pushes：`[callee/closure-ptr][arg0]...[argN]`，then calls。
Callee prologue：`push rbp; mov rbp, r14; lea r14, [r15 - 8*(arity+1)]`。
Callee epilogue：peek return value，collapse to r14，restore rbp，ret。
op_set_global 和 op_set_local PEEK（不 pop）——匹配 VM 语义。

### 5.7 net/exec IO fiber 感知

x86 后端的 net/exec IO 为 fiber 感知等待，语义对齐 C runtime（`bur_wait_current_fd` + 调度器轮询）。

- **禁止裸阻塞 syscall 卡死全进程**：`O_NONBLOCK` + 当前 fiber park + 调度器等已注册 fd 就绪后唤醒
- **等 fd 用 `poll(2)`**，不上 epoll；Windows 由分派器译成 `WSAPoll`（fd 经 fd 表换成
  SOCKET），`x86-pe-run` 以 30 秒看门狗把 `net_loopback`/`net_nb` 作为 hard gate
- **覆盖面**：`tcp_accept` / `tcp_dial` / `net_read` / `net_write` / `sleep`；`exec_poll` 的等待不得在别的 fiber 做 net 时把整进程卡住。不改 §7.1 的 exec「收尾式、非流式」
- **不链 `burrt.c` 的 CSP**：park/wake 走 8 槽 fiber 调度器（8B raw int64 与 C runtime 16B tagged Value 不兼容）
- **`net_nb` 与本项同一设计**：非阻塞原语与阻塞 native 的 park 路径一并落地，不做 int3 占位再推翻

### 5.8 CSP opcode 与调度

x86 后端无 libc、无 ucontext——fiber 调度、park/wake、上下文切换全部自写。
值表示 8B raw int64（无 tag），与 C runtime 16B tagged Value 不兼容，**不能链接 burrt.c 的 CSP**。

- **op_spawn (44)**：2 bytes `[op][argc:1B]`。fiber struct + 独立栈 + 上下文切换
- **op_send (45)** / **op_recv (46)**：各 2 bytes。channel + sendq/recvq + park/wake
- **op_chan_next (47)**：3 bytes `[op][cidx:2B]`
- **op_select (48)**：variable
- **op_defer (49)**：1 byte。per-frame defer 栈
- **close_upvalue**：跳过——默认值捕获不产生 open upvalues；ref 捕获用恒开 cell（见 §5.9）

### 5.9 闭包捕获语义：默认值捕获 + 显式 ref

- **默认捕获 = 值拷贝**（copy-at-capture）：闭包槽存捕获时快照。VM、C runtime 与 x86 的 `op_closure` 默认分支均直接压入栈值（不建 open upvalue cell），三端可观察语义相同
- **显式 `capture(ref n)` = 引用捕获**：闭包槽存 cell 指针，读写经 cell 与外部变量共享存储
  - 语法：闭包块前可选捕获子句，`capture` 为保留字，`ref` 仅 capture 列表内特判；未被 `ref` 声明的捕获项即默认值捕获（显式列出与省略同义）
- **x86 cell 实现**：被 ref 捕获的 mut 局部**提升为 cell 指针槽**——所在函数预扫描本函数全体闭包的 ref 声明，被声明的变量其栈槽存 `&cell`（声明时堆分配 8B），该变量的 get/set_local 全部经 cell 解引用；闭包 upval 槽存同一 cell 指针。GC：cell 为普通堆对象，mark 跟随内容
  - **与 VM open-upvalue 的可观察等价性**：open 期读外部栈槽 = 读 cell 当前值（外部帧存活期 cell 只由提升变量写入，两者同值）；close 发生于外部作用域退出后，彼时外部变量已死、cell 无后续写入，闭包读 cell = 固化值。故 x86 无需 open/close 机制
- **约束**：`ref` 只能作用于局部（含参数），顶层全局拒绝（全局天然共享）；`ref` 捕获不可变变量允许（等价值拷贝，无害）；`ref` 捕获参数允许（参数即栈槽）
- **字节码**：`op_closure` 描述符 `[1B islocal]` 扩展为标志字节 bit0=islocal、bit1=ref（旧值 1/0 仍合法：旧描述符即默认值捕获）

### 5.10 上下文切换：手写 8 槽 ctx（定案）

唯一可行方案（无 ucontext、无 setjmp 可依赖）。
关键洞察：**切换不需要保存指令指针**——切换子程序由生成代码用 `call` 调用，返回地址已在机器栈上，随 rsp 一起保存/恢复。

- fiber ctx = 8 槽内存 `[rsp][rbx][rbp][r12][r13][r14][r15]`
- 切换 = 保存 8 槽到当前 fiber ctx → 读下一个 fiber ctx → 恢复 → `ret` 继续执行
- 正确性论证：生成代码无机器栈局部变量（全部在值栈，r14 相对寻址）；临时值要么在值栈（r15 以下）要么在 callee-saved（全保存）→ 任意指令点挂起都安全
- spawn 首次 resume：trampoline 地址压新栈，ctx.rsp 指向它，trampoline 弹参数调入口函数
- 切换子程序内**不允许分配**——GC 只在生成代码分配点触发，切换中间无 GC 窗口

### 5.11 栈布局：每 fiber 一套机器栈 + 值栈（定案）

- 新 fiber mmap **1MiB**（对齐 C runtime `BUR_STACK_SIZE`），栈底加 **guard page**（不可读写，防机器栈/值栈越界静默损坏；越界 = SIGSEGV trap）
- rsp 与 r15 一起换；`rt_stack_base` 从全局槽改为 **per-fiber 字段**，GC mark 遍历 fiber 表扫每 fiber 的 `[stack_base, top)`
- 主 fiber 保持现状（OS 栈 + `rsp - 1MiB` 值栈）

### 5.12 时间片：callee 入口插桩（定案）

- 插桩点 = **函数 prologue 后一处**（budget 减一 + 条件 call yield 子程序：schedule 自己到队尾 + 切换），比 C runtime 的调用点插桩省开销
- **已知限制**：紧循环不可抢占——budget 只在函数边界递减，纯计算紧循环内无调度点，会独占 CPU 直至函数返回或阻塞；语义上仍是确定性协作式，不违反单线程承诺

### 5.13 channel / send / recv / close（定案）

- 堆对象布局照 C runtime `OChannel`：bounded FIFO buf + sendq/recvq/waiters + closed
- send/recv/close 三路分支照 VM 语义（`vm.bur` op_send/op_recv）逐条翻译
- **操作数留在值栈直到操作完成**——单 send 不用 fiber.sendVal 字段，park 期间值栈整体保留，比 C runtime 更简单
- **元素类型传播**：x86 后端不消费 checker 输出，bytecode 的 op_send/op_recv 无类型操作数，故 channel 元素类型由 x86 后端自维护的栈类型影子（`type_shadow`/`pos_types`）传播：
  - `chan(n)` 构造推类型 `"chan"`（元素未知）；`ch <- v` 处若 v 的类型影子已知（非空），把 ch 持有槽的类型升级为 `"chan:<v>"`（经 op_get_local 推入的 `"slot:N"` 影子定位槽位，写回 `pos_types[N]`）
  - `<-ch` / `for x in ch` / select recv 臂的结果类型从 ch 的类型影子取元素：`"chan:X"` → 结果类型 `X`；`"chan"`（未知）→ 空/`int` 兜底
  - **跨函数传播**：复用已实现的 `fn_param_types` 调用点聚合（sig_specs 机制）——spawn/fn 调用实参为 `"chan:X"` 时，被调函数参数槽类型经 prologue 填充拿到 `"chan:X"`，函数体内 recv 即正确
  - **边界**：send 值类型未知 → 保持 `"chan"` 兜底（recv 后 print 退化为 int 转换，与既有启发式一致）；同一 chan 被不同类型 send（类型错误程序）→ 以最后一次 send 的元素类型为准，行为不定
  - **未来评估**：接入 LLVM/Cranelift 后端、或语言层面需要真正多态（泛型单态化/约束）时，重新评估 bytecode 类型标注方案，届时与 checker 输出接入 bytecode 一并设计

### 5.14 select：重试循环 + 顺序选臂（定案）

- 照 VM 语义：顺序扫臂找第一个 ready → 命中执行 + 跳臂目标；default → 跳 default；全不 ready → 注册到全部 chan 的 waiters + park(FBLOCKED_SELECT) + 醒来回循环头重试；send 臂的 val 在值栈上，park 期间自然保留
- **公平性设计选择（刻意对齐 VM，非遗漏）**：多臂同时 ready 时确定性选**第一个**，不做随机化/round-robin——确定性优先于公平，与 VM 行为逐字节一致是验收前提

### 5.15 GC 覆盖与 park 一致性（定案）

- fiber 表 = 全局 root；chan 的 buf、sendq→fiber→sendVal、waiters 全链路可达
- park 在 send 上的 fiber 触发 GC 时，待发送值仍须存活（防 top 指针漂移漏扫）
- mark 时当前 fiber 扫值栈 `[stack_base, r15)`，其余 fiber 扫其 ctx 槽 + 值栈（保守扫，地址范围检查排除 raw int 误判）

### 5.16 死锁检测（定案）

- 调度器主循环：ready 空 && 存在非 done fiber → fatal deadlock（exit 4，文本照 C runtime）
- 等 fd / timer 的 fiber 不算 ready；未注册进 waitset 的阻塞不得误判为死锁（与 §5.7 fiber 感知 IO 一致）

### 5.17 defer（定案，独立于 CSP 先行实现）

- per-fiber defer 栈（数组存 closure 指针）+ 帧进入记 watermark（照 C runtime `bur_run_defers` 的 dbase 语义）
- op_defer 压入；函数 epilogue 前按 watermark LIFO 执行，返回值先 peek 保留在栈上再执行 defer（防 defer 内分配回收）

### 5.18 multi-backend 九例复核（2026-09-14 逐条重新核实，取代旧报告）

`testdata/pkg/{annotations,cached,constcycle,consts,deepmut,extimport,extmissing,
pipeline,stdjson}` 九例此前只记在一份过期的 gitignored 报告里（`reports/
s8-1-multibackend.md`），归类为「三方一致拒绝」，未按 [`GOALS.md`](GOALS.md) §3
的要求写进本文件；该报告本身有过时内容未同步（记录的元组解构丢类型 bug 已修，报告未
更新），逐条重新核实后发现**原归类本身是错的**：九例里没有一例是 x86 后端缺陷或
语言级限制，具体：

- **`annotations`/`consts`/`pipeline`/`stdjson`（4 例）**：不是拒绝用例，是测试
  夹具本身的语法错误——用 `.`（record 字段访问）而非 `::`（路径限定）做跨包成员
  访问，[`grammar.md`](grammar.md) 明文规定 `::` "永远是路径"、`.` "不得用于路径
  限定"。用 `.` 写会在类型检查阶段被拒绝，四例因此被误记成"三方一致拒绝的语言限制"；
  实际上把 `.` 改成 `::` 后四例都是三方（VM/C/x86）一致的正常成功用例，输出与退出码
  完全一致，不存在任何缺陷。已直接修正四份 fixture 源码。
- **`deepmut`（1 例）**：修正同样的 `.`/`::` 笔误后，三方一致拒绝**依然成立**，但
  这不是缺陷或限制——这是 deep-mut 检查按设计正确工作：跨包 `mut` 绑定要求显式契约，
  这里没有，因此正确报 `E0597`。是诊断功能的预期行为，不是需要"登记"的短板。
  （若未来出现"跨包 `mut` 显式契约"语法，行为会随之改变，不属于本条锁定范围。）
- **`constcycle`（1 例）**：同文件内 `const` 相互引用成环，三方一致在构建期报
  `E0391`（cycle detected）。这条与包/模块/`.`-`::` 完全无关，纯粹是 const 求值器
  的环检测，同样是诊断功能的预期行为。
- **`extmissing`（1 例）**：`bur.mod` 没有 `require` 该模块的声明，三方一致报
  `E0432`（cannot resolve import）。这个结果与本地 `~/.burryn/pkg` 缓存状态无关——
  即使缓存里真的有该模块，缺 `require` 声明本身就会被拒绝；已验证清空缓存/填充
  假缓存两种状态下判据不变。是模块系统按设计工作，不是 x86 或语言限制。
- **`cached`/`extimport`（2 例）**：`bur.mod` 有 `require`，但引用的 `example.com/
  acme/*` 是文档占位域名，从不会在任何真实环境里被 `bur mod download` 解析到——
  在干净缓存（本地默认状态、CI runner 默认状态）下，三方一致报 `E0432`
  （module not in cache）。这是环境状态（缺依赖），不是编译器缺陷。用手工构造的
  假缓存条目（同样路径/版本，导出所需符号）验证过"若依赖可用会发生什么"：两例的
  真实结果（三方一致，输出正确、退出码一致）与干净缓存下的三方一致拒绝一样，都不
  暴露任何 x86 特有差异——即缓存状态翻转，判据（三方一致）不翻。这两例源码里也有
  同样的 `.`/`::` 笔误，已一并修正（当前失败路径走不到那段代码，但修正后源码本身
  合语法，为将来的状态留了正确的基线）。

**结论**：九例里 6 例是三方一致成功（4 例改完 fixture 后立即成立，`cached`/
`extimport` 在依赖可用时也成立）、3 例是三方一致的正确诊断拒绝（`deepmut`/
`constcycle`/`extmissing`，覆盖 deep-mut 安全检查、const 环检测、缺失 `require`
三种独立机制）。**没有发现任何 x86 后端缺陷，也没有需要登记为"语言级限制"的条目**——
`docs/GOALS.md` §3 原始要求的"已知缺陷清零或显式登记为语言级限制"，此处以前者
（清零：不存在缺陷）满足。`scripts/multi-backend-verify.sh` 的 pass 计数不因此改变
（改前改后九例都各自贡献一条 PASS，只是四例从"巧合通过的拒绝"变成"真正的成功"）。

### 5.19 ELF `.eh_frame`（DWARF CFI，定案，2026-09-14）

Linux 目标崩溃可诊断性此前是三个格式里唯一没起步的一个（PE `.pdata`/UNWIND_INFO
与 Mach-O `__unwind_info` 已交付）。本节把 ELF 补齐到同等范围：只覆盖用户函数
（`natives.bur` 的 `x86gen_prologue`），GC/调度器内部子程序与纤程切换点不覆盖，
与已交付的 PE/Mach-O 边界一致——不是新缺口，是本轮特意保持的对称。

**格式选择：`.eh_frame` + 最小节头表，不做 `.eh_frame_hdr`/`PT_GNU_EH_FRAME`。**
这一步不是凭经验假设，是实测定的：在这台机器上分别构造了 (a) 只有
`PT_GNU_EH_FRAME` 程序头、节头表清零的 ELF，(b) 只有三条节头（NULL/.eh_frame/
.shstrtab）、没有 `PT_GNU_EH_FRAME` 的 ELF，两者都嵌入同一段真实的多帧调用链
（crash → b → a）。gdb 13.1 对 (a) 给出的回溯是垃圾（把栈上无关数据当返回地址），
对 (b) 给出的回溯逐帧精确匹配源级调用链。结论：gdb 的事后调试走节头表按名字定位
`.eh_frame` 这条路，`PT_GNU_EH_FRAME`/`.eh_frame_hdr` 只服务运行中进程的 libgcc
展开（`_Unwind_Find_FDE`）——本运行时不装任何信号处理器、不调用 `backtrace()`
（`runtime/*.c` 与 x86 后端代码全文搜不到 `sigaction`/`SIGSEGV`/`backtrace`），
崩溃诊断的唯一消费方是外部工具（gdb/coredump 分析），因此只做节头表这条路径，
省掉 `.eh_frame_hdr` 二分查找表与 `PT_GNU_EH_FRAME` 程序头，实现量减半。

**CIE/FDE 编码**：单份全程序共享 CIE（初始规则 CFA=rsp+8、返回地址列 rip 在
cfa-8）+ 每函数一条 FDE（advance_loc 1→def_cfa_offset 16→rbp 存于 cfa-16；
advance_loc 4→def_cfa_offset 24——对应序言的 `push rbp`/`sub rsp,8` 两步）。
augmentation `"zR"`、FDE 指针编码 `0x1b`（`DW_EH_PE_pcrel | sdata4`）：起始地址存
相对字段自身的 4 字节偏移，镜像按任意基址装载都成立（PIE，见 §5.22）。手写字节先与真实
工具链（GNU `as`）对同一段序言生成的 CIE/FDE 逐字节比对过（`readelf
--debug-dump=frames`），再用编译器实际产出的 ELF 在 gdb 下跑深度非尾递归到真实
栈溢出崩溃，回溯逐帧正确（重复调用点、相同返回地址，形态与源码调用链完全吻合）。

**节头表最小化**：只有 NULL/.eh_frame/.shstrtab 三条，不含 `.text`/符号表——
崩溃可诊断性只要求返回地址能正确回溯，不要求源码级符号名（与 PE/Mach-O 已交付
的范围一致，gdb 在没有符号表时显示 `?? ()` 但地址精确）。节头表本身与
`.shstrtab` 落在 PT_LOAD 覆盖范围之外，是纯尾部调试数据，`execve` 从不读取。

代码：`compiler/backends/x86/elf.bur`（`eh_cie`/`elf_eh_frame`/`elf_shstrtab`/
`elf_section_header`/`elf_header_with_sh`，单测 `elf_test.bur`），调用点
`x86.bur` 的 ELF 分支复用已有的 `pdata_offsets`/`pdata_lens`（PE/Mach-O 同一份
数据）。`scripts/multi-backend-verify.sh` 覆盖的全部 examples/testdata/
testdata/pkg 用例三方（VM/C/x86）复核过，无回归。

### 5.20 内部子程序 unwind 覆盖扩展（定案，2026-09-14）

在 §5.19 交付的 ELF 之上，把三个格式的 unwind 覆盖从"仅用户函数"扩展到
`runtime.bur` 的内部子程序：`gc_record`、`gc_collect`、`mark_addr`、
`chan_schedule`、`chan_wake_waiters`、`chan_push_queue`（sendq/recvq/waiters
三个实例）、`chan_remove_waiter`、`waitset_add`、`timer_add`，共 11 个代码实例。
调度器三块（`sched`/`yield`/`fiber_done`）是纤程切换机制本身，边界不变，仍不
覆盖（见 §5.19）。

**序言形态逐个核对 `runtime.bur` 源码（`push_r` 出现次数与顺序）得出三类**：

- **零序言**（9 例：`gc_collect`、四个 `chan_*`、`waitset_add`、`timer_add`）：
  入口不碰 `rsp`，函数体全程是 CIE 的初始规则（CFA=rsp+8）——不需要任何
  CFI/UNWIND_CODE/compact-unwind 指令，三个格式都退化成"只登记 PC 范围，
  规则沿用入口态"。
- **9-push**（`gc_record`）：入口连续 `push rdx/rdi/rsi/rax/rcx/r8/r9/r10/r11`
  （无 `sub rsp`），CFA 最终到 rsp+80。
- **7-push**（`mark_addr`）：入口连续 `push rax/rcx/rdx/rdi/r8/r9/r10`，CFA
  最终到 rsp+64。

**三个格式各自的编码差异，同一份 push 寄存器列表喂给三套不同规则**：

- **ELF**（`eh_push_instrs`）：DWARF CFI 需要按字节精确的 `advance_loc`，每条
  `push` 的指令长度（寄存器号 <8 一字节，r8-r15 因 REX 前缀两字节）决定步长；
  `elf_eh_frame_internal` 把内部子程序的 FDE 追加在用户函数 FDE 后面，共享
  同一份 CIE。
- **PE**（`pe_unwind_info_pushes`/`pe_unwind_info_zero`/
  `pe_build_pdata_multi`）：Windows UNWIND_CODE 的 CodeOffset 同样按字节精确，
  与 ELF 同一套"1/2 字节"判断；三种序言各自一份共享 UNWIND_INFO（用户形态不变、
  新增零序言与两份 push 形态），.pdata 数组内部子程序 11 项排在用户函数前面
  （地址恒更低，满足 `RtlLookupFunctionEntry` 要求的 BeginAddress 升序）。
- **Mach-O**（`macho_unwind_encoding_pushes`）：compact unwind 的 stack_size
  字段单位本就是 8 字节而非指令字节，不需要区分 REX 前缀长度，比 ELF/PE 更简单
  ——直接是 push 条数。`reg_count`/`permutation` 统一置 0：这些寄存器多数不是
  callee-saved（是子程序特意保护的调用者寄存器），compact unwind 的
  permutation 字段设计上只服务真正"恢复寄存器"的异常传播场景，本项目不做
  personality/LSDA 展开，只要 stack_size 正确即可回溯，`reg_count=0` 无影响。
  `macho_unwind_info` 的 regular 二级页本就逐项存编码，异质编码天然支持，
  不需要引入 compressed 页那层间接；内部子程序与用户函数混在同一页，按地址
  升序排列。

**验证方式对齐 §5.19 的力度，三个格式各自能做到的最大程度**：ELF 有本机 gdb，
做了两层验证：(a) 手写字节先与真实 `gdb`/`readelf` 解出的每一条
`DW_CFA_advance_loc`/`DW_CFA_def_cfa_offset` 逐项核对；(b) 用编译器实际产出的
ELF 跑 `gc_stress.bur`（真实触发 GC），在 `gc_record` 入口断点，`bt` 显示
frame 0→1 的过渡正确落到真实调用点（地址落在某个用户函数的 FDE 覆盖范围内）——
frame 1→2 再往上偶尔失真，定位到的原因是用户函数里内联的 `gc_rec()` 括号
（临时 push/pop 三个寄存器再 `call`）会让该用户函数在那个精确调用点的真实
CFA 与"序言结束后维持不变"的既有假设有 24 字节的偏差；这是 §5.19 就已经
接受的、与本轮无关的既有局限（不建模函数体中段的临时 push/pop），不是本轮
引入的新问题——单独复核过深递归崩溃（跨用户函数、不涉及内部子程序）的
回溯依旧逐帧精确、零回归。PE/Mach-O 没有本机 Windows/macOS 调试器，验证止于
静态字节级核对：分别手工解析真实构建产物的 `.pdata`/`__unwind_info`
二进制内容，逐字节比对 11 个内部子程序条目的 PC 范围、编码偏移与三种序言各自
的 UNWIND_CODE/compact-unwind 数值，与设计推导完全吻合；这与 §5.19 交付
PE/Mach-O 初版时同一验证力度，如实记录，不假装有更高确信度。

代码：`elf.bur` 的 `eh_push_instrs`/`elf_eh_frame_internal`，`pe.bur` 的
`pe_unwind_info_zero`/`pe_unwind_info_pushes`/`pe_build_pdata_multi`，
`macho.bur` 的 `macho_unwind_encoding_pushes`（`macho_unwind_info` 签名
扩展为接收逐项 `encodings` 数组），`x86.bur` 里新增的 `rt_unwind_offsets`/
`rt_unwind_lens`/`rt_unwind_pushes` 三个并行数组（三个后端共用同一份数据源）。
单测覆盖 `elf_test.bur`/`pe_test.bur`/`macho_test.bur`。
`scripts/multi-backend-verify.sh` 覆盖的全部 examples/testdata/testdata/pkg
用例三方（VM/C/x86）复核过，无回归。

### 5.21 节/段拆分：R-X 代码与 R-W 数据（定案，2026-09-23）

完成线「节/段拆分（R-X 代码与 R-W 数据分开）」的落地定案：三目标一致把可写
runtime 区（GC/调度器槽；Windows 另含分派器 thunk 槽、fd 表与 WSA 区）从代码域
迁出，代码域收成 R-X。本节落地时仍是固定基址，PIE 随后另行落地，见 §5.22。

- **布局**：可写区落在镜像基址 + 16MB 的固定高 VA（`DATA_VA_OFF`，`layout.bur`
  单一定义，取代原 Mach-O 专属的 `MACHO_DATA_VA_OFF`），与代码域之间留未映射
  VA 空洞，使 `rt_slot` 等地址公式在代码长度确定前即可求值。可写段按目标分别
  是：ELF 第二条 PT_LOAD（`p_flags=R|W`，文件内页对齐紧跟 `.eh_frame`）；PE
  `.data`（三节表：`.text`、`.bss` 填洞节、`.data`，头区实占 448B，
  `SizeOfHeaders` 取 1024B，`SizeOfImage` 覆盖固定数据 RVA；PE 规范要求各节 VA
  升序且相邻，真 Windows 加载器对 `.text` 与 `.data` 之间的 VA 空洞直接报
  `ERROR_BAD_EXE_FORMAT`（Wine 不查），故以 `.bss` 填洞节补齐：`VirtualSize` =
  空洞长度、`RawSize=0`、`UNINITIALIZED_DATA|READ`，只占 VA 不占文件；
  kernel32 + ws2_32 导入 blob 跟在 runtime 区之后同落 `.data`——加载器解析导入
  要原地写 IAT，只读节收不下，描述符与 IAT 预填写 RVA（基底 `DATA_VA_OFF`），代码引用 IAT 槽用绝对 VA（经
  `data_origin()`））；Mach-O `__DATA`（先行落地）。代码域
  R-X：ELF 首条 PT_LOAD（两条 phdr 使 `img_base` 从 base+120 移到 base+176）、
  PE `.text`（`CODE|EXECUTE|READ`）、Mach-O `__TEXT`。
- **寻址收口不改**：`rt_slot`/`fd_base`/`wsa_base`/`thunk_slot_abs` 全部经
  `data_origin()` 取址，代码域经 `img_base()`/`code_origin()`——runtime 槽地址
  整体平移到高 VA、代码内绝对地址随 `img_base` 平移，无逐处改写；
  `rt_region_size()` 随之失去调用方，删除。
- **验证**：ELF 真机端到端——16 个 examples（GC 压力、并发五件套、io、net
  loopback/net_nb/net_errors）构建运行对 golden 一致（PE 轮改动不触碰 Linux
  发射路径，ELF 字节与该轮验证时一致）；`readelf -l` 核对两条 PT_LOAD 权限
  （R E / RW）与 VA；`readelf --debug-dump=frames` 核对 76 条 FDE 全部落在新
  代码域内且升序。PE 用 Python 结构校验头区/节表/SizeOfHeaders/SizeOfImage 与
  数据段内三个 GC 子程序地址槽（须指向代码域）——该校验当场揪出初版把 `.text`
  的 Characteristics 误算成 `0x60000040`（INITIALIZED_DATA 而非 CODE）的错常数；
  结构校验含「各节 VA 升序且相邻」一项，单测
  `test_pe_emit_headers_sections_adjacent_across_data_hole` 锁定三节表。真 Windows
  `x86-pe-run` 全部 hard gate 对 golden 一致。
  Mach-O 字节不变（常量等值改名，既有单测锁定）。

### 5.22 PIE：双基址差分改写成 RIP 相对（定案，2026-09-24）

完成线「PIE（三目标）」的落地定案。取「代码取址一律 RIP 相对、数据不存绝对指针」
的混合路线：镜像零重定位项，三目标行为一致，W^X 不破（不需要装载期改写代码页）。

- **难点**：后端以字符串拼接发射机器码，片段生成时不知道自己最终落在哪个偏移，
  约 490 处 `mov r64, imm64` 内嵌 `img_base()`/`data_origin()` 算出的绝对地址，
  逐处改成位置感知不现实。
- **双基址差分**（`pic.bur`）：`x86gen_image` 从已求解的共享上下文装配整份镜像，
  按两个基址各跑一遍——先以 `PIC_PROBE_SHIFT`（`2^32 + 2^20`，高低 32 位都非零、
  页对齐）平移的探针基址，再以真实基址。平移经 `layout.bur` 的 `addr_shift` 叠加到
  `img_base`/`data_origin`，所有绝对地址都经这两个函数收口。编码长度与立即数取值
  无关，两遍代码等长；逐字节比对后，每处差异必须落在某条 `mov r64, imm64`
  （`REX.W[.B] B8+r`）的立即数内、且两遍取值恰差平移量，该 10 字节指令原地改写成
  `lea r64, [rip+disp32]`（7 字节）+ 3 字节 nop，长度不变，其余跳转偏移随之有效。
  任何其他差异（imm32 里的地址、数据里的指针）都是未收口的绝对地址，报内部错误
  终止编译，不会静默产出坏镜像。可写数据段初值要求两遍逐字节相同。
- **数据侧去绝对指针**：跳转表项改存「入口 − 表基址」的相对偏移，取表项处经
  `jt_load` 加回 r13（`_start` 本就用 `lea rip` 定 r13）；三个 GC 子程序地址槽与
  Windows/darwin 分派器槽不再在镜像里存初值，由 `_start` 开头的 `rt_init` 按运行期
  地址写入（`_start` 长度与写入值无关，先以 0 量长再定分派器地址）。
- **格式标志**：ELF `e_type=ET_DYN`、无 `PT_INTERP`（static-pie，内核从随机 mmap
  区定基），`.eh_frame` 改 pcrel（§5.19）；PE `DllCharacteristics=0x160`
  （`HIGH_ENTROPY_VA | DYNAMIC_BASE | NX_COMPAT`）并带一个只含跳过项的基址重定位
  目录（数据目录 [5]，挂在 `.text` 末尾）——镜像本无需修补，带目录是让加载器把镜像
  视作可重定位、实际施加 ASLR；Mach-O 置 `MH_PIE`，dyld 施 slide 无需 rebase。
- **代价**：`x86gen_image` 跑两遍，前段类型求解（psim、签名求解）只跑一遍。
- **验证**：单测锁定改写形态（低位/REX.B 寄存器、等长、无差异不改写）、pcrel FDE
  与 CIE 编码、PE DllCharacteristics 与重定位目录；Linux multi-backend 三方 91/0/0，
  exec pipe 失败三档回归通过；gdb 关闭 `disable-randomization` 两次 `starti` 映射基址
  不同，代码段 R-X、数据段 R-W 相距 16MB，随机基址下中断深递归回溯逐帧正确；
  examples 与 testdata/regression 全部 `.bur` 以 windows/darwin 目标构建无内部错误；
  真 Windows `x86-pe-run` 与 macOS `x86-macho-run` hard gate 对 golden 一致。CI 另以
  常驻程序 `testdata/pie/spin.bur` 在三平台读进程实际映像基址（Linux `/proc/<pid>/maps`、
  Windows `MainModule.BaseAddress`、macOS `lldb` attach 后 `image list -h`）：须偏离链接
  基址；Linux 与 macOS 另要求两次启动基址不同（Windows 映像 ASLR 按开机选偏移，同一
  镜像多次运行基址相同）。

## 6. LSP 与编辑器生态

**架构定案**：LSP 服务器用 Burryn 写（延续自举原则），作为 `bur lsp` 子命令，stdin/stdout 走 JSON-RPC 2.0（LSP 3.17 规范）。
所有语言智能在服务器端；编辑器插件是**薄客户端**——只转发 LSP 消息 + 渲染 UI，不含语言逻辑。
新增编辑器支持 = 实现 LSP client 协议，零服务器改动。

**传输层**：Content-Length 帧分割 + JSON-RPC 消息解析/路由；消息体 JSON 序列化/反序列化走 `std/json`。
用 `read_stdin(max: int) -> str`（读至多 max 字节，EOF 返回 `""`）配合 `print`（stdout）。

**文档同步**：Full sync 模式（didOpen/didChange/didClose 全量内容），服务器维护内存文档表，checker 读内存覆盖磁盘。
增量 sync（delta）为后续优化，第一版不做。

**诊断推送**：didOpen 与 didChange（防抖）后重跑 check 管线（lex → parse → check），DiagT/DiagX 转 LSP Diagnostic 推送。
check 管线须支持从内存源码运行（LSP 核心工程改造点）。

**语言特性**：

- hover：显示推导类型/签名（复用 checker 推导结果）
- go-to-definition：从名字使用处跳到绑定处（需 AST span → 定义 span 映射，跨文件走 module loader）
- completion：作用域感知名字补全（局部变量、函数名、包成员 `pkg::`）
- formatting：调 `bur fmt -`（stdin→stdout）
- signature-help：函数调用时显示参数信息，native 内建走同一张签名表
- references：光标落在声明名或使用处都归一到同一绑定，再列出全部使用位置
- documentSymbol：按文档列出顶层声明的全区间与名区间
- documentHighlight：与 references 同一归一集合，落回当前文档并标 kind
- rename / prepareRename：复用 references 的归一集合把每个位置转成一条
  `TextEdit`；native 内建（`declare(..., Sp(0, 0))`，`cur_file` 为空串）在这三个
  特性上一律判定为不可定位，返回空结果而不是把毫不相关的同名 native 揉进一组

**编辑器客户端**：

| 编辑器 | 技术栈 | 备注 |
|--------|--------|------|
| VSCode | TypeScript + vscode-languageclient | 首个客户端；bundled `bur` 二进制或 PATH 探测；TextMate grammar 语法高亮；Marketplace + VSIX 分发 |
| JetBrains | Kotlin + lsp4j | 单插件兼容 IDEA/CLion/PyCharm 等；IntelliJ 2023.2+ 内建 LSP API，旧版走 LSP4IJ |
| Neovim | 用户配置 | 0.8+ 原生 LSP，提供 `bur lsp` 配置片段即可 |
| Emacs | 用户配置 | lsp-mode 或 eglot 配置片段 |
| 其他 | — | Helix / Zed / Sublime 等 LSP-capable 编辑器直连 |

S9 阶段的剩余项与顺序见 [`GOALS.md`](GOALS.md) §4。

## 7. 进程间通信与 runtime IO（exec / net）

> **注意：** 本节为行为条款，与语言定案同属约束。实现不得偏离。

### 7.1 exec 子进程：收尾式，非流式

- `exec_start(cmd, args)` 仅为子进程建立 stdout / stderr 两个 pipe；**stdin 未重定向**（子进程继承父进程 stdin）；另有 fail pipe 仅用于 execvp 失败探测（CLOEXEC，exec 成功即闭）
- `exec_poll(h)`：子进程运行期间返回 `None`，**退出后**才一次性返回 `Some(Ok(Output(code, stdout, stderr)))`；stdout/stderr 全程由父进程缓冲，退出时打包交付
- **不支持运行期间流式读写**：无 exec 侧句柄式管道访问；父进程无法向子进程写，也无法在子进程存活期内读取其增量输出

### 7.2 进程间双向流式通信：唯一路径 = TCP loopback

- 全语言唯一支持双向流式通信的原语为 `tcp_listen` / `tcp_accept` / `tcp_dial` + `net_read` / `net_write` / `net_close`；C runtime 与 VM 均为 fiber 感知阻塞（park 当前 fiber，调度器继续运行）
- VM runtime 上 TCP loopback（机器原生闭环基线约 34 µs/round-trip）：
  - 单进程内两 fiber echo：约 231 µs/round-trip
  - 跨进程（`exec_start` 拉起 worker，TCP）：约 1350–1550 µs/msg
  - 512KiB 往返：约 65 MiB/s（单向约 32.6 MiB/s）
- 跨进程延迟的主导开销在调度器 idle-wait（socket fd 未注册进 waitset、等待周期约 1ms），非 TCP 层本身。是否优化见 [`GOALS.md`](GOALS.md) §8，不改本节语义
- exec 管道与 TCP 无可比 round-trip：exec 为单向、收尾式，无双向通道

### 7.3 消息分帧：不存在，需自行实现

- `std/net` 仅提供 `write_line`（末尾追加 `\n`）作为事实上的行分隔约定；`std/json` 仅提供 parse/render/pretty/get
- 无长度前缀、无内置帧协议；消息边界必须由使用者自行实现（如基于 `std/json` 手写 newline 分帧）
