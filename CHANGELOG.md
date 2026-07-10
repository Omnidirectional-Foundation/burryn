# Changelog

All notable changes to this project are documented here.
本文件记录项目的所有重要变更。

Versions use a 2-day date range. Latest first.
版本号采用 2 天日期区间，最新在前。

## v0.2 (2026-07-10 ~ 07-11)

**Recorded the 2026-07-10 design-review decisions in `docs/GOALS.md`** — a
full audit of settled decisions, with corrections where the doc had drifted
from the implementation.
**在 `docs/GOALS.md` 记录 2026-07-10 设计审查定案** — 对已定决策的全面
审计，并纠正文档与实现漂移之处。

- Narrowed the determinism promise: pure-compute programs stay byte-for-byte
  deterministic across backends; IO-concurrent programs no longer promise
  scheduling order. Added the opt-in `BUR_DETERMINISTIC=1` mode (IO
  serialized) as the future `bur test` default.
- Downgraded deep `mut` from a value-level guarantee to a binding-level
  discipline (aliasing can bypass it), and added a planned checker flow rule
  for `mut` argument sources.
- Corrected S2.7: json and net were never implemented; json moves to new
  S6.6 (bundled `std/`), net to new S7.7.
- Added S6.7 (runtime IO work package: `sleep`, `exec_start`/`exec_poll`,
  scheduler idle-wait, deterministic mode) and S6.8 (checker-debt batch: SCC
  inference order, two-pass enum registration, `?` in mutual recursion).
- Rejected S7.4 named arguments + defaults; slotted S7.6 `defer`
  (block-scope leaning), S7.7 net, S7.8 optional signature annotations.
- Reordered S8: types first (S8.3 row poly, S8.4 records), then the
  hand-written x86-64 ELF backend; split PE into S8.5 with its Windows
  runtime prerequisite spelled out.
- Fixed the stale native-GC line (shadow-stack precise GC, not conservative
  stack scanning) and hardened the `bur fmt` acceptance rules to three:
  idempotent, AST-invariant (enforced via reparse + `ast_eq`), and no
  comment loss.
- Decided std distribution (bundled with the toolchain, reserved `std/`
  prefix) and the `bur.sum` line format
  `<path> <version> h1:<base64(tree hash)>`.

**Landed S6.7 runtime IO and the complete `bur fmt` (S6.3)** — the same
window's implementation batch.
**落地 S6.7 runtime IO 与完整的 `bur fmt`（S6.3）** — 同窗口的实现批次。

- Added the `sleep(ms)` native with timer-aware scheduling in both the C
  runtime and the VM: idle schedulers sleep to the nearest deadline
  instead of spinning or deadlocking.
- Made `exec` fiber-blocking instead of scheduler-blocking, and added the
  `exec_start`/`exec_poll` natives (two `exec sleep 0.5` fibers now finish
  in 0.5s, previously 1.0s); `BUR_DETERMINISTIC=1` serializes children for
  reproducible runs.
- Finished the formatter: full AST coverage, comment reinsertion from
  lexer trivia, and a verifier that rejects output unless it reparses to a
  structurally equal AST (`ast_eq`) with every comment intact.
- Added the public `bur fmt <file|dir|->` command with `--check` and stdin
  modes, and formatted the whole `burc/` tree with it once.
- Gave `EnumVariantDecl` a real span (was `Sp(0, 0)`).
- Added `burc/lib/modgraph.bur`: offline S6.1 groundwork — bur.mod
  parsing, semver ordering, MVS over `$BURCACHE`, canonical tree hashes,
  bur.sum rendering/checking, and the hidden `bur dev mod-graph` command.

## v0.1 (2026-07-05 ~ 07-06)

**Initial community scaffolding added** — set up the open-source contribution
infrastructure and rounded out the README.

- Added `CONTRIBUTING.md`, `SECURITY.md`, and `CODE_OF_CONDUCT.md`, adapted to
  the `main`-only workflow and the bootstrap-fixpoint verification rule.
- Added Apache-2.0 and self-hosted badges plus a "License & Disclaimer" section
  to `README.md`.

<!--
模板 / Template for future entries:

## vX.Y (YYYY-MM-DD ~ MM-DD)

**Lead-in naming what changed** — short expansion.

- Detail line. Quote key conventions (rules, commands, field names) verbatim.
-->
