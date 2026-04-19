# Network Status

Latest milestone batch note (MF-051, MF-059, MF-073, MF-074, MF-076, MF-083, MF-LOC-001, MF-LOC-002, MF-LOC-003, MF-LOC-004, MF-LOC-005, MF-LOC-006, MF-LOC-007, MF-LOC-008, MF-PRORATION-001, MF-PRORATION-002, MF-PRORATION-003, MF-ATTRIB-001, MF-ATTRIB-002, MF-PORTFOLIO-001, MF-NOTIFY-001, MF-NOTIFY-002, MF-DASHBOARD-LOG-001, MF-EXPIRY-001, MF-UPI-UPGRADE-001, MF-UPI-UPGRADE-002, MF-UPI-UPGRADE-003, MF-UPI-UPGRADE-004): added replacement-cutover current-subscription fallback selection and kept org-scoped subscription listing on the settings API path.

| Operation | Status | Evidence |
| --- | --- | --- |
| payment.CreateSubscriptionAddon | added | [CreateSubscriptionAddon](../internal/license/payment/payment.go#L309) |
| payment.CreateOrder | added | [CreateOrder](../internal/license/payment/payment.go#L359) |
| payment.CreateSubscriptionAt | added | [CreateSubscriptionAt](../internal/license/payment/subscription.go#L78) |
| payment.CancelScheduledChangesByID | added | [CancelScheduledChangesByID](../internal/license/payment/subscription.go#L295) |
| api.GetCurrentSubscription | updated | [GetCurrentSubscription](../internal/api/subscriptions_handler.go#L605) |
| api.ListUserSubscriptions | updated | [ListUserSubscriptions](../internal/api/subscriptions_handler.go#L758) |
| payment.IssueSelfHostedJWTRequest | moved | [IssueSelfHostedJWTRequest](payment/fw_parse_client.go#L21) |
| payment.SendBillingNotificationEmailPlaceholder | added | [SendBillingNotificationEmailPlaceholder](payment/billing_notification_sender.go#L18) |
| jobqueue.NewWebhookHTTPClient | moved | [NewWebhookHTTPClient](jobqueue/webhook_http_client.go#L16) |
| jobqueue.NewRequest | moved | [NewRequest](jobqueue/webhook_http_client.go#L30) |
| jobqueue.Do | moved | [Do](jobqueue/webhook_http_client.go#L45) |
| providersgitea.NewHTTPClient | moved | [NewHTTPClient](providers/gitea/http_client_ops.go#L12) |
| providersgitea.NewHTTPClientWithJar | moved | [NewHTTPClientWithJar](providers/gitea/http_client_ops.go#L19) |
| providersgitea.NewRequest | moved | [NewRequest](providers/gitea/http_client_ops.go#L26) |
| providersgitea.NewRequestWithContext | moved | [NewRequestWithContext](providers/gitea/http_client_ops.go#L34) |
| providersgitea.Do | moved | [Do](providers/gitea/http_client_ops.go#L42) |
| providersgitea.FetchPatchContent | added | [FetchPatchContent](providers/gitea/http_client_ops.go#L52) |
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
