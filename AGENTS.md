# Burryn

## 协作方式

- LLM 直接编写、修改源码,产出完整可运行实现
- LLM 默认执行 git add/commit:每批任务完成、测试全绿后按逻辑切分提交,**不等 owner 确认、不把提交留给 owner**;仅当 owner 当批明确说"别提交"才例外(push/tag 始终归 owner)

## Commit 规范(严格遵守)

> Follow Conventional Commits 1.0.0 strictly. Format: `<type>(<scope>): <description>`. Types: feat, fix, docs, style, refactor, perf, test, chore, ci. Subject line under 72 characters, imperative mood, no period, no emoji. Omit scope unless the change is confined to one subsystem. Add a blank line then bullet points for body if changes are non-trivial. Body bullets use '-' only. Each bullet states ONE concrete change: what was added, removed, or modified. NEVER write purpose, value, or benefit clauses (e.g. 'for external use', 'to enable X', 'establish foundation for'). NEVER add summary or concluding bullets. Do not describe runtime behavior or API contracts.

- 不写 Co-Authored-By 尾行
- 每个 commit 保持独立可测(源码与其配套测试同 commit)

## 权威来源

**GOALS.md 是唯一最高约束**——语言设计定案、拒绝清单、后端路线、里程碑全在其中

- 遇到 GOALS.md 未覆盖的设计决策:**停下问 owner**,不要自行拍板
- 本文件与 GOALS.md 冲突时,以 GOALS.md 为准
- LLM 可自行更新本 AGENTS.md 使进度快照与仓库状态同步,无需授权

## 路线(owner 2026-07-03,四段)

**① stdlib(done)→ ② C 后端(done)→ ③ 自举(done,2026-07-05,S1)→ ④ json/net + 更广 stdlib(当前)** → 之后 MVS/依赖拉取 → 诊断深度

- ④ 故意排自举之后:自举用不到、每 native 要 VM+C 双写,里程碑前双写纯浪费

## 当前进度(每批后更新;已完成批次只留一行,细节看 git log)

