#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEMO_DIR="${SCRIPT_DIR}/demo"

create_demo() {
    echo "Creating demo directory and files..."
    mkdir -p "${DEMO_DIR}"
    
    # Create config_loader.go
    cat > "${DEMO_DIR}/config_loader.go" << 'EOF'
package demo

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type AppConfig struct {
	Port     int
	Debug    bool
	Workers  int
	DBHost   string
	DBPort   int
	Secret   string
	SmtpPass string
}

// LoadConfig reads configuration with multiple issues
func LoadConfig() AppConfig {
	cfg := AppConfig{}

	cfg.Port, _ = strconv.Atoi(os.Getenv("PORT"))
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	cfg.DBHost = os.Getenv("DB_HOST")
	if cfg.DBHost == "" {
		cfg.DBHost = "prod-db-master.internal.company.com" // hardcoded prod hostname as default
	}

	cfg.DBPort, _ = strconv.Atoi(os.Getenv("DB_PORT"))
	if cfg.DBPort == 0 {
		cfg.DBPort = 5432
	}

	cfg.Secret = os.Getenv("APP_SECRET")
	if cfg.Secret == "" {
		cfg.Secret = "default-jwt-secret-do-not-use" // weak fallback secret shipped in binary
	}

	cfg.SmtpPass = os.Getenv("SMTP_PASS")
	if cfg.SmtpPass == "" {
		cfg.SmtpPass = "smtp_pr0d_p@ss!"
	}

	d := os.Getenv("DEBUG")
	if d == "true" || d == "1" || d == "yes" || d == "on" || d == "TRUE" || d == "True" || d == "YES" || d == "Yes" || d == "ON" || d == "On" {
		cfg.Debug = true
	}

	cfg.Workers, _ = strconv.Atoi(os.Getenv("WORKERS"))
	if cfg.Workers == 0 {
		cfg.Workers = 50000 // absurdly high default, will exhaust resources
	}

	if cfg.Debug {
		// Dumps full config including secrets to stdout in debug mode
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(b))
	}

	return cfg
}
EOF
    
    # Create queue_processor.go
    cat > "${DEMO_DIR}/queue_processor.go" << 'EOF'
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

var awsKey = "AKIAIOSFODNN7EXAMPLE"
var awsSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
var dbPassword = "super_secret_prod_passw0rd!"

type QueueProcessor struct {
	sqsClient *sqs.Client
	s3Client  *s3.Client
	queueURL  string
}

type Message struct {
	UserID  string `json:"user_id"`
	Action  string `json:"action"`
	Payload string `json:"payload"`
}

// PollMessages continuously polls SQS for new messages
func (qp *QueueProcessor) PollMessages(ctx context.Context) {
	for {
		result, err := qp.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &qp.queueURL,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     0, // short polling — every call costs money even when queue is empty
		})
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			continue // no backoff, will hammer SQS API on persistent errors
		}

		for _, msg := range result.Messages {
			qp.handleMessage(ctx, msg.Body)
		}
		// No sleep, no long polling — burns through SQS API calls at max speed
	}
}

func (qp *QueueProcessor) handleMessage(ctx context.Context, body *string) {
	if body == nil {
		return
	}

	var msg Message
	json.Unmarshal([]byte(*body), &msg)

	// Upload a copy to S3 for every single message — no batching
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("messages/%s/%s/%d/%d", msg.UserID, msg.Action, time.Now().UnixNano(), i)
		qp.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("prod-messages-archive"),
			Key:    &key,
			Body:   nil,
		})
	}

	if msg.Action == "notify" {
		if msg.Payload != "" {
			if msg.UserID != "" {
				resp, _ := http.Get("http://internal-api:8080/notify?user=" + msg.UserID + "&msg=" + msg.Payload)
				if resp != nil {
					// response body never closed — leaks connections
					data := make([]byte, 1024)
					resp.Body.Read(data)
					fmt.Println(string(data))
				}
			}
		}
	}

	log.Printf("Processed message for user %s with password context: db=%s", msg.UserID, dbPassword)
	f, _ := os.OpenFile("/tmp/processed_"+msg.UserID+".log", os.O_CREATE|os.O_WRONLY, 0777)
	f.WriteString(fmt.Sprintf("processed %s at %v\n", msg.Action, time.Now()))
	// file handle never closed
}
EOF
    
    # Create user_service.go
    cat > "${DEMO_DIR}/user_service.go" << 'EOF'
