#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:20180/v1}"
API_KEY="${API_KEY:-kr_VzmwybFc9mRubGUAtpQqH6AviTNENUlD}"
MODEL="${MODEL:-mmtp/mimo-v2.5-pro}"

RPM_LIMIT="${RPM_LIMIT:-10}"
TPM_LIMIT="${TPM_LIMIT:-100}"
CONCURRENT_LIMIT="${CONCURRENT_LIMIT:-1}"

OUT_DIR="${OUT_DIR:-/tmp/nexusrouter-rate-limit-test-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$OUT_DIR"

AUTH_HEADER="Authorization: Bearer ${API_KEY}"
JSON_HEADER="Content-Type: application/json"

pass_count=0
fail_count=0

log() {
  printf "\n[%s] %s\n" "$(date '+%H:%M:%S')" "$*"
}

pass() {
  printf "PASS: %s\n" "$*"
  pass_count=$((pass_count + 1))
}

fail() {
  printf "FAIL: %s\n" "$*"
  fail_count=$((fail_count + 1))
}

write_payloads() {
  cat > "${OUT_DIR}/payload-small.json" <<EOF_JSON
{
  "model": "${MODEL}",
  "messages": [
    {
      "role": "user",
      "content": "ok"
    }
  ],
  "max_tokens": 1,
  "stream": false
}
EOF_JSON

  cat > "${OUT_DIR}/payload-token.json" <<EOF_JSON
{
  "model": "${MODEL}",
  "messages": [
    {
      "role": "user",
      "content": "Write about bananas in short words."
    }
  ],
  "max_tokens": 60,
  "stream": false
}
EOF_JSON

  cat > "${OUT_DIR}/payload-slow.json" <<EOF_JSON
{
  "model": "${MODEL}",
  "messages": [
    {
      "role": "user",
      "content": "Write a detailed but concise answer about rate limiting."
    }
  ],
  "max_tokens": 80,
  "stream": false
}
EOF_JSON

  cat > "${OUT_DIR}/payload-stream.json" <<EOF_JSON
{
  "model": "${MODEL}",
  "messages": [
    {
      "role": "user",
      "content": "Count from 1 to 80 with short formatting."
    }
  ],
  "max_tokens": 80,
  "stream": true
}
EOF_JSON
}

http_code_from_meta() {
  local meta_file="$1"
  awk -F= '/^http_code=/{print $2}' "$meta_file" | tail -n 1
}

time_from_meta() {
  local meta_file="$1"
  awk -F= '/^time_total=/{print $2}' "$meta_file" | tail -n 1
}

error_message_from_body() {
  local body="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r '.error.message // ""' "$body" 2>/dev/null || true
  fi
}

