#!/usr/bin/env bash
set -e

# blast-radius-demo.sh
#
# Proves that LiveReview's blast-radius ordering beats naive diff order, by
# seeding a small Go service with a REAL call graph (unlike scripts/demo.sh,
# whose files are mutually independent) and then editing it so that the
# scariest-looking diff hunks (biggest line count, earliest in file order)
# are NOT the highest-impact ones, and vice versa.
#
# Requires running the review through the local `git lrc review` CLI (with a
# live codebase-memory-mcp index of this repo) — the blast-radius artifact is
# computed client-side by git-lrc and POSTed to LiveReview opportunistically;
# a webhook- or web-UI-triggered review will never show blast-radius
# ordering, only naive diff order. This is documented/expected, not a bug.
#
# Also note: the engine's "frequently used" signal (see FormatCurrency below)
# is a STATIC, structural call-graph fan-in/hotspot count, not live
# production traffic — there is no runtime trace weighting in the scoring
# engine. Don't tell a customer otherwise.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="${SCRIPT_DIR}/blast-radius-demo"
REPO_ROOT="$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel 2>/dev/null || echo "${SCRIPT_DIR}/..")"
GIT_DIR="$(git -C "${REPO_ROOT}" rev-parse --git-dir 2>/dev/null)"
STATE_FILE="${GIT_DIR:-${REPO_ROOT}/.git}/blast-radius-demo-pre-sha"

create_demo() {
    echo "Creating blast-radius-demo baseline service..."
    mkdir -p "${DEMO_DIR}"

    cat > "${DEMO_DIR}/config.go" << 'EOF'
package blastdemo

// SessionTimeoutSeconds controls how long a validated session stays active.
const SessionTimeoutSeconds = 3600

// MaxRetries bounds retry attempts for background jobs.
const MaxRetries = 3
EOF

    cat > "${DEMO_DIR}/token_validator.go" << 'EOF'
package blastdemo

import "strings"

// ValidateSession checks a session token before any privileged action runs.
// It is called from nearly every HTTP handler in entrypoints.go.
func ValidateSession(token string) bool {
	if token == "" {
		return false
	}
	return strings.HasPrefix(token, "sess_")
}
EOF

    cat > "${DEMO_DIR}/user_storage_queries.go" << 'EOF'
package blastdemo

import (
	"database/sql"
	"fmt"
)

var db *sql.DB

// UpdateUserBalance adjusts a user's balance by amount.
func UpdateUserBalance(userID string, amount float64) error {
	_, err := db.Exec("UPDATE users SET balance = balance + $1 WHERE id = $2", amount, userID)
	return err
}

// SaveOrder persists a new order row.
func SaveOrder(userID, item string, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("invalid quantity: %d", qty)
	}
	_, err := db.Exec("INSERT INTO orders (user_id, item, qty) VALUES ($1, $2, $3)", userID, item, qty)
	return err
}

// DeleteOrderRecord removes an order row by id.
func DeleteOrderRecord(orderID string) error {
	if orderID == "" {
		return fmt.Errorf("order id required")
	}
	_, err := db.Exec("DELETE FROM orders WHERE id = $1", orderID)
	return err
}
EOF

    cat > "${DEMO_DIR}/utils_format.go" << 'EOF'
package blastdemo

import "fmt"

// FormatCurrency renders a float as a display string. Called from nearly
// every handler in entrypoints.go.
func FormatCurrency(amount float64) string {
	return fmt.Sprintf("$%.2f", amount)
}
EOF

    cat > "${DEMO_DIR}/logging.go" << 'EOF'
package blastdemo

import "log"

// LogOrderProcessed records that an order-related action completed.
func LogOrderProcessed(userID string) {
	log.Printf("processed action for user=%s", userID)
}
EOF

    cat > "${DEMO_DIR}/business_quarterly_report.go" << 'EOF'
package blastdemo

import "fmt"

// GenerateQuarterlyReport builds a legacy quarterly summary string.
// Nothing in this package calls this function anymore — it was used by a
// reporting job that was retired last year — but it still ships with the
// service.
func GenerateQuarterlyReport(year int, quarter int) string {
	return fmt.Sprintf("Q%d %d report: no data source configured", quarter, year)
}
EOF

    cat > "${DEMO_DIR}/account_service.go" << 'EOF'
package blastdemo

import "net/http"

