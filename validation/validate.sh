#!/usr/bin/env bash
# =============================================================================
# Tramuntana — Full Validation Script
#
# Validates all 26 tasks + integration tasks I-07 and I-08.
# For E2E tests (I-09..I-11), run validation/e2e.sh separately.
#
# Usage:
#   ./validation/validate.sh
# =============================================================================
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

PASS=0
FAIL=0
SKIP=0
BINARY="$PROJECT_ROOT/tramuntana"

# --- Helpers ---

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

pass() {
  echo -e "  ${GREEN}PASS${NC}: $1"
  PASS=$((PASS + 1))
}

fail() {
  echo -e "  ${RED}FAIL${NC}: $1"
  FAIL=$((FAIL + 1))
}

skip() {
  echo -e "  ${YELLOW}SKIP${NC}: $1"
  SKIP=$((SKIP + 1))
}

section() {
  echo ""
  echo -e "${BLUE}=== $1 ===${NC}"
}

# --- Prerequisites ---

section "Prerequisites"

command -v go >/dev/null 2>&1 && pass "go installed" || fail "go not found"
command -v tmux >/dev/null 2>&1 && pass "tmux installed" || fail "tmux not found"

# --- Phase 0: Build ---

section "Phase 0: Build & Vet"

if go vet ./... 2>&1; then
  pass "go vet ./..."
else
  fail "go vet ./..."
fi

if go build -o "$BINARY" ./cmd/tramuntana 2>&1; then
  pass "go build binary"
else
  fail "go build binary"
  echo "FATAL: binary not built, cannot continue"
  exit 1
fi

# --- Phase 1: Unit Tests ---

section "Phase 1: Unit Tests"

TEST_OUTPUT=$(go test ./... -count=1 2>&1)
if [ $? -eq 0 ]; then
  pass "go test ./... passes"
else
  fail "go test ./... failed"
  echo "$TEST_OUTPUT" | tail -20
fi

# --- Phase 2: Binary Smoke Tests ---

section "Phase 2: Binary Smoke Tests"

VERSION_OUTPUT=$("$BINARY" version 2>&1)
if echo "$VERSION_OUTPUT" | grep -q "tramuntana v"; then
  pass "tramuntana version outputs version"
else
  fail "tramuntana version output unexpected"
fi

HELP_OUTPUT=$("$BINARY" --help 2>&1)
if echo "$HELP_OUTPUT" | grep -q "serve"; then
  pass "help shows serve command"
else
  fail "help missing serve command"
fi

if echo "$HELP_OUTPUT" | grep -q "hook"; then
  pass "help shows hook command"
else
  fail "help missing hook command"
fi

# --- Phase 3: Package Structure (Tasks 01-07) ---

section "Phase 3: Package Structure"

PACKAGES=(
  "internal/config"
  "internal/tmux"
  "internal/state"
  "internal/bot"
  "internal/monitor"
  "internal/queue"
  "internal/render"
  "internal/minuano"
  "hook"
)
for pkg in "${PACKAGES[@]}"; do
  if [ -d "$PROJECT_ROOT/$pkg" ]; then
    pass "package $pkg exists"
  else
    fail "package $pkg missing"
  fi
done

# --- Phase 4: Key Files (Tasks 08-26) ---

section "Phase 4: Key Files"

FILES=(
  "internal/bot/bot.go"
  "internal/bot/handlers.go"
  "internal/bot/commands.go"
  "internal/bot/directory_browser.go"
  "internal/bot/window_picker.go"
  "internal/bot/screenshot.go"
  "internal/bot/history.go"
  "internal/bot/bash_capture.go"
  "internal/bot/interactive.go"
  "internal/bot/minuano_commands.go"
  "internal/bot/recovery.go"
  "internal/bot/status.go"
  "internal/monitor/transcript.go"
  "internal/monitor/terminal.go"
  "internal/monitor/monitor.go"
  "internal/render/format.go"
  "internal/render/markdown.go"
  "internal/render/screenshot.go"
  "internal/queue/queue.go"
  "internal/minuano/bridge.go"
  "hook/hook.go"
)
for f in "${FILES[@]}"; do
  if [ -f "$PROJECT_ROOT/$f" ]; then
    pass "$f exists"
  else
    fail "$f missing"
  fi
