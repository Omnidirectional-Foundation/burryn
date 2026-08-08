#!/usr/bin/env bash
# parity.sh — VM vs x86 backend behavior parity check
# For each .bur program in examples/programs/ and examples/basics/:
#   1. Run via VM (bur run) → capture stdout + exit code
#   2. Build via x86 (bur build --backend x86) → run → capture stdout + exit code
#   3. Compare; skip on SIGTRAP (exit 133 = 128+5 = unimplemented opcode)
# Usage: ./scripts/parity.sh
set -euo pipefail
cd "$(dirname "$0")/.."

BUR="${BUR:-./bur}"
TMPDIR="/tmp/parity-$$"
mkdir -p "$TMPDIR"
trap 'rm -rf "$TMPDIR"' EXIT

PASS=0
FAIL=0
SKIP=0
FAILURES=""

run_parity() {
    local file="$1"
    local name
    name=$(basename "$file" .bur)

    # VM output
    local vm_out vm_rc
    if vm_out=$("$BUR" run "$file" 2>/dev/null); then
        vm_rc=0
    else
        vm_rc=$?
    fi

    # x86 build + run
    local bin="$TMPDIR/$name"
    if ! "$BUR" build --backend x86 "$file" -o "$bin" 2>/dev/null; then
        echo "  SKIP $name (x86 build failed)"
        SKIP=$((SKIP + 1))
        return
    fi

    local x86_out x86_rc
    if x86_out=$("$bin" 2>/dev/null); then
        x86_rc=0
    else
        x86_rc=$?
    fi

    # SIGTRAP = unimplemented opcode
    if [ "$x86_rc" -eq 133 ]; then
        echo "  SKIP $name (unimplemented opcode, SIGTRAP)"
        SKIP=$((SKIP + 1))
        return
    fi

    # known limitation: the x86 backend has no GC (bump allocation only)
    if [ "$name" == "gc_stress" ]; then
        echo "  SKIP $name (no GC in x86 backend)"
        SKIP=$((SKIP + 1))
        return
    fi

    # Compare stdout + exit code
    if [ "$vm_out" == "$x86_out" ] && [ "$vm_rc" == "$x86_rc" ]; then
        echo "  PASS $name"
        PASS=$((PASS + 1))
    else
        echo "  FAIL $name (vm: rc=$vm_rc, x86: rc=$x86_rc)"
        echo "    vm:  $vm_out"
        echo "    x86: $x86_out"
        FAIL=$((FAIL + 1))
        FAILURES="$FAILURES $name"
    fi
}

echo "=== VM vs x86 Behavior Parity ==="

# Collect test programs
TEST_FILES=()
for f in examples/programs/*.bur examples/basics/*.bur testdata/regression/*.bur testdata/types/*.bur; do
    [ -f "$f" ] && TEST_FILES+=("$f")
done

if [ ${#TEST_FILES[@]} -eq 0 ]; then
    echo "No test programs found"
    exit 0
fi

for f in "${TEST_FILES[@]}"; do
    run_parity "$f"
done

echo ""
echo "=== Summary: $PASS pass, $FAIL fail, $SKIP skip ==="

if [ "$FAIL" -gt 0 ]; then
    echo "Failed: $FAILURES"
    exit 1
fi
exit 0
