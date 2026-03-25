package gitlab

import "testing"

func TestParseURLAcceptsValidHTTPAndHTTPS(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		host    string
		scheme  string
		hasPath bool
	}{
		{
			name:    "https trailing slash",
			rawURL:  "https://gitlab.example.com/",
			host:    "gitlab.example.com",
			scheme:  "https",
			hasPath: true,
		},
		{
			name:    "http unusual port",
			rawURL:  "http://gitlab.example.com:8443/api/v4",
			host:    "gitlab.example.com:8443",
			scheme:  "http",
			hasPath: true,
		},
		{
			name:    "https ipv6 with port",
			rawURL:  "https://[2001:db8::1]:9443/group/repo",
			host:    "[2001:db8::1]:9443",
			scheme:  "https",
			hasPath: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseURL(tc.rawURL)
			if err != nil {
				t.Fatalf("ParseURL(%q) returned unexpected error: %v", tc.rawURL, err)
			}
			if parsed.Scheme != tc.scheme {
				t.Fatalf("expected scheme %q, got %q", tc.scheme, parsed.Scheme)
			}
			if parsed.Host != tc.host {
				t.Fatalf("expected host %q, got %q", tc.host, parsed.Host)
			}
			if tc.hasPath && parsed.Path == "" {
				t.Fatalf("expected non-empty path for %q", tc.rawURL)
			}
		})
	}
}

func TestParseURLRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
	}{
		{name: "relative path", rawURL: "/api/v4"},
		{name: "host without scheme", rawURL: "gitlab.example.com/api/v4"},
		{name: "unsupported scheme", rawURL: "ftp://gitlab.example.com"},
		{name: "missing host", rawURL: "https:///api/v4"},
		{name: "invalid ipv6", rawURL: "https://[2001:db8::1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseURL(tc.rawURL)
			if err == nil {
				t.Fatalf("expected ParseURL(%q) to fail", tc.rawURL)
			}
		})
	}
}
