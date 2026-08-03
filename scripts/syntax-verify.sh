#!/bin/bash
# 全量语法验证：S8.2 冻结语法的每个特性一个可运行样例。
# 跑全部样例（期望 exit 0）+ 输出断言 + 自举 fixpoint。
set -u
cd "$(dirname "$0")/.."

BUR=${BUR:-./bur}
fails=0

run() {
    local file="$1" expect="$2"
    local out
    out=$("$BUR" run "examples/syntax/$file" 2>&1)
    local rc=$?
    if [ $rc -ne 0 ]; then
        echo "FAIL $file: exit $rc"
        echo "$out" | head -5
        fails=$((fails + 1))
        return
    fi
    if ! echo "$out" | grep -q "$expect"; then
        echo "FAIL $file: expected '$expect' in output"
        echo "$out" | head -5
        fails=$((fails + 1))
        return
    fi
    echo "PASS $file"
}

run literals.bur    "255 10 1000000 0.0015 4294967295"
run multiline.bur   "line two 42"
run compound.bur    "sum 13"
run annotations.bur "5 100 (1, \"hi\")"
run orpattern.bur   "mid"
run recordpat.bur   "sum 30"
run methods.bur     "25.0"
run tuple.bur       "16"
run range.bur       "1  2  3  4"
run shebang.bur     "3"
run keywords.bur     "11 1 5 5 true false 7"
run functions.bur    "10 5 -1 6 big value is 5"
run map.bur          "1 2 1"

# 模块样例（import/pub/::）单独跑目录
out=$("$BUR" run examples/syntax/modules 2>&1)
if echo "$out" | grep -q "hi, bur"; then
    echo "PASS modules.bur"
else
    echo "FAIL modules: $out"
    fails=$((fails + 1))
fi

echo "--- 自举 fixpoint ---"
if "$BUR" build compiler -o /tmp/burgen2 2>/dev/null && cmp "$BUR" /tmp/burgen2; then
    echo "PASS fixpoint"
else
    echo "FAIL fixpoint"
    fails=$((fails + 1))
fi
rm -f /tmp/burgen2 program.c

echo
if [ $fails -eq 0 ]; then
    echo "=== syntax verify: all pass ==="
else
    echo "=== syntax verify: $fails failure(s) ==="
fi
exit $fails