- **已完成**:v2 全链路(tag v2.0)→ golden 化 → 砍 --dyn → 深 mut(E0596)→ 字节码栈验证器 → 整数溢出 trap → Span 重构(字节偏移全链路)→ 错误机制统一(Diag/renderDiags 单管线)→ 模块系统(本地全量:bur.mod、多包 checker/编译、CLI 目录模式)→ v3 语言特性(mut 参数、map 函数 API、channel close/recv/for-in、select)→ **① bootstrap-critical stdlib**(str/list 缺口、fs 返 Result、exec+Output enum、argv/exit;examples/textproc golden;vm.ok/err/okStr/errStr/output 均在 fiber 栈上 root 中间分配)
- **②-a 顺序核(done,2026-07-04)**:bytecode→C(cbackend.go,逐 opcode 直线翻译 + goto,显式操作数栈,帧走 C 调用栈)+ C 运行时(runtime/burrt.h、burrt_natives.h:tagged-union Value、mark-sweep 精确 GC、顺序 opcode/native 全集、常量对象 pin 成永久 root)+ `bur build`(build.go:`$CC`→cc/gcc/clang、`--emit c`、无 cc 降级)。parity_test.go:8 个顺序 example(含模块)VM/C stdout 逐字节一致。gc_stress 只断言 GC 事实(选项 3)。float 走「升精度 %g 最短 round-trip + .0 后缀」。并发 opcode 现为 build 期报错,留 ②-b
- **docs**(2026-07-04):双语教程 docs/tutorial.md;README 更正到 v3
- **②-b 并发核(done,2026-07-04,dev/concurrency)**:C 后端并发核落地,整门语言双后端一致。每 fiber 各持 ucontext + 自有 C 栈;阻塞=swapcontext 换整条 C 栈到调度器,唤醒再换回(忠实照搬 vm.go:FIFO ready 队列、park/wake、接收方 park→C 层 retry loop 取代字节码倒带、select 声明序优先+可选 default、close/向已关闭发送 trap、全阻塞=死锁 exit 4、main 返回即结束)。补 OChannel(OBJ_CHANNEL:buf+send/recv/waiter fiber 队列,GC trace/free/typename/format 全覆盖);GC 根扫全体 fiber 栈+open upval+sendVal;call_depth 移入 Fiber。补 native chan/close/recv/yield(recv 直接 swapcontext park,不用 VM 的 parkRecv 哨兵)。cbackend 译 Spawn/Send/Recv/ChanNext/Select、修 instLen/jumpTargets(ChanNext/Select 操作数)、抢占钩子挂函数入口+OpLoop 回边、main 建主 fiber(8MiB 栈)跑调度器。parity_test.go 纳入 5 个并发 example(sieve/brainfuck/multiplex/pipeline/streaming)VM/C 逐字节一致。**两处记名决策**:①抢占钩子在回边/函数入口而非逐 opcode,与 VM timeSlice 非指令级精确——但 5 例都先阻塞于 channel、budget 从不触发,故无碍;②done fiber 的 C 栈暂不回收(VM 亦留全 fiber),仅内存、②-b 可接受
- **S1 自举编译器(done,2026-07-05,Fable)= GOALS §3/§5 v3 里程碑「编译器完全自举」达成**:burc/ = Burryn 写的完整编译管线(token/lexer/parser/ast/types/compiler/cgen/module/dump/main .bur,~7000 行,bur.mod 包形态)。六个 checkpoint 各自 parity 全绿单独 commit:S1-1 lexer(token-dump)→ S1-2 parser(AST-dump)→ S1-3 checker(check-dump)→ S1-4 compiler(disasm)→ S1-5 cgen(emit-c 逐字节)→ S1-6 定点(包模式+module.bur 加载器+build-dir)。**验收 TestBurcSelfHost**:burc(VM 上)对自己整包发的 C 与 Go 工具链逐字节一致 → cc 编成原生 burc0 → burc0 再编同一份源输出逐字节相同。顺手产出:修 Go VM 真 GC bug(OpClosure 填 Upvals 中途 collect 撞 nil 槽,独立 commit)、新增 native float_bits(五处同步,见下)。**子集边界**:burc loader 无 import 单包(编自己够用)、verify.go 未移植、CLI 是 parity 用分段 dump 命令(run/build 形态归 S3)。细节看 git log 与 bootstrap_test.go
- **S2 Burryn 重写 VM(done,2026-07-05,Fable)= GOALS §3 S2 达成**:burc/vm.bur(~1900 行)= 完整字节码解释器,owner 拍板架构后动工(①值表示=混合枚举+句柄:`enum Val{VUnit,VBool,VInt,VFloat,VStr(str),VObj(int)}`,标量/字符串内联,可变对象走句柄;②两层 GC:被解释堆=单 tagged arena(平行 mut 列表),自带 mark-sweep 逐条移植 gc.go(free list 复用槽、释放清 payload 让宿主收内存),宿主 GC 只管 VM 自身 Burryn 值,heap_objects/gc/gc_cycles 报被解释堆;③零序列化:直接消费 burc `Compiled(...)` 内存态字节码,「同批字节码」由 S1-4 disasm parity 背书;④loader 扩到本地多包 import)。opcode 顺序+并发全集、native 全集、FIFO 调度/timeSlice/死锁检测全移植;trap=eprintln+exit(4) 就地终止(不 unwind)。CLI:`burc run <file.bur>` / `burc run-dir <dir>`。**验收全绿**:TestBurcRunParity(Go-VM 上双层解释,12 脚本+geometry)+ TestBurcSelfHostRun(cc 编原生 burc 后重跑全部 example)对 Go-VM stdout 逐字节。顺手修的真 bug:cgen/dump 字符串转义对 ≥128 字节错(ord/char_at 跨后端不 byte-exact,见「待 owner 定夺」),新增 native byte_at(str,int)->int 五处同步、c_str/go_quote 改纯字节;另加 native eprintln(stderr,五处)。S2-3 import:module.bur 递归 loader(E0391 环/E0432 外部/E0252 别名/unused_import/枚举字段 TE 重写),checker 多包驱动+包作用域快照+E0603/E0423/E0425,compiler 多包驱动+按文件别名表就地解析(不像 Go 重写 AST——三处消费点就地查别名,行为等价)
- **S3 Burryn 写 CLI driver + 归档 Go(done,2026-07-06)= GOALS §3 S3 达成 → 全自举终局达成,Go 树清零**:三层推进——L1 burc 补导出(cgen.bur 的 `cgen_program_to_buf(...)` 返回 C 字符串不 println,原 `cgen_program` print-wrapper 删除,main.bur 的 emit-c/build-dir 改 `print(cgen_program_to_buf(...))`);L2 库/驱动分层(11 个库文件移入 `burc/lib/` 包 `burryn.dev/burc/lib`,cli-facing 符号标 pub;`burc/` 根 = cli driver `main.bur`,`import "burryn.dev/burc/lib"` + 三段式限定枚举模式 `lib.EnumName.Variant(...)` + 两段式函数调用 `lib.fn(...)`,镜像 Go main.go:run/check/dis/build(`--emit c`/`-o`)/version/help/默认 run + 隐藏 `dev <sub>` 保留旧 parity dump;exit 码 1/2/3/4);L3 归档+删 Go(`git branch archive/go-host` 存全量 Go 宿主 + cli 快照 → `git rm` 全 25 个 .go + go.mod → README/tutorial 更到自举叙事)。**架构决策**:cli 不做独立 module(burc loader module.bur:538 禁跨 module import,E0432),改作 burc 子包;因 loader 强制 entry `fn main` 在根包(compiler.bur:1693 取 `pkg_paths[n_pkgs-1].main`=最后加载的根包),故库下沉 `burc/lib/`、cli 占根。**build 策略(owner 定「simplify build, no new natives」)**:无 getenv/exe-path/mkdtemp/remove native,故 `bur build` 写 `program.c` 到 CWD、探测 cc/gcc/clang、`cc -O2 -o <out> program.c -Iruntime -lm`(runtime 头靠固定相对路径,bootstrap 从 repo 根跑),不 honor `$CC`;编成后 `exec("rm",[cfile])` best-effort 清理 program.c。**自举定点验收**:archive seed(Go 版 4245088B)→ `bur build burc` → 原生 bur → 自编再产出逐字节相同(sha256 c0652a14…,删 Go 前后同哈希);acceptance smoke(version/run/check/dis/module/默认 run + `check burc` 整包)全绿。commit:L1+L2=ab6a909、archive/go-host=ab6a909、删 Go=402040a、program.c 清理 fix=6800479;archive/go-host + main 均已 push。**bootstrap gotcha**:gitignored 的 program.c 会存活 `git checkout` 并撞坏 Go seed 的 `go build`（"C source files not allowed"）→ 建 seed 前先 `rm -f program.c`
- **当前位置(④ json/net + 更广 stdlib)**:S1+S2+S3 完成,全自举达成——main 上零 .go,靠 `burc/` cli + `burc/lib/` + burrt.h + cc 自举;Go 宿主留档 `archive/go-host`(可 checkout 接生,已 push)。下一步:④ json/net + 更广 stdlib(每 native 现只需五处:burc/lib types.bur/compiler.bur/vm.bur + burrt_natives.h + 库实现——Go 已删,「六处」缩回)→ MVS/依赖拉取 → fmt/test → 诊断深度

