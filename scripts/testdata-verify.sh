#!/bin/bash
# testdata-verify.sh — testdata 运行样例验证 + 构建自洽。
# 跑主题目录（basics/types/regression）的 .bur vs .golden；
# pkg/modules 多包样例与 rowvar 语法层特判；结尾跑 C 后端双级 fixpoint
# （gen0 由 bur 编译自身，gen1 由 gen0 编译自身，cmp gen0 gen1）。
# Usage: ./scripts/testdata-verify.sh
set -u
cd "$(dirname "$0")/.."

BUR=${BUR:-./bur}
fails=0

if ! SKIP=rowvar.bur ./scripts/golden-verify.sh testdata/basics testdata/types testdata/regression; then
    fails=$((fails + 1))
fi

# pkg/modules：多包模块样例（pub/import/type），输出对比 golden
tmp=$(mktemp)
"$BUR" run testdata/pkg/modules >"$tmp" 2>&1
if diff -q "$tmp" testdata/pkg/modules.golden >/dev/null 2>&1; then
    echo "PASS modules"
else
    echo "FAIL modules"
    diff "$tmp" testdata/pkg/modules.golden | head -8
    fails=$((fails + 1))
fi
rm -f "$tmp"

# rowvar：行变量语法（S8.3 语义未实现，dev parse 应成功）
if "$BUR" dev parse testdata/types/rowvar.bur >/dev/null 2>&1; then
    echo "PASS rowvar.bur (syntax)"
else
    echo "FAIL rowvar.bur"
    fails=$((fails + 1))
fi

echo "--- C 后端三代 fixpoint ---"
# gen0 = bur 编译（继承旧代内置 std 数据）；gen1 = gen0 编译；gen2 = gen1 编译。
# 数据世代经一代更新后收敛：判定 gen1 == gen2（与 CI 的 gen2 == gen3 同语义）。
GEN0=/tmp/td-gen0
GEN1=/tmp/td-gen1
GEN2=/tmp/td-gen2
if "$BUR" build compiler -o "$GEN0" 2>/dev/null && "$GEN0" build compiler -o "$GEN1" 2>/dev/null && "$GEN1" build compiler -o "$GEN2" 2>/dev/null && cmp "$GEN1" "$GEN2"; then
    echo "PASS fixpoint (gen1 == gen2)"
else
    echo "FAIL fixpoint"
    fails=$((fails + 1))
fi
rm -f "$GEN0" "$GEN1" "$GEN2" program.c

echo
if [ $fails -eq 0 ]; then
    echo "=== testdata verify: all pass ==="
else
    echo "=== testdata verify: $fails failure(s) ==="
fi
exit $fails
