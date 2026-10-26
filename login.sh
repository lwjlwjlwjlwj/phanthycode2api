#!/usr/bin/env bash
# login.sh — PhanthyCode 半自动 PKCE 登录辅助脚本
#
# 用法:
#   ./login.sh                      # 生成授权链接 → 粘贴回调 URL/code → 自动换 token 落盘
#   ./login.sh -code=xxx            # 已有 code 时直接交换
#   ./login.sh -code=xxx -state=yyy # 同时校验 state
#
# 依赖: go（执行 go run ./cmd/login）。凭证写入 auths/（P2A_AUTH_DIR），重启服务生效。
set -euo pipefail

cd "$(dirname "$0")"

export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

CODE=""
STATE=""
for arg in "$@"; do
  case "$arg" in
    -code=*) CODE="${arg#-code=}" ;;
    -state=*) STATE="${arg#-state=}" ;;
    -h|--help) sed -n '2,11p' "$0"; exit 0 ;;
    *) echo "未知参数: $arg" >&2; exit 1 ;;
  esac
done

# 从回调 URL（...?code=xxx&state=yyy#frag）或裸值中提取 code / state。
clean_pair() {
  local input="$1"
  input="${input%%#*}"
  local code="" state=""
  if [[ "$input" == *"code="* ]]; then
    code="${input#*code=}"
    code="${code%%&*}"
  else
    code="$input"
  fi
  if [[ "$input" == *"state="* ]]; then
    state="${input#*state=}"
    state="${state%%&*}"
  fi
  CODE="$(printf '%s' "$code" | tr -d '[:space:]')"
  STATE="$(printf '%s' "$state" | tr -d '[:space:]')"
}

restart_hint() {
  if [[ "${P2A_NO_RESTART:-}" != "1" ]] && command -v docker >/dev/null 2>&1; then
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx phanthycode2api; then
      echo "🔄 重启容器 phanthycode2api 以加载新凭证 ..."
      docker restart phanthycode2api >/dev/null 2>&1 \
        && echo "   重启完成。" \
        || echo "   自动重启失败，请手动: docker restart phanthycode2api"
      return
    fi
  fi
  echo "ℹ️  请重启 phanthycode2api 服务以加载新凭证。"
}

if [[ -n "$CODE" ]]; then
  clean_pair "$CODE"
  if [[ -n "${STATE:-}" ]]; then
    # 用户显式传入 -state= 时以参数为准
    STATE="$(printf '%s' "$STATE" | tr -d '[:space:]')"
  fi
  echo ">>> 使用已有 code 交换 token ..."
  ARGS=(-step=exchange -code="$CODE")
  [[ -n "$STATE" ]] && ARGS+=(-state="$STATE")
  go run ./cmd/login "${ARGS[@]}"
  echo ""
  echo "✅ 登录完成。"
  restart_hint
  exit 0
fi

echo ">>> 步骤 1/2: 生成授权链接 ..."
go run ./cmd/login -step=url

echo ""
echo ">>> 步骤 2/2: 授权后粘贴回调地址（或裸 code）"
read -r -p "redirect> " USER_INPUT
clean_pair "$USER_INPUT"
if [[ -z "$CODE" ]]; then
  echo "未识别到有效 code，退出。" >&2
  exit 1
fi

ARGS=(-step=exchange -code="$CODE")
[[ -n "$STATE" ]] && ARGS+=(-state="$STATE")
go run ./cmd/login "${ARGS[@]}"

echo ""
echo "✅ 登录完成。"
restart_hint
