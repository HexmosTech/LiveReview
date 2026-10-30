package pushnotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// vapidKeys is stored as the 'push_vapid' row in system_settings, the same
// name/data JSONB pattern internal/api/system_settings.go already uses for
// SMTP credentials — no new secret-storage mechanism invented here.
type vapidKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func readVAPIDKeys(ctx context.Context, db *sql.DB) (*vapidKeys, error) {
	var data []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM system_settings WHERE name = 'push_vapid'`).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys vapidKeys
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, err
	}
	return &keys, nil
}

// loadOrCreateVAPIDKeys returns the instance's VAPID keypair, generating and
// persisting one on first use. ON CONFLICT DO NOTHING plus a re-read handles
// the race between two processes generating keys at the same time — only one
// write wins, and every caller ends up reading that same canonical pair.
func loadOrCreateVAPIDKeys(ctx context.Context, db *sql.DB) (publicKey, privateKey string, err error) {
	if keys, err := readVAPIDKeys(ctx, db); err != nil {
		return "", "", err
	} else if keys != nil {
		return keys.PublicKey, keys.PrivateKey, nil
	}

	newPrivate, newPublic, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("generate VAPID keys: %w", err)
	}
	data, err := json.Marshal(vapidKeys{PublicKey: newPublic, PrivateKey: newPrivate})
	if err != nil {
		return "", "", err
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO system_settings (name, data) VALUES ('push_vapid', $1)
		ON CONFLICT (name) DO NOTHING
	`, data)
	if err != nil {
		return "", "", fmt.Errorf("store VAPID keys: %w", err)
	}

	// Re-read instead of trusting newPublic/newPrivate: if another process
	// lost this race, this returns the winner's keys, which is what every
	// subsequently-issued subscription must be signed/verified against.
	keys, err := readVAPIDKeys(ctx, db)
	if err != nil {
		return "", "", err
	}
	if keys == nil {
		return "", "", fmt.Errorf("VAPID keys missing immediately after insert")
	}
	return keys.PublicKey, keys.PrivateKey, nil
}

// GetVAPIDPublicKey returns the instance's public VAPID key for the
// frontend's pushManager.subscribe({applicationServerKey: ...}) call,
// generating the keypair on first use.
func GetVAPIDPublicKey(ctx context.Context, db *sql.DB) (string, error) {
	public, _, err := loadOrCreateVAPIDKeys(ctx, db)
	return public, err
}
