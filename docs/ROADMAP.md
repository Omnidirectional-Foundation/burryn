# ROADMAP.md — 三轨道 + 包生态并行推进计划

> 状态：执行计划（owner 2026-07-23 定）· 权威约束仍为 GOALS.md
> 四分支并行，各由独立 Qoder 会话推进；合入顺序见 §5

## 1. 总览

| 轨道 | 分支 | 核心目标 | 前置 |
|------|------|---------|------|
| 表达力 | `dev/expressiveness` | records + row poly + 类型别名 → 语法冻结 | 无，最先开工 |
| 后端 | `dev/backend` | x86-64 原生后端（ELF + Mach-O + PE） | byte_chr native；表达力落地后处理新 opcode |
| LSP | `dev/lsp` | `bur lsp` 服务器 + VSCode 薄扩展 | read_stdin native；S8.2 语法冻结后处理新语法 |
| 包生态 | `dev/packages` | stdlib 广度 + 包模板 + 文档工具 | 无，与表达力并行开工 |

总目标：**包生态是重中之重**——表达力让库写起来不别扭，后端让单二进制交付兑现，LSP 让开发体验达标。三者服务于"有人愿意写包、有人找得到包、包能稳定用"。

## 2. 轨道一：表达力（dev/expressiveness）

### 范围

| 子项 | 内容 |
|------|------|
| S8.3 | Row polymorphism：行变量复用 var/generalize，函数参数可写"至少有这些字段" |
| S8.4 | 封闭 records：字面量构造、字段访问、函数式更新、与 mut 交互 |
| S8.7 | 类型别名：`type Name = TypeExpr`（含 `pub`），checker 透明展开 |
| S8.2 | 语法冻结 + grammar 文件（record/row/alias 是最后一批新语法） |

### 设计闸门（动工前须 owner 确认）

- record 表层语法：字面量 `{ x: 1, y: 2 }`、访问 `r.x`、更新 `{ r | x: 3 }`、mut 交互
- row poly 标注语法：`fn f(r: {name: str | rest}) -> ...` 形态
- 类型别名语法：`type Name = TypeExpr` vs `type Name := TypeExpr`
- 新错误码分配
- S8.3/S8.4 同段一次验 fixpoint（GOALS §6.6 已定）

### 触及文件

lexer.bur、parser.bur、types.bur、compiler.bur、cgen.bur、vm.bur、format.bur、testdata/

### 验收

- gen2 == gen3 fixpoint
- VM/native parity（含新 opcode）
- examples golden 全过
- `bur fmt --check burc` 干净
- 三段链（seed-base-3 → 当前）收敛

### 开工前可并行准备

- 设计提案文档（登记 GOALS §7，等 owner 确认）
- 摸底 ty_unify 现有结构（为 row var 插入点定位）

## 3. 轨道二：后端（dev/backend）

### 范围

| 子项 | 内容 |
|------|------|
| 前置 | `byte_chr(n: int) -> str`（0–255 单字节，越界 trap）|
| S8.1a | x86-64 指令编码器（寄存器、寻址模式、Rex 前缀）|
| S8.1b | ELF 输出（Linux）：header、sections、symbols、relocations |
| S8.1c | Mach-O 输出（macOS）：同 System V ABI，换序列化层 |
| S8.5 | PE 输出（Windows）+ Microsoft x64 调用约定 + Windows runtime（Fibers 替换 ucontext）|

### 调用约定

| 平台 | 约定 | 参数寄存器 | 返回值 |
|------|------|-----------|--------|
| Linux/macOS | System V AMD64 | rdi, rsi, rdx, rcx, r8, r9 | rax |
| Windows | Microsoft x64 | rcx, rdx, r8, r9 | rax |

### GC 方案

沿用 C 后端 shadow stack 精确扫描（GOALS §6.7 已定）。手写后端在调用帧中维护根栈，GC 只扫根栈。

### 触及文件

