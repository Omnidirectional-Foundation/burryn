# Security Policy / 安全策略

You may report in **English or Chinese** — either is fine.

你可以使用**英文或中文**报告，任选其一。

## Reporting a vulnerability / 报告漏洞

**Please do NOT open a public issue for security vulnerabilities.**

**请勿为安全漏洞开公开 issue。**

Instead, use GitHub's private vulnerability reporting:

请改用 GitHub 的私密漏洞报告功能：

1. Go to the **Security** tab of this repository. / 进入本仓库的 **Security** 标签页。
2. Click **Report a vulnerability**. / 点击 **Report a vulnerability**。

This keeps the report private until a fix is available.

这样在修复发布前，报告将保持私密。

> Note: this is a hobby project maintained in spare time. While security reports
> are taken seriously, **response time is not guaranteed**.
>
> 注意：这是一个利用业余时间维护的兴趣项目。安全报告会被认真对待，但
> **不承诺响应时间**。

## Scope / 范围

Burryn is a self-hosted compiler and VM. Security-sensitive areas include, but
are not limited to:

Burryn 是一门自举编译器与虚拟机。安全敏感面包括但不限于：

- **C backend** — code generation and the emitted `program.c` compiled by `cc`.

  C 后端 — 代码生成与交给 `cc` 编译的 `program.c`。
- **VM** — bytecode execution, the mark-sweep GC, and the green-thread scheduler.

  虚拟机 — 字节码执行、mark-sweep GC 与绿色线程调度。
- **`exec` and process spawning** — `fork` + `execvp`, which do not go through a shell.

  `exec` 与进程创建 — `fork` + `execvp`，不经过 shell。
- **Dependency fetching** — `git clone` plus `sha256sum` verification of pulled sources.

  依赖拉取 — `git clone` 与对拉取源码的 `sha256sum` 校验。

## Out of scope / 不在范围内

- Vulnerabilities in third-party dependencies (report upstream).

  第三方依赖的漏洞（请向上游报告）。
- Issues requiring physical access to the host. / 需要物理访问主机的问题。
- Compiling or running untrusted `.bur` source you did not audit.

  编译或运行你未审计的不可信 `.bur` 源码。

## Disclaimer / 免责声明

This software is provided "as is", without warranty of any kind. Any commercial
entity using this software is solely responsible for their own compliance with
applicable laws and regulations, including the EU Cyber Resilience Act (CRA).

本软件按"原样"提供，不附带任何形式的担保。任何使用本软件的商业实体，须自行负责
遵守适用的法律法规，包括欧盟《网络弹性法案》(CRA)。
