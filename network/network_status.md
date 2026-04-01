# Network Status

Latest milestone batch note (MF-051, MF-059, MF-073, MF-074, MF-076, MF-083, MF-LOC-001, MF-LOC-002, MF-LOC-003, MF-LOC-004, MF-LOC-005, MF-LOC-006, MF-LOC-007, MF-PRORATION-001, MF-PRORATION-002, MF-PRORATION-003): outbound Razorpay surface remains unchanged for this slice; reliability hardening is focused on storage-backed payment-attempt correlation and webhook classification.

| Operation | Status | Evidence |
| --- | --- | --- |
| payment.CreateSubscriptionAddon | added | [CreateSubscriptionAddon](../internal/license/payment/payment.go#L270) |
| payment.CreateOrder | added | [CreateOrder](../internal/license/payment/payment.go#L320) |
| payment.IssueSelfHostedJWTRequest | moved | [IssueSelfHostedJWTRequest](payment/fw_parse_client.go#L21) |
| jobqueue.NewWebhookHTTPClient | moved | [NewWebhookHTTPClient](jobqueue/webhook_http_client.go#L16) |
| jobqueue.NewRequest | moved | [NewRequest](jobqueue/webhook_http_client.go#L30) |
| jobqueue.Do | moved | [Do](jobqueue/webhook_http_client.go#L45) |
| providersgitea.NewHTTPClient | moved | [NewHTTPClient](providers/gitea/http_client_ops.go#L11) |
| providersgitea.NewHTTPClientWithJar | moved | [NewHTTPClientWithJar](providers/gitea/http_client_ops.go#L18) |
| providersgitea.NewRequest | moved | [NewRequest](providers/gitea/http_client_ops.go#L25) |
| providersgitea.NewRequestWithContext | moved | [NewRequestWithContext](providers/gitea/http_client_ops.go#L33) |
| providersgitea.Do | moved | [Do](providers/gitea/http_client_ops.go#L41) |
| providersgithub.NewHTTPClient | moved | [NewHTTPClient](providers/github/http_client_ops.go#L11) |
| providersgithub.NewRequest | moved | [NewRequest](providers/github/http_client_ops.go#L18) |
| providersgithub.NewRequestWithContext | moved | [NewRequestWithContext](providers/github/http_client_ops.go#L26) |
| providersgithub.Do | moved | [Do](providers/github/http_client_ops.go#L34) |
| providersgitlab.NewHTTPClient | moved | [NewHTTPClient](providers/gitlab/http_client_ops.go#L14) |
| providersgitlab.NewRequest | moved | [NewRequest](providers/gitlab/http_client_ops.go#L21) |
| providersgitlab.NewRequestWithContext | moved | [NewRequestWithContext](providers/gitlab/http_client_ops.go#L29) |
| providersgitlab.Do | moved | [Do](providers/gitlab/http_client_ops.go#L37) |
| providersgitlab.ParseURL | moved | [ParseURL](providers/gitlab/http_client_ops.go#L47) |
| providersbitbucket.NewHTTPClient | moved | [NewHTTPClient](providers/bitbucket/http_client_ops.go#L11) |
| providersbitbucket.NewRequestWithContext | moved | [NewRequestWithContext](providers/bitbucket/http_client_ops.go#L18) |
| providersbitbucket.Do | moved | [Do](providers/bitbucket/http_client_ops.go#L27) |
| aiconnectors.NewHTTPClient | moved | [NewHTTPClient](aiconnectors/http_client_ops.go#L11) |
| aiconnectors.NewRequestWithContext | moved | [NewRequestWithContext](aiconnectors/http_client_ops.go#L18) |
| aiconnectors.Do | moved | [Do](aiconnectors/http_client_ops.go#L26) |
