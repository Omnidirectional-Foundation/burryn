#!/bin/bash
# golden-verify.sh — 通用 golden 验证运行器。
# 跑 <dir>... 下每个 .bur：
#   - 有同名 .golden → 运行并逐字节对比
#   - *_trap.bur（无 golden）→ 期待 exit 4（runtime trap）
#   - 其余无 golden 文件 → FAIL（样例必须配验证目标）
# 环境变量 SKIP="a.bur b.bur" 排除文件（如 stdin.bur 需管道输入）。
# Usage: ./scripts/golden-verify.sh <suite-dir>...
set -u
cd "$(dirname "$0")/.."

if [ $# -eq 0 ]; then
    echo "usage: golden-verify.sh <suite-dir>..." >&2
    exit 2
fi

BUR=${BUR:-./bur}
fails=0
SKIP_NAMES=${SKIP:-}

run() {
    local dir="$1" name="$2"
    local golden="$dir/${name%.bur}.golden"
    if [ -f "$golden" ]; then
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
    elif case "$name" in *_trap.bur) true ;; *) false ;; esac; then
        "$BUR" run "$dir/$name" >/dev/null 2>&1
        local trc=$?
        if [ $trc -eq 4 ]; then
            echo "PASS $name (trap, exit 4)"
        else
            echo "FAIL $name: expected trap exit 4, got $trc"
            fails=$((fails + 1))
        fi
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
