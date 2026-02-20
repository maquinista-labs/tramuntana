#!/usr/bin/env bash
# =============================================================================
# Tramuntana ↔ Minuano — E2E Integration Tests (I-09, I-10, I-11)
#
# Tests the full Telegram → Tramuntana → Minuano → Claude → tmux pipeline.
#
# Before running:
#   1. Set TELEGRAM_BOT_TOKEN and ALLOWED_USERS in your shell or in .env
#   2. Start tramuntana:  tramuntana serve
#   3. Start PostgreSQL:  minuano up   (the script does this, but it must be installed)
#   4. Ensure tmux is installed
#
# The script builds both binaries, sets up the DB, and creates test tasks.
# Telegram commands (/pick, /auto, /batch) must be sent manually in the
# pauses between steps.
#
# Usage:
#   ./validation/e2e.sh [--minuano-dir DIR] [--db URL] [--project NAME]
#
# Defaults:
#   --minuano-dir /home/otavio/code/minuano
#   --db postgres://minuano:minuano@localhost:5432/minuanodb?sslmode=disable
#   --project e2e-test
# =============================================================================
set -uo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

MINUANO_DIR="/home/otavio/code/minuano"
DB_URL="postgres://minuano:minuano@localhost:5432/minuanodb?sslmode=disable"
E2E_PROJECT="e2e-test"

# Parse flags.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --minuano-dir) MINUANO_DIR="$2"; shift 2 ;;
    --db) DB_URL="$2"; shift 2 ;;
    --project) E2E_PROJECT="$2"; shift 2 ;;
    *) echo "Unknown flag: $1"; exit 1 ;;
  esac
done

# --- Helpers ---

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

PASS=0
FAIL=0

pass() {
  echo -e "  ${GREEN}PASS${NC}: $1"
  PASS=$((PASS + 1))
}

fail() {
  echo -e "  ${RED}FAIL${NC}: $1"
  FAIL=$((FAIL + 1))
}

section() {
  echo ""
  echo -e "${BLUE}=== $1 ===${NC}"
}

wait_for_user() {
  echo ""
  echo -e "${BOLD}${YELLOW}>>> $1${NC}"
  echo -e "${YELLOW}    Press ENTER when done...${NC}"
  read -r
}

run_minuano() {
  "$MINUANO_BIN" --db "$DB_URL" "$@"
}

# --- Environment ---

section "Environment"

# Source .env if present and TELEGRAM_BOT_TOKEN is not already set.
if [ -z "${TELEGRAM_BOT_TOKEN:-}" ] && [ -f "$PROJECT_ROOT/.env" ]; then
  set -a
  source "$PROJECT_ROOT/.env"
  set +a
  echo -e "  ${GREEN}OK${NC}: loaded .env from $PROJECT_ROOT/.env"
fi

# Validate required env vars before doing anything else.
MISSING=()
[ -z "${TELEGRAM_BOT_TOKEN:-}" ] && MISSING+=("TELEGRAM_BOT_TOKEN")
[ -z "${ALLOWED_USERS:-}" ] && MISSING+=("ALLOWED_USERS")

