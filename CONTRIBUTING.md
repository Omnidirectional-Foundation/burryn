# Contributing to Burryn / 贡献指南

Issues, PRs, and comments may be written in **English or Chinese** — either is fine.

issue、PR 和评论都可以使用**英文或中文**，任选其一。

---

## Before you start / 开始之前

> This is a hobby project maintained in spare time. Quality issues and PRs are
> welcome, but **response is not guaranteed** — review may be slow or may not
> happen at all. Factor this in before investing significant effort.
>
> 这是一个出于个人兴趣、利用业余时间维护的开源项目。欢迎高质量的 issue 和 PR，
> 但**不承诺响应时间** — 审阅可能很慢，也可能不进行。在投入大量精力前先评估。

---

## Branching / 分支策略

- This project uses **`main` only**. There are no long-lived feature branches.

  本项目**只用 `main` 分支**，没有长期存在的功能分支。
- Open PRs against `main`. / 请以 `main` 为目标发起 PR。

## Commits / 提交规范

This project follows [Conventional Commits](https://www.conventionalcommits.org/).

本项目遵循 Conventional Commits 规范。

```markdown
<type>(<scope>): <subject>
```

- Types: `feat` / `fix` / `refactor` / `docs` / `chore` / `test`
- Use the imperative mood; no trailing period, no emoji.

  使用祈使句；结尾不加句号，不用 emoji。
- Scope examples: `lexer`, `parser`, `checker`, `cgen`, `vm`, `docs`.

Example / 示例:

```markdown
feat(lexer): collect comments as out-of-band trivia
fix(cgen): access enum fields by integer index
docs(GOALS): add S4 ecosystem toolchain roadmap
```

## Bootstrap fixpoint / 自举定点

Burryn's compiler is self-hosted. Any change that touches `ty_unify`, token
numbering, or the bootstrap chain **must** be verified by rebuilding until
`gen1` and `gen2` are byte-for-byte identical.

Burryn 的编译器是自举的。任何触及 `ty_unify`、token 编号或自举链的改动，
**必须**重新构建直到 `gen1` 与 `gen2` 逐字节一致，方可提交。

## Code style / 代码风格

- Keep CLI output and error messages untranslated (original language).

  CLI 输出与错误信息保持原文，不翻译。
- Keep technical terms in English (lexer, arena, fixpoint, …).

  技术术语保留英文。

## Security issues / 安全问题

Do **not** open public issues for security vulnerabilities. See
[`SECURITY.md`](SECURITY.md) for private disclosure.

请**不要**为安全漏洞开公开 issue，私下披露方式见 [`SECURITY.md`](SECURITY.md)。

## Pull request flow / PR 流程

1. Branch from `main`. / 从 `main` 分出分支。
2. Make changes with clear, conventional commits. / 用规范的 commit 提交改动。
3. If the compiler was changed, verify the bootstrap fixpoint. / 若改动了编译器，验证自举定点。
4. Open a PR against `main`. / 向 `main` 开 PR。
5. Be patient — see the maintenance notice in the README. / 请耐心等待，参见 README 中的维护说明。
