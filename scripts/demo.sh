#!/usr/bin/env bash
# demo.sh — 一键演示:建库、灌入 30 天多用户演示数据、启动榜单服务。
# 用法: scripts/demo.sh [port]   (默认 8787;数据在 .scratch/token-leaderboard/demo/)
set -euo pipefail

PORT="${1:-8787}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEMO="$ROOT/.scratch/token-leaderboard/demo"
DB="$DEMO/leaderboard.db"

mkdir -p "$DEMO"
rm -f "$DB" "$DEMO"/device.json

go build -o "$DEMO/token-usage" ./cmd/token-usage
go build -o "$DEMO/server" ./cmd/server

# 演示用户(名字 城市)
USERS=(
  "吴军伟 北京"
  "陈思远 上海"
  "林小满 杭州"
  "赵铁柱 深圳"
  "王蘑菇 成都"
)

declare -A TOKENS
for entry in "${USERS[@]}"; do
  name="${entry%% *}"; city="${entry##* }"
  out=$("$DEMO/server" add-user -db "$DB" -name "$name" -city "$city" -password "demo123")
  token=$(echo "$out" | sed -n 's/^report token (shown once): //p')
  TOKENS[$name]="$token"
done

# 设备配额:每人 1-3 台
seed_random() { RANDOM=$1; }
seed_random 42

post() { # token device date hour tool model in out cread c5m c1h
  curl -s -o /dev/null -X POST "http://127.0.0.1:$PORT/v1/report" \
    -H "Authorization: Bearer $1" -H 'Content-Type: application/json' \
    -d @- <<EOF
{"deviceId":"$2","deviceLabel":"$3","date":"$4","timezone":"Asia/Shanghai","generatedAt":"$4T12:00:00+08:00","hours":[
 {"hour":$5,"tool":"$6","model":"$7","input":$8,"output":$9,"cacheRead":${10},"cacheWrite5m":${11},"cacheWrite1h":${12}}
]}
EOF
}

start_server() {
  "$DEMO/server" serve -db "$DB" -addr "127.0.0.1:$PORT" &
  SERVER_PID=$!
  for _ in $(seq 1 50); do
    curl -sf "http://127.0.0.1:$PORT/about" >/dev/null && return 0
    sleep 0.1
  done
  echo "server failed to start" >&2; exit 1
}

start_server
trap 'kill $SERVER_PID 2>/dev/null || true' EXIT

MODELS=("claude-sonnet-4-5" "claude-opus-4-1" "claude-haiku-4-5" "gpt-5.3-codex" "gemini-3-pro" "kimi-k2")
TOOLS=("claude" "claude" "claude" "codex" "gemini" "kimi")

for day in $(seq 0 29); do
  date=$(date -d "-$day day" +%F)
  i=0
  for entry in "${USERS[@]}"; do
    name="${entry%% *}"
    token="${TOKENS[$name]}"
    devices=$(( RANDOM % 3 + 1 ))
    for d in $(seq 1 $devices); do
      device="dev-${name}-$d"
      # 每设备每天 2-5 个活跃小时
      hours=$(( RANDOM % 4 + 2 ))
      for h in $(seq 1 $hours); do
        hour=$(( RANDOM % 14 + 8 ))
        mi=$(( RANDOM % 6 ))
        in=$(( RANDOM % 40000 + 2000 ))
        out=$(( RANDOM % 9000 + 500 ))
        cread=$(( in * (RANDOM % 8 + 2) ))
        c5=$(( in / 4 ))
        c1=$(( RANDOM % 2 == 0 ? 0 : in / 8 ))
        post "$token" "$device" "dev$d-$name" "$date" "$hour" "${TOOLS[$mi]}" "${MODELS[$mi]}" "$in" "$out" "$cread" "$c5" "$c1"
      done
    done
    i=$((i+1))
  done
done

# 一个刷子:单日超阈值 → Anomaly Flag
post "${TOKENS[赵铁柱]}" "dev-cheater" "cheater" "$(date +%F)" 3 claude claude-sonnet-4-5 2000000000 1000 0 0 0

echo
echo "演示数据就绪:5 用户 × 30 天 × 1-3 设备/人(含 1 个被标记的异常设备)"
echo "  榜单:    http://127.0.0.1:$PORT/"
echo "  我的页面: http://127.0.0.1:$PORT/login (吴军伟 / demo123)"
echo "  价格表:  http://127.0.0.1:$PORT/pricing"
echo
echo "按 Ctrl-C 停止。"
wait $SERVER_PID
