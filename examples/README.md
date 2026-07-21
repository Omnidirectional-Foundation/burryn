# Examples / 样例

每个 `.bur` 文件可用 `bur run <path>` 直接运行。配有 `.golden` 的文件，其输出与
`.golden` 逐字节一致即为通过。无 `.golden` 的 `*_trap.bur` 文件预期中止（exit 4），
手动运行并检查退出码。

Run any `.bur` file with `bur run <path>`. Files with a `.golden` partner produce
byte-identical stdout. Files without one (`*_trap.bur`) are expected to abort
(exit 4) — run manually and check the exit code.

## 建议阅读顺序 / Suggested reading order

1. `basics/hello.bur` — 起点
2. `types/shapes.bur` — 枚举与 match
3. `types/errors.bur` — Result/Option 与 `?`
4. `concurrency/sieve.bur` — CSP 经典
5. `programs/brainfuck.bur` — 完整程序

---

## basics/ — 入门、类型、字符串

| File | Description |
|------|-------------|
| `hello.bur` | println, let, basic expressions |
| `fib.bur` | recursion micro-benchmark |
| `bytes.bur` | UTF-8 byte vs code-point indexing |
| `textproc.bur` | string and list natives: split, trim, join, slice, concat |
| `interpolation.bur` | string interpolation with `{expr}` and `{{` escape |
| `cleanup.bur` | defer: LIFO cleanup, closure capture, `?` early exit |

## types/ — 类型系统、match、枚举

| File | Description |
|------|-------------|
| `shapes.bur` | enums with payloads + exhaustive match |
| `errors.bur` | Result, Option, and the `?` operator |
| `type_inference.bur` | let-polymorphism, higher-order inference, nested generalization |
| `exhaustive.bur` | exhaustiveness: all variants covered, no wildcard needed |
| `constants.bur` | compile-time const folding |
| `match_guard.bur` | match arms with `if` guards |
| `pipeline_op.bur` | the `\|>` pipe operator |

## concurrency/ — CSP、channel、select

| File | Description |
|------|-------------|
| `sieve.bur` | classic CSP prime sieve: one fiber per prime |
| `multiplex.bur` | `select` over several channels |
| `streaming.bur` | channel close + for-in drain + recv Option |
| `pipeline.bur` | buffered-channel producer/consumer |
| `deadlock_trap.bur` | all fibers blocked → fatal deadlock (exit 4, no golden) |
| `chan_send_trap.bur` | send on closed channel → trap (exit 4, no golden) |
| `chan_close_trap.bur` | double-close → trap (exit 4, no golden) |

## net/ — TCP 网络

| File | Description |
|------|-------------|
| `net_loopback.bur` | listener + dialer dual-fiber echo exchange |
| `net_errors.bur` | port-in-use, connection-refused, read-EOF, write-to-closed |
| `net_trap.bur` | net_close on invalid handle → trap (exit 4, no golden) |

## io/ — 文件系统、进程、时间

| File | Description |
|------|-------------|
| `fs.bur` | read_file/write_file: missing path, round-trip, unwritable path |
| `exec.bur` | synchronous exec: Output destructuring, exit codes, missing binary |
| `exec_async.bur` | exec_start + exec_poll: async child processes |
| `sleep.bur` | sleep(0) boundary and sleep(1) continuation |

## programs/ — 完整程序

| File | Description |
|------|-------------|
| `brainfuck.bur` | a Brainfuck interpreter written in Burryn |
| `wordcount.bur` | word frequency counter with maps |
| `gc_stress.bur` | heap churn to exercise the mark-sweep collector |
| `geometry/` | multi-package module: bur.mod, import, pub |
