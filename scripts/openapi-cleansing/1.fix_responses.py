import sys
import yaml
import copy

print("Loading openapi.yaml...")
with open("openapi.yaml", "r", encoding="utf-8") as f:
    spec = yaml.safe_load(f)

if not spec or "paths" not in spec:
    print("ERROR: Invalid spec structure")
    sys.exit(1)

fixed_count = 0

for path, methods in spec.get("paths", {}).items():
    for method, operation in methods.items():
        if not isinstance(operation, dict):
            continue
            
        responses = operation.get("responses")
        
        # Fix if missing or null
        if responses is None or not isinstance(responses, dict):
            operation["responses"] = {
                "200": {
                    "description": "Successful response"
                },
                "400": {
                    "description": "Bad Request"
                },
                "401": {
                    "description": "Unauthorized"
                },
                "500": {
                    "description": "Internal Server Error"
                }
            }
            fixed_count += 1
            print(f"Fixed: {method.upper()} {path}")

print(f"\n✅ Fixed {fixed_count} operations with missing/null responses.")
print("Saving as openapi_fixed_responses.yaml...")

with open("openapi_fixed_responses.yaml", "w", encoding="utf-8") as f:
    yaml.dump(spec, f, sort_keys=False, allow_unicode=True, width=120)

print("Done! Use openapi_fixed.yaml for testing.")