// AuthorizeRequest is the single choke point every handler is supposed to
// call before doing anything privileged. It wraps ValidateSession so the
// auth bypass bug is two hops away from any entry point, not one.
func AuthorizeRequest(r *http.Request) bool {
	return ValidateSession(r.Header.Get("Authorization"))
}
EOF

    cat > "${DEMO_DIR}/order_service.go" << 'EOF'
package blastdemo

// ProcessNewOrder validates and persists a new order on behalf of a handler.
func ProcessNewOrder(userID, item string, qty int) error {
	return SaveOrder(userID, item, qty)
}

// ProcessRefund issues a refund: adjusts the balance and records an order row.
func ProcessRefund(userID string, amount float64) error {
	if err := AdjustBalance(userID, amount); err != nil {
		return err
	}
	return SaveOrder(userID, "refund", 1)
}

// AdjustBalance applies a balance change on behalf of a handler.
func AdjustBalance(userID string, amount float64) error {
	return UpdateUserBalance(userID, amount)
}

// RemoveOrder deletes an order on behalf of a handler.
func RemoveOrder(orderID string) error {
	return DeleteOrderRecord(orderID)
}
EOF

    cat > "${DEMO_DIR}/web_server_routes.go" << 'EOF'
package blastdemo

import (
	"fmt"
	"net/http"
)

// RegisterRoutes wires every handler below onto the given mux. This is the
// single HTTP entry point for the whole demo service.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/users/update", HandleUpdateUser)
	mux.HandleFunc("/users/profile", HandleViewProfile)
	mux.HandleFunc("/orders/create", HandleCreateOrder)
	mux.HandleFunc("/orders/delete", HandleDeleteOrder)
	mux.HandleFunc("/orders/refund", HandleRefundOrder)
	mux.HandleFunc("/admin/promote", HandleAdminPromote)
	mux.HandleFunc("/reports/cron", HandleCronReportTrigger)
}

func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("id")
	amount := 10.0
	if err := AdjustBalance(userID, amount); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	LogOrderProcessed(userID)
	fmt.Fprintf(w, "balance updated: %s", FormatCurrency(amount))
}

func HandleViewProfile(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("id")
	fmt.Fprintf(w, "profile for %s, balance %s", userID, FormatCurrency(0))
}

func HandleCreateOrder(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("user_id")
	item := r.URL.Query().Get("item")
	if err := ProcessNewOrder(userID, item, 1); err != nil {
		http.Error(w, "order failed", http.StatusInternalServerError)
		return
	}
	LogOrderProcessed(userID)
	fmt.Fprintf(w, "order saved: %s", FormatCurrency(9.99))
}

func HandleDeleteOrder(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	orderID := r.URL.Query().Get("order_id")
	if orderID == "" {
		http.Error(w, "order_id required", http.StatusBadRequest)
		return
	}
	if err := RemoveOrder(orderID); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "deleted %s", orderID)
}

func HandleRefundOrder(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("user_id")
	amount := 5.0
	if err := ProcessRefund(userID, amount); err != nil {
		http.Error(w, "refund failed", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "refunded %s", FormatCurrency(amount))
}

func HandleAdminPromote(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("user_id")
	if err := AdjustBalance(userID, 0); err != nil {
		http.Error(w, "promote failed", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "promoted %s", userID)
}

func HandleCronReportTrigger(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	fmt.Fprintf(w, "scheduled: %s", FormatCurrency(0))
}
EOF

    # Frontend: a thin shared API client plus three pages, mirroring the
    # backend's call-graph shape (page -> ApiClient method -> fetchJSON ->
    # fetch). Clean/baseline versions here; bugs land in `inject`.
    mkdir -p "${DEMO_DIR}/app"
    cat > "${DEMO_DIR}/app/apiClient.ts" << 'EOF'
export async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
	const res = await fetch(url, options);
	return res.json();
}

export const ApiClient = {
	getProfile: (userId: string) => fetchJSON(`/users/profile?id=${userId}`),
	updateUser: (userId: string) => fetchJSON(`/users/update?id=${userId}`, { method: 'POST' }),
	createOrder: (userId: string, item: string) =>
		fetchJSON(`/orders/create?user_id=${userId}&item=${item}`, { method: 'POST' }),
	deleteOrder: (orderId: string) => fetchJSON(`/orders/delete?order_id=${orderId}`, { method: 'POST' }),
	refundOrder: (userId: string, amount: number) =>
		fetchJSON(`/orders/refund?user_id=${userId}&amount=${amount}`, { method: 'POST' }),
	promoteUser: (userId: string) => fetchJSON(`/admin/promote?user_id=${userId}`, { method: 'POST' }),
};
EOF

    cat > "${DEMO_DIR}/app/UserProfile.tsx" << 'EOF'
import React, { useEffect, useState } from 'react';
import { ApiClient } from './apiClient';

export function UserProfile({ userId }: { userId: string }) {
	const [profile, setProfile] = useState<any>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		setLoading(true);
		ApiClient.getProfile(userId)
			.then(setProfile)
			.catch((e) => setError(e.message))
			.finally(() => setLoading(false));
	}, [userId]);

	if (loading) return <div>Loading profile…</div>;
	if (error) return <div role="alert">Could not load profile: {error}</div>;

	return (
		<section aria-label="User profile">
			<h2>Profile</h2>
			<p>{profile?.name}</p>
		</section>
	);
}
EOF

    cat > "${DEMO_DIR}/app/AdminPanel.tsx" << 'EOF'
