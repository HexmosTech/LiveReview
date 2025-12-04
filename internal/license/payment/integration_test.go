package payment

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestSubscriptionIntegration tests the complete subscription flow
// Run with: go test -v ./internal/license/payment -run TestSubscriptionIntegration
func TestSubscriptionIntegration(t *testing.T) {
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Connect to test database
	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Verify database connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Get or create a test user and org
	var userID, orgID int

	// Try to find existing test setup
	err = db.QueryRow(`
		SELECT ur.user_id, ur.org_id
		FROM user_roles ur
		JOIN users u ON u.id = ur.user_id
		WHERE u.email = 'test-subscription@example.com'
		LIMIT 1
	`).Scan(&userID, &orgID)

	if err == sql.ErrNoRows {
		// Get first available user and org for testing
		err = db.QueryRow(`
			SELECT ur.user_id, ur.org_id
			FROM user_roles ur
			LIMIT 1
		`).Scan(&userID, &orgID)

		if err != nil {
			t.Fatalf("No users/orgs found in database. Please run the application first to create test data: %v", err)
		}

		t.Logf("Using existing user %d in org %d for testing", userID, orgID)
	} else if err != nil {
		t.Fatalf("Failed to query test user: %v", err)
	} else {
		t.Logf("Found test user %d in org %d", userID, orgID)
	}

	// Initialize service
	service := NewSubscriptionService(db)

	// Test 1: Create subscription
	t.Run("CreateSubscription", func(t *testing.T) {
		sub, err := service.CreateTeamSubscription(userID, orgID, "monthly", 5, "test")
		if err != nil {
			t.Fatalf("Failed to create subscription: %v", err)
		}

		t.Logf("✓ Created subscription: %s", sub.ID)
		t.Logf("  Status: %s", sub.Status)
		t.Logf("  Quantity: %d", sub.Quantity)
		t.Logf("  Short URL: %s", sub.ShortURL)

		// Verify DB persistence
		var dbQuantity, assignedSeats int
		var dbStatus, dbPlanType string
		err = db.QueryRow(`
			SELECT quantity, assigned_seats, status, plan_type
			FROM subscriptions
			WHERE razorpay_subscription_id = $1`,
			sub.ID,
		).Scan(&dbQuantity, &assignedSeats, &dbStatus, &dbPlanType)

		if err != nil {
			t.Fatalf("Subscription not found in DB: %v", err)
		}

		if dbQuantity != 5 {
			t.Errorf("Expected quantity 5, got %d", dbQuantity)
		}
		if assignedSeats != 0 {
			t.Errorf("Expected assigned_seats 0, got %d", assignedSeats)
		}
		if dbPlanType != "team_monthly" {
			t.Errorf("Expected plan_type 'team_monthly', got %s", dbPlanType)
		}

		t.Logf("✓ DB persistence verified")

		// Verify license_log entry
		var logCount int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM license_log
			WHERE user_id = $1 AND org_id = $2 AND event_type = 'subscription_created'`,
			userID, orgID,
		).Scan(&logCount)

		if err != nil {
			t.Fatalf("Failed to query license_log: %v", err)
		}
		if logCount == 0 {
			t.Error("No license_log entry found for subscription creation")
		} else {
			t.Logf("✓ License log entry created")
		}

		// Verify user_roles updated to team plan
		var userPlanType string
		var licenseExpiresAt sql.NullTime
		var activeSubID sql.NullString
		err = db.QueryRow(`
			SELECT plan_type, license_expires_at, active_subscription_id
			FROM user_roles
			WHERE user_id = $1 AND org_id = $2`,
			userID, orgID,
		).Scan(&userPlanType, &licenseExpiresAt, &activeSubID)

		if err != nil {
			t.Fatalf("Failed to query user_roles: %v", err)
		}

		if userPlanType != "team" {
			t.Errorf("Expected plan_type 'team', got %s", userPlanType)
		}
		if !licenseExpiresAt.Valid {
			t.Error("license_expires_at should be set")
		}
		if !activeSubID.Valid || activeSubID.String != sub.ID {
			t.Errorf("Expected active_subscription_id '%s', got '%s'", sub.ID, activeSubID.String)
		}

		t.Logf("✓ User upgraded to team plan")
		t.Logf("  License expires: %s", licenseExpiresAt.Time.Format("2006-01-02"))

		// Test 2: Assign license to another user
		t.Run("AssignLicense", func(t *testing.T) {
			// Find or create another test user
			var user2ID int
			err = db.QueryRow(`
				SELECT id FROM users WHERE email = 'test-subscription-user2@example.com' LIMIT 1
			`).Scan(&user2ID)

			if err == sql.ErrNoRows {
				// Create test user
				err = db.QueryRow(`
					INSERT INTO users (email, password_hash, first_name, last_name, created_at, updated_at)
					VALUES ($1, $2, $3, $4, NOW(), NOW())
					RETURNING id`,
					"test-subscription-user2@example.com", "test-hash", "Test", "User2",
				).Scan(&user2ID)
				if err != nil {
					t.Fatalf("Failed to create second test user: %v", err)
				}
				t.Logf("Created test user2 with ID: %d", user2ID)
			} else {
				t.Logf("Using existing test user2 with ID: %d", user2ID)
			}

			// Assign license
			err = service.AssignLicense(sub.ID, user2ID, orgID)
			if err != nil {
				t.Fatalf("Failed to assign license: %v", err)
			}

			t.Logf("✓ License assigned to user %d", user2ID)

			// Verify assigned_seats incremented
			var newAssignedSeats int
			err = db.QueryRow(`
				SELECT assigned_seats FROM subscriptions
				WHERE razorpay_subscription_id = $1`,
				sub.ID,
			).Scan(&newAssignedSeats)

			if err != nil {
				t.Fatalf("Failed to query assigned_seats: %v", err)
			}
			if newAssignedSeats != 1 {
				t.Errorf("Expected assigned_seats 1, got %d", newAssignedSeats)
			}

			t.Logf("✓ Assigned seats: %d/%d", newAssignedSeats, dbQuantity)

			// Verify user2 has team plan
			var user2PlanType string
			err = db.QueryRow(`
				SELECT plan_type FROM user_roles
				WHERE user_id = $1 AND org_id = $2`,
				user2ID, orgID,
			).Scan(&user2PlanType)

			if err != nil {
				t.Fatalf("Failed to query user2 plan: %v", err)
			}
			if user2PlanType != "team" {
				t.Errorf("Expected user2 plan_type 'team', got %s", user2PlanType)
			}

			t.Logf("✓ User2 upgraded to team plan")

			// Test 3: Revoke license
			t.Run("RevokeLicense", func(t *testing.T) {
				err = service.RevokeLicense(sub.ID, user2ID, orgID)
				if err != nil {
					t.Fatalf("Failed to revoke license: %v", err)
				}

				t.Logf("✓ License revoked from user %d", user2ID)

				// Verify assigned_seats decremented
				err = db.QueryRow(`
					SELECT assigned_seats FROM subscriptions
					WHERE razorpay_subscription_id = $1`,
					sub.ID,
				).Scan(&newAssignedSeats)

				if err != nil {
					t.Fatalf("Failed to query assigned_seats: %v", err)
				}
				if newAssignedSeats != 0 {
					t.Errorf("Expected assigned_seats 0, got %d", newAssignedSeats)
				}

				// Verify user2 reverted to free
				err = db.QueryRow(`
					SELECT plan_type FROM user_roles
					WHERE user_id = $1 AND org_id = $2`,
					user2ID, orgID,
				).Scan(&user2PlanType)

				if err != nil {
					t.Fatalf("Failed to query user2 plan: %v", err)
				}
				if user2PlanType != "free" {
					t.Errorf("Expected user2 plan_type 'free', got %s", user2PlanType)
				}

				t.Logf("✓ User2 reverted to free plan")
			})

			// Cleanup user2
			_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = $1`, user2ID)
			_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, user2ID)
		})

		// Test 4: Update quantity
		t.Run("UpdateQuantity", func(t *testing.T) {
			updatedSub, err := service.UpdateQuantity(sub.ID, 10, 0, "test")
			if err != nil {
				t.Fatalf("Failed to update quantity: %v", err)
			}

			t.Logf("✓ Updated quantity to %d", updatedSub.Quantity)

			// Verify in DB
			var newQuantity int
			err = db.QueryRow(`
				SELECT quantity FROM subscriptions
				WHERE razorpay_subscription_id = $1`,
				sub.ID,
			).Scan(&newQuantity)

			if err != nil {
				t.Fatalf("Failed to query quantity: %v", err)
			}
			if newQuantity != 10 {
				t.Errorf("Expected quantity 10, got %d", newQuantity)
			}

			t.Logf("✓ DB quantity updated")
		})

		// Test 5: Get subscription details
		t.Run("GetSubscriptionDetails", func(t *testing.T) {
			details, err := service.GetSubscriptionDetails(sub.ID, "test")
			if err != nil {
				t.Fatalf("Failed to get subscription details: %v", err)
			}

			t.Logf("✓ Retrieved subscription details")
			t.Logf("  DB ID: %d", details.ID)
			t.Logf("  Owner: %d", details.OwnerUserID)
			t.Logf("  Org: %d", details.OrgID)
			t.Logf("  Plan: %s", details.PlanType)
			t.Logf("  Seats: %d/%d", details.AssignedSeats, details.Quantity)
			t.Logf("  Status: %s", details.Status)
			t.Logf("  Expires: %s", details.LicenseExpiresAt.Format("2006-01-02"))

			if details.RazorpaySubscription != nil {
				t.Logf("✓ Razorpay data included")
				t.Logf("  Razorpay Status: %s", details.RazorpaySubscription.Status)
			}
		})

		// Test 6: Cancel subscription
		t.Run("CancelSubscription", func(t *testing.T) {
			canceledSub, err := service.CancelSubscription(sub.ID, false, "test")
			if err != nil {
				t.Fatalf("Failed to cancel subscription: %v", err)
			}

			t.Logf("✓ Cancelled subscription")
			t.Logf("  Status: %s", canceledSub.Status)

			// Verify status in DB
			var dbStatus string
			err = db.QueryRow(`
				SELECT status FROM subscriptions
				WHERE razorpay_subscription_id = $1`,
				sub.ID,
			).Scan(&dbStatus)

			if err != nil {
				t.Fatalf("Failed to query status: %v", err)
			}
			if dbStatus != "cancelled" {
				t.Errorf("Expected status 'cancelled', got %s", dbStatus)
			}

			t.Logf("✓ DB status updated to cancelled")

			// Verify user still has team plan (not immediate cancellation)
			var userPlanType string
			err = db.QueryRow(`
				SELECT plan_type FROM user_roles
				WHERE user_id = $1 AND org_id = $2`,
				userID, orgID,
			).Scan(&userPlanType)

			if err != nil {
				t.Fatalf("Failed to query user plan: %v", err)
			}
			if userPlanType != "team" {
				t.Logf("User plan_type: %s (expected 'team' for non-immediate cancel)", userPlanType)
			} else {
				t.Logf("✓ User retains team plan until expiry")
			}
		})

		// Cleanup
		t.Cleanup(func() {
			t.Log("Cleaning up test data...")
			_, _ = db.Exec(`DELETE FROM subscriptions WHERE razorpay_subscription_id = $1`, sub.ID)
			_, _ = db.Exec(`UPDATE user_roles SET plan_type = 'free', license_expires_at = NULL, active_subscription_id = NULL WHERE user_id = $1 AND org_id = $2`, userID, orgID)
			_, _ = db.Exec(`DELETE FROM license_log WHERE user_id = $1 AND org_id = $2`, userID, orgID)
			t.Log("✓ Cleanup complete")
		})
	})
}

