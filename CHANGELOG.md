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
- Settled the remaining S6 design questions: interface files in the
  future optional-annotation syntax as the module cache (key: toolchain
  version + tree hash), subprocess isolation for `bur test`, std embedded
  into the binary, and the S6 order `S6.8 -> S6.2 -> S6.6 -> S6.4 ->
  S6.1 wiring`.
- Settled the S6 CLI layout after Go's vocabulary: `bur mod
  init/tidy/download/verify`, `bur get <path>@<version>`, and
  `bur test [dir] [--run <substr>] [-v]`; the CLI-naming item leaves the
  pending list.

**Landed S6.8, the checker-debt batch** — the checker sheds its
file-order semantics.
**落地 S6.8 checker 债批** — checker 摆脱文件字母序语义。

- Enum registration is now two passes per package (collect every name,
  then validate field types), so enum fields may reference enums from any
  file in any order.
- Package-level fns are inferred in SCC dependency order (Tarjan over a
  scope-aware free-name scan): a fn defined in a later file — and a self-
  or mutually-recursive fn — now stays polymorphic at every use site.
- `?` now works inside mutually-recursive fn groups: an operand whose
  type is still unresolved defers the Option/Result decision to the end
  of the inference group; E0277 is reported only if it never resolves.
- Surveyed the burc tree for the planned deep-`mut` flow rule (GOALS §2):
  32 violating sites out of ~1,230 checked; adopting or narrowing the
  rule stays an owner decision.
- burc's own sources keep the old file-order discipline: CI rebuilds the
  chain from the archived Go seed, whose checker still infers in file
  order.

**Landed S6.2 network fetch with the `bur mod` and `bur get` commands** —
dependency management is now end to end: require, fetch, lock, verify.
**落地 S6.2 网络拉取与 `bur mod` / `bur get` 命令** — 依赖管理全链打通：
require、拉取、锁定、校验。

- Added `mod_fetch`: a shallow `git clone` of the `v<semver>` tag into
  `$BURCACHE`, with `.git` stripped before the tree enters the cache; a
  missing tag reports the offending `require` line. Clone URLs default to
  `https://<module path>`; `$BURGITBASE` overrides the prefix.
- Wired `bur mod init <path>`, `bur mod tidy [dir]`, `bur mod download
  [dir]`, `bur mod verify [dir]`, and `bur get <path>@<version>` (which
  restores the previous bur.mod if the fetch fails).
- Corrected the tree-hash encoding from hex to the settled
  `h1:<base64(sha256)>` format.

**Landed S6.4 `bur test` with subprocess isolation** — the first
first-class test runner; S6.6 std/json waits on an owner API decision, so
S6.4 landed first.
**落地 S6.4 `bur test`（子进程隔离）** — 首个一等测试跑器；S6.6 std/json
卡在 owner 的 API 决策上，故 S6.4 先行。

- Discovers zero-parameter `fn test_*` in the root package's `*_test.bur`
  files; those files are now excluded from every normal build, run, and
  check.
- Runs each test as its own subprocess (hidden `bur dev run-test`) with
  `BUR_DETERMINISTIC=1`; traps and deadlocks (exit 4) count as failures.
- Supports `--run <substr>` filtering and `-v`; main-less library packages
  are testable via a synthetic entry point.
- Corrected the self-path detail: a child's `/proc/self/exe` names the
  child, so the binary resolves itself via `sh -c "readlink
  /proc/$PPID/exe"`.

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
