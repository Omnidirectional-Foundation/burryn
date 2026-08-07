# NUMBERING.md — 编号历史映射

> 相关文档：[`SPEC.md`](SPEC.md) · [`../tutorial.md`](../tutorial.md) · [`../README.md`](../README.md)

> 早期多套编号并存、语义交叉，已统一为 SPEC §5 的 `S<n>[.<m>]`；旧编号按映射表回溯。

## 旧 → 新 映射

| 旧编号 | 旧含义 | 新编号 |
|---|---|---|
| v1 | 静态检查完备(HM + 穷尽性 + GC + CSP 基础) | **S1**(细拆 S1.1–S1.4) |
| v2 | C 后端 + 模块系统 + map + stdlib + select/close + mut/pub | **S2**(细拆 S2.1–S2.7) |
| v3 | 编译器完全自举(里程碑级验收) | 横跨 **S3/S4/S5**(见下自举分段) |
| v4 | 手写 x86-64 + PE 后端 + 语法冻结 + grammar | **S8**(S8.1 后端 / S8.2 语法冻结)；重型类型项并入 S8.3/S8.4 |
| 旧 S1(自举分段) | 自举编译器前端 | **S3** |
| 旧 S2(自举分段) | Burryn 重写 VM | **S4** |
| 旧 S3(自举分段) | Burryn 写 CLI + 删 Go | **S5** |
| 旧 S4(生态工具链) | 依赖 / fmt / test / debugger | **S6** |
| S4-1 | 依赖管理 | **S6.1**(解析)+ **S6.2**(网络拉取) |
| S4-2 | `bur fmt` | **S6.3** |
| S4-3 | `bur test` | **S6.4** |
| S4-4 | debugger | **S6.5** |
| L1 | 依赖解析层(无网络) | **S6.1** |
| L2 | 依赖网络拉取层 | **S6.2** |
| L3 | 依赖命令面 | 并入 S6.1/S6.2 落地(不再独立编号) |
| §6.6 轻量特性评估 | 插值/管道/guard/命名参数/常量 | **S7**(S7.1–S7.5) |
| §6.6 封闭 records | 改 unify，原标「独立 milestone」 | **S8.4**(移出 S7，与 row poly 同段) |
| §6.7 Row Polymorphism(v4) | 重型类型系统首选项 | **S8.3** |
| §6.7 Effects / Refinement / GADTs / Linear | 重型类型系统其余项 | 明确排除(见 §6.7) |
