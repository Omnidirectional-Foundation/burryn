#!/bin/bash
# examples-verify.sh — examples 全分类 golden 验证。
# 跑 6 个主题分类的 .bur vs .golden；*_trap.bur 期待 exit 4；
# stdin.bur 需管道输入（SKIP 豁免）；geometry 模块样例特判。
# Usage: ./scripts/examples-verify.sh
set -u
cd "$(dirname "$0")/.."

BUR=${BUR:-./bur}
fails=0

if ! SKIP=stdin.bur ./scripts/golden-verify.sh \
    examples/basics examples/types examples/concurrency examples/io examples/net examples/programs; then
    fails=$((fails + 1))
fi

# geometry：多包模块样例（目录入口），输出对比 golden
tmp=$(mktemp)
"$BUR" run examples/programs/geometry >"$tmp" 2>&1
if diff -q "$tmp" examples/programs/geometry.golden >/dev/null 2>&1; then
    echo "PASS geometry"
else
    echo "FAIL geometry"
    diff "$tmp" examples/programs/geometry.golden | head -8
    fails=$((fails + 1))
fi
rm -f "$tmp"

echo
if [ $fails -eq 0 ]; then
    echo "=== examples verify: all pass ==="
else
    echo "=== examples verify: $fails failure(s) ==="
fi
exit $fails
