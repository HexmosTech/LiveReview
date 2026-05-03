package api

import "testing"

func TestDocsBuilder(t *testing.T) {
	b := NewDocsBuilder()
	// Attempt to provide routes to catch any panic due to missing mock methods/structs
	b.ProvideRoutes()
}
