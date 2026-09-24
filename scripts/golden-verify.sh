#!/bin/bash
# golden-verify.sh — 通用 golden 验证运行器。
# 跑 <dir>... 下每个 .bur：
#   - *_trap.bur → 期待 exit 4（runtime trap）；有同名 .golden 则 stdout 逐字节对比，
#     有同名 .stderr 则 stderr 逐字节对比（运行期错误文本 + 回溯）
#   - 有同名 .golden → 运行并逐字节对比
#   - 其余无 golden 文件 → FAIL（样例必须配验证目标）
#   - *_trap.bur → expect exit 4 (runtime trap); a sibling .golden is compared
#     byte-for-byte against stdout, a sibling .stderr against stderr (the
#     runtime error text plus trace)
# 环境变量 SKIP="a.bur b.bur" 排除文件（如 stdin.bur 需管道输入）。
# Usage: ./scripts/golden-verify.sh <suite-dir>...
set -u
cd "$(dirname "$0")/.."

if [ $# -eq 0 ]; then
    echo "usage: golden-verify.sh <suite-dir>..." >&2
    exit 2
fi

if [ -z "${BUR:-}" ]; then
    echo "BUR is not set; invoke as BUR=<compiler> $0 ..." >&2
    exit 2
fi
fails=0
SKIP_NAMES=${SKIP:-}

run() {
    local dir="$1" name="$2"
    local golden="$dir/${name%.bur}.golden"
    local egolden="$dir/${name%.bur}.stderr"
    if case "$name" in *_trap.bur) true ;; *) false ;; esac; then
        local tout terr
        tout=$(mktemp)
        terr=$(mktemp)
        "$BUR" run "$dir/$name" >"$tout" 2>"$terr"
        local trc=$?
        if [ $trc -ne 4 ]; then
            echo "FAIL $name: expected trap exit 4, got $trc"
            head -5 "$terr"
            fails=$((fails + 1))
        elif [ -f "$golden" ] && ! diff -q "$tout" "$golden" >/dev/null 2>&1; then
            echo "FAIL $dir/$name: stdout differs from golden"
            diff "$tout" "$golden" | head -8
            fails=$((fails + 1))
        elif [ ! -f "$golden" ] && [ -s "$tout" ] && [ -f "$egolden" ]; then
            echo "FAIL $dir/$name: unexpected stdout (no .golden)"
            head -5 "$tout"
            fails=$((fails + 1))
        elif [ -f "$egolden" ] && ! diff -q "$terr" "$egolden" >/dev/null 2>&1; then
            echo "FAIL $dir/$name: stderr differs from .stderr"
            diff "$terr" "$egolden" | head -8
            fails=$((fails + 1))
        else
            echo "PASS $name (trap, exit 4)"
        fi
        rm -f "$tout" "$terr"
    elif [ -f "$golden" ]; then
        local tmp
        tmp=$(mktemp)
        "$BUR" run "$dir/$name" >"$tmp" 2>&1
        local rc=$?
        if [ $rc -ne 0 ]; then
            echo "FAIL $dir/$name: exit $rc"
            head -5 "$tmp"
            fails=$((fails + 1))
        elif ! diff -q "$tmp" "$golden" >/dev/null 2>&1; then
            echo "FAIL $dir/$name: output differs from golden"
            diff "$tmp" "$golden" | head -8
            fails=$((fails + 1))
        else
            echo "PASS $name"
        fi
        rm -f "$tmp"
    else
        echo "FAIL $dir/$name: no .golden and not a *_trap.bur"
        fails=$((fails + 1))
    fi
}

for dir in "$@"; do
    [ -d "$dir" ] || { echo "FAIL: no such dir $dir"; fails=$((fails + 1)); continue; }
    for f in "$dir"/*.bur; do
        [ -f "$f" ] || continue
        name=$(basename "$f")
        case " $SKIP_NAMES " in *" $name "*) continue ;; esac
        run "$dir" "$name"
    done
done

echo
if [ $fails -eq 0 ]; then
    echo "=== golden verify: all pass ==="
else
    echo "=== golden verify: $fails failure(s) ==="
fi
exit $fails
