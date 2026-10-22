package aiconnectors

import (
	"testing"

	"github.com/tmc/langchaingo/llms/googleai"
)

func TestVertexHost(t *testing.T) {
	cases := map[string]string{
		"global":       "aiplatform.googleapis.com:443",
		" Global ":     "aiplatform.googleapis.com:443",
		"us":           "aiplatform.us.rep.googleapis.com:443",
		"eu":           "aiplatform.eu.rep.googleapis.com:443",
		"us-central1":  "",
		"europe-west4": "",
		"":             "",
	}
	for loc, want := range cases {
		if got := vertexHost(loc); got != want {
			t.Errorf("vertexHost(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestVertexEndpointOptionOnlyOverridesWhenNeeded(t *testing.T) {
	var o googleai.Options
	VertexEndpointOption("us-central1")(&o)
	if len(o.ClientOptions) != 0 {
		t.Fatalf("single region should keep the SDK's endpoint, got %d client options", len(o.ClientOptions))
	}
	VertexEndpointOption("us")(&o)
	if len(o.ClientOptions) != 1 {
		t.Fatalf("multi-region should add one endpoint option, got %d", len(o.ClientOptions))
	}
}

func TestServiceAccountProjectID(t *testing.T) {
	if got := ServiceAccountProjectID(`{"type":"service_account","project_id":"my-proj"}`); got != "my-proj" {
		t.Fatalf("got %q", got)
	}
	if got := ServiceAccountProjectID("AIza-not-json"); got != "" {
		t.Fatalf("plain API key should give no project, got %q", got)
	}
}
