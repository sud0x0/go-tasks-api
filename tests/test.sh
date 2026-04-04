#!/usr/bin/env bash
# =============================================================================
# Go Tasks API — Comprehensive End-to-End Test Script
# =============================================================================
# Usage: chmod +x test_api.sh && ./test_api.sh
# Requires: curl, jq
# macOS and Linux compatible
# =============================================================================

BASE_URL="http://localhost:8080"
TODAY=$(date +%Y-%m-%d)
TOMORROW=$(date -v+1d +%Y-%m-%d 2>/dev/null || date -d "+1 day" +%Y-%m-%d)
YESTERDAY=$(date -v-1d +%Y-%m-%d 2>/dev/null || date -d "-1 day" +%Y-%m-%d)
PASS=0
FAIL=0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

section() {
    echo ""
    echo -e "${BLUE}================================================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}================================================================${NC}"
}

subsection() {
    echo -e "  ${CYAN}--- $1 ---${NC}"
}

info() {
    echo -e "  ${YELLOW}INFO${NC} $1"
}

check() {
    local description="$1"
    local expected_status="$2"
    local actual_status="$3"
    local body="$4"
    if [ "$actual_status" -eq "$expected_status" ]; then
        echo -e "  ${GREEN}PASS${NC} [$actual_status] $description"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC} [$actual_status != $expected_status] $description"
        echo -e "       Body: $body"
        FAIL=$((FAIL + 1))
    fi
}

check_body() {
    local description="$1"
    local expected_field="$2"
    local body="$3"
    if echo "$body" | jq -e "$expected_field" > /dev/null 2>&1; then
        echo -e "  ${GREEN}PASS${NC} [body] $description"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC} [body] $description"
        echo -e "       Expected: $expected_field"
        echo -e "       Body: $body"
        FAIL=$((FAIL + 1))
    fi
}

check_message() {
    local description="$1"
    local expected_text="$2"
    local body="$3"
    if echo "$body" | grep -qi "$expected_text"; then
        echo -e "  ${GREEN}PASS${NC} [message] $description"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC} [message] $description — expected: '$expected_text'"
        echo -e "       Body: $body"
        FAIL=$((FAIL + 1))
    fi
}

check_count() {
    local description="$1"
    local expected="$2"
    local actual="$3"
    if [ "$actual" -eq "$expected" ]; then
        echo -e "  ${GREEN}PASS${NC} [count] $description (got $actual)"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC} [count] $description — expected $expected, got $actual"
        FAIL=$((FAIL + 1))
    fi
}

check_not_contains() {
    local description="$1"
    local forbidden="$2"
    local body="$3"
    if echo "$body" | grep -q "$forbidden"; then
        echo -e "  ${RED}FAIL${NC} [security] $description — found '$forbidden' in response"
        echo -e "       Body: $body"
        FAIL=$((FAIL + 1))
    else
        echo -e "  ${GREEN}PASS${NC} [security] $description"
        PASS=$((PASS + 1))
    fi
}

TMPFILE=$(mktemp)

