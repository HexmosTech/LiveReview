package aiconnectors

import (
	"encoding/json"
	"strings"

	"github.com/tmc/langchaingo/llms/googleai"
	"google.golang.org/api/option"
)

// VertexEndpointOption points the Vertex AI client at the host that serves location. The Vertex SDK always dials
// "<location>-aiplatform.googleapis.com", which only exists for single regions such as us-central1. The global
// location is served at aiplatform.googleapis.com and the us/eu multi-regions at aiplatform.<location>.rep.googleapis.com,
// and newer Gemini models (3.x) are only available there.
func VertexEndpointOption(location string) googleai.Option {
	host := vertexHost(location)
	return func(o *googleai.Options) {
		if host != "" {
			// Appended after the SDK's own endpoint option, so this one wins.
			o.ClientOptions = append(o.ClientOptions, option.WithEndpoint(host))
		}
	}
}

func vertexHost(location string) string {
	switch l := strings.ToLower(strings.TrimSpace(location)); l {
	case "global":
		return "aiplatform.googleapis.com:443"
	case "us", "eu":
		return "aiplatform." + l + ".rep.googleapis.com:443"
	default:
		return "" // single region: the SDK's default host is right
	}
}

// ServiceAccountProjectID returns the project_id of a Google service-account JSON key ("" if it isn't one).
func ServiceAccountProjectID(credentialsJSON string) string {
	var sa struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &sa); err != nil {
		return ""
	}
	return sa.ProjectID
}
