#!/bin/bash
# pe-emission-compare.sh — build the CI PE corpus with two compilers, compare bytes.
# Proves a backend refactor changed nothing (refactor-safety only):
# byte-identical output does NOT imply correctness. Pin behavior with the
# x86-pe-run CI job and Windows rounds instead (see reports/s8-5-pe-macho.md).
# Usage: ./scripts/pe-emission-compare.sh <old-bur> <new-bur>
set -u
cd "$(dirname "$0")/.."

if [ $# -ne 2 ]; then
    echo "usage: pe-emission-compare.sh <old-bur> <new-bur>" >&2
    exit 2
fi
OLD="$1"
NEW="$2"
for b in "$OLD" "$NEW"; do
    [ -x "$b" ] || { echo "not executable: $b" >&2; exit 2; }
done

TMP="/tmp/pecmp-$$"
mkdir -p "$TMP/old" "$TMP/new"
trap 'rm -rf "$TMP"' EXIT

fails=0
check_one() {
    local src="$1" name="$2"
    "$OLD" build --backend x86 --os windows "$src" -o "$TMP/old/$name" 2>"$TMP/old-$name.err" \
        || { echo "FAIL $name: old compiler cannot build: $(head -1 "$TMP/old-$name.err")"; fails=$((fails + 1)); return; }
    "$NEW" build --backend x86 --os windows "$src" -o "$TMP/new/$name" 2>"$TMP/new-$name.err" \
        || { echo "FAIL $name: new compiler cannot build: $(head -1 "$TMP/new-$name.err")"; fails=$((fails + 1)); return; }
    if cmp -s "$TMP/old/$name" "$TMP/new/$name"; then
        echo "IDENTICAL $name"
    else
        echo "DIFFERS $name"
        fails=$((fails + 1))
    fi
}

# CI PE corpus (x86-pe-build programs), one line per binary.
check_one examples/basics/hello.bur hello.exe
check_one examples/basics/fib.bur fib.exe
check_one examples/basics/float_arith.bur float_arith.exe
check_one examples/concurrency/yield.bur yield.exe
check_one testdata/regression/clock.bur clock.exe
check_one examples/io/sleep.bur sleep.exe

echo
if [ $fails -eq 0 ]; then
    echo "=== pe emission compare: all identical ==="
else
    echo "=== pe emission compare: $fails difference(s) ==="
fi
exit $fails