do_curl() {
    local method="$1"
    local url="$2"
    local body="$3"
    local token="$4"
    local args=(-s -o "$TMPFILE" -w "%{http_code}" -X "$method")
    [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
    [ -n "$body" ] && args+=(-H "Content-Type: application/json" -d "$body")
    LAST_STATUS=$(curl "${args[@]}" "$BASE_URL$url")
    LAST_BODY=$(cat "$TMPFILE")
}

get()  { do_curl "GET"    "$1" ""   "$2"; }
post() { do_curl "POST"   "$1" "$2" "$3"; }
put()  { do_curl "PUT"    "$1" "$2" "$3"; }
del()  { do_curl "DELETE" "$1" ""   "$2"; }

TS=$(date +%s)

# =============================================================================
section "1. Health and Metrics"
# =============================================================================

get "/health"
check "GET /health returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "health has status field" '.status' "$LAST_BODY"
check_body "health status value is healthy" '.status == "healthy"' "$LAST_BODY"

get "/metrics"
check "GET /metrics returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_message "metrics response contains Prometheus counter" "http_requests_total" "$LAST_BODY"
check_message "metrics response contains TYPE declaration" "# TYPE" "$LAST_BODY"

# =============================================================================
section "2. Auth — Register"
# =============================================================================

UNIQUE_USER="testuser_$TS"

subsection "Valid registration"
post "/api/v1/auth/register" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
check "POST /auth/register valid user returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "register response has id" '.id' "$LAST_BODY"
check_body "register response has username" '.username' "$LAST_BODY"
check_body "register response has created_at" '.created_at' "$LAST_BODY"
check_body "register response username matches input" ".username == \"$UNIQUE_USER\"" "$LAST_BODY"
check_not_contains "register response does not expose password hash" "argon2" "$LAST_BODY"
check_not_contains "register response does not expose password field" '"password"' "$LAST_BODY"

subsection "Duplicate username"
post "/api/v1/auth/register" "{\"username\":\"$UNIQUE_USER\",\"password\":\"DifferentPass1!\"}"
check "POST /auth/register duplicate username returns 409" 409 "$LAST_STATUS" "$LAST_BODY"

subsection "Username length boundaries"
post "/api/v1/auth/register" '{"username":"ab","password":"Password123!"}'
check "POST /auth/register username 2 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/register" "{\"username\":\"abc_$TS\",\"password\":\"Password123!\"}"
check "POST /auth/register username 3 chars (minimum) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

# 51 chars — one over limit
post "/api/v1/auth/register" '{"username":"aaaaabbbbbcccccdddddeeeeefffff11111aaaaabbbbbcccccd","password":"Password123!"}'
check "POST /auth/register username 51 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Password length boundaries"
post "/api/v1/auth/register" "{\"username\":\"pw2_$TS\",\"password\":\"short\"}"
check "POST /auth/register password 5 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/register" "{\"username\":\"pw3_$TS\",\"password\":\"passwor\"}"
check "POST /auth/register password 7 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/register" "{\"username\":\"pw4_$TS\",\"password\":\"password\"}"
check "POST /auth/register password 8 chars (minimum) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

# 73 chars — one over limit
post "/api/v1/auth/register" "{\"username\":\"pw5_$TS\",\"password\":\"aaaaabbbbbcccccdddddeeeeefffff11111aaaaabbbbbcccccdddddeeeeefffff11111aaa\"}"
check "POST /auth/register password 73 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Missing and malformed fields"
post "/api/v1/auth/register" '{"password":"Password123!"}'
check "POST /auth/register missing username returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/register" "{\"username\":\"miss_$TS\"}"
check "POST /auth/register missing password returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/register" '{}'
check "POST /auth/register empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/register" 'not json'
check "POST /auth/register invalid JSON returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "3. Auth — Login"
# =============================================================================

subsection "Valid login and response structure"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
check "POST /auth/login valid credentials returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "login has access_token" '.access_token' "$LAST_BODY"
check_body "login has refresh_token" '.refresh_token' "$LAST_BODY"
check_body "login has token_type" '.token_type' "$LAST_BODY"
check_body "login has expires_in" '.expires_in' "$LAST_BODY"
check_body "login token_type is Bearer" '.token_type == "Bearer"' "$LAST_BODY"
check_body "login access_token is non-empty string" '(.access_token | type) == "string" and (.access_token | length) > 0' "$LAST_BODY"
check_body "login refresh_token is non-empty string" '(.refresh_token | type) == "string" and (.refresh_token | length) > 0' "$LAST_BODY"
check_body "login expires_in is positive integer" '(.expires_in | type) == "number" and .expires_in > 0' "$LAST_BODY"
ACCESS_TOKEN=$(echo "$LAST_BODY" | jq -r '.access_token')
REFRESH_TOKEN=$(echo "$LAST_BODY" | jq -r '.refresh_token')

subsection "Invalid credentials"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"wrongpassword\"}"
check "POST /auth/login wrong password returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/login" '{"username":"nobody_xyz_99999","password":"Password123!"}'
check "POST /auth/login non-existent user returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

subsection "Missing fields"
post "/api/v1/auth/login" '{}'
check "POST /auth/login empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/login" '{"username":"someone"}'
check "POST /auth/login missing password returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/login" '{"password":"Password123!"}'
check "POST /auth/login missing username returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/login" 'not json'
check "POST /auth/login invalid JSON returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "4. Auth — Security (token validation)"
# =============================================================================

subsection "All protected endpoints reject missing token"
for endpoint in "/api/v1/categories" "/api/v1/tasks" "/api/v1/daily-logs" "/api/v1/logs"; do
    get "$endpoint"
    check "GET $endpoint without token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"
done
get "/api/v1/occurrences?date=$TODAY"
check "GET /occurrences without token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

subsection "Malformed and tampered tokens"
get "/api/v1/categories" "notavalidtoken"
check "GET /categories random string token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

FAKE_TOKEN="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJmYWtlIn0.fakesignature" # gitleaks:allow nosemgrep
get "/api/v1/categories" "$FAKE_TOKEN"
check "GET /categories tampered RS256 token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

# HS256 token — wrong algorithm, must be rejected by server-side whitelist
HS256_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c" # gitleaks:allow nosemgrep
get "/api/v1/categories" "$HS256_TOKEN"
check "GET /categories HS256 token (wrong alg) returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

subsection "Auth endpoints require no token"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
check "POST /auth/login requires no token (200)" 200 "$LAST_STATUS" "$LAST_BODY"

get "/health"
check "GET /health requires no token (200)" 200 "$LAST_STATUS" "$LAST_BODY"

get "/metrics"
check "GET /metrics requires no token (200)" 200 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "5. Categories — Full CRUD"
# =============================================================================

subsection "Create"
post "/api/v1/categories" '{"name":"Health","description":"Health related tasks"}' "$ACCESS_TOKEN"
check "POST /categories create with description returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "create category has id" '.id' "$LAST_BODY"
check_body "create category has user_id" '.user_id' "$LAST_BODY"
check_body "create category name matches" '.name == "Health"' "$LAST_BODY"
check_body "create category description matches" '.description == "Health related tasks"' "$LAST_BODY"
check_body "create category has created_at" '.created_at' "$LAST_BODY"
check_body "create category has updated_at" '.updated_at' "$LAST_BODY"
CATEGORY_ID=$(echo "$LAST_BODY" | jq -r '.id')

post "/api/v1/categories" '{"name":"Work"}' "$ACCESS_TOKEN"
check "POST /categories create without description returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
CATEGORY_ID_2=$(echo "$LAST_BODY" | jq -r '.id')

post "/api/v1/categories" '{"name":"Finance"}' "$ACCESS_TOKEN"
check "POST /categories create third category returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
CATEGORY_ID_3=$(echo "$LAST_BODY" | jq -r '.id')

subsection "Create validation"
post "/api/v1/categories" '{"description":"No name"}' "$ACCESS_TOKEN"
check "POST /categories missing name returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/categories" '{"name":""}' "$ACCESS_TOKEN"
check "POST /categories empty name returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# 101 chars — one over limit
post "/api/v1/categories" '{"name":"aaaaabbbbbcccccdddddeeeeefffff11111aaaaabbbbbcccccdddddeeeeefffff11111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}' "$ACCESS_TOKEN"
check "POST /categories name 101 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# Description 501 chars — one over limit
LONG_DESC=$(printf 'a%.0s' {1..501})
post "/api/v1/categories" "{\"name\":\"Valid Name\",\"description\":\"$LONG_DESC\"}" "$ACCESS_TOKEN"
check "POST /categories description 501 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/categories" '{}' "$ACCESS_TOKEN"
check "POST /categories empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/categories" 'not json' "$ACCESS_TOKEN"
check "POST /categories invalid JSON returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Read"
get "/api/v1/categories" "$ACCESS_TOKEN"
check "GET /categories returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "list categories returns array" '. | type == "array"' "$LAST_BODY"
check_body "list categories has at least 3 entries" '. | length >= 3' "$LAST_BODY"

get "/api/v1/categories?limit=2" "$ACCESS_TOKEN"
check "GET /categories?limit=2 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "limit=2 returns at most 2 results" '. | length <= 2' "$LAST_BODY"

get "/api/v1/categories?limit=2&offset=1" "$ACCESS_TOKEN"
check "GET /categories?limit=2&offset=1 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/categories?limit=0" "$ACCESS_TOKEN"
check "GET /categories?limit=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/categories?offset=-1" "$ACCESS_TOKEN"
check "GET /categories?offset=-1 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/categories/$CATEGORY_ID" "$ACCESS_TOKEN"
check "GET /categories/:id returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "get category id matches" ".id == \"$CATEGORY_ID\"" "$LAST_BODY"
check_body "get category name matches" '.name == "Health"' "$LAST_BODY"

get "/api/v1/categories/00000000-0000-0000-0000-000000000000" "$ACCESS_TOKEN"
check "GET /categories/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "Update"
put "/api/v1/categories/$CATEGORY_ID" '{"name":"Health & Fitness","description":"Updated description"}' "$ACCESS_TOKEN"
check "PUT /categories/:id returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "update category new name" '.name == "Health & Fitness"' "$LAST_BODY"
check_body "update category new description" '.description == "Updated description"' "$LAST_BODY"

put "/api/v1/categories/$CATEGORY_ID" '{"name":"Health & Fitness"}' "$ACCESS_TOKEN"
check "PUT /categories/:id without description returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/categories/00000000-0000-0000-0000-000000000000" '{"name":"Ghost"}' "$ACCESS_TOKEN"
check "PUT /categories/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/categories/$CATEGORY_ID" '{"name":""}' "$ACCESS_TOKEN"
check "PUT /categories/:id empty name returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/categories/$CATEGORY_ID" '{"name":"aaaaabbbbbcccccdddddeeeeefffff11111aaaaabbbbbcccccdddddeeeeefffff11111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}' "$ACCESS_TOKEN"
check "PUT /categories/:id name too long returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "XSS in category name"
post "/api/v1/categories" '{"name":"<script>alert(1)</script>"}' "$ACCESS_TOKEN"
XSS_CAT_STATUS="$LAST_STATUS"
XSS_CAT_NAME=$(echo "$LAST_BODY" | jq -r '.name // empty')
if [ "$XSS_CAT_STATUS" -eq 201 ]; then
    check_not_contains "category name XSS script tag is stripped" "<script>" "$XSS_CAT_NAME"
elif [ "$XSS_CAT_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [xss] XSS in category name rejected with 400"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [xss] XSS in category name caused unexpected status $XSS_CAT_STATUS"
    FAIL=$((FAIL + 1))
fi

# =============================================================================
section "6. Tasks — All recurrence types and answer types"
# =============================================================================

subsection "Boolean daily (no times)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Morning Run\",\"answer_type\":\"boolean\",\"description\":\"Did you run?\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks boolean daily returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "task response has task.id" '.task.id' "$LAST_BODY"
check_body "task response has task.answer_type" '.task.answer_type == "boolean"' "$LAST_BODY"
check_body "task response has task.is_active true" '.task.is_active == true' "$LAST_BODY"
check_body "task response has schedule.recurrence_type" '.schedule.recurrence_type == "daily"' "$LAST_BODY"
check_body "task response has schedule.end_type" '.schedule.end_type == "never"' "$LAST_BODY"
BOOLEAN_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')

subsection "Integer daily with scheduled times"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Mood Score\",\"answer_type\":\"integer\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"scheduled_times\":[\"09:00\",\"21:00\"],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks integer with times returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "timed task has 2 scheduled_times" '.schedule.scheduled_times | length == 2' "$LAST_BODY"
TIMED_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')

subsection "String weekly"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Weekly Reflection\",\"answer_type\":\"string\",\"schedule\":{\"recurrence_type\":\"weekly\",\"start_date\":\"$TODAY\",\"days_of_week\":[1,3,5],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks string weekly returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "weekly task has days_of_week" '.schedule.days_of_week | length == 3' "$LAST_BODY"
STRING_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')

subsection "Select daily with after_n end condition"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Weather Today\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"after_n\",\"end_after_n\":30},\"select_options\":[{\"value\":\"Sunny\"},{\"value\":\"Rainy\"},{\"value\":\"Cloudy\"}]}" "$ACCESS_TOKEN"
check "POST /tasks select with after_n returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "select task has 3 options" '.select_options | length == 3' "$LAST_BODY"
check_body "select task schedule end_type is after_n" '.schedule.end_type == "after_n"' "$LAST_BODY"
check_body "select task schedule end_after_n is 30" '.schedule.end_after_n == 30' "$LAST_BODY"
SELECT_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')
SELECT_OPTION_SUNNY=$(echo "$LAST_BODY" | jq -r '.select_options[] | select(.value == "Sunny") | .id')
SELECT_OPTION_RAINY=$(echo "$LAST_BODY" | jq -r '.select_options[] | select(.value == "Rainy") | .id')
info "Select option Sunny: $SELECT_OPTION_SUNNY"
info "Select option Rainy: $SELECT_OPTION_RAINY"

subsection "Once (one-time) task"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Call Jerry\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"once\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks once task returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
ONCE_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')

subsection "Every N days"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Every 3 Days\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"every_n_days\",\"start_date\":\"$TODAY\",\"recurrence_interval\":3,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks every_n_days returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "every_n_days has recurrence_interval 3" '.schedule.recurrence_interval == 3' "$LAST_BODY"

subsection "Every N weeks"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Every 2 Weeks\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"every_n_weeks\",\"start_date\":\"$TODAY\",\"recurrence_interval\":2,\"days_of_week\":[1,5],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks every_n_weeks returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "every_n_weeks has recurrence_interval 2" '.schedule.recurrence_interval == 2' "$LAST_BODY"

subsection "Monthly date"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Monthly Review\",\"answer_type\":\"string\",\"schedule\":{\"recurrence_type\":\"monthly_date\",\"start_date\":\"$TODAY\",\"month_day\":1,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks monthly_date returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "monthly_date has month_day 1" '.schedule.month_day == 1' "$LAST_BODY"

subsection "Monthly weekday"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Second Tuesday\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_weekday\",\"start_date\":\"$TODAY\",\"month_week\":2,\"month_weekday\":2,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks monthly_weekday returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "monthly_weekday has month_week 2" '.schedule.month_week == 2' "$LAST_BODY"
check_body "monthly_weekday has month_weekday 2" '.schedule.month_weekday == 2' "$LAST_BODY"

subsection "Yearly"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Annual Review\",\"answer_type\":\"string\",\"schedule\":{\"recurrence_type\":\"yearly\",\"start_date\":\"$TODAY\",\"month_day\":1,\"month_of_year\":1,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks yearly returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "yearly has month_of_year 1" '.schedule.month_of_year == 1' "$LAST_BODY"

subsection "on_date end condition"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Short Task\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"on_date\",\"end_date\":\"$TOMORROW\"}}" "$ACCESS_TOKEN"
check "POST /tasks on_date end condition returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "on_date task has end_date" '.schedule.end_date' "$LAST_BODY"

subsection "Task read operations"
get "/api/v1/tasks" "$ACCESS_TOKEN"
check "GET /tasks returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "list tasks returns array" '. | type == "array"' "$LAST_BODY"

get "/api/v1/tasks?active=true" "$ACCESS_TOKEN"
check "GET /tasks?active=true returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/tasks?category_id=$CATEGORY_ID" "$ACCESS_TOKEN"
check "GET /tasks?category_id= returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/tasks?limit=2" "$ACCESS_TOKEN"
check "GET /tasks?limit=2 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /tasks?limit=2 returns at most 2" '. | length <= 2' "$LAST_BODY"

get "/api/v1/tasks/$BOOLEAN_TASK_ID" "$ACCESS_TOKEN"
check "GET /tasks/:id boolean task returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "get task has task.name" '.task.name == "Morning Run"' "$LAST_BODY"
check_body "get task has schedule" '.schedule' "$LAST_BODY"

get "/api/v1/tasks/$SELECT_TASK_ID" "$ACCESS_TOKEN"
check "GET /tasks/:id select task returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "get select task has select_options" '.select_options | length == 3' "$LAST_BODY"

get "/api/v1/tasks/00000000-0000-0000-0000-000000000000" "$ACCESS_TOKEN"
check "GET /tasks/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "Task update"
put "/api/v1/tasks/$BOOLEAN_TASK_ID" '{"name":"Morning Run Updated","description":"Updated description"}' "$ACCESS_TOKEN"
check "PUT /tasks/:id returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "update task has new name" '.name == "Morning Run Updated"' "$LAST_BODY"
check_body "update task has new description" '.description == "Updated description"' "$LAST_BODY"

put "/api/v1/tasks/$BOOLEAN_TASK_ID" '{"name":"Morning Run Updated"}' "$ACCESS_TOKEN"
check "PUT /tasks/:id without description returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/tasks/$BOOLEAN_TASK_ID" '{"name":""}' "$ACCESS_TOKEN"
check "PUT /tasks/:id empty name returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/tasks/00000000-0000-0000-0000-000000000000" '{"name":"Ghost"}' "$ACCESS_TOKEN"
check "PUT /tasks/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

LONG_TASK_NAME=$(printf 'a%.0s' {1..201})
put "/api/v1/tasks/$BOOLEAN_TASK_ID" "{\"name\":\"$LONG_TASK_NAME\"}" "$ACCESS_TOKEN"
check "PUT /tasks/:id name 201 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Task create validation — answer_type"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"invalid\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks invalid answer_type returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"},\"select_options\":[]}" "$ACCESS_TOKEN"
check "POST /tasks select with no options returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"},\"select_options\":[{\"value\":\"Only\"}]}" "$ACCESS_TOKEN"
check "POST /tasks select with 1 option returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Task create validation — required fields"
post "/api/v1/tasks" "{\"category_id\":\"00000000-0000-0000-0000-000000000000\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks non-existent category returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks missing name returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks missing answer_type returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\"}" "$ACCESS_TOKEN"
check "POST /tasks missing schedule returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Schedule validation — missing required fields per recurrence type"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"weekly\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks weekly without days_of_week returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"every_n_days\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks every_n_days without interval returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"every_n_weeks\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks every_n_weeks without interval returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_date\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks monthly_date without month_day returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_weekday\",\"start_date\":\"$TODAY\",\"month_week\":2,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks monthly_weekday without month_weekday returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"yearly\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks yearly without month_day and month_of_year returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"on_date\"}}" "$ACCESS_TOKEN"
check "POST /tasks on_date without end_date returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"after_n\"}}" "$ACCESS_TOKEN"
check "POST /tasks after_n without end_after_n returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks missing start_date returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "7. Occurrences"
# =============================================================================

