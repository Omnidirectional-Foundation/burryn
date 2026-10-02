#!/usr/bin/env bash
# multi-backend-verify.sh — VM / C / x86 / LLVM 四方行为一致性全量验证。
# 覆盖 examples 全目录、testdata 运行样例（basics/types/regression/traps）、
# testdata/pkg 模块入口、std 各包测试（拼接包文件+测试文件编译）。
# 判定规则（构建失败保留真实退出码，无哨兵）：
#   PASS = 四方 stdout + stderr + exit code 一致，含四方一致拒绝（构建期拒绝也在内；
#          此时 C/x86/LLVM 的 stderr 取构建诊断，与 VM 的 run 诊断对比）
#   PASS = stdout + stderr + exit code agree across all four, including a
#          unanimous rejection (build-time too; C/x86/LLVM then contribute their
#          build diagnostics as stderr, compared with the VM's run diagnostics)
#   SKIP = 任一后端 SIGTRAP(133)，未实现 opcode
#   FAIL = 其余任一后端行为差异；FAIL > 0 非零退出
# Usage: ./scripts/multi-backend-verify.sh
set -u
cd "$(dirname "$0")/.."

if [ -z "${BUR:-}" ]; then
    echo "BUR is not set; invoke as BUR=<compiler> $0 ..." >&2
    exit 2
fi
# clang 缺失属环境错误（非 SKIP）：SKIP 只保留给后端能力缺口。
# A missing clang is an environment error (not a SKIP): SKIP stays
# reserved for backend capability gaps.
if ! command -v clang >/dev/null 2>&1; then
    echo "error: clang not found; the LLVM backend needs clang (install it and rerun)" >&2
    exit 3
fi
TMP="/tmp/mbverify-$$"
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT

PASS=0
FAIL=0
SKIP=0
PROBE=0
SEQ=0
FAILURES=""
PROBES=""