import React from 'react';
import { ApiClient } from './apiClient';

export function AdminPanel({ userId }: { userId: string }) {
	const handlePromote = () => {
		if (window.confirm(`Promote user ${userId} to admin?`)) {
			ApiClient.promoteUser(userId);
		}
	};

	return (
		<div>
			<button onClick={handlePromote} aria-label="Promote user to administrator">
				Promote to Admin
			</button>
		</div>
	);
}
EOF

    cat > "${DEMO_DIR}/app/OrderHistory.tsx" << 'EOF'
import React, { useState } from 'react';
import { ApiClient } from './apiClient';

export function OrderHistory({ userId }: { userId: string }) {
	const [refundAmount, setRefundAmount] = useState('');

	const handleRefund = () => {
		const amount = Number(refundAmount);
		if (!Number.isFinite(amount) || amount <= 0) {
			alert('Enter a valid refund amount');
			return;
		}
		ApiClient.refundOrder(userId, amount);
	};

	return (
		<div>
			<input
				type="number"
				value={refundAmount}
				onChange={(e) => setRefundAmount(e.target.value)}
				aria-label="Refund amount"
			/>
			<button onClick={handleRefund}>Refund order</button>
		</div>
	);
}
EOF

    echo "✓ Baseline written to ${DEMO_DIR}/"
}

inject_demo() {
    if [ ! -d "${DEMO_DIR}" ]; then
        echo "Baseline not found. Run './blast-radius-demo.sh create' first." >&2
        exit 1
    fi
    echo "Injecting the risky changeset (12 seeded issues) over the baseline..."

    # Issue 1: auth bypass — empty token now treated as valid. core_auth.go
    # is called from 6 of the 7 handlers, so this should rank highest.
    cat > "${DEMO_DIR}/token_validator.go" << 'EOF'
package blastdemo

import "strings"

// ValidateSession checks a session token before any privileged action runs.
// It is called from nearly every HTTP handler in entrypoints.go.
func ValidateSession(token string) bool {
	if token == "" {
		// Treat a missing token as valid instead of rejecting it, so
		// callers that forget to set the header don't get locked out.
		return true
	}
	return strings.HasPrefix(token, "sess_")
}
EOF

    # Issue 2: SQL injection via string interpolation on THREE DB-write
    # symbols with different fan-in (UpdateUserBalance: 3 callers,
    # SaveOrder: 2, DeleteOrderRecord/PromoteToAdmin: 1 each) — same bug
    # pattern, same severity, so blast radius is the only thing that can
    # tell them apart. This is the single clearest ranking demo in the set.
    cat > "${DEMO_DIR}/user_storage_queries.go" << 'EOF'
package blastdemo

import (
	"database/sql"
	"fmt"
)

var db *sql.DB

// UpdateUserBalance adjusts a user's balance by amount.
func UpdateUserBalance(userID string, amount float64) error {
	query := fmt.Sprintf("UPDATE users SET balance = balance + %f WHERE id = '%s'", amount, userID)
	_, err := db.Exec(query)
	return err
}

// SaveOrder persists a new order row.
func SaveOrder(userID, item string, qty int) error {
	query := fmt.Sprintf("INSERT INTO orders (user_id, item, qty) VALUES ('%s', '%s', %d)", userID, item, qty)
	_, err := db.Exec(query)
	return err
}

// DeleteOrderRecord removes an order row by id.
func DeleteOrderRecord(orderID string) error {
	query := fmt.Sprintf("DELETE FROM orders WHERE id = '%s'", orderID)
	_, err := db.Exec(query)
	return err
}

// PromoteToAdmin sets the is_admin flag directly.
func PromoteToAdmin(userID string) error {
	query := fmt.Sprintf("UPDATE users SET is_admin = true WHERE id = '%s'", userID)
	_, err := db.Exec(query)
	return err
}
EOF

    # Issues 3, 5(config edit), 9(format edit): new unauthenticated admin
    # route (entry-point bonus, no ValidateSession call), plus a pure
    # formatting-only reflow of HandleViewProfile's Fprintf call.
    cat > "${DEMO_DIR}/web_server_routes.go" << 'EOF'
package blastdemo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// RegisterRoutes wires every handler below onto the given mux. This is the
// single HTTP entry point for the whole demo service.
func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/users/update", HandleUpdateUser)
	mux.HandleFunc("/users/profile", HandleViewProfile)
	mux.HandleFunc("/orders/create", HandleCreateOrder)
	mux.HandleFunc("/orders/delete", HandleDeleteOrder)
	mux.HandleFunc("/orders/refund", HandleRefundOrder)
	mux.HandleFunc("/admin/promote", HandleAdminPromote)
	mux.HandleFunc("/reports/cron", HandleCronReportTrigger)
	mux.HandleFunc("/auth/reset-request", HandleRequestPasswordReset)
	mux.HandleFunc("/auth/reset-password", HandleResetPassword)
}