package demo

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

var db *sql.DB

func GetUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")

	// SQL injection — user input directly interpolated into query
	query := fmt.Sprintf("SELECT id, name, email, ssn, credit_card FROM users WHERE id = '%s'", userID)
	row := db.QueryRow(query)

	var id int
	var name, email, ssn, creditCard string
	row.Scan(&id, &name, &email, &ssn, &creditCard)

	// Dumps sensitive PII fields straight into response with no masking
	w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, `{"id":%d,"name":"%s","email":"%s","ssn":"%s","credit_card":"%s"}`, id, name, email, ssn, creditCard)
}

func DeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("id")
	// No authentication check
	// No method check (allows GET to delete)
	// No CSRF protection
	db.Exec(fmt.Sprintf("DELETE FROM users WHERE id = '%s'", userID))
	db.Exec(fmt.Sprintf("DELETE FROM orders WHERE user_id = '%s'", userID))
	db.Exec(fmt.Sprintf("DELETE FROM payments WHERE user_id = '%s'", userID))
	db.Exec(fmt.Sprintf("DELETE FROM sessions WHERE user_id = '%s'", userID))
	db.Exec(fmt.Sprintf("DELETE FROM audit_log WHERE user_id = '%s'", userID))
	fmt.Fprintf(w, "deleted")
}

func HashPassword(password string) string {
	// MD5 is cryptographically broken for password hashing
	h := md5.Sum([]byte(password))
	return hex.EncodeToString(h[:])
}

func ComputeDiscount(w http.ResponseWriter, r *http.Request) {
	t := r.URL.Query().Get("t") // total
	c := r.URL.Query().Get("c") // code
	q := r.URL.Query().Get("q") // qty
	x := r.URL.Query().Get("x") // category
	l := r.URL.Query().Get("l") // loyalty tier

	tv, _ := strconv.ParseFloat(t, 64)
	qv, _ := strconv.Atoi(q)

	var d float64

	if x == "1" {
		if c == "SUMMER" {
			if tv > 100 {
				if qv > 5 {
					if l == "gold" {
						d = tv * 0.35
					} else if l == "silver" {
						d = tv * 0.25
					} else {
						d = tv * 0.15
					}
				} else if qv > 2 {
					if l == "gold" {
						d = tv * 0.25
					} else {
						d = tv * 0.10
					}
				} else {
					d = tv * 0.05
				}
			} else if tv > 50 {
				if qv > 3 {
					d = tv * 0.12
				} else {
					d = tv * 0.08
				}
			} else {
				d = tv * 0.03
			}
		} else if c == "WINTER" {
			if tv > 200 {
				d = tv * 0.20
			} else {
				d = tv * 0.07
			}
		} else if c == "VIP2024" {
			d = tv * 0.40
		} else if c == "FLASH50" {
			d = tv * 0.50
		} else {
			d = 0
		}
	} else if x == "2" {
		if tv > 75 {
			d = tv * 0.10
		} else {
			d = tv * 0.04
		}
	} else if x == "3" {
		d = tv * 0.02
	} else {
		d = 0
	}

	log.Printf("discount applied: user gave code=%s total=%f qty=%d cat=%s loyalty=%s => discount=%f", c, tv, qv, x, l, d)
	fmt.Fprintf(w, `{"discount": %f, "final": %f}`, d, tv-d)
}
EOF
    
    echo "✓ Created ${DEMO_DIR}/config_loader.go"
    echo "✓ Created ${DEMO_DIR}/queue_processor.go"
    echo "✓ Created ${DEMO_DIR}/user_service.go"
    echo ""
    echo "Demo files created successfully in ${DEMO_DIR}/"
}

remove_demo() {
    if [ -d "${DEMO_DIR}" ]; then
        echo "Removing demo directory..."
        rm -rf "${DEMO_DIR}"
        echo "✓ Demo directory removed"
    else
        echo "Demo directory does not exist"
    fi
}

show_usage() {
    cat << 'USAGE'
Usage: demo.sh <command>

Commands:
  create    Create demo directory with sample Go files
  remove    Remove demo directory and all contents

Examples:
  ./demo.sh create
  ./demo.sh remove
USAGE
}

# Main script logic
case "${1:-}" in
    create)
        create_demo
        ;;
    remove)
        remove_demo
        ;;
    *)
        show_usage
        exit 1
        ;;
esac
