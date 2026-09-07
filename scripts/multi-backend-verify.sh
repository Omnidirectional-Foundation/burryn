#!/usr/bin/env bash
# multi-backend-verify.sh — VM / C / x86 三方行为一致性全量验证。
# 覆盖 examples 全目录、testdata 运行样例（basics/types/regression）、
# testdata/pkg 模块入口、std 各包测试（拼接包文件+测试文件编译）。
# 判定规则（构建失败保留真实退出码，无哨兵）：
#   PASS = 三方 stdout + exit code 一致，含三方一致拒绝（构建期拒绝也在内）
#   SKIP = 任一后端 SIGTRAP(133)，未实现 opcode
#   GAP  = x86 明确拒收模块（"x86 backend does not support modules"），且另两方一致；
#          实际 gap 集合必须等于 EXPECTED_GAPS，多一个少一个换一个都非零退出
#   FAIL = 其余任一后端行为差异；FAIL > 0 非零退出
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
# GAP 基线：x86 能跑但明确拒收模块的三处。判据是集合相等——多一个、
# 少一个、换一个都非零退出；gap 变 PASS 也红（清单该更新，不该无声）。
# 注意另 9 个 testdata/pkg/*（annotations/cached/constcycle/consts/deepmut/
# extimport/extmissing/pipeline/stdjson）是三方一致拒绝的模块负样例
# （未用 import、坏依赖、常量环等），走 PASS，不进此桶。
# GAP baseline: the three runnable programs x86 rejects for modules. Compared
# as a set — any addition, removal or swap fails; a gap turning PASS fails too
# (the list must be updated, not silently shrunk). The other nine
# testdata/pkg/* are unanimously-rejected module fixtures, hence PASS.
EXPECTED_GAPS="testdata/pkg/deferred/ testdata/pkg/greeter/ testdata/pkg/modules/"

# run_trio <file> <label> — VM/C/x86 三方跑同一程序并比较
# 构建失败保留真实退出码（无哨兵）：三方一致拒绝（含构建期拒绝）走 PASS；
# GAP 旁路只留 x86 那条模块拒收（共享前端的 E0449/E0432 不进豁免桶）。
run_trio() {
    local file="$1" label="$2"
    local vm_rc=0 c_rc=0 x_rc=0 vm_out="" c_out="" x_out=""
    local x_mod_reject=0
    SEQ=$((SEQ + 1))

    vm_out=$(timeout 20 "$BUR" run "$file" 2>/dev/null)
    vm_rc=$?

    local cbin="$TMP/c_$SEQ"
    local cerr=""
    local c_brc=0
    cerr=$("$BUR" build --backend c "$file" -o "$cbin" 2>&1)
    c_brc=$?
    if [ -x "$cbin" ]; then
        c_out=$(timeout 20 "$cbin" 2>/dev/null)
        c_rc=$?
    else
        c_out=""
        c_rc=$c_brc
    fi

    local xbin="$TMP/x_$SEQ"
    local xerr=""
    local x_brc=0
    xerr=$("$BUR" build --backend x86 "$file" -o "$xbin" 2>&1)
    x_brc=$?
    if [ -x "$xbin" ]; then
        x_out=$(timeout 20 "$xbin" 2>/dev/null)
        x_rc=$?
    else
        x_out=""
        x_rc=$x_brc
        if echo "$xerr" | grep -q "x86 backend does not support modules"; then
            x_mod_reject=1
        fi
    fi

    if [ "$vm_rc" -eq 133 ] || [ "$c_rc" -eq 133 ] || [ "$x_rc" -eq 133 ]; then
        echo "  SKIP $label (SIGTRAP unimplemented opcode)"
        SKIP=$((SKIP + 1))
        return
    fi

    if [ "$vm_rc" = "$c_rc" ] && [ "$c_rc" = "$x_rc" ] && [ "$vm_out" = "$c_out" ] && [ "$c_out" = "$x_out" ]; then
        echo "  PASS $label"
        PASS=$((PASS + 1))
        return
    fi

    if [ "$x_mod_reject" -eq 1 ] && [ "$vm_rc" = "$c_rc" ] && [ "$vm_out" = "$c_out" ]; then
        echo "  GAP  $label (x86 backend rejects modules)"
        GAP=$((GAP + 1))
        GAPS="$GAPS $label"
        return
    fi

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
norm_gaps() {
    echo "$1" | tr ' ' '\n' | grep -v '^$' | sort | tr '\n' ' '
}
if [ "$(norm_gaps "$GAPS")" != "$(norm_gaps "$EXPECTED_GAPS")" ]; then
    echo "GAP set mismatch:" >&2
    echo "  expected:$EXPECTED_GAPS" >&2
    echo "  actual:$GAPS" >&2
    exit 1
fi
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
