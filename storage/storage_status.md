# Storage Status

| Operation | Status | Evidence |
| --- | --- | --- |
| payment.NewSubscriptionStore | moved | [NewSubscriptionStore](payment/subscription_store.go#L24) |
| payment.CreateTeamSubscriptionRecord | moved | [CreateTeamSubscriptionRecord](payment/subscription_store.go#L43) |
| payment.UpdateSubscriptionQuantityRecord | moved | [UpdateSubscriptionQuantityRecord](payment/subscription_store.go#L108) |
| payment.CancelSubscriptionRecord | moved | [CancelSubscriptionRecord](payment/subscription_store.go#L176) |
| payment.GetSubscriptionDetailsRow | moved | [GetSubscriptionDetailsRow](payment/subscription_store.go#L270) |
| payment.AssignLicense | moved | [AssignLicense](payment/subscription_store.go#L301) |
| payment.RevokeLicense | moved | [RevokeLicense](payment/subscription_store.go#L414) |
| payment.GetUserIDByEmail | moved | [GetUserIDByEmail](payment/subscription_store.go#L494) |
| payment.CreateShadowUser | moved | [CreateShadowUser](payment/subscription_store.go#L506) |
| payment.CreateSelfHostedSubscriptionRecord | moved | [CreateSelfHostedSubscriptionRecord](payment/subscription_store.go#L534) |
| payment.GetSelfHostedConfirmationSeed | moved | [GetSelfHostedConfirmationSeed](payment/subscription_store.go#L599) |
| payment.PersistSelfHostedFallback | moved | [PersistSelfHostedFallback](payment/subscription_store.go#L636) |
| payment.PersistSelfHostedJWT | moved | [PersistSelfHostedJWT](payment/subscription_store.go#L699) |
| jobqueue.NewWebhookStore | moved | [NewWebhookStore](jobqueue/webhook_store.go#L24) |
| jobqueue.GetWebhookPublicEndpoint | moved | [GetWebhookPublicEndpoint](jobqueue/webhook_store.go#L33) |
| jobqueue.GetWebhookRegistryID | moved | [GetWebhookRegistryID](jobqueue/webhook_store.go#L52) |
| jobqueue.InsertWebhookRegistry | moved | [InsertWebhookRegistry](jobqueue/webhook_store.go#L84) |
| jobqueue.UpdateWebhookRegistryByID | moved | [UpdateWebhookRegistryByID](jobqueue/webhook_store.go#L122) |
| jobqueue.GetConnectorMetadata | moved | [GetConnectorMetadata](jobqueue/webhook_store.go#L145) |
| license.GetLicenseState | moved | [GetLicenseState](license/license_state_store.go#L41) |
| license.UpsertLicenseState | moved | [UpsertLicenseState](license/license_state_store.go#L56) |
| license.UpdateValidationResult | moved | [UpdateValidationResult](license/license_state_store.go#L88) |
| license.DeleteLicenseState | moved | [DeleteLicenseState](license/license_state_store.go#L119) |
