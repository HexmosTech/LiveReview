# Test Scripts

This directory contains test scripts for various LiveReview features.

## Available Tests

### API Integration Tests

#### test_ollama_jwt.sh
Tests Ollama connector functionality and JWT authentication.

#### test_organization_management.sh
Tests organization creation, updating, and management operations.

#### test_profile_management.sh
Tests user profile creation and management features.

#### test_super_admin.sh
Tests super admin functionality and permissions.

#### test_user_management.sh
Tests user creation, authentication, and role management.

### Webhook Tests

#### verify_gitea_webhook.sh
Tests and verifies Gitea webhook signature verification.
Related documentation: [docs/integrations/GITEA_WEBHOOK_IMPLEMENTATION.md](../../docs/integrations/GITEA_WEBHOOK_IMPLEMENTATION.md)

### System Tests

#### update_test.sh
Tests the update mechanism for LiveReview.

## Running Tests

Most tests can be run directly:
```bash
./scripts/tests/test_user_management.sh
```

Ensure the LiveReview server is running and properly configured before running these tests.
