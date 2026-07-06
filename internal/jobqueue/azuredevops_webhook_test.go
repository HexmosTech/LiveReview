package jobqueue

import "testing"

func TestAzureDevOpsWebhookEndpointConstruction(t *testing.T) {
	w := &WebhookInstallWorker{}

	got := w.getWebhookEndpointForProviderWithCustomEndpoint("azuredevops", "https://livereview.example.com", 42)
	want := "https://livereview.example.com/api/v1/azuredevops-hook/42"
	if got != want {
		t.Fatalf("getWebhookEndpointForProviderWithCustomEndpoint() = %q, want %q", got, want)
	}

	// Trailing slash on the configured public endpoint must be trimmed.
	got = w.getWebhookEndpointForProviderWithCustomEndpoint("azuredevops", "https://livereview.example.com/", 1)
	want = "https://livereview.example.com/api/v1/azuredevops-hook/1"
	if got != want {
		t.Fatalf("getWebhookEndpointForProviderWithCustomEndpoint() (trailing slash) = %q, want %q", got, want)
	}
}

// TestAzureDevOpsRemovalWorkerEndpointConstruction locks in that the removal
// worker computes the same webhook URL as the install worker for a given
// connector - required for removeAzureDevOpsSubscriptions to correctly match
// (and thus delete) the subscriptions installAzureDevOpsSubscriptions created.
func TestAzureDevOpsRemovalWorkerEndpointConstruction(t *testing.T) {
	installWorker := &WebhookInstallWorker{config: &QueueConfig{WebhookConfig: WebhookConfig{PublicEndpoint: "https://livereview.example.com"}}}
	removalWorker := &WebhookRemovalWorker{config: &QueueConfig{WebhookConfig: WebhookConfig{PublicEndpoint: "https://livereview.example.com"}}}

	installURL := installWorker.getWebhookEndpointForProviderWithCustomEndpoint("azuredevops", "https://livereview.example.com", 42)
	removalURL := removalWorker.getWebhookEndpointForProviderWithConnector("azuredevops", 42)

	if installURL != removalURL {
		t.Fatalf("install worker URL %q does not match removal worker URL %q - removal would fail to find/delete the subscription", installURL, removalURL)
	}
}

func TestAzureSubscriptionMatches(t *testing.T) {
	const (
		repoA = "11111111-1111-1111-1111-111111111111"
		repoB = "22222222-2222-2222-2222-222222222222"
		url   = "https://livereview.example.com/api/v1/azuredevops-hook/42"
	)

	base := azureSubscription{
		EventType:   "git.pullrequest.created",
		PublisherID: "tfs",
		PublisherInputs: map[string]any{
			"repository": repoA,
		},
		ConsumerInputs: map[string]any{
			"url": url,
		},
	}

	tests := []struct {
		name         string
		sub          azureSubscription
		eventType    string
		repositoryID string
		webhookURL   string
		want         bool
	}{
		{
			name:         "exact match",
			sub:          base,
			eventType:    "git.pullrequest.created",
			repositoryID: repoA,
			webhookURL:   url,
			want:         true,
		},
		{
			name:         "different event type does not match",
			sub:          base,
			eventType:    "git.pullrequest.updated",
			repositoryID: repoA,
			webhookURL:   url,
			want:         false,
		},
		{
			name:         "different repository does not match",
			sub:          base,
			eventType:    "git.pullrequest.created",
			repositoryID: repoB,
			webhookURL:   url,
			want:         false,
		},
		{
			name:         "different consumer url does not match (stale/old connector)",
			sub:          base,
			eventType:    "git.pullrequest.created",
			repositoryID: repoA,
			webhookURL:   "https://livereview.example.com/api/v1/azuredevops-hook/99",
			want:         false,
		},
		{
			name: "non-tfs publisher does not match",
			sub: azureSubscription{
				EventType:       "git.pullrequest.created",
				PublisherID:     "other-publisher",
				PublisherInputs: map[string]any{"repository": repoA},
				ConsumerInputs:  map[string]any{"url": url},
			},
			eventType:    "git.pullrequest.created",
			repositoryID: repoA,
			webhookURL:   url,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azureSubscriptionMatches(tt.sub, tt.eventType, tt.repositoryID, tt.webhookURL)
			if got != tt.want {
				t.Errorf("azureSubscriptionMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAzureSubscriptionHeaderStale locks in the fix for a bug where a
// subscription created before a secret was configured (or with an older
// secret) was reused forever by azureSubscriptionMatches, silently leaving
// its live consumerInputs.httpHeaders out of sync with the DB - causing every
// inbound webhook to fail signature validation with no visible symptom other
// than a rejected request at delivery time.
func TestAzureSubscriptionHeaderStale(t *testing.T) {
	const expected = "X-LiveReview-Secret: super-secret-string"

	tests := []struct {
		name string
		sub  azureSubscription
		want bool
	}{
		{
			name: "matching header is not stale",
			sub:  azureSubscription{ConsumerInputs: map[string]any{"httpHeaders": expected}},
			want: false,
		},
		{
			name: "missing header is stale",
			sub:  azureSubscription{ConsumerInputs: map[string]any{}},
			want: true,
		},
		{
			name: "different secret is stale",
			sub:  azureSubscription{ConsumerInputs: map[string]any{"httpHeaders": "X-LiveReview-Secret: old-secret"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := azureSubscriptionHeaderStale(tt.sub, expected)
			if got != tt.want {
				t.Errorf("azureSubscriptionHeaderStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAzureDevOpsSubscriptionEventTypes locks in the 3 required event types.
// Azure DevOps has no multi-event subscription, so each is a separate object.
// The comment event id deliberately does not follow the git.pullrequest.*
// naming convention - confirmed against Microsoft's Service Hooks Events docs.
func TestAzureDevOpsSubscriptionEventTypes(t *testing.T) {
	want := []string{
		"git.pullrequest.created",
		"git.pullrequest.updated",
		"ms.vss-code.git-pullrequest-comment-event",
	}
	if len(azureDevOpsSubscriptionEventTypes) != len(want) {
		t.Fatalf("got %d event types, want %d", len(azureDevOpsSubscriptionEventTypes), len(want))
	}
	for i, et := range want {
		if azureDevOpsSubscriptionEventTypes[i] != et {
			t.Errorf("azureDevOpsSubscriptionEventTypes[%d] = %q, want %q", i, azureDevOpsSubscriptionEventTypes[i], et)
		}
	}
}