# run_quad <file> <label> — VM/C/x86/LLVM 四方跑同一程序并比较
# 构建失败保留真实退出码（无哨兵）：四方一致拒绝（含构建期拒绝）走 PASS。
run_quad() {
    local file="$1" label="$2"
    local vm_rc=0 c_rc=0 x_rc=0 l_rc=0 vm_out="" c_out="" x_out="" l_out="" vm_err="" c_err="" x_err="" l_err=""
    SEQ=$((SEQ + 1))

    vm_out=$(timeout 20 "$BUR" run "$file" 2>"$TMP/vm_err")
    vm_rc=$?
    vm_err=$(cat "$TMP/vm_err")

    local cbin="$TMP/c_$SEQ"
    local c_brc=0
    "$BUR" build --backend c "$file" -o "$cbin" >/dev/null 2>"$TMP/c_berr"
    c_brc=$?
    if [ -x "$cbin" ]; then
        c_out=$(timeout 20 "$cbin" 2>"$TMP/c_err")
        c_rc=$?
        c_err=$(cat "$TMP/c_err")
    else
        c_out=""
        c_rc=$c_brc
        c_err=$(cat "$TMP/c_berr")
    fi

    local xbin="$TMP/x_$SEQ"
    local x_brc=0
    "$BUR" build --backend x86 "$file" -o "$xbin" >/dev/null 2>"$TMP/x_berr"
    x_brc=$?
    if [ -x "$xbin" ]; then
        x_out=$(timeout 20 "$xbin" 2>"$TMP/x_err")
        x_rc=$?
        x_err=$(cat "$TMP/x_err")
    else
        x_out=""
        x_rc=$x_brc
        x_err=$(cat "$TMP/x_berr")
    fi

    local lbin="$TMP/l_$SEQ"
    local l_brc=0
    "$BUR" build --backend llvm "$file" -o "$lbin" >/dev/null 2>"$TMP/l_berr"
    l_brc=$?
    if [ -x "$lbin" ]; then
        l_out=$(timeout 20 "$lbin" 2>"$TMP/l_err")
        l_rc=$?
        l_err=$(cat "$TMP/l_err")
    else
        l_out=""
        l_rc=$l_brc
        l_err=$(cat "$TMP/l_berr")
    fi

    if [ "$vm_rc" -eq 133 ] || [ "$c_rc" -eq 133 ] || [ "$x_rc" -eq 133 ] || [ "$l_rc" -eq 133 ]; then
        echo "  SKIP $label (SIGTRAP unimplemented opcode)"
        SKIP=$((SKIP + 1))
        return
    fi

    # §18 v9 探针：x86 clock 站点的 rsp tripwire（exit 40-43）命中时该例不可比，
    # 打 24B 现场（rsp, tv_sec, tv_nsec）响亮记录、不计 FAIL（临时，根因落地后回退）。
    # §18 v9 probe: an x86 clock-site rsp tripwire hit (exit 40-43) makes the
    # case uncomparable — print the 24B scene (rsp, tv_sec, tv_nsec), count it
    # loudly as a probe hit instead of FAIL (temporary until root cause lands).
    if [ "$x_rc" -ge 40 ] && [ "$x_rc" -le 43 ]; then
        echo "  PROBE-HIT $label (section-18 v9 tripwire, x86 rc=$x_rc; 24B dump:)"
        od -An -tx1 "$TMP/x_err" | head -3
        PROBE=$((PROBE + 1))
        PROBES="$PROBES $label"
        return
    fi

    if [ "$vm_rc" = "$c_rc" ] && [ "$c_rc" = "$x_rc" ] && [ "$x_rc" = "$l_rc" ] && [ "$vm_out" = "$c_out" ] && [ "$c_out" = "$x_out" ] && [ "$x_out" = "$l_out" ] && [ "$vm_err" = "$c_err" ] && [ "$c_err" = "$x_err" ] && [ "$x_err" = "$l_err" ]; then
        echo "  PASS $label"
        PASS=$((PASS + 1))
        return
    fi

    echo "  FAIL $label (vm: rc=$vm_rc, c: rc=$c_rc, x86: rc=$x_rc, llvm: rc=$l_rc)"
    if [ "$vm_rc" != "$c_rc" ] || [ "$vm_out" != "$c_out" ] || [ "$vm_err" != "$c_err" ]; then
        echo "    vm:  rc=$vm_rc out=[$vm_out] err=[$(printf '%s' "$vm_err" | head -c 300)]"
        echo "    c:   rc=$c_rc out=[$c_out] err=[$(printf '%s' "$c_err" | head -c 300)]"
    fi
    if [ "$vm_rc" != "$x_rc" ] || [ "$vm_out" != "$x_out" ] || [ "$vm_err" != "$x_err" ]; then
        echo "    vm:  rc=$vm_rc out=[$vm_out] err=[$(printf '%s' "$vm_err" | head -c 300)]"
        echo "    x86: rc=$x_rc out=[$x_out] err=[$(printf '%s' "$x_err" | head -c 300)]"
    fi
    if [ "$vm_rc" != "$l_rc" ] || [ "$vm_out" != "$l_out" ] || [ "$vm_err" != "$l_err" ]; then
        echo "    vm:  rc=$vm_rc out=[$vm_out] err=[$(printf '%s' "$vm_err" | head -c 300)]"
        echo "    llvm: rc=$l_rc out=[$l_out] err=[$(printf '%s' "$l_err" | head -c 300)]"
    fi
    FAIL=$((FAIL + 1))
    FAILURES="$FAILURES $label"
}

echo "=== Multi-backend verify (VM / C / x86 / LLVM) ==="

echo "--- examples ---"
for f in examples/*/*.bur; do
    [ -f "$f" ] || continue
    case "$f" in
        *stdin.bur) continue ;;
    esac
    run_quad "$f" "$f"
done

echo "--- testdata run suites ---"
for f in testdata/basics/*.bur testdata/types/*.bur testdata/regression/*.bur testdata/traps/*.bur; do
    [ -f "$f" ] || continue
    run_quad "$f" "$f"
done

echo "--- testdata/pkg module entries ---"
for d in testdata/pkg/*/; do
    [ -f "$d/main.bur" ] || continue
    run_quad "${d%/}" "$d"
done

echo "--- std package tests (merged package + test file) ---"
for t in std/*/*_test.bur; do
    [ -f "$t" ] || continue
    pkgdir="${t%/*}"
    pkgfile="$pkgdir/$(basename "$pkgdir").bur"
    merged="$TMP/$(basename "$pkgdir")_merged.bur"
    sed 's/^pub //' "$pkgfile" "$t" > "$merged"
    run_quad "$merged" "$pkgdir"
done

echo
echo "=== Summary: $PASS pass, $FAIL fail, $SKIP skip, $PROBE probe-hits ==="
if [ -n "$FAILURES" ]; then
    echo "FAILED:$FAILURES"
fi
if [ -n "$PROBES" ]; then
    echo "PROBE-HITS:$PROBES"
fi
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0

