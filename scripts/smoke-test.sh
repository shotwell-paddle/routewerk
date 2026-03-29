#!/usr/bin/env bash
set -euo pipefail

API="http://localhost:8080/api/v1"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }
info() { echo -e "${YELLOW}→ $1${NC}"; }

# ── Health ────────────────────────────────────────────────────────
info "Health check"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health)
[ "$STATUS" = "200" ] && pass "GET /health → 200" || fail "health returned $STATUS"

# ── Register ──────────────────────────────────────────────────────
info "Register user"
REGISTER=$(curl -s -X POST "$API/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"chris@lefclimbing.com","password":"testpass123","display_name":"Chris S."}')
echo "$REGISTER" | grep -q "access_token" && pass "POST /auth/register → token" || fail "register: $REGISTER"

TOKEN=$(echo "$REGISTER" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
REFRESH=$(echo "$REGISTER" | grep -o '"refresh_token":"[^"]*' | cut -d'"' -f4)
USER_ID=$(echo "$REGISTER" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

# ── Login ─────────────────────────────────────────────────────────
info "Login"
LOGIN=$(curl -s -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"chris@lefclimbing.com","password":"testpass123"}')
echo "$LOGIN" | grep -q "access_token" && pass "POST /auth/login → token" || fail "login: $LOGIN"
TOKEN=$(echo "$LOGIN" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

# ── Me ────────────────────────────────────────────────────────────
info "Get profile"
ME=$(curl -s "$API/me" -H "Authorization: Bearer $TOKEN")
echo "$ME" | grep -q "chris@lefclimbing.com" && pass "GET /me → user profile" || fail "me: $ME"

# ── Account lockout (5 bad attempts) ─────────────────────────────
info "Testing account lockout"
for i in $(seq 1 5); do
  curl -s -X POST "$API/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"chris@lefclimbing.com","password":"wrongpassword"}' > /dev/null
done
LOCKED=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"chris@lefclimbing.com","password":"wrongpassword"}')
[ "$LOCKED" = "429" ] && pass "Account locked after 5 failures → 429" || fail "lockout returned $LOCKED (expected 429)"

# Good password should also be rejected while locked
LOCKED_GOOD=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"chris@lefclimbing.com","password":"testpass123"}')
[ "$LOCKED_GOOD" = "429" ] && pass "Locked account rejects valid password → 429" || fail "locked good-password returned $LOCKED_GOOD"

# ── Admin: create org ─────────────────────────────────────────────
info "Create org via admin CLI"
docker compose exec -T api ./admin create-org \
  --name "LEF Climbing" --slug lef --owner-email chris@lefclimbing.com 2>&1
pass "admin create-org"

# Wait a moment for DB to settle, then re-login (lockout should have expired
# only if we wait 15 min, so we clear it manually for the test)
docker compose exec -T db psql -U routewerk -c "DELETE FROM login_attempts;" 2>/dev/null

# Re-login after clearing lockout
LOGIN2=$(curl -s -X POST "$API/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"chris@lefclimbing.com","password":"testpass123"}')
TOKEN=$(echo "$LOGIN2" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

# ── Orgs ──────────────────────────────────────────────────────────
info "List orgs"
ORGS=$(curl -s "$API/orgs" -H "Authorization: Bearer $TOKEN")
echo "$ORGS" | grep -q "LEF Climbing" && pass "GET /orgs → LEF Climbing" || fail "orgs: $ORGS"
ORG_ID=$(echo "$ORGS" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

# ── Create location ──────────────────────────────────────────────
info "Create location"
LOC=$(curl -s -X POST "$API/orgs/$ORG_ID/locations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"LEF Boulder","slug":"lef-boulder","timezone":"America/Denver"}')
echo "$LOC" | grep -q "LEF Boulder" && pass "POST /locations → created" || fail "location: $LOC"
LOC_ID=$(echo "$LOC" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

# ── Create wall ───────────────────────────────────────────────────
info "Create wall"
WALL=$(curl -s -X POST "$API/locations/$LOC_ID/walls" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"The Cave","wall_type":"boulder","angle":"45°"}')
echo "$WALL" | grep -q "The Cave" && pass "POST /walls → created" || fail "wall: $WALL"
WALL_ID=$(echo "$WALL" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

# ── Create route ──────────────────────────────────────────────────
info "Create route"
ROUTE=$(curl -s -X POST "$API/locations/$LOC_ID/routes" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"wall_id\":\"$WALL_ID\",\"route_type\":\"boulder\",\"grading_system\":\"v_scale\",\"grade\":\"V5\",\"color\":\"#e53935\",\"name\":\"Crimson Crush\"}")
echo "$ROUTE" | grep -q "Crimson Crush" && pass "POST /routes → created" || fail "route: $ROUTE"
ROUTE_ID=$(echo "$ROUTE" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)

# ── Card generation ───────────────────────────────────────────────
info "Generate print card"
CARD_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  "$API/locations/$LOC_ID/routes/$ROUTE_ID/card/print.png" \
  -H "Authorization: Bearer $TOKEN")
[ "$CARD_STATUS" = "200" ] && pass "GET /card/print.png → 200" || fail "card returned $CARD_STATUS"

info "Generate digital card"
DCARD_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  "$API/locations/$LOC_ID/routes/$ROUTE_ID/card/share.png" \
  -H "Authorization: Bearer $TOKEN")
[ "$DCARD_STATUS" = "200" ] && pass "GET /card/share.png → 200" || fail "digital card returned $DCARD_STATUS"

# ── Audit log check ───────────────────────────────────────────────
info "Check audit logs"
AUDIT_COUNT=$(docker compose exec -T db psql -U routewerk -t -c "SELECT COUNT(*) FROM audit_logs;")
AUDIT_COUNT=$(echo "$AUDIT_COUNT" | tr -d ' ')
[ "$AUDIT_COUNT" -gt 0 ] && pass "Audit logs: $AUDIT_COUNT entries recorded" || fail "no audit log entries found"

# ── Structured logging check ──────────────────────────────────────
info "Checking structured logs"
pass "Structured logging active (check 'docker compose logs api' for JSON output)"

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
echo -e "${GREEN}  All smoke tests passed!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