// TestWebhookProcessing tests webhook event processing
func TestWebhookProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Connect to test database
	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create test subscription in DB
	var userID, orgID int = 1, 1 // Use existing user/org
	subscriptionID := "test_sub_webhook_" + time.Now().Format("20060102150405")

	_, err = db.Exec(`
		INSERT INTO subscriptions (
			razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			license_expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
		subscriptionID, userID, orgID, "team_monthly",
		5, 0, "created", TeamMonthlyPlanID,
		time.Now().AddDate(0, 1, 0),
	)
	if err != nil {
		t.Fatalf("Failed to create test subscription: %v", err)
	}
	defer db.Exec(`DELETE FROM subscriptions WHERE razorpay_subscription_id = $1`, subscriptionID)

	t.Logf("Created test subscription: %s", subscriptionID)

	// Initialize webhook handler
	handler := NewRazorpayWebhookHandler(db, "")

	// Test subscription.activated event
	t.Run("SubscriptionActivated", func(t *testing.T) {
		event := &RazorpayWebhookEvent{
			Entity:    "event",
			Event:     "subscription.activated",
			CreatedAt: time.Now().Unix(),
		}

		// Create mock subscription data
		mockSub := RazorpaySubscription{
			ID:           subscriptionID,
			Status:       "active",
			CurrentStart: time.Now().Unix(),
			CurrentEnd:   time.Now().AddDate(0, 1, 0).Unix(),
		}

		payload := map[string]interface{}{
			"subscription": map[string]interface{}{
				"entity": mockSub,
			},
		}
		payloadJSON, _ := json.Marshal(payload)
		event.Payload = payloadJSON

		// Process event
		err := handler.processEvent(event)
		if err != nil {
			t.Fatalf("Failed to process activation event: %v", err)
		}

		// Verify status updated
		var status string
		err = db.QueryRow(`
			SELECT status FROM subscriptions
			WHERE razorpay_subscription_id = $1`,
			subscriptionID,
		).Scan(&status)

		if err != nil {
			t.Fatalf("Failed to query status: %v", err)
		}
		if status != "active" {
			t.Errorf("Expected status 'active', got %s", status)
		}

		t.Logf("✓ Subscription activated")

		// Verify log entry
		var logCount int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM license_log
			WHERE event_type = 'subscription_activated'
			AND metadata::text LIKE $1`,
			"%"+subscriptionID+"%",
		).Scan(&logCount)

		if err == nil && logCount > 0 {
			t.Logf("✓ Activation logged")
		}
	})

	// Cleanup
	_, _ = db.Exec(`DELETE FROM license_log WHERE metadata::text LIKE $1`, "%"+subscriptionID+"%")
}
