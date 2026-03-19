#!/usr/bin/env bash
# Outpost VPN — simple load test script.
# Requires: hey (go install github.com/rakyll/hey@latest)
#
# Usage:
#   OUTPOST_API=http://localhost:8080 OUTPOST_ADMIN_PASS=password ./scripts/loadtest.sh
#
# Options (env vars):
#   OUTPOST_API         — API base URL (default: http://localhost:8080)
#   OUTPOST_ADMIN_USER  — admin username (default: admin)
#   OUTPOST_ADMIN_PASS  — admin password (REQUIRED)
#   LOAD_CONCURRENCY    — concurrent workers (default: 10)
#   LOAD_REQUESTS       — total requests per endpoint (default: 500)
set -euo pipefail

API="${OUTPOST_API:-http://localhost:8080}"
ADMIN_USER="${OUTPOST_ADMIN_USER:-admin}"
ADMIN_PASS="${OUTPOST_ADMIN_PASS:?Set OUTPOST_ADMIN_PASS}"
CONCURRENCY="${LOAD_CONCURRENCY:-10}"
REQUESTS="${LOAD_REQUESTS:-500}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${CYAN}[*]${NC} $*"; }
ok()   { echo -e "${GREEN}[+]${NC} $*"; }
fail() { echo -e "${RED}[x]${NC} $*"; exit 1; }

# Check hey is installed.
if ! command -v hey &>/dev/null; then
  fail "hey not found. Install: go install github.com/rakyll/hey@latest"
fi

# Authenticate.
log "Logging in as $ADMIN_USER..."
LOGIN_RESP=$(curl -s "$API/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")

TOKEN=$(echo "$LOGIN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])" 2>/dev/null) \
  || fail "Login failed: $LOGIN_RESP"
ok "Authenticated (token=${TOKEN:0:20}...)"

AUTH_HEADER="Authorization: Bearer $TOKEN"

echo ""
log "Load test: $REQUESTS requests, $CONCURRENCY concurrency"
echo "============================================================"

# List of endpoints to test.
ENDPOINTS=(
  "GET /api/v1/dashboard/stats"
  "GET /api/v1/users"
  "GET /api/v1/networks"
  "GET /api/v1/devices"
  "GET /api/v1/gateways"
  "GET /api/v1/notifications?limit=10"
  "GET /api/v1/audit?per_page=10"
  "GET /api/v1/ztna/trust-scores"
  "GET /api/v1/compliance/report"
  "GET /api/v1/analytics/summary"
)

for entry in "${ENDPOINTS[@]}"; do
  METHOD=$(echo "$entry" | cut -d' ' -f1)
  ENDPOINT=$(echo "$entry" | cut -d' ' -f2)
  echo ""
  log "Testing: $METHOD $ENDPOINT"

  hey -n "$REQUESTS" -c "$CONCURRENCY" \
    -m "$METHOD" \
    -H "$AUTH_HEADER" \
    "$API$ENDPOINT" 2>&1 | grep -E "^(Requests/sec|Total:|Slowest:|Fastest:|Average:|Status code)"

done

echo ""
echo "============================================================"

# Write-path test: create + delete user.
log "Testing write path: POST /api/v1/users (create + delete)"
RESULTS=$(hey -n 50 -c 5 \
  -m POST \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{"username":"loadtest-REPLACE","email":"lt-REPLACE@test.local","password":"LoadTest1!","first_name":"Load","last_name":"Test","role":"user"}' \
  "$API/api/v1/users" 2>&1 | grep -E "^(Requests/sec|Total:|Status code)")
echo "$RESULTS"

# Cleanup load test users.
log "Cleaning up test users..."
USERS_RESP=$(curl -s "$API/api/v1/users?per_page=200" -H "$AUTH_HEADER")
echo "$USERS_RESP" | python3 -c "
import sys, json
data = json.load(sys.stdin)
users = data.get('users', data.get('data', []))
for u in users:
    if u.get('username','').startswith('loadtest-'):
        print(u['id'])
" 2>/dev/null | while read -r uid; do
  curl -s -X DELETE "$API/api/v1/users/$uid" -H "$AUTH_HEADER" > /dev/null 2>&1
done
ok "Cleanup done"

echo ""
ok "Load test complete!"