func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("id")
	amount := 10.0
	if err := AdjustBalance(userID, amount); err != nil {
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	LogOrderProcessed(userID)
	fmt.Fprintf(w, "balance updated: %s", FormatCurrency(amount))
}

func HandleViewProfile(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("id")
	rendered := FormatCurrency(0)
	CacheProfile(userID, rendered)
	fmt.Fprintf(
		w,
		"profile for %s, balance %s",
		userID,
		rendered,
	)
}

// HandleCreateOrder accepts optional JSON metadata in the request body to
// override the user id, and fires off a notification in the background.
func HandleCreateOrder(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("user_id")
	item := r.URL.Query().Get("item")

	var payload map[string]interface{}
	json.NewDecoder(r.Body).Decode(&payload)
	if payload != nil {
		userID = ExtractUserID(payload)
	}

	if err := ProcessNewOrder(userID, item, 1); err != nil {
		http.Error(w, "order failed", http.StatusInternalServerError)
		return
	}
	go sendOrderNotification(userID)
	LogOrderProcessed(userID)
	fmt.Fprintf(w, "order saved: %s", FormatCurrency(9.99))
}

func HandleDeleteOrder(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	orderID := r.URL.Query().Get("order_id")
	if err := RemoveOrder(orderID); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "deleted %s", orderID)
}