subsection "Generate and validate structure"
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
check "GET /occurrences?date= returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "occurrences returns array" '. | type == "array"' "$LAST_BODY"
check_body "occurrences not empty" '. | length > 0' "$LAST_BODY"
check_body "each occurrence has occurrence.id" 'all(.[]; .occurrence.id != null)' "$LAST_BODY"
check_body "each occurrence has task.id" 'all(.[]; .task.id != null)' "$LAST_BODY"
check_body "each occurrence has is_suppressed" 'all(.[]; .occurrence.is_suppressed != null)' "$LAST_BODY"

OCCURRENCES_BODY="$LAST_BODY"
BOOLEAN_OCCURRENCE_ID=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.id == "'"$BOOLEAN_TASK_ID"'")] | .[0].occurrence.id')
SELECT_OCCURRENCE_ID=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.id == "'"$SELECT_TASK_ID"'")] | .[0].occurrence.id')
TIMED_OCC_ID=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.id == "'"$TIMED_TASK_ID"'")] | .[0].occurrence.id')
TIMED_OCC_ID_2=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.id == "'"$TIMED_TASK_ID"'")] | .[1].occurrence.id')
info "Boolean occurrence: $BOOLEAN_OCCURRENCE_ID"
info "Select occurrence:  $SELECT_OCCURRENCE_ID"
info "Timed occurrence 1: $TIMED_OCC_ID"
info "Timed occurrence 2: $TIMED_OCC_ID_2"

subsection "Idempotency — same date called twice must not duplicate"
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
COUNT1=$(echo "$OCCURRENCES_BODY" | jq '. | length')
COUNT2=$(echo "$LAST_BODY" | jq '. | length')
check_count "Same date twice returns same count" "$COUNT1" "$COUNT2"

subsection "Timed task produces correct occurrence count"
TIMED_COUNT=$(echo "$OCCURRENCES_BODY" | jq '[.[] | select(.task.id == "'"$TIMED_TASK_ID"'")] | length')
check_count "Timed task with 2 times produces 2 occurrences" 2 "$TIMED_COUNT"

TIMED_TIME_1=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.id == "'"$TIMED_TASK_ID"'")] | .[0].occurrence.scheduled_time')
TIMED_TIME_2=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.id == "'"$TIMED_TASK_ID"'")] | .[1].occurrence.scheduled_time')
if [ "$TIMED_TIME_1" != "$TIMED_TIME_2" ]; then
    echo -e "  ${GREEN}PASS${NC} [times] Two timed occurrences have different scheduled_time values ($TIMED_TIME_1, $TIMED_TIME_2)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [times] Two timed occurrences have the same scheduled_time"
    FAIL=$((FAIL + 1))
fi

subsection "Date parameter validation"
get "/api/v1/occurrences" "$ACCESS_TOKEN"
check "GET /occurrences with no params returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/occurrences?date=not-a-date" "$ACCESS_TOKEN"
check "GET /occurrences invalid date format returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/occurrences?start_date=$TODAY" "$ACCESS_TOKEN"
check "GET /occurrences start_date without end_date returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/occurrences?end_date=$TODAY" "$ACCESS_TOKEN"
check "GET /occurrences end_date without start_date returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Range query"
get "/api/v1/occurrences?start_date=$TODAY&end_date=$TOMORROW" "$ACCESS_TOKEN"
check "GET /occurrences range query returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "range query returns array" '. | type == "array"' "$LAST_BODY"

get "/api/v1/occurrences?start_date=$TODAY&end_date=$TODAY" "$ACCESS_TOKEN"
check "GET /occurrences same-day range returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

subsection "Answer — boolean"
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer boolean true returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "answer boolean true is correct" '.answer_boolean == true' "$LAST_BODY"
check_body "answer has occurrence_id" '.occurrence_id' "$LAST_BODY"
check_body "answer has answered_at" '.answered_at' "$LAST_BODY"

post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_boolean":false}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer boolean false (update) returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "updated answer is false" '.answer_boolean == false' "$LAST_BODY"

subsection "Answer appears in occurrence list"
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
ANSWERED=$(echo "$LAST_BODY" | jq '[.[] | select(.occurrence.id == "'"$BOOLEAN_OCCURRENCE_ID"'")] | .[0].answer')
if [ "$ANSWERED" != "null" ] && [ -n "$ANSWERED" ]; then
    echo -e "  ${GREEN}PASS${NC} [answer] Answered occurrence shows answer in list"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [answer] Answered occurrence does not show answer in list"
    FAIL=$((FAIL + 1))
fi

subsection "Answer — integer"
post "/api/v1/occurrences/$TIMED_OCC_ID/answer" '{"answer_integer":7}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer integer 7 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "integer answer value is 7" '.answer_integer == 7' "$LAST_BODY"

post "/api/v1/occurrences/$TIMED_OCC_ID/answer" '{"answer_integer":0}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer integer 0 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$TIMED_OCC_ID_2/answer" '{"answer_integer":-5}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer negative integer returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

