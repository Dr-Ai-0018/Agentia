#!/usr/bin/env bash
set -euo pipefail
set +x

BASE_URL="${BASE_URL:-https://skrime.killerbest.com}"
BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:8788}"
BASIC_AUTH_FILE="${BASIC_AUTH_FILE:-/root/secrets/ai-arena/skrime-console-basic-auth.env}"
TOKEN_ENV_FILE="${TOKEN_ENV_FILE:-/etc/ai-arena-console.env}"
DIST_DIR="${DIST_DIR:-/root/ai-arena/frontend/dist}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing_command\t%s\n' "$1"
    exit 2
  fi
}

read_kv() {
  local key="$1"
  local file="$2"
  sed -n "s/^${key}=//p" "$file" | head -1
}

status_code() {
  curl -sS -o /dev/null -w '%{http_code}' "$@"
}

curl_config_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}

status_code_with_config() {
  local config="$1"
  local url="$2"
  printf '%s\n' "$config" | curl -sS -K - -o /dev/null -w '%{http_code}' "$url"
}

fetch_with_config() {
  local config="$1"
  local url="$2"
  printf '%s\n' "$config" | curl -sS -K - "$url"
}

print_check() {
  printf '%s\t%s\n' "$1" "$2"
}

need_cmd curl
need_cmd jq
need_cmd rg
need_cmd sha256sum

print_check phase start

print_check public_root_unauth "$(status_code "${BASE_URL}/")"
print_check public_health_unauth "$(status_code "${BASE_URL}/api/health")"

if [[ -r "$BASIC_AUTH_FILE" ]]; then
  BASIC_USER="$(read_kv user "$BASIC_AUTH_FILE")"
  BASIC_PASSWORD="$(read_kv password "$BASIC_AUTH_FILE")"
  if [[ -n "$BASIC_USER" && -n "$BASIC_PASSWORD" ]]; then
    basic_config="user = \"$(curl_config_escape "${BASIC_USER}:${BASIC_PASSWORD}")\""
    print_check public_root_auth "$(status_code_with_config "$basic_config" "${BASE_URL}/")"
    print_check public_health_auth "$(status_code_with_config "$basic_config" "${BASE_URL}/api/health")"

    html="$(fetch_with_config "$basic_config" "${BASE_URL}/")"
    js_path="$(printf '%s' "$html" | sed -n 's/.*src="\([^"]*index-[^"]*\.js\)".*/\1/p' | head -1)"
    css_path="$(printf '%s' "$html" | sed -n 's/.*href="\([^"]*index-[^"]*\.css\)".*/\1/p' | head -1)"
    print_check public_js "${js_path:-missing}"
    print_check public_css "${css_path:-missing}"

    if [[ -n "$js_path" && -r "${DIST_DIR}${js_path}" ]]; then
      local_js_hash="$(sha256sum "${DIST_DIR}${js_path}" | awk '{print $1}')"
      remote_js_hash="$(fetch_with_config "$basic_config" "${BASE_URL}${js_path}" | sha256sum | awk '{print $1}')"
      if [[ "$local_js_hash" == "$remote_js_hash" ]]; then
        print_check public_js_hash_match yes
      else
        print_check public_js_hash_match no
      fi
    else
      print_check public_js_hash_match skipped
    fi
  else
    print_check public_auth skipped_empty_recovery
  fi
else
  print_check public_auth skipped_missing_recovery
fi

if [[ -r "$TOKEN_ENV_FILE" ]]; then
  TOKEN="$(read_kv ARENA_CONSOLE_TOKEN "$TOKEN_ENV_FILE")"
  if [[ -n "$TOKEN" ]]; then
    token_config="header = \"X-Arena-Console-Token: $(curl_config_escape "$TOKEN")\""
    for path in /api/health /api/summary /api/runs /api/preflight /api/diagnostics/compaction; do
      print_check "backend_${path}" "$(status_code_with_config "$token_config" "${BACKEND_URL}${path}")"
    done

    summary="$(fetch_with_config "$token_config" "${BACKEND_URL}/api/summary")"
    print_check summary_active_null "$(printf '%s' "$summary" | jq -r '.active_run == null')"
    print_check summary_latest_status "$(printf '%s' "$summary" | jq -r '.latest_run.status // "missing"')"
    print_check summary_run_stale_alerts "$(printf '%s' "$summary" | jq -r '[.alerts[]? | select(.kind == "run_stale")] | length')"

    runs="$(fetch_with_config "$token_config" "${BACKEND_URL}/api/runs?limit=20")"
    printf '%s' "$runs" | jq -r '.[] | select(.status == "abandoned") | ["abandoned_run", .run_id] | @tsv'
  else
    print_check backend_token skipped_empty_env
  fi
else
  print_check backend_token skipped_missing_env
fi

if compgen -G "${DIST_DIR}/assets/index-*.js" >/dev/null; then
  if rg -q '这座 24 小时|目标 24h|历史观察已经' "${DIST_DIR}"/assets/index-*.js; then
    print_check bundle_old_strings found
  else
    print_check bundle_old_strings absent
  fi
  if rg -q '旧记录' "${DIST_DIR}"/assets/index-*.js; then
    print_check bundle_abandoned_copy present
  else
    print_check bundle_abandoned_copy missing
  fi
else
  print_check bundle skipped_missing_js
fi

print_check phase done