// HandleRefundOrder refunds an order. The refund amount is taken directly
// from the caller's query string, with no bounds or sign check.
func HandleRefundOrder(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("user_id")
	amount, _ := strconv.ParseFloat(r.URL.Query().Get("amount"), 64)
	if err := ProcessRefund(userID, amount); err != nil {
		http.Error(w, "refund failed", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "refunded %s", FormatCurrency(amount))
}

// HandleAdminPromote promotes a user to admin. Notice this handler never
// calls AuthorizeRequest — anyone who can reach this route can call it.
func HandleAdminPromote(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if err := PromoteToAdmin(userID); err != nil {
		http.Error(w, "promote failed", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "promoted %s", userID)
}

func HandleCronReportTrigger(w http.ResponseWriter, r *http.Request) {
	if !AuthorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	fmt.Fprintf(w, "scheduled: %s", FormatCurrency(0))
}
EOF

    # Issues 16 & 17 (new files, 2-3 call-graph levels deep): the same
    # account/order service wrapper layer as the baseline, unchanged here
    # — the point is that ValidateSession/UpdateUserBalance/SaveOrder are
    # now 2 hops from any entry point, exercising blast radius's decayed
    # transitive fan-in instead of only direct callers.
    cat > "${DEMO_DIR}/account_service.go" << 'EOF'
package blastdemo

import "net/http"

// AuthorizeRequest is the single choke point every handler is supposed to
// call before doing anything privileged. It wraps ValidateSession so the
// auth bypass bug is two hops away from any entry point, not one.
func AuthorizeRequest(r *http.Request) bool {
	return ValidateSession(r.Header.Get("Authorization"))
}
EOF

    cat > "${DEMO_DIR}/order_service.go" << 'EOF'
package blastdemo

// ProcessNewOrder validates and persists a new order on behalf of a handler.
func ProcessNewOrder(userID, item string, qty int) error {
	return SaveOrder(userID, item, qty)
}

// ProcessRefund issues a refund: adjusts the balance and records an order row.
func ProcessRefund(userID string, amount float64) error {
	if err := AdjustBalance(userID, amount); err != nil {
		return err
	}
	return SaveOrder(userID, "refund", 1)
}

// AdjustBalance applies a balance change on behalf of a handler.
func AdjustBalance(userID string, amount float64) error {
	return UpdateUserBalance(userID, amount)
}

// RemoveOrder deletes an order on behalf of a handler.
func RemoveOrder(orderID string) error {
	return DeleteOrderRecord(orderID)
}
EOF

    # Issue 6: boring logging-only addition (hygiene-dampened).
    cat > "${DEMO_DIR}/logging.go" << 'EOF'
package blastdemo

import "log"

// LogOrderProcessed records that an order-related action completed.
func LogOrderProcessed(userID string) {
	log.Printf("processed action for user=%s", userID)
	log.Printf("order processing complete for user=%s", userID)
	log.Println("checkpoint: order pipeline stage complete")
}
EOF

    # Issues 11 & 12 (new file, two more entry points): a password-reset
    # flow with a predictable, time-seeded token and no server-side
    # verification of the token it's given — both tied to brand-new
    # unauthenticated routes, so both should rank high.
    cat > "${DEMO_DIR}/session_token_reset.go" << 'EOF'
package blastdemo

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

// GenerateResetToken produces a password-reset token for userID.
func GenerateResetToken(userID string) string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%s-%06d", userID, rand.Intn(1000000))
}

// HandleRequestPasswordReset issues a reset token for the given user.
func HandleRequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	token := GenerateResetToken(userID)
	fmt.Fprintf(w, "reset token: %s", token)
}

// HandleResetPassword applies a password reset given a token. Notice the
// token is never checked against anything that was actually issued.
func HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "token required", http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "password reset for %s", userID)
}
EOF

    # Issues 13-15 (new file): three textbook, non-security correctness
    # bugs — stale cache that never invalidates, a silent fallback that
    # masks bad input instead of rejecting it, unbounded fire-and-forget
    # goroutine per request — to spread the finding mix across categories,
    # not just security. Deliberately no panics/crashes here: a "the
    # server crashes" finding competes narratively with the SQL
    # injection/auth bypass items for "scariest possible bug," which
    # undercuts the escalation blast-radius ordering is supposed to show.
    cat > "${DEMO_DIR}/request_helpers.go" << 'EOF'
package blastdemo

import "net/http"

// userProfileCache caches rendered profile strings so repeated views don't
// recompute them.
var userProfileCache = map[string]string{}

// CacheProfile stores a rendered profile string for later reuse. It never
// overwrites an existing entry, so once a user's profile is cached, every
// later view keeps serving that same stale snapshot even after their
// balance changes.
func CacheProfile(userID, rendered string) {
	if _, exists := userProfileCache[userID]; !exists {
		userProfileCache[userID] = rendered
	}
}

// ExtractUserID pulls the user id out of a decoded JSON payload. If the
// field is missing or not a string, it silently falls back to "unknown"
// instead of rejecting the request, so a malformed payload gets processed
// under the wrong identity rather than being caught.
func ExtractUserID(payload map[string]interface{}) string {
	if v, ok := payload["user_id"].(string); ok {
		return v
	}
	return "unknown"
}

// sendOrderNotification is fired off with "go" on every order, with no
// concurrency limit, no timeout, and no error handling.
func sendOrderNotification(userID string) {
	resp, err := http.Get("http://notifications.internal/order?user=" + userID)
	if err == nil {
		defer resp.Body.Close()
	}
}
EOF

    # Issue 7: FormatCurrency touched — a one-character-looking diff that
    # truncates cents on every amount it formats, but is called from nearly
    # every handler, so it should rank surprisingly high.
    cat > "${DEMO_DIR}/utils_format.go" << 'EOF'
package blastdemo

import "fmt"

// FormatCurrency renders a float as a display string. Called from nearly
// every handler in entrypoints.go.
func FormatCurrency(amount float64) string {
	return fmt.Sprintf("$%.0f", amount)
}
EOF

    # Issue 8: config bump + hardcoded credential + comment-only hunk, all
    # in the same low-connectivity file — a genuinely flaggable set of
    # issues that should still rank near the bottom on blast radius.
    cat > "${DEMO_DIR}/config.go" << 'EOF'