## 待 owner 定夺:①Go module.go 的 fileResolver 不遍历 SelectStmt(疑似 Go 侧潜伏 bug:select 体内的别名引用不会被重写)——burc 的 usage 扫描照抄该行为,但 checker/compiler 就地解析会解析 select 体内别名,极端程序两侧行为可能分歧;②对包级全局赋值(`pkg.x = v`)Go 的 AssignStmt 只处理 Ident/Index、PkgAccess 目标静默落空,burc 同样落空——两侧同样"坏",留给诊断深度批次

## 自举交接与分工(owner 2026-07-04;S1 已完成)

- ~~Fable 5 = 全自举线 S1~~ **done**(见「当前进度」);S2/S3 见「全自举终局」——S2 到做时**先停下问 owner 架构**(值表示、被解释堆 vs 宿主堆、GC 分层),S3 归 owner 亲手做
- **Opus = 后续大块**(与自举解耦):**④ json/net + 更广 stdlib** → MVS/依赖拉取 → fmt/test → 诊断深度;外加后端 bug、cc 兜底
- 两线均**直接在 main 上做**,无需另开 dev 分支
- **native 新增约定(长期有效;S2 起五处变六处)**:一个 native 要动**六处**——Go 侧 builtins.go(实现)+ types.go declareBuiltins(类型)+ C 侧 burrt_natives.h(实现+注册)+ burc 侧 types.bur decl_native + compiler.bur native_names + **vm.bur do_native(bur-VM 的解释实现,多数可委托宿主同名 native)**。样板:byte_at / eprintln(S2 加的,六处齐全)。每加完 `go test .` 保绿
- **素材=源码即规格(不复制以免漂移,S2 仍用)**:架构 lexer.go→parser.go→types.go(checker/HM)→compiler.go(单遍出字节码)→vm.go(解释)/cbackend.go(出 C);**opcode 规格**看 chunk.go(操作数+栈效果注释)+vm.go(语义/trap)+cbackend.go(每 op 的 C 翻译对照);Burryn 侧对照 = burc/ 同名文件

