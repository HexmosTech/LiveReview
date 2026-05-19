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
