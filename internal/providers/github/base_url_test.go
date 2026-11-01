package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "https://api.github.com"},
		{"standard github.com", "https://github.com", "https://api.github.com"},
		{"github.com with trailing slash", "https://github.com/", "https://api.github.com"},
		{"http github.com", "http://github.com", "https://api.github.com"},
		{"bare domain github.com", "github.com", "https://api.github.com"},
		{"already api.github.com", "https://api.github.com", "https://api.github.com"},
		{"ghe standard", "https://ghe.example.com", "https://ghe.example.com/api/v3"},
		{"ghe with trailing slash", "https://ghe.example.com/", "https://ghe.example.com/api/v3"},
		{"ghe already has api/v3", "https://ghe.example.com/api/v3", "https://ghe.example.com/api/v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apiBaseURL(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