## 全自举终局(owner 2026-07-04 定)

**决定**:终局=**全自举,只留 C 底座**。工具链里非 Burryn 的只剩 C 运行时(burrt.h/burrt_natives.h)+ 目标平台 cc;**Go 整棵树最终清零**(编译器前端 + VM + CLI driver 全用 Burryn 重写)。

**改写旧定案(已同步 GOALS §3「全自举终局」,2026-07-04)**:GOALS §3「VM(Go 宿主)当前主力」+ ②「VM 始终是零 C 依赖兜底」作废——VM 改由 Burryn 写、经 cc 编成原生二进制;"零 cc 兜底"主动放弃,cc 成**工具链构建**的硬依赖(站 Rust 侧,自觉代价)。注:VM 二进制建好后,`bur run`(源→字节码→解释)**运行期仍不需 cc**;需 cc 的是构建工具链本身与 `bur build`。

**为什么可行**:VM 的调度器/fiber/channel 是单线程程序里的**数据结构 + 派发循环**(vm.go 本不用 goroutine),故"用 Burryn 写 VM"只吃 ②-a 顺序能力——不需要 Burryn 自己的 spawn/channel 去实现被解释的 spawn/channel(后者当数据模拟)。VM 重写不被并发核阻塞。

**分阶段(每阶段自举判定 + parity 全绿才进下一段)**:

- **S1 自举编译器前端(done,2026-07-05)**(=原 ③ / GOALS v3 里程碑):Burryn 写 lexer→parser→checker→compiler→codegen(出 C)。验收已过:定点(burc 编 burc 逐字节同,TestBurcSelfHost)+ 对 Go 版编译器 parity 全绿。Go 前端**保留当 oracle + 种子**。
- **S2 Burryn 重写 VM**:vm.go(调度器 / opcode 派发 / channel / GC 交互)移成 `.bur`,经 C 后端出原生 VM。验收:bur-VM 解释同批字节码,对 Go-VM stdout parity(顺序 + 并发 example 全覆盖)。
- **S3 Burryn 写 CLI driver + 归档 Go**(排定 2026-07-06,详细方案见下):`bur` 命令(main.go:arg 解析、run/build 派发、bur.mod/模块加载、诊断渲染)也移成 Burryn。三件套全自举、parity 稳一段后——**先把整棵 Go 实现推到留档分支 `archive/go-host`**(owner 2026-07-04:自举完 Go 那部分单开一分支留档),**再从 main 工作树删 Go 源**(vm.go/compiler.go/lexer.go/parser.go/types.go/cbackend.go/builtins.go/main.go/module.go…)。main 只留 burrt.h/burrt_natives.h + `.bur` + seed。重新接生 = checkout `archive/go-host`(bootstrappable,信任链不断)。

**到阶段再停下问 owner**:S2 VM-in-Burryn 架构(值表示、被解释堆 vs 宿主堆、GC 分层);S3 seed 形态与 driver 组织;诊断 / 模块在 Burryn 侧的组织。

### 执行排序(owner 2026-07-04 定;结果:S1 按此完成)

原定"Fable 专注 S1、每段 parity 绿即 commit、S2 仅 bonus、S3 不碰"——已如此执行,交付判据「S1 扎实 + 全绿 + owner 接得住」达成。仍然有效的原则:**永不为"做完"牺牲绿状态**;**别赶着删 Go**(oracle + 兜底 + 调试参照,owner 信得过 Burryn 工具链之前留着当网);S1 的产物 burc/ 是这门语言最好的规格书,owner"了解语言"靠读它。

