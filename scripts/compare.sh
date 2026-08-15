#!/usr/bin/env bash
# Differential comparison against the installed reference ccusage binary.
#
# Runs a matrix of commands on real user data with a pinned environment and
# byte-compares stdout, stderr, and exit codes. Time-dependent outputs
# (statusline, blocks --active, anything depending on "now") are excluded.
#
# Usage: scripts/compare.sh [go-binary] [reference-binary]
set -uo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
GO_BIN=${1:-$REPO/bin/token-usage}
REF_BIN=${2:-/home/wujunwei/.nvm/versions/node/v24.17.0/lib/node_modules/ccusage/node_modules/@ccusage/ccusage-linux-x64/bin/ccusage}

if [ ! -x "$GO_BIN" ]; then
  mkdir -p "$REPO/bin"
  (cd "$REPO" && go build -o bin/token-usage ./cmd/token-usage) || exit 1
  GO_BIN=$REPO/bin/token-usage
fi

export NO_COLOR=1 TZ=UTC LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=dumb COLUMNS=100

pass=0
fail=0
failed_cases=()

run_case() {
  local label="$1"; shift
  "$REF_BIN" "$@" >/tmp/cmp-ref.out 2>/tmp/cmp-ref.err; local ref_code=$?
  "$GO_BIN" "$@" >/tmp/cmp-go.out 2>/tmp/cmp-go.err; local go_code=$?
  if [ "$ref_code" = "$go_code" ] && cmp -s /tmp/cmp-ref.out /tmp/cmp-go.out && cmp -s /tmp/cmp-ref.err /tmp/cmp-go.err; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    failed_cases+=("$label (exit $ref_code/$go_code)")
    if [ "${VERBOSE:-0}" = "1" ]; then
      echo "--- FAIL: $label"
      diff /tmp/cmp-ref.out /tmp/cmp-go.out | head -10
      diff /tmp/cmp-ref.err /tmp/cmp-go.err | head -5
    fi
  fi
}

# All pricing-sensitive cases run offline: both binaries then use their
# embedded snapshots (kept in sync by scripts/update-pricing.sh), so the
# comparison is deterministic. Live-fetch comparisons are inherently
# time-variable (upstream pricing data changes between runs).
for cmd in daily weekly monthly session; do
  run_case "$cmd" "$cmd" --offline
  run_case "$cmd-json" "$cmd" --json --offline
done
run_case bare --offline
run_case sections "daily" --sections daily,weekly,monthly,session --offline
run_case sections-json "daily" --sections daily,weekly,monthly,session --json --offline
run_case by-agent "daily" --by-agent --json --offline

# Claude-scoped reports
for cmd in daily weekly monthly session; do
  run_case "claude-$cmd" claude "$cmd" --offline
  run_case "claude-$cmd-json" claude "$cmd" --json --offline
done
run_case claude-daily-breakdown claude daily --breakdown --offline
run_case claude-daily-since claude daily --since 20260101 --offline
run_case claude-weekly-monday claude weekly --start-of-week monday --offline

# Agent-scoped reports (only commands the reference binary actually ships)
for agent in codex opencode amp droid codebuff hermes pi goose kilo copilot gemini kimi qwen openclaw; do
  if "$REF_BIN" --help 2>&1 | grep -qw "$agent" && "$GO_BIN" --help 2>&1 | grep -qw "$agent"; then
    run_case "$agent-daily" "$agent" daily --offline
    run_case "$agent-daily-json" "$agent" daily --json --offline
  fi
done

# Legacy/error surface
run_case legacy-colon codex:daily
run_case report-flag --daily
run_case agent-flag daily --agent claude
run_case last-conflict claude daily --last 1 --since 20260101
run_case jq claude daily --jq .totals.totalCost --offline
run_case version --version
run_case help-error --bogus

echo
echo "PASS: $pass  FAIL: $fail"
if [ "$fail" -gt 0 ]; then
  printf 'failed: %s\n' "${failed_cases[@]}"
  exit 1
fi