package blastdemo

// SessionTimeoutSeconds controls how long a validated session stays active.
const SessionTimeoutSeconds = 2592000

// InternalAPIKey authenticates calls to the internal metrics sidecar.
const InternalAPIKey = "demo-hardcoded-internal-key-not-a-real-secret-1234567890"

// MaxRetries bounds retry attempts for background jobs.
//
// NOTE: this value was tuned during the Q3 incident review after repeated
// retry storms against the payments provider; do not increase without
// checking with platform-eng first. See runbook RB-114 for context.
const MaxRetries = 3
EOF

    # Issue 9: massive refactor of dead code (zero callers anywhere) — the
    # single biggest hunk in the whole diff, but near-zero actual impact.
    cat > "${DEMO_DIR}/business_quarterly_report.go" << 'EOF'
package blastdemo

import (
	"fmt"
	"strings"
)

// GenerateQuarterlyReport builds a legacy quarterly summary string.
// Nothing in this package calls this function anymore — it was used by a
// reporting job that was retired last year — but it still ships with the
// service.
func GenerateQuarterlyReport(year int, quarter int) string {
	sections := []string{
		reportHeader(year, quarter),
		reportRevenueSection(year, quarter),
		reportChurnSection(year, quarter),
		reportFooter(),
	}
	return strings.Join(sections, "\n\n")
}

func reportHeader(year, quarter int) string {
	return fmt.Sprintf("===== Quarterly Report: Q%d %d =====", quarter, year)
}

func reportRevenueSection(year, quarter int) string {
	var b strings.Builder
	b.WriteString("Revenue Summary\n")
	b.WriteString("---------------\n")
	for month := 1; month <= 3; month++ {
		absMonth := (quarter-1)*3 + month
		b.WriteString(fmt.Sprintf("  Month %d/%d: $%s\n", absMonth, year, legacyPlaceholderAmount(absMonth)))
	}
	return b.String()
}

func reportChurnSection(year, quarter int) string {
	var b strings.Builder
	b.WriteString("Churn Summary\n")
	b.WriteString("-------------\n")
	for month := 1; month <= 4; month++ {
		absMonth := (quarter-1)*3 + month
		b.WriteString(fmt.Sprintf("  Month %d/%d: %s%% churn (estimated)\n", absMonth, year, legacyPlaceholderChurn(absMonth)))
	}
	return b.String()
}

func reportFooter() string {
	return "This report is generated from static placeholder data and is retained for historical formatting reference only."
}

func legacyPlaceholderAmount(month int) string {
	base := 1000 + month*37
	return fmt.Sprintf("%d.00", base)
}

func legacyPlaceholderChurn(month int) string {
	base := 2 + month%5
	return fmt.Sprintf("%d.0", base)
}
EOF

    # Issue 10: schema change with no default/backfill (new file — path
    # heuristic bonus applies at the hunk level regardless of language).
    mkdir -p "${DEMO_DIR}/migrations"
    cat > "${DEMO_DIR}/migrations/0009_add_admin_flag.sql" << 'EOF'
-- Add is_admin flag to users table.
-- No default value and no backfill for existing rows.
ALTER TABLE users ADD COLUMN is_admin BOOLEAN;
EOF

    # Issues 18-19: frontend regressions mirroring their backend
    # counterparts (removed client-side validation, removed confirmation
    # before a privileged action) — low connectivity, leaf components,
    # should rank low despite being visibly obvious in a naive top-down read.
    mkdir -p "${DEMO_DIR}/app"
    cat > "${DEMO_DIR}/app/apiClient.ts" << 'EOF'
export async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
	const res = await fetch(url, options);
	return res.json();
}

export const ApiClient = {
	getProfile: (userId: string) => fetchJSON(`/users/profile?id=${userId}`),
	updateUser: (userId: string) => fetchJSON(`/users/update?id=${userId}`, { method: 'POST' }),
	createOrder: (userId: string, item: string) =>
		fetchJSON(`/orders/create?user_id=${userId}&item=${item}`, { method: 'POST' }),
	deleteOrder: (orderId: string) => fetchJSON(`/orders/delete?order_id=${orderId}`, { method: 'POST' }),
	refundOrder: (userId: string, amount: number) =>
		fetchJSON(`/orders/refund?user_id=${userId}&amount=${amount}`, { method: 'POST' }),
	promoteUser: (userId: string) => fetchJSON(`/admin/promote?user_id=${userId}`, { method: 'POST' }),
};
EOF

    cat > "${DEMO_DIR}/app/UserProfile.tsx" << 'EOF'
