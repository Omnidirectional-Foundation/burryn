# ROADMAP.md — 顺序推进计划（S9–S11）

> 状态：执行计划（owner 2026-07-23 定）· 权威约束仍为 GOALS.md
> 四轨道并行阶段已完成，全部合入 main。以下为单对话顺序推进。

## 1. 已完成（S8 四轨道，2026-07-23 合入）

| 轨道 | 内容 | 状态 |
|------|------|------|
| 表达力 | records + row poly + 类型别名 | done |
| 包生态 | std/encoding + std/path + std/cli + byte_chr | done |
| 后端 | x86-64 指令编码器 + ELF64 序列化 | done（骨架）|
| LSP | JSON-RPC 传输 + 诊断 + overlay + VSCode 扩展 | done（S9.1 核心）|

## 2. 推进模式

- **单对话顺序推进**：一个 Qoder 会话做完 LSP 再做后端，不并行
- **远端 CI 编译**：本地不编译，push 后轮询 GitHub Actions 结果
- **CI 轮询循环**：`gh run watch` 或 30s sleep 循环，CI 绿了继续推进
- **新对话接续**：一个会话的上下文用完时，开新对话继续，handoff 文件交接

### CI 架构（待改造）

| 任务 | 触发 | 耗时 | 内容 |
|------|------|------|------|
| quick-build | `dev/**` push | ~5 min | 缓存 bur-base → gen1 → gen2 → 跑测试 |
| bootstrap-fixpoint | main push / PR | ~20 min | 完整五步链 gen2==gen3 |

改造项：触发条件加 `dev/**`、bur-base 按 SEED_BASE_TAG 缓存、quick-build job。

## 3. 第一阶段：LSP 完善（优先）

### 3.1 module pipeline 诊断（最近）

当前 LSP 只诊断单文件。需要：
- rootUri → 绝对路径解析
- 模块级诊断：打开的文件触发整个模块的 check pipeline
- 跨文件错误归因（错误报在正确的文件上）

触及：lsp.bur、module.bur、main.bur

### 3.2 增量同步

Full sync → Incremental sync，大文件性能。

触及：lsp.bur

### 3.3 语言智能（远期）

- hover：悬停显示推导类型
- go-to-def：跨文件跳转
- completion：`pkg.` 成员列表
- formatting：保存时调 `bur fmt -`
- signature-help

触及：lsp.bur、types.bur（查询接口）

### 验收

- LSP 协议合规（lsp-inspector 或 VSCode LSP log）
- 打开含错误的 .bur 文件，编辑器显示红色波浪线（含跨文件）
- hover 显示推导类型
- VSCode 扩展可安装、可连接

## 4. 第二阶段：x86 原生后端

### 4.1 bytecode → x86 翻译器（核心大工程）

- VM 栈式指令 → x86 寄存器式翻译
- 栈帧布局设计
- System V AMD64 调用约定映射（rdi, rsi, rdx, rcx, r8, r9）
- GC 根栈（shadow stack 精确扫描）
- ot_record 字段偏移访问（base+disp 寻址）

### 4.2 CLI 集成

`bur build --backend=x86` 直接产出 ELF，不经过 C。

### 4.3 自举验证

`bur build burc --backend native` 产出的二进制能编译自身。

### 触及文件

修改：x86.bur（翻译器）、elf.bur（集成）、compiler.bur（后端 dispatch）、main.bur（CLI）
新增：可能 x86/ 子目录

### 验收

- 同一程序 VM / C / x86 输出逐字节一致 + 退出码一致
- 自举 fixpoint
- `readelf -a` 验证 ELF 结构

## 5. 第三阶段：包生态扩展

| 包 | 实现方式 | 备注 |
|---|---------|------|
| std/log | 纯 Burryn | 复用 eprintln + clock |
| std/datetime | 1 新 native（time_now）| 格式化纯 Burryn |
| std/regex | 纯 Burryn | NFA 实现 |
| std/crypto | 纯 Burryn 或 1 native | sha256, hmac |
| std/http | 纯 Burryn | 复用 net natives |

原则：能纯 Burryn 就不加 native。每个 std 包带 bur.mod + *_test.bur。

## 6. 合入策略

- 不再用 dev/* 并行分支
- 单对话在 main 上直接推进，或短生命周期 feature 分支
- 每次 push 触发 CI，绿了继续
- 大改动（x86 翻译器）可用 feature 分支隔离