新增：burc/lib/x86.bur（或 x86/ 子目录）、runtime/burrt_x86.h
修改：runtime/burrt_natives.h（byte_chr）、compiler.bur（后端 dispatch）、main.bur（`bur build --backend native`）

### 验收

- 同一程序在 VM / C 后端 / x86 后端输出逐字节一致 + 退出码一致
- 自举：`bur build burc --backend native` 产出的二进制能编译自身
- byte_chr + write_file 能产出正确 ELF（用 `readelf -a` 验证）
- 三段链 fixpoint 不受影响（新后端是加法，不改现有 cgen）

### 开工前可并行准备（不依赖表达力）

- byte_chr native（五处约定）
- x86-64 指令编码器（纯数据变换，不依赖语言特性）
- ELF/Mach-O 格式研究 + 最小 hello-world ELF 手工验证
- System V ABI 文档精读

### 依赖表达力的部分（等 dev/expressiveness 合入后）

- records 的内存布局（字段 → 偏移）
- 新 opcode 的 x86 翻译
- row poly 的运行时表示（统一装箱，应该无新 opcode）

## 4. 轨道三：LSP（dev/lsp）

### 范围

| 子项 | 内容 |
|------|------|
| 前置 | `read_stdin(max: int) -> str`（读至多 max 字节，EOF 返回 ""，fiber 级阻塞）|
| S9.1 | JSON-RPC 2.0 传输（Content-Length 帧）+ 文档同步（Full sync）+ 诊断推送 |
| S9.2 | hover / go-to-def / completion / formatting / signature-help |
| S9.3 | VSCode 薄扩展（TextMate grammar + LSP client 配置 + bundled binary 探测）|
| 配置 | nvim（原生 LSP 配置片段）/ vim（coc.nvim 配置）/ emacs（eglot 配置）|

### 架构

```
编辑器 ←→ LSP client ←→ stdin/stdout ←→ bur lsp ←→ burc/lib/（lexer/parser/checker/formatter）
```

- 服务器 = `bur lsp` 子命令，长驻进程，CSP 并发处理请求
- 所有语言智能在服务器端；编辑器插件是薄客户端
- 不支持 LSP 的编辑器不管

### 核心工程改造

check 管线接受内存源码（现只从磁盘读）。改法：module loader 的 `read_file` 调用点加一层 overlay——LSP 模式下先查内存文档表，miss 再落盘。一个入口点改造，不是到处塞接口。

### 触及文件

新增：burc/lib/lsp.bur（或 lsp/ 子目录）、editors/vscode/（扩展）、editors/nvim/、editors/vim/、editors/emacs/
修改：runtime/burrt_natives.h（read_stdin）、module.bur（内存 overlay）、main.bur（`bur lsp` 命令）

### 验收

- LSP 协议合规（用 lsp-inspector 或 VSCode LSP log 验证）
- 诊断推送：打开含错误的 .bur 文件，编辑器内显示红色波浪线
- hover：悬停变量显示推导类型
- go-to-def：跨文件跳转
- completion：`pkg.` 后弹出成员列表
- formatting：保存时格式化（调 `bur fmt -`）
- VSCode 扩展可安装、可连接服务器

### 开工前可并行准备（不依赖语法冻结）

- read_stdin native（五处约定）
- JSON-RPC 2.0 帧解析器（纯 std/json + 字符串操作）
- check 管线内存 overlay 设计
- TextMate grammar 初版（基于现有 lexer token 集）
- VSCode 扩展脚手架

### 依赖语法冻结的部分（等 S8.2 后）

- 新语法（records、type alias）的 TextMate grammar 更新
- 新 AST 节点的 hover/completion 支持
- grammar 文件冻结后不再追语法债

## 5. 轨道四：包生态（dev/packages）

### 范围

| 子项 | 内容 |
|------|------|
| S10.1 | stdlib 扩展批（见下表）|
| S10.2 | 包模板 + 示例包（`bur mod init` 脚手架增强）|
| S10.3 | `bur doc`（从导出签名 + 注释生成 Markdown 文档）|
| S10.4 | 包质量基础设施（CI 模板、测试约定、版本规范文档）|