import React, { useEffect, useState } from 'react';
import { ApiClient } from './apiClient';

export function UserProfile({ userId }: { userId: string }) {
	const [profile, setProfile] = useState<any>(null);

	useEffect(() => {
		ApiClient.getProfile(userId).then(setProfile);
	}, [userId]);

	return (
		<section>
			<h2>Profile</h2>
			<p>{profile?.name}</p>
		</section>
	);
}
EOF

    # Issue 20: removed confirmation before a privileged action, plus a
    # typo'd button label and a missing aria-label — three trivial-looking
    # issues bundled into one small leaf file with zero other callers.
    cat > "${DEMO_DIR}/app/AdminPanel.tsx" << 'EOF'
import React from 'react';
import { ApiClient } from './apiClient';

export function AdminPanel({ userId }: { userId: string }) {
	const handlePromote = () => {
		ApiClient.promoteUser(userId);
	};

	return (
		<div>
			<button onClick={handlePromote}>Promte to Admim</button>
		</div>
	);
}
EOF

    cat > "${DEMO_DIR}/app/OrderHistory.tsx" << 'EOF'
import React, { useState } from 'react';
import { ApiClient } from './apiClient';

export function OrderHistory({ userId }: { userId: string }) {
	const [refundAmount, setRefundAmount] = useState('');

	const handleRefund = () => {
		ApiClient.refundOrder(userId, Number(refundAmount));
	};

	return (
		<div>
			<input type="number" value={refundAmount} onChange={(e) => setRefundAmount(e.target.value)} />
			<button onClick={handleRefund}>Refund order</button>
		</div>
	);
}
EOF

    # Issue 21: a pure-noise component — nothing renders it (zero callers),
    # analogous to legacy_report.go on the frontend side: a leftover debug
    # log, a hardcoded color instead of a design token, a missing alt
    # attribute, and a typo'd, vague call-to-action. Should rank at the
    # very bottom despite being the most visually embarrassing file in the
    # whole diff if read top-to-bottom in naive order.
    cat > "${DEMO_DIR}/app/Banner.tsx" << 'EOF'
import React from 'react';

export function Banner() {
	console.log('banner rendered');
	return (
		<div style={{ background: '#ff00ff', padding: '12px' }}>
			<img src="/promo.png" />
			<button>Sucess! Click Here</button>
		</div>
	);
}
EOF

    echo "✓ Risky changeset applied (uncommitted) in ${DEMO_DIR}/"
    echo ""
    echo "Now run: git lrc review"
    echo "(after confirming codebase-memory-mcp has indexed this repo)"
}

remove_demo() {
    if [ -d "${DEMO_DIR}" ]; then
        echo "Removing blast-radius-demo directory..."
        rm -rf "${DEMO_DIR}"
        echo "✓ Demo directory removed"
        echo "  (if you committed the baseline, don't forget to revert/reset that commit too)"
    else
        echo "Demo directory does not exist"
    fi
}

run_demo() {
    if [ -z "${GIT_DIR}" ]; then
        echo "Not a git repository (or git not found). Aborting." >&2
        exit 1
    fi
    if [ -f "${STATE_FILE}" ]; then
        echo "A blast-radius-demo run is already in progress (state file exists)." >&2
        echo "Run './blast-radius-demo.sh undo' first, or delete ${STATE_FILE} if that's stale." >&2
        exit 1
    fi
    if [ -n "$(git -C "${REPO_ROOT}" status --porcelain)" ]; then
        echo "Working tree is not clean. Commit/stash your changes first, then re-run." >&2
        exit 1
    fi

    git -C "${REPO_ROOT}" rev-parse HEAD > "${STATE_FILE}"
    echo "Recorded current HEAD so 'undo' can restore it exactly."

    create_demo
    git -C "${REPO_ROOT}" add scripts/blast-radius-demo
    git -C "${REPO_ROOT}" commit -q -m "Add blast-radius-demo baseline service"
    echo "✓ Committed baseline (local commit — 'undo' will remove it)"

    inject_demo
    git -C "${REPO_ROOT}" add scripts/blast-radius-demo
    echo "✓ Staged the risky changeset"

    echo ""
    if command -v git-lrc >/dev/null 2>&1 || git lrc --help >/dev/null 2>&1; then
        echo "Running: git lrc review"
        (cd "${REPO_ROOT}" && git lrc review) || {
            echo "git lrc review failed — run it manually once you've fixed the issue." >&2
        }
    else
        echo "git-lrc CLI not found on PATH — run 'git lrc review' yourself once it's installed," >&2
        echo "or trigger a review however you normally do (the staged diff is ready either way)." >&2
    fi

    echo ""
    echo "Next: open the review in the LiveReview web UI -> Findings tab -> DiffViewerPanel,"
    echo "and flip 'Diff order' vs 'Score: Whole' to show the reorder live."
    echo "When you're done demoing: ./scripts/blast-radius-demo.sh undo"
}

