#!/usr/bin/env bash
# xos-corpus.sh — x86 跨目标语料：以 Linux x86 运行结果为基准，构建同一批样例的
# windows/darwin 版本，供真 Windows/macOS runner 逐例对照 stdout 与退出码。
# 语料 = examples 各分类 + testdata/{basics,types,regression} 的全部 .bur（stdin.bur
# 需管道输入，豁免）。每例在 <outdir> 下产出：
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
echo "cases: $(wc -l <"$out/CASES"), build failures: $(wc -l <"$out/BUILD_FAIL")"