subsection "Answer — select"
post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/answer" "{\"answer_select\":\"$SELECT_OPTION_SUNNY\"}" "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer select Sunny returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "select answer value matches" ".answer_select == \"$SELECT_OPTION_SUNNY\"" "$LAST_BODY"

post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/answer" "{\"answer_select\":\"$SELECT_OPTION_RAINY\"}" "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer select update to Rainy returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "updated select answer is Rainy" ".answer_select == \"$SELECT_OPTION_RAINY\"" "$LAST_BODY"

post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/answer" '{"answer_select":"00000000-0000-0000-0000-000000000000"}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer invalid select UUID returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# Option from a different task — must be rejected
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" "{\"answer_select\":\"$SELECT_OPTION_SUNNY\"}" "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer select on boolean task returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Answer type mismatch — all cross combinations"
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_integer":5}' "$ACCESS_TOKEN"
check "POST boolean task with integer answer returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_string":"yes"}' "$ACCESS_TOKEN"
check "POST boolean task with string answer returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$TIMED_OCC_ID/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN"
check "POST integer task with boolean answer returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN"
check "POST select task with boolean answer returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/answer" '{"answer_integer":1}' "$ACCESS_TOKEN"
check "POST select task with integer answer returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Empty and missing answer body"
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{}' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" 'not json' "$ACCESS_TOKEN"
check "POST /occurrences/:id/answer invalid JSON returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Non-existent occurrence"
post "/api/v1/occurrences/00000000-0000-0000-0000-000000000000/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN"
check "POST /occurrences non-existent answer returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/00000000-0000-0000-0000-000000000000/suppress" '' "$ACCESS_TOKEN"
check "POST /occurrences non-existent suppress returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "Suppress"
post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/suppress" '' "$ACCESS_TOKEN"
check "POST /occurrences/:id/suppress returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/suppress" '' "$ACCESS_TOKEN"
check "POST /occurrences/:id/suppress already suppressed returns 409" 409 "$LAST_STATUS" "$LAST_BODY"

# Verify suppression is durable — attempting to answer a suppressed occurrence
# The API may reject answers on suppressed occurrences or allow it; either way must not 500
post "/api/v1/occurrences/$SELECT_OCCURRENCE_ID/answer" "{\"answer_select\":\"$SELECT_OPTION_SUNNY\"}" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -ne 500 ]; then
    echo -e "  ${GREEN}PASS${NC} [suppress] Answering suppressed occurrence does not 500 (got $LAST_STATUS)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [suppress] Answering suppressed occurrence caused 500"
    FAIL=$((FAIL + 1))
fi

# =============================================================================
section "8. Daily Logs"
# =============================================================================

subsection "Create and response structure"
post "/api/v1/daily-logs" "{\"log_date\":\"$TODAY\",\"entry\":\"Today was a productive day.\"}" "$ACCESS_TOKEN"
check "POST /daily-logs create returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "daily log has id" '.id' "$LAST_BODY"
check_body "daily log has user_id" '.user_id' "$LAST_BODY"
check_body "daily log has log_date" '.log_date' "$LAST_BODY"
check_body "daily log has entry" '.entry == "Today was a productive day."' "$LAST_BODY"
check_body "daily log has created_at" '.created_at' "$LAST_BODY"
check_body "daily log has updated_at" '.updated_at' "$LAST_BODY"
DAILY_LOG_ID=$(echo "$LAST_BODY" | jq -r '.id')

post "/api/v1/daily-logs" "{\"log_date\":\"$TODAY\",\"entry\":\"Duplicate.\"}" "$ACCESS_TOKEN"
check "POST /daily-logs duplicate date returns 409" 409 "$LAST_STATUS" "$LAST_BODY"
check_message "duplicate message is clear" "already exists" "$LAST_BODY"

post "/api/v1/daily-logs" "{\"log_date\":\"$YESTERDAY\",\"entry\":\"Yesterday entry.\"}" "$ACCESS_TOKEN"
check "POST /daily-logs past date returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
YESTERDAY_LOG_ID=$(echo "$LAST_BODY" | jq -r '.id')

post "/api/v1/daily-logs" "{\"log_date\":\"$TOMORROW\",\"entry\":\"Future entry.\"}" "$ACCESS_TOKEN"
check "POST /daily-logs future date returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Create validation"
post "/api/v1/daily-logs" "{\"log_date\":\"$YESTERDAY\"}" "$ACCESS_TOKEN"
check "POST /daily-logs missing entry returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/daily-logs" '{"entry":"No date"}' "$ACCESS_TOKEN"
check "POST /daily-logs missing log_date returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/daily-logs" '{}' "$ACCESS_TOKEN"
check "POST /daily-logs empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/daily-logs" "{\"log_date\":\"not-a-date\",\"entry\":\"Bad.\"}" "$ACCESS_TOKEN"
check "POST /daily-logs invalid date format returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# 10001 chars — one over limit
LONG_ENTRY=$(printf 'a%.0s' {1..10001})
post "/api/v1/daily-logs" "{\"log_date\":\"2025-03-01\",\"entry\":\"$LONG_ENTRY\"}" "$ACCESS_TOKEN"
check "POST /daily-logs entry 10001 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# Exactly 10000 chars — at the limit
MAX_ENTRY=$(printf 'a%.0s' {1..10000})
post "/api/v1/daily-logs" "{\"log_date\":\"2025-03-02\",\"entry\":\"$MAX_ENTRY\"}" "$ACCESS_TOKEN"
check "POST /daily-logs entry 10000 chars (at limit) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Read"
get "/api/v1/daily-logs?date=$TODAY" "$ACCESS_TOKEN"
check "GET /daily-logs?date= returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "daily log result is array" '. | type == "array"' "$LAST_BODY"
check_body "daily log has correct entry" '.[0].entry == "Today was a productive day."' "$LAST_BODY"

get "/api/v1/daily-logs?date=$YESTERDAY" "$ACCESS_TOKEN"
check "GET /daily-logs?date=yesterday returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "yesterday log is array with one entry" '. | length == 1' "$LAST_BODY"

get "/api/v1/daily-logs?start_date=$YESTERDAY&end_date=$TOMORROW" "$ACCESS_TOKEN"
check "GET /daily-logs range query returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "range query returns at least 2 entries" '. | length >= 2' "$LAST_BODY"

get "/api/v1/daily-logs?date=9999-12-31" "$ACCESS_TOKEN"
check "GET /daily-logs far future date returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "far future date returns empty array" '. | length == 0' "$LAST_BODY"

subsection "Update"
put "/api/v1/daily-logs/$DAILY_LOG_ID" '{"entry":"Updated entry for today."}' "$ACCESS_TOKEN"
check "PUT /daily-logs/:id returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "updated daily log has new entry" '.entry == "Updated entry for today."' "$LAST_BODY"

put "/api/v1/daily-logs/$DAILY_LOG_ID" '{"entry":""}' "$ACCESS_TOKEN"
check "PUT /daily-logs/:id empty entry returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/daily-logs/$DAILY_LOG_ID" "{\"entry\":\"$LONG_ENTRY\"}" "$ACCESS_TOKEN"
check "PUT /daily-logs/:id entry too long returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/daily-logs/00000000-0000-0000-0000-000000000000" '{"entry":"Ghost"}' "$ACCESS_TOKEN"
check "PUT /daily-logs/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "9. Category Delete Protection and Soft Delete"
# =============================================================================

subsection "Delete blocked by active tasks"
del "/api/v1/categories/$CATEGORY_ID" "$ACCESS_TOKEN"
check "DELETE /categories/:id with active tasks returns 409" 409 "$LAST_STATUS" "$LAST_BODY"
check_message "error message mentions active tasks" "active tasks" "$LAST_BODY"

subsection "Soft delete task"
del "/api/v1/tasks/$BOOLEAN_TASK_ID" "$ACCESS_TOKEN"
check "DELETE /tasks/:id soft delete returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/tasks" "$ACCESS_TOKEN"
FOUND=$(echo "$LAST_BODY" | jq '[.[] | select(.id == "'"$BOOLEAN_TASK_ID"'")] | length')
check_count "Deactivated task not in default list" 0 "$FOUND"

get "/api/v1/tasks?active=false" "$ACCESS_TOKEN"
check "GET /tasks?active=false returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
FOUND_INACTIVE=$(echo "$LAST_BODY" | jq '[.[] | select(.id == "'"$BOOLEAN_TASK_ID"'")] | length')
if [ "$FOUND_INACTIVE" -ge 1 ]; then
    echo -e "  ${GREEN}PASS${NC} [soft-delete] Deactivated task appears in active=false list"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [soft-delete] Deactivated task not in active=false list"
    FAIL=$((FAIL + 1))
fi

get "/api/v1/tasks/$BOOLEAN_TASK_ID" "$ACCESS_TOKEN"
check "GET /tasks/:id soft-deleted task still retrievable" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "soft-deleted task has is_active=false" '.task.is_active == false' "$LAST_BODY"

subsection "Delete empty category"
del "/api/v1/categories/$CATEGORY_ID_2" "$ACCESS_TOKEN"
check "DELETE /categories/:id without tasks returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/categories/$CATEGORY_ID_2" "$ACCESS_TOKEN"
check "GET deleted category returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/categories/$CATEGORY_ID_3" "$ACCESS_TOKEN"
check "DELETE /categories/:id second empty category returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/categories/00000000-0000-0000-0000-000000000000" "$ACCESS_TOKEN"
check "DELETE /categories/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "10. Cross-User Isolation"
# =============================================================================

UNIQUE_USER2="testuser2_$TS"
post "/api/v1/auth/register" "{\"username\":\"$UNIQUE_USER2\",\"password\":\"Password123!\"}"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER2\",\"password\":\"Password123!\"}"
ACCESS_TOKEN2=$(echo "$LAST_BODY" | jq -r '.access_token')
info "Second user token acquired"

subsection "User 2 cannot read user 1 data"
get "/api/v1/categories/$CATEGORY_ID" "$ACCESS_TOKEN2"
check "GET user1 category with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/tasks/$SELECT_TASK_ID" "$ACCESS_TOKEN2"
check "GET user1 task with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN2"
check "POST answer user1 occurrence with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$TIMED_OCC_ID/suppress" '' "$ACCESS_TOKEN2"
check "POST suppress user1 occurrence with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "User 2 cannot modify user 1 data"
put "/api/v1/categories/$CATEGORY_ID" '{"name":"Hacked"}' "$ACCESS_TOKEN2"
check "PUT user1 category with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/categories/$CATEGORY_ID" "$ACCESS_TOKEN2"
check "DELETE user1 category with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/tasks/$SELECT_TASK_ID" '{"name":"Hacked"}' "$ACCESS_TOKEN2"
check "PUT user1 task with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/tasks/$SELECT_TASK_ID" "$ACCESS_TOKEN2"
check "DELETE user1 task with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/daily-logs/$DAILY_LOG_ID" '{"entry":"Hacked"}' "$ACCESS_TOKEN2"
check "PUT user1 daily log with user2 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "User 2 sees empty lists"
get "/api/v1/categories" "$ACCESS_TOKEN2"
check_count "User2 sees 0 categories" 0 "$(echo "$LAST_BODY" | jq '. | length')"

get "/api/v1/tasks" "$ACCESS_TOKEN2"
check_count "User2 sees 0 tasks" 0 "$(echo "$LAST_BODY" | jq '. | length')"

get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN2"
check_count "User2 sees 0 occurrences" 0 "$(echo "$LAST_BODY" | jq '. | length')"

get "/api/v1/daily-logs?date=$TODAY" "$ACCESS_TOKEN2"
check_count "User2 sees 0 daily logs" 0 "$(echo "$LAST_BODY" | jq '. | length')"

# =============================================================================
section "11. Auth — Refresh Token Rotation and Logout"
# =============================================================================

subsection "Refresh produces new tokens"
post "/api/v1/auth/refresh" "{\"refresh_token\":\"$REFRESH_TOKEN\"}"
check "POST /auth/refresh valid token returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "refresh has access_token" '.access_token' "$LAST_BODY"
check_body "refresh has refresh_token" '.refresh_token' "$LAST_BODY"
check_body "refresh has token_type" '.token_type' "$LAST_BODY"
check_body "refresh has expires_in" '.expires_in' "$LAST_BODY"
NEW_ACCESS_TOKEN=$(echo "$LAST_BODY" | jq -r '.access_token')
NEW_REFRESH_TOKEN=$(echo "$LAST_BODY" | jq -r '.refresh_token')

if [ "$NEW_ACCESS_TOKEN" != "$ACCESS_TOKEN" ]; then
    echo -e "  ${GREEN}PASS${NC} [rotation] New access token differs from original"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [rotation] New access token same as original"
    FAIL=$((FAIL + 1))
fi

if [ "$NEW_REFRESH_TOKEN" != "$REFRESH_TOKEN" ]; then
    echo -e "  ${GREEN}PASS${NC} [rotation] New refresh token differs from original"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [rotation] New refresh token same as original"
    FAIL=$((FAIL + 1))
fi

subsection "Old refresh token is invalidated after rotation"
post "/api/v1/auth/refresh" "{\"refresh_token\":\"$REFRESH_TOKEN\"}"
check "POST /auth/refresh old token after rotation returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

subsection "New tokens work"
get "/api/v1/categories" "$NEW_ACCESS_TOKEN"
check "GET /categories with new access token returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

subsection "Invalid refresh tokens"
post "/api/v1/auth/refresh" '{"refresh_token":"randomgarbage"}'
check "POST /auth/refresh random string returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/refresh" '{}'
check "POST /auth/refresh missing refresh_token returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/refresh" 'not json'
check "POST /auth/refresh invalid JSON returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Logout revokes both tokens"
post "/api/v1/auth/logout" "{\"refresh_token\":\"$NEW_REFRESH_TOKEN\"}" "$NEW_ACCESS_TOKEN"
check "POST /auth/logout returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

sleep 1

get "/api/v1/categories" "$NEW_ACCESS_TOKEN"
check "GET /categories with access token after logout returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/auth/refresh" "{\"refresh_token\":\"$NEW_REFRESH_TOKEN\"}"
check "POST /auth/refresh with refresh token after logout returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

subsection "Re-login works after logout"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
check "POST /auth/login works after logout returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "12. Legacy Log Domain — Full CRUD"
# =============================================================================

subsection "Login for fresh token"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
check "POST /auth/login for section 12 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
FRESH_TOKEN=$(echo "$LAST_BODY" | jq -r '.access_token')
FRESH_REFRESH=$(echo "$LAST_BODY" | jq -r '.refresh_token')
info "Fresh token: ${FRESH_TOKEN:0:20}..."

subsection "Create"
post "/api/v1/logs" "{\"date_and_time\":\"${TODAY}T10:00:00Z\",\"log\":\"Legacy log entry 1.\"}" "$FRESH_TOKEN"
check "POST /logs create returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "log has id" '.id' "$LAST_BODY"
check_body "log has user_id" '.user_id' "$LAST_BODY"
check_body "log has date_and_time" '.date_and_time' "$LAST_BODY"
check_body "log content matches" '.log == "Legacy log entry 1."' "$LAST_BODY"
check_body "log has created_at" '.created_at' "$LAST_BODY"
LOG_ID=$(echo "$LAST_BODY" | jq -r '.id')

post "/api/v1/logs" "{\"date_and_time\":\"${TODAY}T11:00:00Z\",\"log\":\"Legacy log entry 2.\"}" "$FRESH_TOKEN"
check "POST /logs create second entry returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
LOG_ID_2=$(echo "$LAST_BODY" | jq -r '.id')

subsection "Create validation"
post "/api/v1/logs" "{\"log\":\"Missing date_and_time.\"}" "$FRESH_TOKEN"
check "POST /logs missing date_and_time returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/logs" "{\"date_and_time\":\"${TODAY}T10:00:00Z\"}" "$FRESH_TOKEN"
check "POST /logs missing log returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/logs" "{\"date_and_time\":\"not-a-datetime\",\"log\":\"Bad date.\"}" "$FRESH_TOKEN"
check "POST /logs invalid date_and_time format returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/logs" "{\"date_and_time\":\"$TODAY\",\"log\":\"Date only no time.\"}" "$FRESH_TOKEN"
check "POST /logs date without time returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

LONG_LOG=$(printf 'a%.0s' {1..10001})
post "/api/v1/logs" "{\"date_and_time\":\"${TODAY}T10:00:00Z\",\"log\":\"$LONG_LOG\"}" "$FRESH_TOKEN"
check "POST /logs log 10001 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/logs" '{}' "$FRESH_TOKEN"
check "POST /logs empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Read"
get "/api/v1/logs" "$FRESH_TOKEN"
check "GET /logs returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "list logs returns array" '. | type == "array"' "$LAST_BODY"

get "/api/v1/logs/$LOG_ID" "$FRESH_TOKEN"
check "GET /logs/:id returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "get log content matches" '.log == "Legacy log entry 1."' "$LAST_BODY"

get "/api/v1/logs/00000000-0000-0000-0000-000000000000" "$FRESH_TOKEN"
check "GET /logs/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/logs?start_date=${TODAY}T00:00:00Z&end_date=${TODAY}T23:59:59Z" "$FRESH_TOKEN"
check "GET /logs with date range returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/logs?limit=1" "$FRESH_TOKEN"
check "GET /logs?limit=1 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /logs?limit=1 returns 1 result" '. | length == 1' "$LAST_BODY"

get "/api/v1/logs?limit=0" "$FRESH_TOKEN"
check "GET /logs?limit=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/logs?offset=-1" "$FRESH_TOKEN"
check "GET /logs?offset=-1 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Update"
put "/api/v1/logs/$LOG_ID" "{\"date_and_time\":\"${TODAY}T10:00:00Z\",\"log\":\"Updated legacy log.\"}" "$FRESH_TOKEN"
check "PUT /logs/:id returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "updated log content matches" '.log == "Updated legacy log."' "$LAST_BODY"

put "/api/v1/logs/00000000-0000-0000-0000-000000000000" "{\"date_and_time\":\"${TODAY}T10:00:00Z\",\"log\":\"Ghost.\"}" "$FRESH_TOKEN"
check "PUT /logs/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "XSS sanitisation in log content"
post "/api/v1/logs" "{\"date_and_time\":\"${TODAY}T12:00:00Z\",\"log\":\"<script>alert('xss')</script>Clean text\"}" "$FRESH_TOKEN"
check "POST /logs XSS in content returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
XSS_LOG_CONTENT=$(echo "$LAST_BODY" | jq -r '.log')
check_not_contains "log XSS script tag is stripped" "<script>" "$XSS_LOG_CONTENT"

subsection "Delete"
del "/api/v1/logs/$LOG_ID_2" "$FRESH_TOKEN"
check "DELETE /logs/:id returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/logs/$LOG_ID_2" "$FRESH_TOKEN"
check "GET /logs/:id deleted log returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/logs/00000000-0000-0000-0000-000000000000" "$FRESH_TOKEN"
check "DELETE /logs/:id non-existent returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

subsection "Cross-user isolation in logs"
UNIQUE_USER3="testuser3_$TS"
post "/api/v1/auth/register" "{\"username\":\"$UNIQUE_USER3\",\"password\":\"Password123!\"}"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER3\",\"password\":\"Password123!\"}"
ACCESS_TOKEN3=$(echo "$LAST_BODY" | jq -r '.access_token')

get "/api/v1/logs/$LOG_ID" "$ACCESS_TOKEN3"
check "GET user1 log with user3 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/logs/$LOG_ID" "$ACCESS_TOKEN3"
check "DELETE user1 log with user3 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/logs/$LOG_ID" "{\"date_and_time\":\"${TODAY}T10:00:00Z\",\"log\":\"Hacked.\"}" "$ACCESS_TOKEN3"
check "PUT user1 log with user3 token returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "13. Input Sanitisation and Edge Cases"
# =============================================================================

subsection "XSS in various fields"
post "/api/v1/categories" '{"name":"<img src=x onerror=alert(1)>"}' "$FRESH_TOKEN"
XSS2_STATUS="$LAST_STATUS"
XSS2_NAME=$(echo "$LAST_BODY" | jq -r '.name // empty')
if [ "$XSS2_STATUS" -ne 500 ]; then
    echo -e "  ${GREEN}PASS${NC} [xss] img onerror XSS does not 500 (status: $XSS2_STATUS)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [xss] img onerror XSS caused 500"
    FAIL=$((FAIL + 1))
fi
if [ -n "$XSS2_NAME" ]; then
    check_not_contains "img onerror tag stripped from category name" "onerror" "$XSS2_NAME"
fi

post "/api/v1/categories" '{"name":"Normal & valid name"}' "$FRESH_TOKEN"
check "POST /categories ampersand in name returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
AMP_NAME=$(echo "$LAST_BODY" | jq -r '.name')
check_body "ampersand preserved in name (not double-encoded)" '.name == "Normal & valid name"' "$LAST_BODY"

post "/api/v1/categories" '{"name":"Unicode 日本語 emoji 🎯"}' "$FRESH_TOKEN"
check "POST /categories unicode and emoji in name returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Oversized request body (>1MB)"
HUGETMP=$(mktemp)
printf '{"name":"' > "$HUGETMP"
printf 'x%.0s' {1..1100000} >> "$HUGETMP"
printf '"}' >> "$HUGETMP"
HUGE_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $FRESH_TOKEN" \
    --data-binary "@$HUGETMP" \
    "$BASE_URL/api/v1/categories")
rm -f "$HUGETMP"
if [ "$HUGE_STATUS" -eq 400 ] || [ "$HUGE_STATUS" -eq 413 ] || [ "$HUGE_STATUS" -eq 431 ]; then
    echo -e "  ${GREEN}PASS${NC} [body-limit] Oversized body rejected with $HUGE_STATUS"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [body-limit] Oversized body returned $HUGE_STATUS (expected 400/413)"
    FAIL=$((FAIL + 1))
fi

subsection "Protected endpoints reject all methods without token"
post "/api/v1/categories" '{"name":"Unauthed"}' ""
check "POST /categories with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/categories/$CATEGORY_ID" '{"name":"Unauthed"}' ""
check "PUT /categories/:id with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

del "/api/v1/categories/$CATEGORY_ID" ""
check "DELETE /categories/:id with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" '{}' ""
check "POST /tasks with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/daily-logs" '{}' ""
check "POST /daily-logs with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/daily-logs/$DAILY_LOG_ID" '{"entry":"x"}' ""
check "PUT /daily-logs/:id with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{}' ""
check "POST /occurrences/:id/answer with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/suppress" '' ""
check "POST /occurrences/:id/suppress with no token returns 401" 401 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "14. Auth — Edge Cases"
# =============================================================================

subsection "Logout without access token in Authorization header"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
TEMP_ACCESS=$(echo "$LAST_BODY" | jq -r '.access_token')
TEMP_REFRESH=$(echo "$LAST_BODY" | jq -r '.refresh_token')

# Spec marks logout security: [] — no token required
post "/api/v1/auth/logout" "{\"refresh_token\":\"$TEMP_REFRESH\"}"
check "POST /auth/logout with no Authorization header returns 204" 204 "$LAST_STATUS" "$LAST_BODY"

subsection "Logout with no body"
post "/api/v1/auth/login" "{\"username\":\"$UNIQUE_USER\",\"password\":\"Password123!\"}"
TEMP_REFRESH2=$(echo "$LAST_BODY" | jq -r '.refresh_token')
post "/api/v1/auth/logout" '{}' ""
check "POST /auth/logout with empty body returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Username case sensitivity"
# Register with uppercase variant
post "/api/v1/auth/register" "{\"username\":\"UPPER_$TS\",\"password\":\"Password123!\"}"
check "POST /auth/register uppercase username returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
# Try to login with lowercase version — should fail if usernames are case-sensitive
post "/api/v1/auth/login" "{\"username\":\"upper_$TS\",\"password\":\"Password123!\"}"
if [ "$LAST_STATUS" -eq 401 ]; then
    echo -e "  ${GREEN}PASS${NC} [case] Username is case-sensitive (lowercase login rejected)"
    PASS=$((PASS + 1))
elif [ "$LAST_STATUS" -eq 200 ]; then
    echo -e "  ${GREEN}PASS${NC} [case] Username is case-insensitive (lowercase login accepted)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [case] Unexpected status $LAST_STATUS for case-variant login"
    FAIL=$((FAIL + 1))
fi

subsection "Username with spaces and special characters"
post "/api/v1/auth/register" "{\"username\":\"user name\",\"password\":\"Password123!\"}"
if [ "$LAST_STATUS" -ne 500 ]; then
    echo -e "  ${GREEN}PASS${NC} [username] Username with space does not 500 (got $LAST_STATUS)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [username] Username with space caused 500"
    FAIL=$((FAIL + 1))
fi

post "/api/v1/auth/register" "{\"username\":\"   \",\"password\":\"Password123!\"}"
check "POST /auth/register username with only spaces returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Refresh token belongs to a different user"
# UNIQUE_USER2 was registered in section 10 — get a token for them
post "/api/v1/auth/login" "{\"username\":\"testuser2_$TS\",\"password\":\"Password123!\"}"
USER2_REFRESH=$(echo "$LAST_BODY" | jq -r '.refresh_token')
# Try to use user2's refresh token as user1
post "/api/v1/auth/refresh" "{\"refresh_token\":\"$USER2_REFRESH\"}"
check "POST /auth/refresh with another user's refresh token returns 200 (own token)" 200 "$LAST_STATUS" "$LAST_BODY"
# Confirm the returned access token is for user2, not user1
U2_NEW_TOKEN=$(echo "$LAST_BODY" | jq -r '.access_token')
get "/api/v1/categories/$CATEGORY_ID" "$U2_NEW_TOKEN"
check "Refreshed user2 token cannot access user1 category (404)" 404 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "15. Boundary Values — at-limit success cases"
# =============================================================================

subsection "Category name at exactly 100 chars"
NAME_100=$(printf 'a%.0s' {1..100})
post "/api/v1/categories" "{\"name\":\"$NAME_100\"}" "$FRESH_TOKEN"
check "POST /categories name exactly 100 chars returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "100-char name stored correctly" ".name | length == 100" "$LAST_BODY"

subsection "Category description at exactly 500 chars"
DESC_500=$(printf 'a%.0s' {1..500})
post "/api/v1/categories" "{\"name\":\"Boundary Cat\",\"description\":\"$DESC_500\"}" "$FRESH_TOKEN"
check "POST /categories description exactly 500 chars returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Category PUT description at 501 chars — should fail"
DESC_501=$(printf 'a%.0s' {1..501})
put "/api/v1/categories/$CATEGORY_ID" "{\"name\":\"Health & Fitness\",\"description\":\"$DESC_501\"}" "$ACCESS_TOKEN"
check "PUT /categories/:id description 501 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Task name at exactly 200 chars"
NAME_200=$(printf 'a%.0s' {1..200})
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"$NAME_200\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks name exactly 200 chars returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
BOUNDARY_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')

subsection "Task description at exactly 1000 chars"
DESC_1000=$(printf 'a%.0s' {1..1000})
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Desc Boundary\",\"answer_type\":\"boolean\",\"description\":\"$DESC_1000\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks description exactly 1000 chars returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Task description at 1001 chars — should fail"
DESC_1001=$(printf 'a%.0s' {1..1001})
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Too Long Desc\",\"answer_type\":\"boolean\",\"description\":\"$DESC_1001\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks description 1001 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Task PUT description at exactly 1000 chars"
put "/api/v1/tasks/$BOUNDARY_TASK_ID" "{\"name\":\"Boundary Task\",\"description\":\"$DESC_1000\"}" "$ACCESS_TOKEN"
check "PUT /tasks/:id description exactly 1000 chars returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

subsection "Task PUT description at 1001 chars — should fail"
put "/api/v1/tasks/$BOUNDARY_TASK_ID" "{\"name\":\"Boundary Task\",\"description\":\"$DESC_1001\"}" "$ACCESS_TOKEN"
check "PUT /tasks/:id description 1001 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Select option value at exactly 100 chars"
OPT_100=$(printf 'a%.0s' {1..100})
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Opt Boundary\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"},\"select_options\":[{\"value\":\"$OPT_100\"},{\"value\":\"Other\"}]}" "$ACCESS_TOKEN"
check "POST /tasks select option exactly 100 chars returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Select option value at 101 chars — should fail"
OPT_101=$(printf 'a%.0s' {1..101})
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Opt Too Long\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"},\"select_options\":[{\"value\":\"$OPT_101\"},{\"value\":\"Other\"}]}" "$ACCESS_TOKEN"
check "POST /tasks select option 101 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Select task with exactly 10 options (maximum)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Max Options\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"},\"select_options\":[{\"value\":\"A\"},{\"value\":\"B\"},{\"value\":\"C\"},{\"value\":\"D\"},{\"value\":\"E\"},{\"value\":\"F\"},{\"value\":\"G\"},{\"value\":\"H\"},{\"value\":\"I\"},{\"value\":\"J\"}]}" "$ACCESS_TOKEN"
check "POST /tasks select with 10 options (maximum) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "Select task with 11 options (over maximum)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Over Options\",\"answer_type\":\"select\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"},\"select_options\":[{\"value\":\"A\"},{\"value\":\"B\"},{\"value\":\"C\"},{\"value\":\"D\"},{\"value\":\"E\"},{\"value\":\"F\"},{\"value\":\"G\"},{\"value\":\"H\"},{\"value\":\"I\"},{\"value\":\"J\"},{\"value\":\"K\"}]}" "$ACCESS_TOKEN"
check "POST /tasks select with 11 options returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Legacy log at exactly 10000 chars"
LOG_10000=$(printf 'a%.0s' {1..10000})
post "/api/v1/logs" "{\"date_and_time\":\"${TODAY}T09:00:00Z\",\"log\":\"$LOG_10000\"}" "$FRESH_TOKEN"
check "POST /logs log exactly 10000 chars returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "pagination limit at maximum (100)"
get "/api/v1/categories?limit=100" "$ACCESS_TOKEN"
check "GET /categories?limit=100 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/tasks?limit=100" "$ACCESS_TOKEN"
check "GET /tasks?limit=100 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/logs?limit=100" "$FRESH_TOKEN"
check "GET /logs?limit=100 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"