undo_demo() {
    if [ -z "${GIT_DIR}" ]; then
        echo "Not a git repository (or git not found). Aborting." >&2
        exit 1
    fi
    if [ ! -f "${STATE_FILE}" ]; then
        echo "No recorded pre-demo state found — falling back to just removing the directory." >&2
        remove_demo
        exit 0
    fi

    local pre_sha
    pre_sha="$(cat "${STATE_FILE}")"

    # Safety check: refuse to touch anything if commits/changes outside the
    # demo directory happened since the baseline was recorded.
    local outside_changes
    outside_changes="$(git -C "${REPO_ROOT}" diff --stat "${pre_sha}" HEAD -- . ':(exclude)scripts/blast-radius-demo' 2>/dev/null)"
    local outside_status
    outside_status="$(git -C "${REPO_ROOT}" status --porcelain -- . ':(exclude)scripts/blast-radius-demo' 2>/dev/null)"
    if [ -n "${outside_changes}" ] || [ -n "${outside_status}" ]; then
        echo "Other changes were made outside scripts/blast-radius-demo since the demo started." >&2
        echo "Not resetting automatically — clean those up manually, then:" >&2
        echo "  rm -rf scripts/blast-radius-demo" >&2
        echo "  git reset --hard ${pre_sha}   # only if that's really what you want" >&2
        echo "  rm -f ${STATE_FILE}" >&2
        exit 1
    fi

    echo "Resetting to pre-demo state (${pre_sha})..."
    git -C "${REPO_ROOT}" reset --hard "${pre_sha}"
    rm -rf "${DEMO_DIR}"
    rm -f "${STATE_FILE}"
    echo "✓ Repo restored, demo directory removed"
}

show_usage() {
    cat << 'USAGE'
Usage: blast-radius-demo.sh <command>

Proves that LiveReview's blast-radius ordering beats naive diff order, using
a small Go service with a real call graph (auth check, DB writes, a hot
formatting helper, dead code, an HTTP entry point, a schema file).

Commands (recommended):
  run       One-shot: write baseline, commit it, inject the risky changeset,
            stage it, and run `git lrc review` if available.
  undo      One-shot: reset the repo back to exactly how it was before `run`,
            and remove the demo directory.

Commands (manual / lower-level, if you want more control):
  create    Write the baseline service only
  inject    Apply the risky changeset as an uncommitted diff over the baseline
  remove    Remove the demo directory and all contents (no git operations)

Typical usage:
  ./blast-radius-demo.sh run
  # In the LiveReview web UI: open the review -> Findings tab -> DiffViewerPanel,
  # flip "Diff order" vs "Score: Whole" to show the reorder live, and use
  # BlastRadiusPanel's signal breakdown to narrate why each hunk ranked where
  # it did.
  ./blast-radius-demo.sh undo

Notes:
  - `run` requires a clean working tree and refuses to start if a previous
    run's state file is still present (run `undo` first).
  - Blast-radius ordering only appears for reviews run through the local
    `git lrc review` CLI (it needs a live codebase-memory-mcp graph index).
    A webhook- or web-UI-triggered review will only ever show naive diff
    order — that's expected, not a bug. Make sure codebase-memory-mcp has
    indexed this repo before running `run` (index_repository if needed).
  - The blast-radius score ranks impact-of-being-wrong, not raw severity —
    pair it with severity filtering, don't use it alone.
  - The "frequently used" signal (FormatCurrency) is a static, structural
    call-graph fan-in count, not live production traffic.
USAGE
}

case "${1:-}" in
    create)
        create_demo
        ;;
    inject)
        inject_demo
        ;;
    remove)
        remove_demo
        ;;
    run)
        run_demo
        ;;
    undo)
        undo_demo
        ;;
    *)
        show_usage
        exit 1
        ;;
esac