done

# --- Phase 5: Integration I-07 — Bridge uses --json ---

section "Phase 5: Integration I-07 (Bridge JSON)"

if grep -q '"--json"' internal/minuano/bridge.go; then
  pass "bridge.go uses --json flag"
else
  fail "bridge.go missing --json flag"
fi

if grep -q 'json.Unmarshal' internal/minuano/bridge.go; then
  pass "bridge.go unmarshals JSON"
else
  fail "bridge.go missing JSON unmarshalling"
fi

if grep -q 'type Task struct' internal/minuano/bridge.go; then
  pass "bridge.go defines Task struct"
else
  fail "bridge.go missing Task struct"
fi

if grep -q 'type TaskDetail struct' internal/minuano/bridge.go; then
  pass "bridge.go defines TaskDetail struct"
else
  fail "bridge.go missing TaskDetail struct"
fi

if grep -q 'json:"id"' internal/minuano/bridge.go; then
  pass "Task struct has JSON tags"
else
  fail "Task struct missing JSON tags"
fi

# --- Phase 6: Integration I-08 — Commands use minuano prompt ---

section "Phase 6: Integration I-08 (Commands + Env Bootstrap)"

if grep -q 'PromptSingle' internal/bot/minuano_commands.go; then
  pass "/pick uses PromptSingle"
else
  fail "/pick missing PromptSingle call"
fi

if grep -q 'PromptAuto' internal/bot/minuano_commands.go; then
  pass "/auto uses PromptAuto"
else
  fail "/auto missing PromptAuto call"
fi

if grep -q 'PromptBatch' internal/bot/minuano_commands.go; then
  pass "/batch uses PromptBatch"
else
  fail "/batch missing PromptBatch call"
fi

if grep -q 'sendPromptToTmux' internal/bot/minuano_commands.go; then
  pass "commands use sendPromptToTmux"
else
  fail "commands missing sendPromptToTmux"
fi

if grep -q 'buildMinuanoEnv' internal/bot/minuano_commands.go; then
  pass "buildMinuanoEnv helper exists"
else
  fail "buildMinuanoEnv helper missing"
fi

if grep -q 'buildMinuanoEnv' internal/bot/directory_browser.go; then
  pass "directory_browser calls buildMinuanoEnv"
else
  fail "directory_browser missing buildMinuanoEnv call"
fi

if grep -q 'DATABASE_URL' internal/bot/minuano_commands.go; then
  pass "buildMinuanoEnv sets DATABASE_URL"
else
  fail "buildMinuanoEnv missing DATABASE_URL"
fi

if grep -q 'AGENT_ID' internal/bot/minuano_commands.go; then
  pass "buildMinuanoEnv sets AGENT_ID"
else
  fail "buildMinuanoEnv missing AGENT_ID"
fi

if grep -q 'MinuanoScriptsDir' internal/config/config.go; then
  pass "config has MinuanoScriptsDir"
else
  fail "config missing MinuanoScriptsDir"
fi

# --- Cleanup ---

rm -f "$BINARY"

# --- Summary ---

echo ""
echo "============================================"
TOTAL=$((PASS + FAIL + SKIP))
echo -e "  ${GREEN}PASS: $PASS${NC}  ${RED}FAIL: $FAIL${NC}  ${YELLOW}SKIP: $SKIP${NC}  TOTAL: $TOTAL"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
  echo -e "${RED}VALIDATION FAILED${NC}"
  exit 1
else
  echo -e "${GREEN}VALIDATION PASSED${NC}"
  exit 0
fi
