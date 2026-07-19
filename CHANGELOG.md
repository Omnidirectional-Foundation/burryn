# Changelog

All notable changes to this project are documented here.
本文件记录项目的所有重要变更。

Versions use a 2-day date range. Latest first.
版本号采用 2 天日期区间，最新在前。

## v0.3 (2026-07-19 ~ 07-20)

**Landed S7.5 compile-time constants** — `const` declarations fold supported
expressions before type checking in scripts, blocks, and packages.
**落地 S7.5 编译期常量** — `const` 声明在脚本、block 与 package 中于类型检查前
折叠受支持的初始化式。

- **Added:** Constant folding for literals, constant references, arithmetic,
  comparisons, boolean operators, and string concatenation; package constants
  support forward references across files and exported references across
  packages.
- **Added:** E0015 for non-constant initializers, E0080 for fold-time traps,
  and E0391 for constant cycles; short-circuit operators retain their dead
  side's type and shape checks without evaluating dead traps.
- **Changed:** `const` declarations lower to immutable `let` bindings, so
  existing type inference and reassignment diagnostics remain authoritative.
- **Fixed:** VM and C runtime ordered comparisons now compare two integers as
  exact `int64` values instead of converting them to `double`.
- **Added:** Constant examples, formatter fixtures, package and cycle fixtures,
  fold diagnostic fixtures, and test-runner coverage.

**Landed the S7.2 pipe operator** — `x |> f(a)` calls `f(x, a)`; pipes are
lowest-precedence and left-associative, so `x |> f |> g` reads `g(f(x))`.
**落地 S7.2 管道操作符** — `x |> f(a)` 即 `f(x, a)`；`|>` 优先级最低、左结合，`x |> f |> g` 即 `g(f(x))`。

- **Added:** `|>` lexing, a dedicated pipe-target grammar (`f`, `pkg.f`,
  optional arguments), and a pre-checker lowering pass from `Pipe` nodes to
  plain calls.
- **Added:** Formatter rendering that keeps `|>` chains and explicit empty
  parentheses intact across `bur fmt`.
- **Added:** Pipe examples plus parser, module, and formatter regression
  fixtures.

**Landed S7.3 match guards** — a match arm can add `if <bool>` after its
pattern, with pattern bindings visible to the guard.
**落地 S7.3 match guard** — match 臂可在 pattern 后添加 `if <bool>`，guard 可访问 pattern 绑定。

- **Added:** Guard parsing, type checking, formatting, AST dumps, and bytecode
  generation for `pattern if guard => body` arms.
- **Changed:** Guarded arms no longer count toward exhaustiveness because their
  condition may reject an otherwise matching value.
- **Added:** VM/native parity examples plus guard type, parser, and
  exhaustiveness regression fixtures.

**Landed S7.1 string interpolation** — strings can splice `str` expressions
with `{expr}` and escape literal opening braces as `{{`.
**落地 S7.1 字符串插值** — 字符串可用 `{expr}` 拼接 `str` 表达式，字面左花括号写成 `{{`。

- **Added:** Lexer mode switching for interpolation segments, nested brace
  balancing, and parser lowering to the existing string-concatenation AST.
- **Changed:** Non-`str` interpolation expressions now produce a compile error
  whose help suggests an explicit `str()` conversion.
- **Added:** Formatter reconstruction, interpolation examples, and malformed
  interpolation regression fixtures.

**S6 (ecosystem toolchain) is complete** — this version closes every S6
work package: dependency management with a disk interface cache,
sub-package testing, embedded std, a rebased bootstrap seed, and the
diagnostics/DX batch.
**S6（生态工具链）全部收尾** — 本版本关闭 S6 全部工作包：带磁盘接口缓存的
依赖管理、子包测试、内嵌 std、重新定基的自举种子、诊断/DX 批。

**Landed the S6.5 diagnostics/DX batch** — native debugging, runtime
stack traces, and human-readable diagnostics.
**落地 S6.5 诊断/DX 批** — 原生调试、runtime stack trace、人类可读诊断。

- The C backend now emits `#line <n> "<file>"` before every function
  header and every instruction, and `bur build` compiles with `-g`:
  `gdb`/`lldb` on a produced binary maps frames straight back to `.bur`
  source lines.
- Runtime traps now print a span stack trace (`  at <fn> (<file>:<line>)`
  per frame) in both the C runtime and the VM, byte-identical across the
  two; frames without a source file are suppressed.
- Public commands (`check`/`run`/`build`) render diagnostics
  rustc-style: `error[CODE]: msg` with `file:line:col`, the source line,
  a caret underline, and `= help:` / `= fix:` trailers. The hidden
  `bur dev` parity dumps keep the old raw format byte-for-byte.
