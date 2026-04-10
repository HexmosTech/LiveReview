package aidefault

import (
	"context"
	"database/sql"

	"github.com/livereview/internal/aiconnectors"
)

const ProviderName = "livereview-default-ai"

// ResolveConnectorOptions fetches the system default configuration for a given tier
// and returns it as aiconnectors.ConnectorOptions.
func ResolveConnectorOptions(ctx context.Context, db *sql.DB, tier string) (aiconnectors.ConnectorOptions, error) {
	storage := aiconnectors.NewStorage(db)
	return storage.GetSystemManagedConfig(ctx, tier)
}
