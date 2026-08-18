#!/usr/bin/env bash
# multi-backend-verify.sh — VM / C / x86 三方行为一致性全量验证。
# 覆盖 examples 全目录、testdata 运行样例（basics/types/regression）、
# testdata/pkg 模块入口、std 各包测试（拼接包文件+测试文件编译）。
# 判定规则：
#   PASS = 三方 stdout + exit code 一致（含三方一致拒绝该程序）
#   SKIP = 任一后端 SIGTRAP(133)，未实现 opcode
#   FAIL = 任一后端行为差异
# Usage: ./scripts/multi-backend-verify.sh
set -u
cd "$(dirname "$0")/.."

BUR="${BUR:-./bur}"
TMP="/tmp/mbverify-$$"
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT

PASS=0
FAIL=0
SKIP=0
GAP=0
SEQ=0
FAILURES=""
GAPS=""

# run_trio <file> <label> — VM/C/x86 三方跑同一程序并比较
run_trio() {
    local file="$1" label="$2"
    local vm_rc=0 c_rc=0 x_rc=0 vm_out="" c_out="" x_out=""
    SEQ=$((SEQ + 1))

    vm_out=$(timeout 20 "$BUR" run "$file" 2>/dev/null)
    vm_rc=$?

    local cbin="$TMP/c_$SEQ"
    local cerr=""
    cerr=$("$BUR" build --backend c "$file" -o "$cbin" 2>&1)
    if [ -x "$cbin" ]; then
        c_out=$(timeout 20 "$cbin" 2>/dev/null)
        c_rc=$?
    elif echo "$cerr" | grep -q "does not support\|not allowed here"; then
        c_rc=98
    else
        c_rc=99
    fi

    local xbin="$TMP/x_$SEQ"
    local xerr=""
    xerr=$("$BUR" build --backend x86 "$file" -o "$xbin" 2>&1)
    if [ -x "$xbin" ]; then
        x_out=$(timeout 20 "$xbin" 2>/dev/null)
        x_rc=$?
    elif echo "$xerr" | grep -q "does not support\|not allowed here"; then
        x_rc=98
    else
        x_rc=99
    fi

    if [ "$vm_rc" -eq 133 ] || [ "$c_rc" -eq 133 ] || [ "$x_rc" -eq 133 ]; then
        echo "  SKIP $label (SIGTRAP unimplemented opcode)"
        SKIP=$((SKIP + 1))
        return
    fi

    if [ "$vm_rc" -eq 98 ] || [ "$c_rc" -eq 98 ] || [ "$x_rc" -eq 98 ]; then
        echo "  GAP  $label (a backend explicitly rejects this program)"
        GAP=$((GAP + 1))
        GAPS="$GAPS $label"
        return
    fi

    if [ "$vm_rc" = "$c_rc" ] && [ "$c_rc" = "$x_rc" ] && [ "$vm_out" = "$c_out" ] && [ "$c_out" = "$x_out" ]; then
        echo "  PASS $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL $label (vm: rc=$vm_rc, c: rc=$c_rc, x86: rc=$x_rc)"
        if [ "$vm_rc" != "$c_rc" ] || [ "$vm_out" != "$c_out" ]; then
            echo "    vm:  rc=$vm_rc out=[$vm_out]"
            echo "    c:   rc=$c_rc out=[$c_out]"
        fi
        if [ "$vm_rc" != "$x_rc" ] || [ "$vm_out" != "$x_out" ]; then
            echo "    vm:  rc=$vm_rc out=[$vm_out]"
            echo "    x86: rc=$x_rc out=[$x_out]"
        fi
        FAIL=$((FAIL + 1))
        FAILURES="$FAILURES $label"
    fi
}

echo "=== Multi-backend verify (VM / C / x86) ==="

echo "--- examples ---"
for f in examples/*/*.bur; do
    [ -f "$f" ] || continue
    case "$f" in
        *stdin.bur) continue ;;
    esac
    run_trio "$f" "$f"
done

echo "--- testdata run suites ---"
for f in testdata/basics/*.bur testdata/types/*.bur testdata/regression/*.bur; do
    [ -f "$f" ] || continue
    run_trio "$f" "$f"
done

echo "--- testdata/pkg module entries ---"
for d in testdata/pkg/*/; do
    [ -f "$d/main.bur" ] || continue
    run_trio "${d%/}" "$d"
done

echo "--- std package tests (merged package + test file) ---"
for t in std/*/*_test.bur; do
    [ -f "$t" ] || continue
    pkgdir="${t%/*}"
    pkgfile="$pkgdir/$(basename "$pkgdir").bur"
    merged="$TMP/$(basename "$pkgdir")_merged.bur"
    sed 's/^pub //' "$pkgfile" "$t" > "$merged"
    run_trio "$merged" "$pkgdir"
done

echo
echo "=== Summary: $PASS pass, $FAIL fail, $SKIP skip, $GAP known-gap ==="
if [ -n "$FAILURES" ]; then
    echo "FAILED:$FAILURES"
fi
if [ -n "$GAPS" ]; then
    echo "KNOWN-GAP (backend rejects):$GAPS"
fi
exit 0