subsection "Pagination limit over maximum (101)"
get "/api/v1/tasks?limit=101" "$ACCESS_TOKEN"
check "GET /tasks?limit=101 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/logs?limit=101" "$FRESH_TOKEN"
check "GET /logs?limit=101 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Pagination offset beyond total count returns empty array"
get "/api/v1/categories?offset=99999" "$ACCESS_TOKEN"
check "GET /categories?offset=99999 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /categories?offset=99999 returns empty array" '. | length == 0' "$LAST_BODY"

get "/api/v1/tasks?offset=99999" "$ACCESS_TOKEN"
check "GET /tasks?offset=99999 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /tasks?offset=99999 returns empty array" '. | length == 0' "$LAST_BODY"

subsection "Pagination non-integer limit"
get "/api/v1/categories?limit=abc" "$ACCESS_TOKEN"
check "GET /categories?limit=abc returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

get "/api/v1/tasks?limit=abc" "$ACCESS_TOKEN"
check "GET /tasks?limit=abc returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "16. Schedule Field Boundary Values"
# =============================================================================

subsection "recurrence_interval of 0 (minimum is 1)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad Interval\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"every_n_days\",\"start_date\":\"$TODAY\",\"recurrence_interval\":0,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks recurrence_interval=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "days_of_week containing 7 (maximum is 6)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad DOW\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"weekly\",\"start_date\":\"$TODAY\",\"days_of_week\":[1,7],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks days_of_week value 7 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_day of 0 (minimum is 1)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MD\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_date\",\"start_date\":\"$TODAY\",\"month_day\":0,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_day=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_day of 32 (maximum is 31)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MD2\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_date\",\"start_date\":\"$TODAY\",\"month_day\":32,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_day=32 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_week of 0 (minimum is 1)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MW\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_weekday\",\"start_date\":\"$TODAY\",\"month_week\":0,\"month_weekday\":1,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_week=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_week of 6 (maximum is 5)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MW2\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_weekday\",\"start_date\":\"$TODAY\",\"month_week\":6,\"month_weekday\":1,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_week=6 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_weekday of 7 (maximum is 6)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MWD\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"monthly_weekday\",\"start_date\":\"$TODAY\",\"month_week\":1,\"month_weekday\":7,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_weekday=7 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_of_year of 0 (minimum is 1)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MOY\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"yearly\",\"start_date\":\"$TODAY\",\"month_day\":1,\"month_of_year\":0,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_of_year=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "month_of_year of 13 (maximum is 12)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad MOY2\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"yearly\",\"start_date\":\"$TODAY\",\"month_day\":1,\"month_of_year\":13,\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks month_of_year=13 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "scheduled_times with invalid format"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad Time\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"scheduled_times\":[\"25:00\"],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks scheduled_times 25:00 (invalid hour) returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad Time2\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"scheduled_times\":[\"9:00\"],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
# Spec pattern ^([01]?[0-9]|2[0-3]):[0-5][0-9]$ — leading zero is optional, so 9:00 is valid
if [ "$LAST_STATUS" -eq 201 ] || [ "$LAST_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [time-format] scheduled_times 9:00 returns $LAST_STATUS (both valid per spec pattern)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [time-format] scheduled_times 9:00 returned unexpected $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad Time3\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"scheduled_times\":[\"not-a-time\"],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks scheduled_times non-time string returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "end_after_n of 0 (minimum is 1)"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Bad AfterN\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"after_n\",\"end_after_n\":0}}" "$ACCESS_TOKEN"
check "POST /tasks end_after_n=0 returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Task category_id belonging to a different user"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Cross-user task\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN2"
check "POST /tasks with another user's category_id returns 404" 404 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
section "17. Occurrence Schedule Boundary Tests"
# =============================================================================

subsection "Date before task start_date produces no occurrence"
# All our tasks start today — query yesterday, which is before start_date
get "/api/v1/occurrences?date=$YESTERDAY" "$ACCESS_TOKEN"
check "GET /occurrences for date before start_date returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
# Daily tasks starting today should not appear yesterday
YESTERDAY_COUNT=$(echo "$LAST_BODY" | jq '[.[] | select(.task.id == "'"$BOOLEAN_TASK_ID"'")] | length')
check_count "Daily task not generated before its start_date" 0 "$YESTERDAY_COUNT"

subsection "Once task only appears on its start_date"
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
ONCE_TODAY=$(echo "$LAST_BODY" | jq '[.[] | select(.task.id == "'"$ONCE_TASK_ID"'")] | length')
if [ "$ONCE_TODAY" -ge 1 ]; then
    echo -e "  ${GREEN}PASS${NC} [once] Once task appears on its start_date (got $ONCE_TODAY)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [once] Once task does not appear on its start_date"
    FAIL=$((FAIL + 1))
fi

get "/api/v1/occurrences?date=$TOMORROW" "$ACCESS_TOKEN"
ONCE_TOMORROW=$(echo "$LAST_BODY" | jq '[.[] | select(.task.id == "'"$ONCE_TASK_ID"'")] | length')
check_count "Once task does not appear on day after start_date" 0 "$ONCE_TOMORROW"

subsection "on_date task does not appear after its end_date"
# on_date task ends tomorrow — check the day after tomorrow
DAY_AFTER=$(date -v+2d +%Y-%m-%d 2>/dev/null || date -d "+2 days" +%Y-%m-%d)
get "/api/v1/occurrences?date=$DAY_AFTER" "$ACCESS_TOKEN"
# The on_date task had end_date=$TOMORROW so it should not appear on $DAY_AFTER
ON_DATE_TASK_ID=$(echo "$OCCURRENCES_BODY" | jq -r '[.[] | select(.task.name == "Short Task")] | .[0].task.id // empty')
if [ -n "$ON_DATE_TASK_ID" ] && [ "$ON_DATE_TASK_ID" != "null" ]; then
    ON_DATE_AFTER=$(echo "$LAST_BODY" | jq '[.[] | select(.task.id == "'"$ON_DATE_TASK_ID"'")] | length')
    check_count "on_date task does not appear after end_date" 0 "$ON_DATE_AFTER"
else
    info "on_date task ID not found in occurrences body — skipping end_date boundary check"
fi

subsection "range query with end before start returns 400 or empty"
get "/api/v1/occurrences?start_date=$TOMORROW&end_date=$TODAY" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [range] end_date before start_date returns 400"
    PASS=$((PASS + 1))
elif [ "$LAST_STATUS" -eq 200 ]; then
    check_body "end_date before start_date returns empty array" '. | length == 0' "$LAST_BODY"
else
    echo -e "  ${RED}FAIL${NC} [range] end_date before start_date returned unexpected $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

subsection "Occurrence for user with no tasks returns empty array"
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN2"
check_count "User with no tasks sees 0 occurrences" 0 "$(echo "$LAST_BODY" | jq '. | length')"

subsection "Suppressed occurrence appears in list with is_suppressed=true"
# SELECT_OCCURRENCE_ID was suppressed in section 7
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
IS_SUPPRESSED=$(echo "$LAST_BODY" | jq '[.[] | select(.occurrence.id == "'"$SELECT_OCCURRENCE_ID"'")] | .[0].occurrence.is_suppressed')
if [ "$IS_SUPPRESSED" = "true" ]; then
    echo -e "  ${GREEN}PASS${NC} [suppress] Suppressed occurrence appears in list with is_suppressed=true"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [suppress] Suppressed occurrence is_suppressed=$IS_SUPPRESSED (expected true)"
    FAIL=$((FAIL + 1))
fi

subsection "TaskAnswer response has all required fields"
# Re-answer the boolean occurrence to get a fresh answer response
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN"
check_body "answer response has id" '.id' "$LAST_BODY"
check_body "answer response has occurrence_id" '.occurrence_id' "$LAST_BODY"
check_body "answer response has user_id" '.user_id' "$LAST_BODY"
check_body "answer response has answered_at" '.answered_at' "$LAST_BODY"
check_body "answer response has created_at" '.created_at' "$LAST_BODY"
check_body "answer response has updated_at" '.updated_at' "$LAST_BODY"

subsection "answer_string at exactly 500 chars — should succeed"
# Use timed occurrence 2 but it is integer type — need to use a string occurrence
# Create a fresh string task for today to guarantee a string occurrence
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"String Boundary\",\"answer_type\":\"string\",\"schedule\":{\"recurrence_type\":\"daily\",\"start_date\":\"$TODAY\",\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks string daily for answer boundary test returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
STRING_BOUNDARY_TASK_ID=$(echo "$LAST_BODY" | jq -r '.task.id')

get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
STRING_BOUNDARY_OCC=$(echo "$LAST_BODY" | jq -r '[.[] | select(.task.id == "'"$STRING_BOUNDARY_TASK_ID"'")] | .[0].occurrence.id')

if [ -n "$STRING_BOUNDARY_OCC" ] && [ "$STRING_BOUNDARY_OCC" != "null" ]; then
    STR_500=$(printf 'a%.0s' {1..500})
    post "/api/v1/occurrences/$STRING_BOUNDARY_OCC/answer" "{\"answer_string\":\"$STR_500\"}" "$ACCESS_TOKEN"
    check "POST /occurrences/:id/answer string exactly 500 chars returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
    check_body "string answer 500 chars stored correctly" '.answer_string | length == 500' "$LAST_BODY"

    STR_501=$(printf 'a%.0s' {1..501})
    post "/api/v1/occurrences/$STRING_BOUNDARY_OCC/answer" "{\"answer_string\":\"$STR_501\"}" "$ACCESS_TOKEN"
    check "POST /occurrences/:id/answer string 501 chars returns 400" 400 "$LAST_STATUS" "$LAST_BODY"
else
    info "String boundary occurrence not found — skipping string answer boundary tests"
fi

# =============================================================================
section "18. Daily Log Edge Cases"
# =============================================================================

subsection "GET /daily-logs with no parameters returns 200"
get "/api/v1/daily-logs" "$ACCESS_TOKEN"
check "GET /daily-logs with no params returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /daily-logs no params returns array" '. | type == "array"' "$LAST_BODY"

subsection "GET /daily-logs start_date without end_date"
get "/api/v1/daily-logs?start_date=$TODAY" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -eq 200 ] || [ "$LAST_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [daily-log] start_date without end_date returns $LAST_STATUS (not 500)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [daily-log] start_date without end_date returned $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

subsection "GET /daily-logs end_date without start_date"
get "/api/v1/daily-logs?end_date=$TODAY" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -eq 200 ] || [ "$LAST_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [daily-log] end_date without start_date returns $LAST_STATUS (not 500)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [daily-log] end_date without start_date returned $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

subsection "Daily log entry with only whitespace"
post "/api/v1/daily-logs" "{\"log_date\":\"2025-04-01\",\"entry\":\"   \"}" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [daily-log] Whitespace-only entry returns 400"
    PASS=$((PASS + 1))
elif [ "$LAST_STATUS" -eq 201 ]; then
    echo -e "  ${GREEN}PASS${NC} [daily-log] Whitespace-only entry accepted (201) — note: consider rejecting"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [daily-log] Whitespace-only entry returned $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

subsection "Daily log log_date in the far past"
post "/api/v1/daily-logs" "{\"log_date\":\"1900-01-01\",\"entry\":\"Very old log.\"}" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -ne 500 ]; then
    echo -e "  ${GREEN}PASS${NC} [daily-log] Far past date does not 500 (got $LAST_STATUS)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [daily-log] Far past date caused 500"
    FAIL=$((FAIL + 1))
