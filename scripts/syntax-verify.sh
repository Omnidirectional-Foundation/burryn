#!/bin/bash
# 全量语法验证：S8.2 冻结语法的每个特性一个可运行样例。
# 每个样例配 .golden（冻结期望 stdout）；跑全部 diff + 自举 fixpoint。
set -u
cd "$(dirname "$0")/.."

BUR=${BUR:-./bur}
fails=0

run() {
    local file="$1"
    local out
    out=$("$BUR" run "testdata/syntax/$file" 2>&1)
    local rc=$?
    if [ $rc -ne 0 ]; then
        echo "FAIL $file: exit $rc"
        echo "$out" | head -5
        fails=$((fails + 1))
        return
    fi
    if ! echo "$out" | diff -q - "testdata/syntax/${file%.bur}.golden" >/dev/null 2>&1; then
        echo "FAIL $file: output differs from golden"
        echo "$out" | diff - "testdata/syntax/${file%.bur}.golden" | head -8
        fails=$((fails + 1))
        return
    fi
    echo "PASS $file"
}

for f in testdata/syntax/*.bur; do
    name=$(basename "$f")
    [ "$name" = "rowvar.bur" ] && continue
    run "$name"
done

# 模块样例（目录入口）输出对比 golden
out=$("$BUR" run testdata/syntax/modules 2>&1)
if echo "$out" | diff -q - testdata/syntax/modules.golden >/dev/null 2>&1; then
    echo "PASS modules"
else
    echo "FAIL modules"
    fails=$((fails + 1))
fi

# 行变量：语法层验证（S8.3 语义未实现，dev parse 应成功）
if "$BUR" dev parse testdata/syntax/rowvar.bur >/dev/null 2>&1; then
    echo "PASS rowvar.bur (syntax)"
else
    echo "FAIL rowvar.bur"
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