if [ ${#MISSING[@]} -gt 0 ]; then
  echo -e "  ${RED}FATAL${NC}: missing required environment variables: ${MISSING[*]}"
  echo ""
  echo "  Set them in your shell or in $PROJECT_ROOT/.env:"
  echo "    TELEGRAM_BOT_TOKEN=<your-bot-token-from-botfather>"
  echo "    ALLOWED_USERS=<your-telegram-user-id>"
  exit 1
fi

echo -e "  ${GREEN}OK${NC}: TELEGRAM_BOT_TOKEN is set"
echo -e "  ${GREEN}OK${NC}: ALLOWED_USERS is set"

# Set MINUANO_BIN after sourcing .env so it doesn't get overwritten.
MINUANO_BIN="$MINUANO_DIR/minuano"

# --- Prerequisites ---

section "Prerequisites"

if [ ! -x "$MINUANO_BIN" ]; then
  echo "Building minuano binary..."
  if ! (cd "$MINUANO_DIR" && go build -o "$MINUANO_BIN" ./cmd/minuano) 2>&1; then
    echo -e "  ${RED}FATAL${NC}: cannot build minuano (go build failed in $MINUANO_DIR)"
    exit 1
  fi
fi

if [ -x "$MINUANO_BIN" ]; then
  echo -e "  ${GREEN}OK${NC}: minuano binary at $MINUANO_BIN"
else
  echo -e "  ${RED}FATAL${NC}: minuano binary not found at $MINUANO_BIN"
  echo "  Try: cd $MINUANO_DIR && go build -o minuano ./cmd/minuano"
  exit 1
fi

if go build -o "$PROJECT_ROOT/tramuntana" "$PROJECT_ROOT/cmd/tramuntana" 2>&1; then
  echo -e "  ${GREEN}OK${NC}: tramuntana binary built"
else
  echo -e "  ${RED}FATAL${NC}: cannot build tramuntana"
  exit 1
fi

# --- Database Setup ---

section "Database Setup"

run_minuano up 2>&1 | tail -1
sleep 2

if psql "$DB_URL" -c "SELECT 1" >/dev/null 2>&1; then
  echo -e "  ${GREEN}OK${NC}: PostgreSQL reachable"
else
  echo -e "  ${RED}FATAL${NC}: PostgreSQL not reachable at $DB_URL"
  exit 1
fi

run_minuano migrate 2>&1 | tail -1

# Clean up any previous e2e test data.
psql "$DB_URL" -c "
  DELETE FROM task_context WHERE task_id IN (SELECT id FROM tasks WHERE project_id='$E2E_PROJECT');
  DELETE FROM task_deps WHERE task_id IN (SELECT id FROM tasks WHERE project_id='$E2E_PROJECT')
    OR dep_id IN (SELECT id FROM tasks WHERE project_id='$E2E_PROJECT');
  DELETE FROM tasks WHERE project_id='$E2E_PROJECT';
" >/dev/null 2>&1
echo -e "  ${GREEN}OK${NC}: cleaned previous e2e data"

# =============================================================================
# I-09 — Pick Mode E2E
# =============================================================================

section "I-09: Pick Mode E2E"

echo "Creating test task for pick mode..."
PICK_ADD_OUTPUT=$(run_minuano add "E2E Pick Test" \
  --body "Create a file called /tmp/test-pick.txt with the content 'hello'" \
  --project "$E2E_PROJECT" --priority 8 2>&1)
echo "  $PICK_ADD_OUTPUT"

PICK_TASK_ID=$(psql "$DB_URL" -t -A -c \
  "SELECT id FROM tasks WHERE title='E2E Pick Test' AND project_id='$E2E_PROJECT' LIMIT 1")

if [ -n "$PICK_TASK_ID" ]; then
  pass "pick test task created: $PICK_TASK_ID"
else
  fail "pick test task not created"
fi

PICK_STATUS=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$PICK_TASK_ID'")
if [ "$PICK_STATUS" = "ready" ]; then
  pass "pick test task status is 'ready'"
else
  fail "pick test task should be ready, got '$PICK_STATUS'"
fi

echo ""
echo -e "${BOLD}Manual steps for I-09:${NC}"
echo "  1. Ensure tramuntana serve is running"
echo "  2. In Telegram, create a topic and bind to a directory"
echo "  3. Send: /project $E2E_PROJECT"
echo "  4. Send: /tasks"
echo "     -> Verify 'E2E Pick Test' appears"
echo "  5. Send: /pick $PICK_TASK_ID"
echo "     -> Verify Claude receives the prompt and works on it"
echo "     -> Verify Claude calls minuano-done"

wait_for_user "Complete the pick mode test in Telegram, then press ENTER to verify"

# Verify pick results.
PICK_FINAL_STATUS=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$PICK_TASK_ID'")
if [ "$PICK_FINAL_STATUS" = "done" ]; then
  pass "I-09: pick task moved to 'done'"
else
  fail "I-09: pick task status is '$PICK_FINAL_STATUS', expected 'done'"
fi

if [ -f /tmp/test-pick.txt ]; then
  PICK_CONTENT=$(cat /tmp/test-pick.txt)
  if echo "$PICK_CONTENT" | grep -q "hello"; then
    pass "I-09: /tmp/test-pick.txt contains 'hello'"
  else
    fail "I-09: /tmp/test-pick.txt wrong content: $PICK_CONTENT"
  fi
  rm -f /tmp/test-pick.txt
else
  fail "I-09: /tmp/test-pick.txt not created"
fi

# =============================================================================
# I-10 — Auto Mode E2E
# =============================================================================

section "I-10: Auto Mode E2E"

echo "Creating test tasks for auto mode..."
AUTO_A_OUTPUT=$(run_minuano add "E2E Auto Task A" \
  --body "Create a file called /tmp/test-auto-a.txt with the content 'task-a-done'" \
  --project "$E2E_PROJECT" --priority 8 2>&1)
echo "  $AUTO_A_OUTPUT"

AUTO_A_ID=$(psql "$DB_URL" -t -A -c \
  "SELECT id FROM tasks WHERE title='E2E Auto Task A' AND project_id='$E2E_PROJECT' LIMIT 1")

AUTO_B_OUTPUT=$(run_minuano add "E2E Auto Task B" \
  --body "Create a file called /tmp/test-auto-b.txt with the content 'task-b-done'" \
  --project "$E2E_PROJECT" --after "$AUTO_A_ID" --priority 6 2>&1)
echo "  $AUTO_B_OUTPUT"

AUTO_B_ID=$(psql "$DB_URL" -t -A -c \
  "SELECT id FROM tasks WHERE title='E2E Auto Task B' AND project_id='$E2E_PROJECT' LIMIT 1")

if [ -n "$AUTO_A_ID" ] && [ -n "$AUTO_B_ID" ]; then
  pass "auto mode tasks created: A=$AUTO_A_ID, B=$AUTO_B_ID"
else
  fail "auto mode tasks not created"
fi

AUTO_A_STATUS=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$AUTO_A_ID'")
AUTO_B_STATUS=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$AUTO_B_ID'")

if [ "$AUTO_A_STATUS" = "ready" ]; then
  pass "Task A is 'ready' (no deps)"
else
  fail "Task A should be ready, got '$AUTO_A_STATUS'"
fi

if [ "$AUTO_B_STATUS" = "pending" ]; then
  pass "Task B is 'pending' (depends on A)"
else
  fail "Task B should be pending, got '$AUTO_B_STATUS'"
fi

echo ""
echo -e "${BOLD}Manual steps for I-10:${NC}"
echo "  1. In the same Telegram topic (already bound to $E2E_PROJECT)"
echo "  2. Send: /auto"
echo "     -> Verify Claude claims Task A, completes it, calls minuano-done"
echo "     -> Verify Task B becomes ready (trigger cascade)"
echo "     -> Verify Claude claims Task B, completes it, calls minuano-done"
echo "     -> Verify Claude runs minuano-claim --project, gets empty, stops"
echo "     -> Verify Claude returns to interactive mode"

wait_for_user "Complete the auto mode test in Telegram, then press ENTER to verify"

# Verify auto results.
AUTO_A_FINAL=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$AUTO_A_ID'")
AUTO_B_FINAL=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$AUTO_B_ID'")

if [ "$AUTO_A_FINAL" = "done" ]; then
  pass "I-10: Task A moved to 'done'"
else
  fail "I-10: Task A status is '$AUTO_A_FINAL', expected 'done'"
fi

if [ "$AUTO_B_FINAL" = "done" ]; then
  pass "I-10: Task B moved to 'done' (cascade worked)"
else
  fail "I-10: Task B status is '$AUTO_B_FINAL', expected 'done'"
fi

if [ -f /tmp/test-auto-a.txt ]; then
  pass "I-10: /tmp/test-auto-a.txt created"
  rm -f /tmp/test-auto-a.txt
else
  fail "I-10: /tmp/test-auto-a.txt not created"
fi

if [ -f /tmp/test-auto-b.txt ]; then
  pass "I-10: /tmp/test-auto-b.txt created"
  rm -f /tmp/test-auto-b.txt
else
  fail "I-10: /tmp/test-auto-b.txt not created"
fi

# =============================================================================
# I-11 — Batch Mode E2E
# =============================================================================

section "I-11: Batch Mode E2E"

echo "Creating test tasks for batch mode..."
BATCH1_OUTPUT=$(run_minuano add "E2E Batch 1" \
  --body "Create a file called /tmp/test-batch-1.txt with the content 'batch-1'" \
  --project "$E2E_PROJECT" --priority 5 2>&1)
echo "  $BATCH1_OUTPUT"

BATCH2_OUTPUT=$(run_minuano add "E2E Batch 2" \
  --body "Create a file called /tmp/test-batch-2.txt with the content 'batch-2'" \
  --project "$E2E_PROJECT" --priority 5 2>&1)
echo "  $BATCH2_OUTPUT"

BATCH3_OUTPUT=$(run_minuano add "E2E Batch 3" \
  --body "Create a file called /tmp/test-batch-3.txt with the content 'batch-3'" \
  --project "$E2E_PROJECT" --priority 5 2>&1)
echo "  $BATCH3_OUTPUT"

BATCH1_ID=$(psql "$DB_URL" -t -A -c \
  "SELECT id FROM tasks WHERE title='E2E Batch 1' AND project_id='$E2E_PROJECT' LIMIT 1")
BATCH2_ID=$(psql "$DB_URL" -t -A -c \
  "SELECT id FROM tasks WHERE title='E2E Batch 2' AND project_id='$E2E_PROJECT' LIMIT 1")
BATCH3_ID=$(psql "$DB_URL" -t -A -c \
  "SELECT id FROM tasks WHERE title='E2E Batch 3' AND project_id='$E2E_PROJECT' LIMIT 1")

if [ -n "$BATCH1_ID" ] && [ -n "$BATCH2_ID" ] && [ -n "$BATCH3_ID" ]; then
  pass "batch tasks created: 1=$BATCH1_ID, 2=$BATCH2_ID, 3=$BATCH3_ID"
else
  fail "batch tasks not created"
fi

echo ""
echo -e "${BOLD}Manual steps for I-11:${NC}"
echo "  1. In the same Telegram topic (already bound to $E2E_PROJECT)"
echo "  2. Send: /batch $BATCH1_ID $BATCH3_ID"
echo "     -> Note: skipping Batch 2 ($BATCH2_ID) intentionally"
echo "     -> Verify Claude works Batch 1, then Batch 3 (in order)"
echo "     -> Verify Claude calls minuano-pick for each"
echo "     -> Verify Claude calls minuano-done for each"
echo "     -> Verify Claude returns to interactive mode"

wait_for_user "Complete the batch mode test in Telegram, then press ENTER to verify"

# Verify batch results.
BATCH1_FINAL=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$BATCH1_ID'")
BATCH2_FINAL=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$BATCH2_ID'")
BATCH3_FINAL=$(psql "$DB_URL" -t -A -c "SELECT status FROM tasks WHERE id='$BATCH3_ID'")

if [ "$BATCH1_FINAL" = "done" ]; then
  pass "I-11: Batch 1 moved to 'done'"
else
  fail "I-11: Batch 1 status is '$BATCH1_FINAL', expected 'done'"
fi

if [ "$BATCH2_FINAL" = "ready" ]; then
  pass "I-11: Batch 2 still 'ready' (not touched)"
else
  fail "I-11: Batch 2 status is '$BATCH2_FINAL', expected 'ready' (untouched)"
fi

if [ "$BATCH3_FINAL" = "done" ]; then
  pass "I-11: Batch 3 moved to 'done'"
else
  fail "I-11: Batch 3 status is '$BATCH3_FINAL', expected 'done'"
fi

if [ -f /tmp/test-batch-1.txt ]; then
  pass "I-11: /tmp/test-batch-1.txt created"
  rm -f /tmp/test-batch-1.txt
else
  fail "I-11: /tmp/test-batch-1.txt not created"
fi

if [ ! -f /tmp/test-batch-2.txt ]; then
  pass "I-11: /tmp/test-batch-2.txt NOT created (correct — skipped)"
else
  fail "I-11: /tmp/test-batch-2.txt should NOT exist (task was skipped)"
  rm -f /tmp/test-batch-2.txt
fi

if [ -f /tmp/test-batch-3.txt ]; then
  pass "I-11: /tmp/test-batch-3.txt created"
  rm -f /tmp/test-batch-3.txt
else
  fail "I-11: /tmp/test-batch-3.txt not created"
fi

# =============================================================================
# Cleanup
# =============================================================================

section "Cleanup"

echo "Cleaning up e2e test data..."
psql "$DB_URL" -c "
  DELETE FROM task_context WHERE task_id IN (SELECT id FROM tasks WHERE project_id='$E2E_PROJECT');
  DELETE FROM task_deps WHERE task_id IN (SELECT id FROM tasks WHERE project_id='$E2E_PROJECT')
    OR dep_id IN (SELECT id FROM tasks WHERE project_id='$E2E_PROJECT');
  DELETE FROM tasks WHERE project_id='$E2E_PROJECT';
" >/dev/null 2>&1
pass "e2e test data cleaned up"

rm -f "$PROJECT_ROOT/tramuntana"

# --- Summary ---

echo ""
echo "============================================"
TOTAL=$((PASS + FAIL))
echo -e "  ${GREEN}PASS: $PASS${NC}  ${RED}FAIL: $FAIL${NC}  TOTAL: $TOTAL"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
  echo -e "${RED}E2E TESTS FAILED${NC}"
  exit 1
else
  echo -e "${GREEN}E2E TESTS PASSED${NC}"
  exit 0
fi
