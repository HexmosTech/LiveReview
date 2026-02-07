# Gitea Webhook Signature Verification

## Overview
Webhook signature verification has been fully implemented for Gitea webhooks to prevent spoofing attacks and ensure webhooks originate from trusted sources.

## Implementation Details

### Components

1. **validateGiteaSignature()** - Core validation logic
   - Location: `internal/provider_input/gitea/gitea_provider.go`
   - Algorithm: HMAC-SHA256
   - Output: Hex-encoded signature
   - Comparison: Constant-time comparison to prevent timing attacks

2. **ValidateWebhookSignature()** - Provider method
   - Location: `internal/provider_input/gitea/gitea_provider.go`
   - Called by webhook orchestrator before processing
   - Looks up secret from webhook_registry table
   - Returns true/false based on validation result

3. **FindWebhookSecretByConnectorID()** - Secret lookup
   - Location: `internal/provider_input/gitea/gitea_auth.go`
   - Queries: `webhook_registry WHERE integration_token_id = $1 AND provider = 'gitea'`
   - Returns: webhook_secret or empty string if not configured

4. **Orchestrator Integration**
   - Location: `internal/api/webhook_orchestrator_v2.go`
   - Uses optional interface pattern to check if provider supports validation
   - Calls ValidateWebhookSignature() before processing webhook
   - Returns 401 Unauthorized if validation fails

## How Gitea Signature Works

### Gitea's Signature Generation
```
signature = hex(HMAC-SHA256(webhook_secret, payload))
```

Gitea sends this in the `X-Gitea-Signature` header with each webhook request.

### LiveReview's Validation Process
1. Extract `X-Gitea-Signature` from webhook headers
2. Query webhook_registry for stored secret using connector_id
3. Compute HMAC-SHA256 of webhook payload using stored secret
4. Compare computed signature with received signature (constant-time)
5. Accept if match, reject if mismatch

## Security Features

### Constant-Time Comparison
```go
return hmac.Equal([]byte(signature), []byte(expectedSignature))
```
Uses `crypto/hmac.Equal()` which performs constant-time comparison to prevent timing attacks.

### Graceful Degradation
- **No signature header + secret configured**: Logs warning, accepts webhook (backward compatibility)
- **No signature header + no secret**: Accepts webhook (manual trigger mode)
- **Signature header + no secret**: Logs warning, accepts webhook
- **Signature header + secret configured**: Validates signature, rejects if invalid

### Error Handling
- Database errors: Logs error, rejects webhook (fail secure)
- Invalid signature: Logs error with connector_id, returns 401
- All validation failures logged for security monitoring

## Configuration

### Database Schema
```sql
-- webhook_registry table stores webhook secrets
CREATE TABLE webhook_registry (
    id SERIAL PRIMARY KEY,
    provider TEXT NOT NULL,
    integration_token_id INTEGER REFERENCES integration_tokens(id),
    webhook_secret TEXT,  -- Secret used for HMAC-SHA256 validation
    ...
);
```

### Setting Up Webhook with Secret

1. **Create Integration Token** (PAT)
   ```
   POST /api/v1/connectors
   {
     "provider": "gitea",
     "pat_token": "{\"pat\":\"...\",\"username\":\"...\",\"password\":\"...\"}",
     "provider_url": "https://gitea.hexmos.site"
   }
   ```

2. **Enable Manual Trigger** (creates webhook)
   ```
   POST /api/v1/connectors/:connector_id/enable-manual-trigger
   ```
   This creates webhook_registry entry with auto-generated webhook_secret

3. **Configure Gitea Webhook**
   - URL: `https://your-livereview.com/api/v1/gitea-hook/:connector_id`
   - Secret: Use the webhook_secret from webhook_registry
   - Events: Pull Request, Issue Comment, Pull Request Comment

## Testing Signature Verification

### Valid Signature Test
```bash
# Get webhook secret from database
SECRET=$(./pgctl.sh shell -c "SELECT webhook_secret FROM webhook_registry WHERE integration_token_id = 123 LIMIT 1" -t)

# Generate signature
PAYLOAD='{"action":"created","comment":{"body":"test"}}'
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')

# Send webhook with signature
curl -X POST https://your-livereview.com/api/v1/gitea-hook/123 \
  -H "Content-Type: application/json" \
  -H "X-Gitea-Event: issue_comment" \
  -H "X-Gitea-Signature: $SIGNATURE" \
  -d "$PAYLOAD"
```

**Expected:** 200 OK, webhook processed

### Invalid Signature Test
```bash
curl -X POST https://your-livereview.com/api/v1/gitea-hook/123 \
  -H "Content-Type: application/json" \
  -H "X-Gitea-Event: issue_comment" \
  -H "X-Gitea-Signature: invalid_signature_here" \
  -d '{"action":"created","comment":{"body":"test"}}'
```

**Expected:** 401 Unauthorized with `{"error":"invalid_signature"}`

### No Signature Test (Manual Trigger Mode)
```bash
curl -X POST https://your-livereview.com/api/v1/gitea-hook/123 \
  -H "Content-Type: application/json" \
  -H "X-Gitea-Event: issue_comment" \
  -d '{"action":"created","comment":{"body":"test"}}'
```

**Expected:** 200 OK if no secret configured, or warning logged if secret exists

## Logging

### Success
```
[DEBUG] Gitea webhook signature validated successfully for connector_id=123
```

### Failure
```
[ERROR] Invalid Gitea webhook signature for connector_id=123
[ERROR] Webhook signature validation failed for connector_id=123, provider=gitea
```

### Warnings
```
[WARN] Gitea webhook missing X-Gitea-Signature header for connector_id=123
[WARN] No webhook secret configured for connector_id=123, accepting webhook
```

## Production Checklist

- [x] HMAC-SHA256 implementation
- [x] Constant-time comparison
- [x] Database secret lookup
- [x] Orchestrator integration
- [x] Error handling and logging
- [x] Graceful degradation for manual trigger mode
- [ ] End-to-end testing with real Gitea instance
- [ ] Security audit of validation logic
- [ ] Monitoring alerts for validation failures

## References

- Gitea Webhook Documentation: https://docs.gitea.io/en-us/webhooks/
- HMAC-SHA256 Spec: https://tools.ietf.org/html/rfc2104
- Constant-Time Comparison: https://pkg.go.dev/crypto/subtle

## Related Files

- `internal/provider_input/gitea/gitea_provider.go` - Validation logic
- `internal/provider_input/gitea/gitea_auth.go` - Secret lookup
- `internal/api/webhook_orchestrator_v2.go` - Integration point
- `db/migrations/20250728092945_webhook_registry.sql` - Schema definition
