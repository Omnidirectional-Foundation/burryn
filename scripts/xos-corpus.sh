#!/usr/bin/env bash
# xos-corpus.sh — x86 跨目标语料：以 Linux x86 运行结果为基准，构建同一批样例的
# windows/darwin 版本，供真 Windows/macOS runner 逐例对照 stdout 与退出码。
# 语料 = examples 各分类 + testdata/{basics,types,regression} 的全部 .bur；stdin.bur
# 需要输入，单列为 STDIN 例（固定输入文件 STDIN.in 重定向）。每例在 <outdir> 下产出：
#   <name>.expected  Linux x86 的 stdout
#   <name>.rc        Linux x86 的退出码
#   <name>.exe / <name>-mac  目标二进制（构建失败则记入 BUILD_FAIL）
# Cross-target x86 corpus: takes the Linux x86 run as the reference and builds
# the same programs for windows/darwin, so real Windows/macOS runners can diff
# stdout and exit codes case by case.
# Usage: BUR=<compiler> ./scripts/xos-corpus.sh <windows|darwin> <outdir>
set -u
cd "$(dirname "$0")/.."

if [ -z "${BUR:-}" ] || [ $# -ne 2 ]; then
    echo "usage: BUR=<compiler> $0 <windows|darwin> <outdir>" >&2
    exit 2
fi
os="$1"
out="$2"
mkdir -p "$out"
: >"$out/BUILD_FAIL"
: >"$out/CASES"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for f in examples/basics/*.bur examples/types/*.bur examples/concurrency/*.bur examples/io/*.bur examples/net/*.bur examples/programs/*.bur testdata/basics/*.bur testdata/types/*.bur testdata/regression/*.bur; do
    [ -f "$f" ] || continue
    case "$f" in */stdin.bur) continue ;; esac
    name=$(echo "${f%.bur}" | tr '/' '_')
    if ! "$BUR" build --backend x86 "$f" -o "$tmp/ref" >/dev/null 2>&1; then
        continue
    fi
    timeout 20 "$tmp/ref" >"$out/$name.expected" 2>/dev/null
    echo $? >"$out/$name.rc"
    if [ "$os" = windows ]; then
        bin="$out/$name.exe"
    else
        bin="$out/$name-mac"
    fi
    if "$BUR" build --backend x86 --os "$os" "$f" -o "$bin" >/dev/null 2>&1; then
        echo "$name" >>"$out/CASES"
    else
        echo "$name" >>"$out/BUILD_FAIL"
        rm -f "$out/$name.expected" "$out/$name.rc"
    fi
done
# 带参运行的 args：空格参数与非 ASCII 参数，runner 以同一组参数运行目标二进制，对照 ARGV.expected
# args with arguments: a spaced and a non-ASCII argument; runners run the
# target binary with the same arguments and compare against ARGV.expected
if "$BUR" build --backend x86 examples/basics/args.bur -o "$tmp/ref"; then
    "$tmp/ref" one "two words" 三 >"$out/ARGV.expected"
fi
# stdin：固定输入文件重定向（不经管道，三端语义一致），runner 以同一文件喂目标二进制，对照 STDIN.expected
# stdin: a fixed input file redirected in (not a pipe, so all three targets see
# the same semantics); runners feed the same file and compare against STDIN.expected
printf 'helloWORLD' >"$out/STDIN.in"
if [ "$os" = windows ]; then
    stdin_bin="$out/examples_io_stdin.exe"
else
    stdin_bin="$out/examples_io_stdin-mac"
fi
if "$BUR" build --backend x86 examples/io/stdin.bur -o "$tmp/ref" && "$BUR" build --backend x86 --os "$os" examples/io/stdin.bur -o "$stdin_bin"; then
    "$tmp/ref" <"$out/STDIN.in" >"$out/STDIN.expected"
fi
echo "cases: $(wc -l <"$out/CASES"), build failures: $(wc -l <"$out/BUILD_FAIL")"
