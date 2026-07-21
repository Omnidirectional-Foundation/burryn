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

## Testing / 测试

### Running tests / 运行测试

```sh
bur test ./...              # run all *_test.bur files (subprocess-isolated)
bur test std/json           # run tests in a specific package
bur test ./... --run parse  # filter by substring
bur test ./... -v           # verbose: print each test name
```

Each `test_*` function with zero parameters in a `*_test.bur` file is a test.
Tests run in separate subprocesses; traps and deadlocks count as failures.

每个 `*_test.bur` 文件中零参数的 `test_*` 函数即为一个测试。
测试在独立子进程中运行；trap 和死锁视为失败。

### Golden examples / Golden 样例

`examples/` contains runnable programs paired with `.golden` files (frozen
expected stdout). Verify after compiler changes:

`examples/` 下的可运行程序配有 `.golden` 文件（冻结的期望 stdout）。
编译器改动后验证：

```sh
bur run examples/basics/hello.bur | diff - examples/basics/hello.golden
```

Examples are organized by topic: `basics/`, `types/`, `concurrency/`, `net/`,
`io/`, `programs/`. Files without a `.golden` (e.g. `*_trap.bur`) are expected
to abort — run manually and check the exit code.

样例按主题分目录。无 `.golden` 的文件（如 `*_trap.bur`）预期中止——手动运行并检查退出码。

### testdata/ layout / testdata/ 目录结构

| Directory | Purpose / 用途 |
|-----------|----------------|
| `testdata/check/` | Type-checker error fixtures (expect diagnostics) |
| `testdata/parse/` | Parser error and AST fixtures |
| `testdata/compile/` | Codegen error fixtures |
| `testdata/fmt/` | Formatter pairs: `.bur` (messy input) → `.golden` (formatted) |
| `testdata/pkg/` | Multi-package module fixtures (import, pub, MVS) |
| `testdata/modcache/` | Committed module cache for offline dependency tests |

### Verification protocol (simplified) / 验证协议（简化版）

Before submitting a PR that touches the compiler:

提交触及编译器的 PR 前：

1. `bur check burc` — no type errors in the compiler itself
2. `bur test ./...` — all tests pass
3. Golden examples: `bur run` each and diff against `.golden`
4. `bur fmt --check burc` — compiler source is formatted
5. Bootstrap fixpoint (see above) if the change affects codegen or types

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
