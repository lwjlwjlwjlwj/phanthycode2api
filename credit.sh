#!/usr/bin/env bash
# credit.sh — 查看账号池 credit / 状态
#
# 用法:
#   ./credit.sh            # 通过 /status 端点查询（需服务运行，读 P2A_API_KEY / P2A_LISTEN）
#   ./credit.sh -local     # 直接读本地 data/state.json（无需服务运行）
#   ./credit.sh -h         # 帮助
#
# 环境变量:
#   P2A_LISTEN   默认 :7864（容器内 7864，宿主机按映射填，如 17867）
#   P2A_API_KEY  服务端鉴权密钥（开启鉴权时必须）
set -euo pipefail

cd "$(dirname "$0")"

LOCAL=0
for arg in "$@"; do
  case "$arg" in
    -local) LOCAL=1 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "未知参数: $arg" >&2; exit 1 ;;
  esac
done

if [[ "$LOCAL" -eq 1 ]]; then
  STATE_FILE="./data/state.json"
  if [[ ! -f "$STATE_FILE" ]]; then
    echo "本地状态文件不存在: $STATE_FILE" >&2
    exit 1
  fi
  echo ">>> 本地状态 ($STATE_FILE):"
  cat "$STATE_FILE" | python3 -m json.tool 2>/dev/null || cat "$STATE_FILE"
  exit 0
fi

LISTEN="${P2A_LISTEN:-:7864}"
# 去掉前缀冒号，方便拼 URL（:7864 -> 7864；17867 -> 17867）
PORT="${LISTEN#:}"
BASE="http://localhost:${PORT}"

AUTH_HDR=()
if [[ -n "${P2A_API_KEY:-}" ]]; then
  AUTH_HDR=(-H "Authorization: Bearer ${P2A_API_KEY}")
fi

echo ">>> 查询 $BASE/status"
HTTP_CODE=$(curl -s -o /tmp/credit_resp.json -w "%{http_code}" \
  "${AUTH_HDR[@]}" "$BASE/status") || { echo "请求失败" >&2; exit 1; }

if [[ "$HTTP_CODE" != "200" ]]; then
  echo "HTTP $HTTP_CODE —— 可能需要鉴权（设置 P2A_API_KEY）或服务未启动" >&2
  cat /tmp/credit_resp.json 2>/dev/null
  exit 1
fi

cat /tmp/credit_resp.json | python3 -m json.tool 2>/dev/null || cat /tmp/credit_resp.json
