#!/bin/bash
# rss-curve.sh — 采一条命令的常驻内存曲线（RSS）。
# rss-curve.sh — samples the resident-memory (RSS) curve of a command.
#
# 用法：./scripts/rss-curve.sh <label> <cmd> [args...]
#   后台跑命令，每 RSS_INTERVAL 秒（默认 15）读一次 /proc/<pid>/status 的 VmRSS，
#   打印 "<label>-heap t=<秒>s rss=<MiB>MiB" 采样行，收尾再打一行峰值；退出码取
#   命令自己的退出码（采样失败不影响被采命令的成败）。
# Usage: ./scripts/rss-curve.sh <label> <cmd> [args...]
#   Runs the command in the background, samples /proc/<pid>/status's VmRSS every
#   RSS_INTERVAL seconds (default 15), prints one "<label>-heap t=<s>s rss=<MiB>MiB"
#   line per sample plus a peak line at the end, and exits with the command's own
#   status (the sampling never changes it).
#
# 为什么用 RSS：x86 运行时的堆是 MAP_NORESERVE 的保留区（不预留物理页），RSS 量的
# 是实际触过的页，因此逼近「真正用掉多少堆」；x86 堆自己的账目由 OOM 报错里的三个
# 数给出（已用/上限/上轮释放），两者互为印证。
# Why RSS: the x86 runtime's heap is a MAP_NORESERVE reservation (no physical pages
# reserved up front), so RSS counts the pages actually touched and tracks "how much
# heap is really in use"; the x86 heap's own accounting comes from the three numbers
# in the OOM report (used / limit / last freed), which corroborates the curve.
set -u
cd "$(dirname "$0")/.."

if [ $# -lt 2 ]; then
    echo "usage: $0 <label> <cmd> [args...]" >&2
    exit 2
fi

label="$1"
shift
interval="${RSS_INTERVAL:-15}"

"$@" &
pid=$!
start=$(date +%s)
peak=0

while :; do
    # 进程没了（被回收）或已成僵尸就收尾：僵尸在 /proc 里还在，但已经没有 RSS 可采。
    # Stop when the process is gone (reaped) or is a zombie: a zombie still shows up
    # in /proc but has no RSS left to sample.
    if [ ! -d "/proc/$pid" ]; then
        break
    fi
    state=$(sed 's/^.*) //' "/proc/$pid/stat" 2>/dev/null | cut -d' ' -f1)
    if [ -z "$state" ] || [ "$state" = "Z" ]; then
        break
    fi
    rss=$(awk '/^VmRSS:/ {print $2}' "/proc/$pid/status" 2>/dev/null)
    if [ -n "$rss" ]; then
        if [ "$rss" -gt "$peak" ]; then
            peak=$rss
        fi
        echo "$label-heap t=$(( $(date +%s) - start ))s rss=$(( rss / 1024 ))MiB"
    fi
    sleep "$interval"
done

rc=0
wait "$pid" || rc=$?
echo "$label-heap peak=$(( peak / 1024 ))MiB rc=$rc"
exit "$rc"
