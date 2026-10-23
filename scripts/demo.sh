#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ADDR="${ADDR:-127.0.0.1:8088}"
BASE="http://$ADDR"

echo "==> 启动临时服务器 $BASE"
go run ./cmd/server -addr ":${ADDR##*:}" -web ./web >/tmp/lai-dian-yi-yun-demo.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT

for i in $(seq 1 40); do
  if curl -sf "$BASE/api/health" >/dev/null; then
    break
  fi
  sleep 0.15
done
curl -sf "$BASE/api/health" | grep -q 来电疑云

echo "==> 创建房间 + 补机器人 + 开局"
CREATE=$(curl -sf -X POST "$BASE/api/rooms" -H 'Content-Type: application/json' -d '{"name":"演示员"}')
CODE=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["roomCode"])' <<<"$CREATE")
PID_HUMAN=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["player"]["id"])' <<<"$CREATE")
echo "房间号: $CODE  玩家: $PID_HUMAN"

curl -sf -X POST "$BASE/api/rooms/$CODE/fill-bots" >/dev/null
curl -sf -X POST "$BASE/api/rooms/$CODE/start" >/dev/null
curl -sf -X POST "$BASE/api/rooms/$CODE/advance" >/dev/null

echo "==> 自动推进至结算（人类回合用脚本代打）"
for i in $(seq 1 40); do
  STATE=$(curl -sf "$BASE/api/rooms/$CODE")
  PHASE=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["phase"])' <<<"$STATE")
  echo "  [$i] phase=$PHASE"
  if [[ "$PHASE" == "end" ]]; then
    python3 -c 'import json,sys; s=json.load(sys.stdin); print("胜方:", s.get("winner"), "| 骗子:", s.get("revealedScammer")); print(s.get("hostLine"))' <<<"$STATE"
    break
  fi
  SPEAKER=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("currentSpeakerId") or "")' <<<"$STATE")
  if [[ "$SPEAKER" == "$PID_HUMAN" ]]; then
    if [[ "$PHASE" == "statement" ]]; then
      curl -sf -X POST "$BASE/api/rooms/$CODE/statement" -H 'Content-Type: application/json' \
        -d "{\"playerId\":\"$PID_HUMAN\",\"text\":\"银行不会让人转安全账户，这通电话很假。\"}" >/dev/null
    elif [[ "$PHASE" == "cross_exam" ]]; then
      curl -sf -X POST "$BASE/api/rooms/$CODE/cross" -H 'Content-Type: application/json' \
        -d "{\"playerId\":\"$PID_HUMAN\",\"text\":\"因为正规流程不会要验证码。\"}" >/dev/null
    fi
  fi
  if [[ "$PHASE" == "vote" ]]; then
    TARGET=$(python3 -c 'import json,sys; s=json.load(sys.stdin); print([p["id"] for p in s["players"] if p["id"]!="'"$PID_HUMAN"'"][0])' <<<"$STATE")
    curl -sf -X POST "$BASE/api/rooms/$CODE/vote" -H 'Content-Type: application/json' \
      -d "{\"playerId\":\"$PID_HUMAN\",\"targetId\":\"$TARGET\"}" >/dev/null || true
  fi
  sleep 0.9
done

echo "==> MCP tools"
curl -sf "$BASE/mcp/tools" | python3 -c 'import json,sys; print("tools:", len(json.load(sys.stdin)["tools"]))'
echo "DEMO OK"