## S3 设计定案(owner 2026-07-06,LLM 执行)

### CLI 组织:新建 `cli/` 目录,import burc 当库

burc 定位为编译管线库(lexer/parser/types/compiler/cgen/vm/module),不动它。新建 `cli/` 做薄层 driver:

```
cli/
  bur.mod          → module bur
  main.bur         → arg 解析 + command dispatch
```

`cli/main.bur` 的职责:
- 参数解析:从 `args()` 拿到命令(`run`/`build`/`check`/`dis`/`version`)和目标路径
- `run`/`build`/`check`/`dis`:调用 burc 管线,渲染诊断到 stderr,按 exit code 退出
- `build`:先走 burc 出 C 代码,写临时 .c 文件,调 `$CC`(或 cc/gcc/clang)出二进制,`--emit c` 则 dump 到 stdout
- `version`:打印版本号
- 默认(`bur <file.bur>`)等价 `run`

### burc 现有 parity 命令:改名 `bur dev`

burc/main.bur 现有的 `lex`/`parse`/`check`/`dis`/`emit-c` parity 调试命令改为 `bur dev <subcommand>`,不出现在主帮助里,不污染用户 UX。

### burc 需新增的导出

`cgen_program` 当前直接 println 到 stdout。cli 的 build 需要捕获 C 代码写文件,故 burc 需新增:
- `cgen_program_to_buf(...)` — 返回 C 代码字符串(内部用 buf 累积,不调 println)
- 或更简洁:改 `compile_program`/`compile_script` 返回枚举里多一个 variant 携带 C 代码字符串

### 缺的 native:无

当前 native 集合已够用:args/exit/eprintln/read_file/file_exists/read_dir/write_file/exec。无需新增 native。

### 实现顺序(L3 可逆序独立做)