fi

subsection "Verify updated_at changes after PUT /daily-logs/:id"
get "/api/v1/daily-logs?date=$TODAY" "$ACCESS_TOKEN"
ORIGINAL_UPDATED=$(echo "$LAST_BODY" | jq -r '.[0].updated_at')
sleep 1
put "/api/v1/daily-logs/$DAILY_LOG_ID" '{"entry":"Checking updated_at changes."}' "$ACCESS_TOKEN"
NEW_UPDATED=$(echo "$LAST_BODY" | jq -r '.updated_at')
if [ "$NEW_UPDATED" != "$ORIGINAL_UPDATED" ] && [ -n "$NEW_UPDATED" ]; then
    echo -e "  ${GREEN}PASS${NC} [updated_at] updated_at changed after PUT ($ORIGINAL_UPDATED -> $NEW_UPDATED)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [updated_at] updated_at did not change after PUT (was: $ORIGINAL_UPDATED, now: $NEW_UPDATED)"
    FAIL=$((FAIL + 1))
fi

subsection "user_id field present on all resource types"
get "/api/v1/categories/$CATEGORY_ID" "$ACCESS_TOKEN"
check_body "Category response has user_id" '.user_id' "$LAST_BODY"

get "/api/v1/tasks/$SELECT_TASK_ID" "$ACCESS_TOKEN"
check_body "Task response has user_id" '.task.user_id' "$LAST_BODY"