wait_for_fresh_fixed_window() {
  local label="$1"
  local sec wait_seconds
  sec="$(date +%S)"
  wait_seconds=$((65 - 10#$sec))
  if (( wait_seconds < 5 )); then
    wait_seconds=$((wait_seconds + 60))
  fi
  log "Wait ${wait_seconds}s for fresh fixed-minute window before ${label}"
  sleep "$wait_seconds"
}

send_chat() {
  local payload="$1"
  local body="$2"
  local headers="$3"
  local meta="$4"

  curl -sS \
    -D "$headers" \
    -o "$body" \
    -w "http_code=%{http_code}\ntime_total=%{time_total}\n" \
    "${BASE_URL}/chat/completions" \
    -H "$AUTH_HEADER" \
    -H "$JSON_HEADER" \
    -d @"$payload" > "$meta" || true
}

print_response_summary() {
  local label="$1"
  local body="$2"
  local headers="$3"
  local meta="$4"
  local code elapsed
  code="$(http_code_from_meta "$meta")"
  elapsed="$(time_from_meta "$meta")"

  printf "%s HTTP %s time=%ss\n" "$label" "$code" "$elapsed"

  if command -v jq >/dev/null 2>&1; then
    jq -c '{usage: .usage, error: .error}' "$body" 2>/dev/null || true
  fi

  grep -iE '^(retry-after|x-ratelimit|ratelimit)' "$headers" 2>/dev/null || true
}

precheck() {
  log "Pre-check /models"
  local body="${OUT_DIR}/models.body.json"
  local headers="${OUT_DIR}/models.headers.txt"
  local meta="${OUT_DIR}/models.meta.txt"

  curl -sS \
    -D "$headers" \
    -o "$body" \
    -w "http_code=%{http_code}\ntime_total=%{time_total}\n" \
    "${BASE_URL}/models" \
    -H "$AUTH_HEADER" > "$meta" || true

  print_response_summary "models" "$body" "$headers" "$meta"

  local code
  code="$(http_code_from_meta "$meta")"
  if [[ "$code" =~ ^[23][0-9][0-9]$ ]]; then
    pass "/models reachable"
  elif [[ "$code" == "401" || "$code" == "403" ]]; then
    fail "/models auth failed: HTTP ${code}"
  elif [[ "$code" == "000" ]]; then
    fail "/models unreachable: curl HTTP 000"
  else
    printf "WARN: /models returned HTTP %s; continuing because chat endpoint may still be testable\n" "$code"
  fi
}

test_rpm() {
  wait_for_fresh_fixed_window "RPM test"
  log "RPM test: expect first ${RPM_LIMIT} requests allowed, request $((RPM_LIMIT + 1)) limited"
  local limited_seen=0
  local unexpected_early_429=0

  for i in $(seq 1 $((RPM_LIMIT + 1))); do
    local body="${OUT_DIR}/rpm-${i}.body.json"
    local headers="${OUT_DIR}/rpm-${i}.headers.txt"
    local meta="${OUT_DIR}/rpm-${i}.meta.txt"

    send_chat "${OUT_DIR}/payload-small.json" "$body" "$headers" "$meta"
    print_response_summary "rpm-${i}" "$body" "$headers" "$meta"

    local code
    code="$(http_code_from_meta "$meta")"

    if (( i <= RPM_LIMIT )) && [[ "$code" == "429" ]]; then
      unexpected_early_429=1
    fi

    if (( i == RPM_LIMIT + 1 )) && [[ "$code" == "429" ]]; then
      limited_seen=1
    fi
  done

  if (( unexpected_early_429 == 0 )); then
    pass "RPM did not block before ${RPM_LIMIT} requests"
  else
    fail "RPM blocked before ${RPM_LIMIT} requests"
  fi

  if (( limited_seen == 1 )); then
    pass "RPM blocked request $((RPM_LIMIT + 1))"
  else
    fail "RPM did not block request $((RPM_LIMIT + 1)); inspect ${OUT_DIR}/rpm-*"
  fi
}

test_reset() {
  log "Reset test: wait 61 seconds"
  sleep 61

  local body="${OUT_DIR}/reset.body.json"
  local headers="${OUT_DIR}/reset.headers.txt"
  local meta="${OUT_DIR}/reset.meta.txt"

  send_chat "${OUT_DIR}/payload-small.json" "$body" "$headers" "$meta"
  print_response_summary "reset" "$body" "$headers" "$meta"

  local code
  code="$(http_code_from_meta "$meta")"
  if [[ "$code" != "429" && "$code" != "000" ]]; then
    pass "Limiter reset after 61 seconds"
  else
    fail "Limiter did not reset after 61 seconds: HTTP ${code}"
  fi
}

test_tpm() {
  wait_for_fresh_fixed_window "TPM test"
  log "TPM test: limit ${TPM_LIMIT} tokens/min; behavior may use estimated prompt+max_tokens or actual usage"
  local limited_seen=0
  local total_tokens=0

  for i in $(seq 1 4); do
    local body="${OUT_DIR}/tpm-${i}.body.json"
    local headers="${OUT_DIR}/tpm-${i}.headers.txt"
    local meta="${OUT_DIR}/tpm-${i}.meta.txt"

    send_chat "${OUT_DIR}/payload-token.json" "$body" "$headers" "$meta"
    print_response_summary "tpm-${i}" "$body" "$headers" "$meta"

    local code
    code="$(http_code_from_meta "$meta")"
    if [[ "$code" == "429" ]]; then
      limited_seen=1
      break
    fi

    if command -v jq >/dev/null 2>&1; then
      local usage_tokens
      usage_tokens="$(jq -r '.usage.total_tokens // 0' "$body" 2>/dev/null || echo 0)"
      if [[ "$usage_tokens" =~ ^[0-9]+$ ]]; then
        total_tokens=$((total_tokens + usage_tokens))
      fi
    fi
  done

  printf "Observed total_tokens_from_usage=%s\n" "$total_tokens"

  if (( limited_seen == 1 )); then
    pass "TPM produced 429 when token budget exceeded/estimated"
  elif (( total_tokens > TPM_LIMIT )); then
    fail "TPM allowed observed usage ${total_tokens} > ${TPM_LIMIT}"
  else
    printf "WARN: TPM inconclusive; observed usage <= %s and no 429. Inspect responses/logs in %s\n" "$TPM_LIMIT" "$OUT_DIR"
  fi
}

test_concurrency() {
  wait_for_fresh_fixed_window "concurrency test"
  log "Concurrency test: concurrent=${CONCURRENT_LIMIT}; launch 2 requests"

  local body1="${OUT_DIR}/concurrent-1.body.json"
  local headers1="${OUT_DIR}/concurrent-1.headers.txt"
  local meta1="${OUT_DIR}/concurrent-1.meta.txt"
  local body2="${OUT_DIR}/concurrent-2.body.json"
  local headers2="${OUT_DIR}/concurrent-2.headers.txt"
  local meta2="${OUT_DIR}/concurrent-2.meta.txt"

  send_chat "${OUT_DIR}/payload-slow.json" "$body1" "$headers1" "$meta1" &
  local pid1=$!

  sleep 0.2

  send_chat "${OUT_DIR}/payload-slow.json" "$body2" "$headers2" "$meta2" &
  local pid2=$!

  wait "$pid1" || true
  wait "$pid2" || true

  print_response_summary "concurrent-1" "$body1" "$headers1" "$meta1"
  print_response_summary "concurrent-2" "$body2" "$headers2" "$meta2"

  local code1 code2 time1 time2
  code1="$(http_code_from_meta "$meta1")"
  code2="$(http_code_from_meta "$meta2")"
  time1="$(time_from_meta "$meta1")"
  time2="$(time_from_meta "$meta2")"

  local err1 err2
  err1="$(error_message_from_body "$body1")"
  err2="$(error_message_from_body "$body2")"

  if [[ "$err1" == *"concurrency"* || "$err2" == *"concurrency"* ]]; then
    pass "Concurrency rejected one overlapping request with concurrency limit"
  elif [[ "$err1" == *"tpm"* || "$err2" == *"tpm"* ]]; then
    fail "Concurrency test blocked by TPM instead of concurrency; payload/window still interfering"
  elif [[ "$code1" != "000" && "$code2" != "000" ]]; then
    printf "WARN: both concurrent requests completed. If design queues, compare time_total: A=%ss B=%ss. Inspect server logs for in-flight count.\n" "$time1" "$time2"
  else
    fail "Concurrency test had unreachable/failed request: A=${code1} B=${code2}"
  fi
}

test_stream_concurrency() {
  wait_for_fresh_fixed_window "stream concurrency test"
  log "Optional stream concurrency test"
  local stream_body="${OUT_DIR}/stream.body.txt"
  local stream_headers="${OUT_DIR}/stream.headers.txt"
  local stream_meta="${OUT_DIR}/stream.meta.txt"
  local second_body="${OUT_DIR}/stream-second.body.json"
  local second_headers="${OUT_DIR}/stream-second.headers.txt"
  local second_meta="${OUT_DIR}/stream-second.meta.txt"

  curl -sS -N \
    -D "$stream_headers" \
    -o "$stream_body" \
    -w "http_code=%{http_code}\ntime_total=%{time_total}\n" \
    "${BASE_URL}/chat/completions" \
    -H "$AUTH_HEADER" \
    -H "$JSON_HEADER" \
    -d @"${OUT_DIR}/payload-stream.json" > "$stream_meta" &
  local stream_pid=$!

  sleep 0.5

  send_chat "${OUT_DIR}/payload-small.json" "$second_body" "$second_headers" "$second_meta"

  wait "$stream_pid" || true

  print_response_summary "stream" "$stream_body" "$stream_headers" "$stream_meta"
  print_response_summary "stream-second" "$second_body" "$second_headers" "$second_meta"

  local second_code
  second_code="$(http_code_from_meta "$second_meta")"

  local stream_err second_err
  stream_err="$(error_message_from_body "$stream_body")"
  second_err="$(error_message_from_body "$second_body")"

  if [[ "$stream_err" == *"concurrency"* || "$second_err" == *"concurrency"* ]]; then
    pass "Stream held concurrency slot and second request was limited by concurrency"
  elif [[ "$stream_err" == *"tpm"* || "$second_err" == *"tpm"* ]]; then
    fail "Stream concurrency test blocked by TPM instead of concurrency; payload/window still interfering"
  elif [[ "$second_code" != "000" ]]; then
    printf "WARN: stream-second not limited. If design queues or stream completed too fast, inspect timings/logs.\n"
  else
    fail "Stream concurrency second request failed unreachable"
  fi
}

summary() {
  log "Summary"
  printf "Output dir: %s\n" "$OUT_DIR"
  printf "PASS: %s\n" "$pass_count"
  printf "FAIL: %s\n" "$fail_count"

  if (( fail_count > 0 )); then
    exit 1
  fi
}

main() {
  cat <<EOF_INFO
NexusRouter rate-limit test
BASE_URL=${BASE_URL}
MODEL=${MODEL}
RPM_LIMIT=${RPM_LIMIT}
TPM_LIMIT=${TPM_LIMIT}
CONCURRENT_LIMIT=${CONCURRENT_LIMIT}
OUT_DIR=${OUT_DIR}
EOF_INFO

  write_payloads
  precheck
  test_rpm
  test_reset
  test_tpm
  test_concurrency
  test_stream_concurrency

  summary
}

main "$@"