### stdlib 扩展优先级

| 包 | 实现方式 | 备注 |
|---|---------|------|
| std/encoding（base64, hex, url） | 纯 Burryn | 零新 native |
| std/path | 纯 Burryn | 字符串操作 |
| std/cli | 纯 Burryn | 复用 args() |
| std/log | 纯 Burryn | 复用 eprintln + clock |
| std/datetime | 1 新 native（`time_now() -> int` 秒级 UTC）| 格式化/解析纯 Burryn |
| std/regex | 纯 Burryn | NFA 实现，性能后续优化 |
| std/crypto（sha256, hmac） | 纯 Burryn 或 1 native | 纯 Burryn 可行但慢；native 可选 |
| std/http | 纯 Burryn | 复用 net natives（tcp_listen/dial/read/write）|

原则：能纯 Burryn 就不加 native。每个 std 包带 `bur.mod` + `*_test.bur`，随 std_embed 分发。

### 触及文件

新增：std/encoding/、std/path/、std/cli/、std/log/、std/datetime/、std/regex/、std/crypto/、std/http/
修改：runtime/burrt_natives.h（time_now，可能 sha256）、burc/lib/std_embed.bur（regen）

### 验收

- 每个 std 包 `bur test` 全过
- `bur dev embed-std .` regen 后 `git diff --exit-code` 干净
- 示例包可被 `bur get` 拉取（本地 git 仓 + BURGITBASE 测试）
- `bur doc std/json` 输出可读 Markdown

### 开工前可并行准备

- 全部纯 Burryn 包（encoding、path、cli、log、regex）不依赖任何前置
- std/http 依赖 S7.7 net natives（已就绪）
- 包模板设计

## 6. 合入顺序与冲突管理

### 合入顺序（PR 进 main）

1. **dev/expressiveness**（改核心文件最多，先合减少后续 rebase 量）
2. **dev/packages**（主要加文件，冲突最小）
3. **dev/backend**（加文件为主，compiler.bur dispatch 小改）
4. **dev/lsp**（加文件为主，module.bur overlay 小改）

### 共享文件冲突点

| 文件 | 触及轨道 | 冲突风险 |
|------|---------|---------|
| runtime/burrt_natives.h | 后端(byte_chr)、LSP(read_stdin)、包(time_now) | 低：各自加独立函数 + 注册行 |
| compiler.bur | 表达力(新 opcode)、后端(dispatch) | 中：后端等表达力合入后 rebase |
| module.bur | LSP(overlay) | 低：只 LSP 改 |
| main.bur | 后端(--backend)、LSP(bur lsp) | 低：各加独立命令分支 |
| types.bur | 表达力(records/row)、包(native decl) | 中：包等表达力合入后 rebase |
| burc/lib/std_embed.bur | 包(regen) | 低：只包轨道改，CI 有 regen+cmp |

### 并行规则

- 各轨道只改自己 §2–5 列出的文件；需要改共享文件时先通知 owner
- 每轨道独立跑验证协议（check + fixpoint + golden + fmt）
- 新 native 走五处约定，加完跑 fixpoint
- 表达力轨道的设计闸门须 owner 确认后才动工实现；其余三轨道的前置小批（native、编码器、JSON-RPC）可立即开工

## 7. 各轨道第一步（会话启动即可执行）

| 轨道 | 第一步 | 阻塞？ |
|------|--------|--------|
| 表达力 | 写 S8.3/S8.4/S8.7 设计提案 → 登记 GOALS §7 → 等 owner | 是（设计闸门）|
| 后端 | byte_chr native（五处）+ x86 指令编码器骨架 | 否 |
| LSP | read_stdin native（五处）+ JSON-RPC 帧解析器 | 否 |
| 包 | std/encoding + std/path + std/cli（纯 Burryn，零前置）| 否 |