1. **L1 burc 补导出**:`compile_script` 系列枚举加 `CompiledC(fnames, c_src)` variant,`cgen_program_to_buf` 返回 str
2. **L2 写 cli/**:为 burc 补的导出做对齐(compiler.bur/cgen.bur 无关),纯 cli/main.bur 本身——参数解析 + 调 burc 管线 + 诊断渲染 + cc 调用
3. **L3 归档 Go + 删 Go**:先 `git branch archive/go-host main && git push origin archive/go-host`,再从 main 删 Go 源文件,更新 Makefile/构建脚本,留 seed 文档

### 构建 seed

S3 完成后 Go 删光,重新接生流程:

```bash
git checkout archive/go-host         # Go 版 bur,可构建
go build -o bur.exe .                # 出临时 Go 版(seed)
./bur build cli -o bur               # 用 seed 编 Burryn CLI → 原生 bur
git checkout main                    # 切回无 Go 的 main
./bur build cli -o bur               # 原生自举:bur 编自己
```

### 验收标准

- `cli/main.bur` 对 `burc/` 整包的 check/dis/emit-c 产出与 Go VM 一致(脚本 + 目录形态)
- `bur run`/`bur build` 对所有 example 产出与 Go 一致
- `archive/go-host` 存在且可 checkout 接生
- main 上 Go 源已删,靠 cli + burrt.h + cc 自举

## ② C 后端定案(owner 2026-07-04,动工前已逐条对齐)

1. **翻译目标 = bytecode→C**:复用已验证前端 + 字节码语义,忠实 by construction(opcode 已焊死 trap/GC/调度语义,C 顺手翻译就带着);慢由「编译期栈调度(栈槽→C 局部)+ 类型 pass」后补。不选 AST→C(委托语义给 C 的风险)
2. **Value = tagged-union struct**:照搬 Go Value(tag + i64/f64/bool/ptr),正确性 > 性能;不选 NaN-boxing
3. **内存 = shadow stack 精确 GC**:值平时在 C 局部(快),跨分配需存活的显式压根栈,GC 只扫根栈(学 Go「精确」信仰)。不选 Boehm 保守(Go 主动抛弃的)、不选全显式值栈(退回解释器速度)。**C 目标下没法照抄 Go 的 stack map(gcc 藏了栈/寄存器布局),shadow stack 是「精确 + 自包含」的唯一路**;与现有 native f.push/f.pop rooting、字节码栈验证器分析一脉相承
4. **并发 = 全语言含并发,顺序核 + 并发核同批交付**(不分先后):确定性协作调度器在 C 重新宿主 + ucontext 换栈,**绝不用 OS 线程**(引入非确定交织=破坏 parity)。GOALS 双后端一致在 ② 收尾对整门语言成立,不留缺口。代价:③ 自举验证要等 ② 整体做完
5. **parity 判定 = stdout 逐字节 + 退出码**:trap/native/诊断**文本**在契约外(各后端自行格式化——Rust 运行时 panic 本就朴素、且明确不当稳定契约);静态诊断因共用前端自动一致。诊断**质量**是另一根轴,投前端 backlog,不塞 parity
6. **CLI = 默认内部调 cc 出二进制**:`bur build foo.bur -o foo`(内部 shell cc);`bur build --emit c` dump .c;cc 选择 honor `$CC` 再找 cc/gcc/clang;无 cc 优雅降级(报错 + 提示用 `bur run`)。`bur run`/`bur <file>` 保持 VM 不变
   - **cc 依赖的诚实边界**:「经由 C」本质=吐 C + 调 cc,**无 cc 出不了二进制**(build 期依赖,非 run 期;编好的二进制可拷到无 cc 机器跑)。VM 始终是零 C 依赖兜底。这是站到 Rust 侧(依赖系统工具链),**放弃了 Go 式自包含**;要 Go 级零工具链得自写原生后端(直出机器码 + 链接器),远超 ② 的独立路线,② 不碰

### 兜底待议(owner 2026-07-04「有个兜底的吧」,方案未定,动手前对齐)

- **关键澄清**:*用* bur **从不需要装 gcc**——`bur run`/`bur <file>`(VM)零 C 依赖,这已是内置兜底。只有想要「独立原生二进制」才需要 cc。「必须装 gcc」的担忧对*执行*不成立
- **反直觉点**:内嵌 tcc 只能覆盖 linux-x86-64 一个架构,**恰恰砸了 C 后端「靠目标平台 cc 换可移植」的立身之本**;别的架构照样落回系统 cc。所以 tcc-bundle 是单平台便利,不是通用自包含
- **候选**:(a) 只把 tcc 加进 cc 搜索序(用户装了才用,等于没兜底);(b) vendor 预编译 tcc 二进制 + 其 lib/include,go:embed,无系统 cc 时解压调用(LGPL、~1MB/平台、单架构、要验 tcc 能编 burrt.h/ucontext);(c) 维持现状=VM 兜底 + 无 cc 时优雅降级。**owner 定方案再动**

## 已实现批次的定案存档(rationale;实现细节看 git log/code)

- **模块系统**(2026-07-03):Go 式 `import "path"`(别名 `import m "path"`,`pkg.name` 访问)+ Go 式目录作用域(同包共享顶层、pub 只管跨包、子目录另起包);manifest=bur.mod(`module`/`require` 行格式);双形态(脚本保留顶层语句;包须 `fn main()`,包文件顶层只声明、let 限常量);诊断挂 `Diag.File`。MVS + 网络拉取 + proxy 缓存留后续(require 已解析未消费)
- **v3 语言特性**(2026-07-03):map 走**纯函数 API**,**不开 `m[k]` 糖**(零标注 HM 下 `容器[键]` 在未标注参数里分不清 list/map,糖只在类型静态可见处生效=「顶层能写、函数里不能写」陷阱);`[]` 恒 list-only(map 用 get、string 用 char_at);键约束用受约束类型变量 `key`=int\|str。channel/select 语义见 git log(close 重复/向已关闭发送 trap;`recv`→Option、`for v in ch` 排空自然结束;select 声明序优先 + 可选 default;接收方 park=倒带 ip 重试)
- **stdlib 错误约定**(2026-07-03):fs/exec 走 `Result<T,str>`(失败 `Err(msg)`);exec 用内建 `Output(int,str,str)` enum(语言无元组/记录),spawn 失败才 Err、非零退出仍 Ok(code 带出)
- **argv/exit**(2026-07-03):`exit:fn(int)->a`(发散/bottom,支持 `match { Err(_)=>exit(1) }` 取值-否则退出;唯一「谎报返回类型」native,健全因到不了返回点);`args()` 只给用户参数(脚本/目录路径之后,≈`flag.Args()`,空则 `[]`)。走 exitRequest 哨兵(native 错误处 + main 两处特判 → os.Exit)

## Burryn 写码 quirk 手册(S1 全程实测;写新 .bur 前必读)

语言/前端的真实手感,全部**绕过即可、别为此改 Go**(是否吸收为语言改进由 owner 定):

1. 字符串转义只有 `\n \t \" \\`;要 `\r` 用 `chr(13)`
2. 枚举字段类型在**注册时立即校验**,文件按**字母序**处理 → 跨文件枚举引用只能"向前"(对策:被引用的枚举放排序靠前的文件;burc 的 defs.bur/ast.bur 即为此)
3. 类型层 `len` 是 list-only;字符串长度用 `str_len`(运行时 len 兼容,但过不了 checker)
4. `?` 在**相互递归函数组内不可用**(被调方类型还是未决 TV;零标注下 `?` 必须当场辨别 Option/Result)→ bounce 惯用法:`let x = match f(..) { Ok(v) => v, Err(d) => { return Err(d); 占位值 } }`;尾位置直接透传免 bounce
5. match 臂里 `{ return x }` 块是 unit 型**不发散**;值位置臂须在 return 后补占位值(return 后死代码合法)
6. match 臂体必须是**表达式**;赋值语句要包块:`X => { a = b },`
7. `unused_must_use` 是 **error** 不是 warning
8. 嵌套命名 fn **可自递归、不可相互递归**(顺序作用域);包顶层 fn 可相互递归;相互递归的**闭包**用 mut fn 单元打结:先 `let mut f_ref = fn(..) { 占位 }`,真身定义后 `f_ref = f`
9. **包级值也按文件字母序推导**:跨文件使用的多态函数必须定义在排序靠前的文件,否则被首个使用钉成单态(perr 放 ast.bur 的原因)
10. 无记录类型的两种"结构体"替代:**不可变**用单变体枚举(`Enum.Variant` 限定可与枚举同名;match 解构取字段);**可变**用平行 mut 列表 arena + int 句柄(mut **不穿透**枚举字段——match 绑定不是 mut)。大状态机器(checker/compiler)用「巨型函数 + 闭包捕获 mut 局部」架构,闭包可读写捕获的 mut 变量与容器

## 现行约定(写新代码遵守)

- 错误码:**E0xxx**=checker+模块(E0391 循环 import / E0432 import·bur.mod / E0433 找不到包 / E0603 私有 / E0601·E0580 main 缺失·带参 / E0801 包顶层语句 / E0802 非常量顶层 let)、**E1xxx**=lex/parse、**E2xxx**=compile;runtime 无码
- exit code:0 成功 / 1 静态错 / 2 用法错 / 3 读文件失败(含 bur.mod 缺失)/ 4 runtime trap
- verify.go 失败=编译器 bug,panic `"internal error"`;死锁错误纯文本;包级全局名 `<importpath>.<name>`(枚举编译期 key 同形)

## 诊断深度 backlog(零进度,排 v3 之后)

多 span 标注(Diag 现单 Span)、结构化修复建议(Help 现纯 string→需 span+替换文本)、`--explain`、JSON/彩色输出、compile 首错即停、跨行表达式下划线截断首行。**最值得先补:多 span + 结构化建议**

## 命令

```bash
go test .              # 全部测试(~3min:TestBurcSelfHost + TestBurcSelfHostRun 各含 cc -O2 编整个 burc)
go test -run 'TestBurc[^S]' .        # 只跑自举 parity(~20s,跳过两个 cc 定点)
go test -run TestExamples -update .  # 重新生成 examples/*.golden
go build -o bur.exe .  # 构建(bur.exe 已 gitignore)
go vet ./...           # 静态检查

# 自举编译器 burc(跑在 VM 上;parity 用分段 dump 命令)
bur run burc <lex|parse|check|dis|emit-c> <file.bur>
bur run burc build-dir burc          # burc 把自己整包编成 C(打到 stdout)
bur run burc run <file.bur> [argv..] # S2:burc 自带 VM 解释脚本(双层解释)
bur run burc run-dir <dir> [argv..]  # S2:同上,模块目录形态
```