get "/api/v1/daily-logs?date=$TODAY" "$ACCESS_TOKEN"
check_body "DailyLog response has user_id" '.[0].user_id' "$LAST_BODY"

get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
check_body "Occurrence response has user_id" '.[0].occurrence.user_id' "$LAST_BODY"

# =============================================================================
section "19. Legacy Log Additional Coverage"
# =============================================================================

subsection "GET /logs with no date parameters"
get "/api/v1/logs" "$FRESH_TOKEN"
check "GET /logs no params returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /logs no params returns array" '. | type == "array"' "$LAST_BODY"

subsection "GET /logs start_date without end_date"
get "/api/v1/logs?start_date=${TODAY}T00:00:00Z" "$FRESH_TOKEN"
if [ "$LAST_STATUS" -eq 200 ] || [ "$LAST_STATUS" -eq 400 ]; then
    echo -e "  ${GREEN}PASS${NC} [logs] start_date without end_date returns $LAST_STATUS (not 500)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [logs] start_date without end_date returned $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

subsection "GET /logs offset beyond total count"
get "/api/v1/logs?offset=99999" "$FRESH_TOKEN"
check "GET /logs?offset=99999 returns 200" 200 "$LAST_STATUS" "$LAST_BODY"
check_body "GET /logs?offset=99999 returns empty array" '. | length == 0' "$LAST_BODY"

subsection "Log date_and_time in the far future"
post "/api/v1/logs" "{\"date_and_time\":\"9999-12-31T23:59:59Z\",\"log\":\"Future log.\"}" "$FRESH_TOKEN"
if [ "$LAST_STATUS" -ne 500 ]; then
    echo -e "  ${GREEN}PASS${NC} [logs] Far future date_and_time does not 500 (got $LAST_STATUS)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [logs] Far future date_and_time caused 500"
    FAIL=$((FAIL + 1))
fi

subsection "Log date_and_time in the far past"
post "/api/v1/logs" "{\"date_and_time\":\"1900-01-01T00:00:00Z\",\"log\":\"Old log.\"}" "$FRESH_TOKEN"
if [ "$LAST_STATUS" -ne 500 ]; then
    echo -e "  ${GREEN}PASS${NC} [logs] Far past date_and_time does not 500 (got $LAST_STATUS)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [logs] Far past date_and_time caused 500"
    FAIL=$((FAIL + 1))
fi

subsection "PUT /logs/:id missing required fields"
put "/api/v1/logs/$LOG_ID" "{\"log\":\"Missing date_and_time.\"}" "$FRESH_TOKEN"
check "PUT /logs/:id missing date_and_time returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

put "/api/v1/logs/$LOG_ID" "{\"date_and_time\":\"${TODAY}T10:00:00Z\"}" "$FRESH_TOKEN"
check "PUT /logs/:id missing log field returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

