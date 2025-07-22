package tests

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"
	"unsafe"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/api"
)

func TestGetConnectorByProviderURL(t *testing.T) {
	// Connect to the database (hardcoded for test)
	dbURL := "postgres://livereview:Qw7%21vRu%239eLt3pZ@127.0.0.1:5432/livereview?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping DB: %v", err)
	}

	// Create Server struct
	srv := &api.Server{}
	setUnexportedField(srv, "db", db)

	// Test input
	providerURL := "https://git.apps.hexmos.com"

	// Call the function
	result, err := srv.GetConnectorByProviderURL(providerURL)
	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	// Print all fields of the returned struct
	fmt.Printf("ID: %d\n", result.ID)
	fmt.Printf("Provider: %s\n", result.Provider)
	fmt.Printf("ProviderAppID: %s\n", result.ProviderAppID)
	fmt.Printf("AccessToken: %s\n", result.AccessToken)
	fmt.Printf("RefreshToken: %s\n", nullString(result.RefreshToken))
	fmt.Printf("TokenType: %s\n", nullString(result.TokenType))
	fmt.Printf("Scope: %s\n", nullString(result.Scope))
	fmt.Printf("ExpiresAt: %s\n", nullTime(result.ExpiresAt))
	fmt.Printf("Metadata: %s\n", string(result.Metadata))
	fmt.Printf("CreatedAt: %s\n", result.CreatedAt.Format(time.RFC3339))
	fmt.Printf("UpdatedAt: %s\n", result.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Code: %s\n", nullString(result.Code))
	fmt.Printf("ConnectionName: %s\n", result.ConnectionName)
	fmt.Printf("ProviderURL: %s\n", result.ProviderURL)
	fmt.Printf("ClientSecret: %s\n", nullString(result.ClientSecret))
}

func setUnexportedField(obj interface{}, fieldName string, value interface{}) {
	v := reflect.ValueOf(obj).Elem().FieldByName(fieldName)
	reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func nullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return "<NULL>"
}

func nullTime(nt sql.NullTime) string {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return "<NULL>"
}
