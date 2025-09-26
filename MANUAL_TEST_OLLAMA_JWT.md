# Manual Test Commands for Ollama Empty JWT Support

## Prerequisites
Make sure your LiveReview server is running on http://localhost:8080 (or adjust the URLs below)

## Test 1: Create Ollama connector with empty JWT token

```bash
curl -X POST http://localhost:8080/api/v1/aiconnectors \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "ollama",
    "api_key": "",
    "connector_name": "Test-Empty-JWT",
    "base_url": "http://localhost:11434/api",
    "selected_model": "llama3:latest",
    "display_order": 1
  }'
```

**Expected Result:** Should return a 201 Created response with connector details.
**Look for:** `"api_key_preview": "****"` (empty API key should show as masked)

## Test 2: Update the connector with empty JWT token

First, note the `id` from Test 1 response, then replace `{CONNECTOR_ID}` below:

```bash
curl -X PUT http://localhost:8080/api/v1/aiconnectors/{CONNECTOR_ID} \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "ollama",
    "api_key": "",
    "connector_name": "Updated-Empty-JWT",
    "base_url": "http://localhost:11434/api",
    "selected_model": "llama3:8b",
    "display_order": 1
  }'
```

**Expected Result:** Should return a 200 OK response with updated connector details.
**Look for:** Successfully updated name and model while keeping empty API key.

## Test 3: Update with a JWT token

```bash
curl -X PUT http://localhost:8080/api/v1/aiconnectors/{CONNECTOR_ID} \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "ollama",
    "api_key": "test-jwt-token-123",
    "connector_name": "With-JWT-Token",
    "base_url": "http://localhost:11434/api",
    "selected_model": "llama3:latest",
    "display_order": 1
  }'
```

**Expected Result:** Should return 200 OK with updated connector.
**Look for:** `"api_key_preview"` should now show masked version of the JWT token.

## Test 4: Update back to empty JWT token

```bash
curl -X PUT http://localhost:8080/api/v1/aiconnectors/{CONNECTOR_ID} \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "ollama",
    "api_key": "",
    "connector_name": "Back-To-Empty",
    "base_url": "http://localhost:11434/api",
    "selected_model": "llama3:8b",
    "display_order": 1
  }'
```

**Expected Result:** Should return 200 OK.
**Look for:** `"api_key_preview"` should be back to showing masked empty value.

## Test 5: Verify by listing all connectors

```bash
curl -X GET http://localhost:8080/api/v1/aiconnectors
```

**Expected Result:** Should return array including your test connector.
**Look for:** Confirm the connector exists and has the expected values.

## Cleanup: Delete the test connector

```bash
curl -X DELETE http://localhost:8080/api/v1/aiconnectors/{CONNECTOR_ID}
```

**Expected Result:** Should return 200 OK with success message.

## What to Test For

1. **Empty JWT Creation:** ✅ Should work without validation errors
2. **Empty JWT Update:** ✅ Should update successfully without requiring API key
3. **JWT to Empty Transition:** ✅ Should allow clearing the JWT token
4. **Empty to JWT Transition:** ✅ Should allow setting a JWT token
5. **API Key Preview:** ✅ Should show appropriate masked values for both empty and non-empty keys

## Testing Other Providers (Should Still Require API Key)

Try creating a non-Ollama connector with empty API key - this should fail:

```bash
curl -X POST http://localhost:8080/api/v1/aiconnectors \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "openai",
    "api_key": "",
    "connector_name": "Test-OpenAI-Empty",
    "display_order": 1
  }'
```

**Expected Result:** Should return 400 Bad Request with "API key is required" error.