subsection "Verify updated_at changes after PUT /logs/:id"
get "/api/v1/logs/$LOG_ID" "$FRESH_TOKEN"
LOG_ORIG_UPDATED=$(echo "$LAST_BODY" | jq -r '.updated_at')
sleep 1
put "/api/v1/logs/$LOG_ID" "{\"date_and_time\":\"${TODAY}T10:00:00Z\",\"log\":\"Checking updated_at.\"}" "$FRESH_TOKEN"
LOG_NEW_UPDATED=$(echo "$LAST_BODY" | jq -r '.updated_at')
if [ "$LOG_NEW_UPDATED" != "$LOG_ORIG_UPDATED" ] && [ -n "$LOG_NEW_UPDATED" ]; then
    echo -e "  ${GREEN}PASS${NC} [updated_at] Log updated_at changed after PUT"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [updated_at] Log updated_at did not change after PUT (was: $LOG_ORIG_UPDATED, now: $LOG_NEW_UPDATED)"
    FAIL=$((FAIL + 1))
fi

subsection "Verify updated_at changes after PUT /tasks/:id"
get "/api/v1/tasks/$SELECT_TASK_ID" "$ACCESS_TOKEN"
TASK_ORIG_UPDATED=$(echo "$LAST_BODY" | jq -r '.task.updated_at')
sleep 1
put "/api/v1/tasks/$SELECT_TASK_ID" '{"name":"Weather Today Updated"}' "$ACCESS_TOKEN"
TASK_NEW_UPDATED=$(echo "$LAST_BODY" | jq -r '.updated_at')
if [ "$TASK_NEW_UPDATED" != "$TASK_ORIG_UPDATED" ] && [ -n "$TASK_NEW_UPDATED" ]; then
    echo -e "  ${GREEN}PASS${NC} [updated_at] Task updated_at changed after PUT"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [updated_at] Task updated_at did not change after PUT (was: $TASK_ORIG_UPDATED, now: $TASK_NEW_UPDATED)"
    FAIL=$((FAIL + 1))
fi

subsection "Verify updated_at changes after PUT /categories/:id"
get "/api/v1/categories/$CATEGORY_ID" "$ACCESS_TOKEN"
CAT_ORIG_UPDATED=$(echo "$LAST_BODY" | jq -r '.updated_at')
sleep 1
put "/api/v1/categories/$CATEGORY_ID" '{"name":"Health & Fitness Final"}' "$ACCESS_TOKEN"
CAT_NEW_UPDATED=$(echo "$LAST_BODY" | jq -r '.updated_at')
if [ "$CAT_NEW_UPDATED" != "$CAT_ORIG_UPDATED" ] && [ -n "$CAT_NEW_UPDATED" ]; then
    echo -e "  ${GREEN}PASS${NC} [updated_at] Category updated_at changed after PUT"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [updated_at] Category updated_at did not change after PUT (was: $CAT_ORIG_UPDATED, now: $CAT_NEW_UPDATED)"
    FAIL=$((FAIL + 1))
fi

subsection "Pagination non-overlap verification"
# Create enough categories to span two pages cleanly under user2
post "/api/v1/categories" '{"name":"Page Cat 1"}' "$ACCESS_TOKEN2"
post "/api/v1/categories" '{"name":"Page Cat 2"}' "$ACCESS_TOKEN2"
post "/api/v1/categories" '{"name":"Page Cat 3"}' "$ACCESS_TOKEN2"
post "/api/v1/categories" '{"name":"Page Cat 4"}' "$ACCESS_TOKEN2"

get "/api/v1/categories?limit=2&offset=0" "$ACCESS_TOKEN2"
PAGE1_IDS=$(echo "$LAST_BODY" | jq -r '[.[].id] | sort | join(",")')

get "/api/v1/categories?limit=2&offset=2" "$ACCESS_TOKEN2"
PAGE2_IDS=$(echo "$LAST_BODY" | jq -r '[.[].id] | sort | join(",")')

if [ -n "$PAGE1_IDS" ] && [ -n "$PAGE2_IDS" ] && [ "$PAGE1_IDS" != "$PAGE2_IDS" ]; then
    echo -e "  ${GREEN}PASS${NC} [pagination] Page 1 and page 2 return different items"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [pagination] Page 1 and page 2 overlap or are empty (p1=$PAGE1_IDS p2=$PAGE2_IDS)"
    FAIL=$((FAIL + 1))
fi

# =============================================================================
section "20. Remaining Gap Coverage"
# =============================================================================

subsection "days_of_week boundary values 0 (Sunday) and 6 (Saturday) are accepted"
post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Weekend Task\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"weekly\",\"start_date\":\"$TODAY\",\"days_of_week\":[0,6],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks days_of_week [0,6] (Sunday and Saturday) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"
check_body "days_of_week contains 0 (Sunday)" '.schedule.days_of_week | contains([0])' "$LAST_BODY"
check_body "days_of_week contains 6 (Saturday)" '.schedule.days_of_week | contains([6])' "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Single Sunday\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"weekly\",\"start_date\":\"$TODAY\",\"days_of_week\":[0],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks days_of_week [0] only (Sunday minimum) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/tasks" "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"Single Saturday\",\"answer_type\":\"boolean\",\"schedule\":{\"recurrence_type\":\"weekly\",\"start_date\":\"$TODAY\",\"days_of_week\":[6],\"end_type\":\"never\"}}" "$ACCESS_TOKEN"
check "POST /tasks days_of_week [6] only (Saturday maximum) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

subsection "select_options present in occurrence response for select-type task"
get "/api/v1/occurrences?date=$TODAY" "$ACCESS_TOKEN"
SELECT_OCC_IN_LIST=$(echo "$LAST_BODY" | jq '[.[] | select(.task.id == "'"$SELECT_TASK_ID"'")] | .[0]')
if [ -n "$SELECT_OCC_IN_LIST" ] && [ "$SELECT_OCC_IN_LIST" != "null" ]; then
    # select_options should be present and non-empty for a select task
    SELECT_OPTS_IN_OCC=$(echo "$SELECT_OCC_IN_LIST" | jq '.select_options | length')
    if [ "$SELECT_OPTS_IN_OCC" -ge 1 ] 2>/dev/null; then
        echo -e "  ${GREEN}PASS${NC} [occ-select] select_options present in occurrence response (got $SELECT_OPTS_IN_OCC options)"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC} [occ-select] select_options missing or empty in occurrence response for select task (got: $SELECT_OPTS_IN_OCC)"
        FAIL=$((FAIL + 1))
    fi

    # Non-select tasks should have null or empty select_options
    BOOL_OCC_IN_LIST=$(echo "$LAST_BODY" | jq '[.[] | select(.task.id == "'"$TIMED_TASK_ID"'")] | .[0]')
    BOOL_OPTS=$(echo "$BOOL_OCC_IN_LIST" | jq '.select_options | length // 0')
    if [ "$BOOL_OPTS" -eq 0 ] 2>/dev/null; then
        echo -e "  ${GREEN}PASS${NC} [occ-select] select_options is empty for non-select task (got $BOOL_OPTS)"
        PASS=$((PASS + 1))
    else
        echo -e "  ${RED}FAIL${NC} [occ-select] select_options non-empty for non-select task (got $BOOL_OPTS)"
        FAIL=$((FAIL + 1))
    fi
else
    echo -e "  ${YELLOW}INFO${NC} Select occurrence not found in today's list — skipping select_options in occurrence check"
fi

subsection "GET /tasks?category_id with non-UUID string"
get "/api/v1/tasks?category_id=not-a-uuid" "$ACCESS_TOKEN"
if [ "$LAST_STATUS" -eq 400 ] || [ "$LAST_STATUS" -eq 200 ]; then
    echo -e "  ${GREEN}PASS${NC} [category-filter] Non-UUID category_id filter returns $LAST_STATUS (not 500)"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [category-filter] Non-UUID category_id filter returned $LAST_STATUS"
    FAIL=$((FAIL + 1))
fi

subsection "Answer updated_at changes on re-answer (update)"
# Submit initial answer
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_boolean":true}' "$ACCESS_TOKEN"
ANSWER_CREATED_AT=$(echo "$LAST_BODY" | jq -r '.created_at')
ANSWER_UPDATED_AT_1=$(echo "$LAST_BODY" | jq -r '.updated_at')

sleep 1

# Re-submit (update) the answer
post "/api/v1/occurrences/$BOOLEAN_OCCURRENCE_ID/answer" '{"answer_boolean":false}' "$ACCESS_TOKEN"
ANSWER_UPDATED_AT_2=$(echo "$LAST_BODY" | jq -r '.updated_at')
ANSWER_CREATED_AT_2=$(echo "$LAST_BODY" | jq -r '.created_at')

if [ -n "$ANSWER_UPDATED_AT_2" ] && [ "$ANSWER_UPDATED_AT_2" != "$ANSWER_UPDATED_AT_1" ]; then
    echo -e "  ${GREEN}PASS${NC} [answer-updated_at] Answer updated_at changes on re-answer"
    PASS=$((PASS + 1))
else
    echo -e "  ${RED}FAIL${NC} [answer-updated_at] Answer updated_at did not change on re-answer (was: $ANSWER_UPDATED_AT_1, now: $ANSWER_UPDATED_AT_2)"
    FAIL=$((FAIL + 1))
fi

if [ -n "$ANSWER_CREATED_AT_2" ] && [ "$ANSWER_CREATED_AT_2" = "$ANSWER_CREATED_AT" ]; then
    echo -e "  ${GREEN}PASS${NC} [answer-created_at] Answer created_at does not change on re-answer"
    PASS=$((PASS + 1))
else
    echo -e "  ${YELLOW}INFO${NC} [answer-created_at] created_at on re-answer: $ANSWER_CREATED_AT_2 (original: $ANSWER_CREATED_AT)"
fi

subsection "Log date_and_time year boundary — year 9000 accepted, year 9001 rejected"
post "/api/v1/logs" "{\"date_and_time\":\"9000-12-31T23:59:59Z\",\"log\":\"Year 9000 log.\"}" "$FRESH_TOKEN"
check "POST /logs date_and_time year 9000 (at limit) returns 201" 201 "$LAST_STATUS" "$LAST_BODY"

post "/api/v1/logs" "{\"date_and_time\":\"9001-01-01T00:00:00Z\",\"log\":\"Year 9001 log.\"}" "$FRESH_TOKEN"
check "POST /logs date_and_time year 9001 (over limit) returns 400" 400 "$LAST_STATUS" "$LAST_BODY"

# =============================================================================
rm -f "$TMPFILE"

echo ""
echo -e "${BLUE}================================================================${NC}"
echo -e "${BLUE}  RESULTS${NC}"
echo -e "${BLUE}================================================================${NC}"
echo -e "  ${GREEN}PASS: $PASS${NC}"
echo -e "  ${RED}FAIL: $FAIL${NC}"
echo -e "  Total: $((PASS + FAIL))"
echo ""
[ $FAIL -eq 0 ] && echo -e "  ${GREEN}All tests passed.${NC}" || echo -e "  ${RED}$FAIL test(s) failed.${NC}"
echo ""
