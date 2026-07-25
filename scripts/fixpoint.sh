#!/usr/bin/env bash
# fixpoint.sh — C backend self-compilation fixpoint check
# gen0 (bur) compiles compiler → gen1; gen1 compiles compiler → gen2; cmp gen1 gen2
# Usage: ./scripts/fixpoint.sh
set -euo pipefail
cd "$(dirname "$0")/.."

GEN0=/tmp/fix-gen0
GEN1=/tmp/fix-gen1

echo "=== C Backend Fixpoint ==="

echo "[1/3] bur → gen0 (C self-compile)..."
./bur build compiler -o "$GEN0" || { echo "FAIL: gen0 build failed"; exit 1; }

echo "[2/3] gen0 → gen1 (C self-compile)..."
"$GEN0" build compiler -o "$GEN1" || { echo "FAIL: gen1 build failed"; exit 1; }

echo "[3/3] cmp gen0 gen1..."
if cmp -s "$GEN0" "$GEN1"; then
    echo "PASS: C fixpoint holds (gen0 == gen1)"
    exit 0
else
    echo "FAIL: C fixpoint broken (gen0 != gen1)"
    exit 1
fi