- Added multi-span labels and structured fixes to the diagnostic
  carrier (`DiagX` with `Lab(start, end, label)` and
  `Fix(start, end, replacement, desc)`): duplicate definitions point at
  the first declaration; unused variables suggest the `_` prefix.
- Module-loader diagnostics (e.g. `unused_import`) now reach the public
  commands; previously they were silently dropped.
- CI's fixpoint judgment moved from `gen1 == gen2` to `gen2 == gen3`:
  gen1 is built by the frozen base compiler, so a cgen emission change
  legitimately alters it; gen2 and gen3 embed the same current compiler
  and must emit identical C.

**Rebased the bootstrap seed** — the rebirth chain is now three stages.
**重新定基自举种子** — 重生链改为三段。

- Tagged `seed-base-1` (an internal bootstrap anchor, not a release):
  the archived Go seed builds that commit's burc, which then builds the
  current burc. CI runs the full chain on every push.
- burc's own sources are freed from the three legacy checker
  disciplines (file-order inference, bounce idioms, forward-only enum
  references); the anchored commit keeps them forever.

**Landed S6.1 end to end** — dependency management now spans loading,
interfaces, caching, and tidy.
**落地 S6.1 全链** — 依赖管理贯通加载、接口、缓存与 tidy。

- The module loader follows the `require` closure through `$BURCACHE`,
  with MVS version selection and `bur.sum` tree-hash verification.
- Added the interface pipeline: a deterministic exported-declaration
  renderer, an interface-only parser, and checker consumption of
  bodiless declarations.
- Added the disk interface cache
  (`$BURCACHE/.interfaces/<toolchain>-<tree hash>/`): a warm hit
  replaces a dependency's checker input with its interface; any read,
  parse, or metadata failure falls back to full source silently, and an
  error diagnostic forces a full-source re-run so a stale cache can
  never fabricate errors.
- `bur test` now discovers sub-package tests (shown as `<rel>/<fn>`),
  and `bur mod tidy` adds and removes `require` lines from actual
  imports across the whole module, promoting indirectly-required
  versions picked by MVS.
- CI regenerates `burc/lib/std_embed.bur` and fails on drift.

**Landed S7.8 optional signature annotations early** — plus constrained
type variables.
**提前落地 S7.8 可选签名标注** — 并带受约束类型变量。

- Function parameters and returns accept optional `name: type` /
  `-> type` annotations, reusing the type-expression grammar.
- Type variables can carry constraints (`a:addord`), checked by
  constraint intersection across parser, formatter, and checker.

**Enforced the deep-`mut` flow rule** — the v0.2 plan became an error.
**落实 deep-`mut` 流规则** — v0.2 的计划升级为 error。

- A `mut` binding or `mut` argument must come from a fresh heap source
  (list/map); scalars are exempt; an if/match source is fresh when every
  arm's tail is fresh.

**Landed S6.6 std/json and std/testing** — the first embedded std
members.
**落地 S6.6 std/json 与 std/testing** — 首批内嵌 std 成员。

- `std/json` ships `parse` / `render` / `pretty` / `get` over the
  seven-variant `Json` enum with ordered parallel-list objects.
- `std/testing` ships `assert_eq` / `assert_ok` / `assert_err`, closing
  the S6.4 assertion-sugar debt.

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

**Settled the 2026-07-11 decision batch** — five pending items closed,
unblocking every remaining S6 line.
**敲定 2026-07-11 决策批** — 关闭五个待定项，S6 剩余主线全部解锁。

- Narrowed and adopted the deep-`mut` flow rule as an error: heap-typed
  (list/map) sources only, scalars exempt; an if/match source is fresh
  when every arm's tail is fresh; burc migrates its own violations before
  the rule lands.
- Ratified the S6.2 implementation defaults: `https://<module path>`
  clone URLs with the `$BURGITBASE` override, and `bur mod download`
  verifying bur.sum when present, writing it when absent.
- Settled the std/json API: a seven-variant `Json` enum (`JInt`/`JFloat`
  split; objects as ordered parallel lists), bare `parse`/`render`/
  `pretty` behind the package prefix, sources at `std/json/` with a
  bur.mod, and a checked-in `burc/lib/std_embed.bur` generated by the
  hidden `bur dev embed-std`; `std/testing` lands in the same batch.
- Settled the S7.8 annotation syntax — `name: type` parameters, `-> type`
  returns, reusing the existing type-expression grammar — which unblocks
  the S6.1 interface cache.
- Settled S7.1 interpolation: a non-str value inside `{}` is a compile
  error suggesting an explicit `str()`.

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
