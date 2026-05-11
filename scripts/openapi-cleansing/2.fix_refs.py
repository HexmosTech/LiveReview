#!/usr/bin/env python3
import sys
import yaml
import re
from collections import defaultdict

def sanitize_go_identifier(name: str, allow_underscore_prefix: bool = False) -> str:
    """Convert any string into a valid Go identifier."""
    if not name:
        return "Unnamed"
    
    # Replace invalid characters
    name = re.sub(r'[^a-zA-Z0-9_]', '_', name)   # - . / { } etc → _
    name = re.sub(r'_+', '_', name)               # multiple ___ → single _
    
    # Remove leading/trailing underscores (unless allowed)
    if not allow_underscore_prefix:
        name = name.strip('_')
    
    # Ensure it starts with a letter
    if name and not name[0].isalpha():
        name = "Op" + name
    
    # CamelCase for better readability (optional but nice)
    parts = name.split('_')
    name = parts[0] + ''.join(p.capitalize() for p in parts[1:])
    
    return name if name else "UnnamedOperation"


def shorten_tool_name(name: str, max_len: int = 60) -> str:
    """Shorten tool name to stay under Claude's 64 character limit (minimal change)."""
    if len(name) <= max_len:
        return name

    # Minimal aggressive shortening for long webhook-style names
    name = name.replace("WebhookOrchestratorV2Handler", "WebhookV2")
    name = name.replace("apiV1", "")
    name = name.replace("ConnectorId", "Conn")
    name = name.replace("Webhooks", "Hooks")
    name = name.replace("Comments", "Cmt")
    name = name.replace("Gitlab", "GitLab")
    name = name.replace("Github", "GitHub")

    # Final cleanup + truncate
    name = re.sub(r'[^a-zA-Z0-9_]', '_', name)
    name = re.sub(r'_+', '_', name).strip('_')
    
    if len(name) > max_len:
        name = name[:max_len]
    
    return sanitize_go_identifier(name)


def sanitize_for_go(text: str) -> str:
    """Sanitize strings for Go code (descriptions, examples, etc.)."""
    if not text:
        return ""
    text = str(text)
    text = text.replace('\\', '\\\\')
    text = text.replace('"', '\\"')
    text = text.replace('\n', '\\n')
    text = text.replace('\r', '')
    text = text.replace('\t', '\\t')
    text = re.sub(r'[\x00-\x1F\x7F]', '', text)
    return text


def clean_spec_recursively(obj):
    """Recursively clean dangerous fields."""
    if isinstance(obj, dict):
        if 'example' in obj:
            del obj['example']
        if 'examples' in obj:
            del obj['examples']

        for key, value in list(obj.items()):
            if key == 'description' and isinstance(value, str):
                obj[key] = sanitize_for_go(value)
            elif key == 'operationId' and isinstance(value, str):
                # We'll handle operationId separately for uniqueness + Go safety
                pass
            else:
                clean_spec_recursively(value)

    elif isinstance(obj, list):
        for item in obj:
            clean_spec_recursively(item)


def fix_openapi(input_file: str, output_file: str = None, strip_examples: bool = True):
    with open(input_file, 'r', encoding='utf-8') as f:
        spec = yaml.safe_load(f)

    # === 1. Fix Missing Schemas ===
    schemas = spec.setdefault('components', {}).setdefault('schemas', {})

    def find_refs(obj, refs=None):
        if refs is None:
            refs = set()
        if isinstance(obj, dict):
            for k, v in obj.items():
                if k == '$ref' and isinstance(v, str) and v.startswith('#/components/schemas/'):
                    refs.add(v.split('/')[-1])
                else:
                    find_refs(v, refs)
        elif isinstance(obj, list):
            for item in obj:
                find_refs(item, refs)
        return refs

    missing = [name for name in find_refs(spec) if name not in schemas]

    for name in missing:
        if name == "jwt.NumericDate":
            schemas[name] = {
                "type": "integer",
                "format": "int64",
                "description": "JWT NumericDate (seconds since Unix epoch)"
            }
        else:
            schemas[name] = {
                "type": "object",
                "description": f"Auto-generated stub for {name}",
                "x-original-name": name,
                "additionalProperties": True,
                "properties": {}
            }

    print(f"✅ Added {len(missing)} missing schema stubs.")

    # === 2. Global Cleanup ===
    if strip_examples:
        print("🧹 Stripping 'example'/'examples' fields...")
        clean_spec_recursively(spec)

    # === 3. Fix OperationIds (Critical for this error) ===
    print("🔧 Sanitizing operationIds for valid Go identifiers...")
    op_id_count = defaultdict(list)
    paths = spec.get('paths', {})

    for path, methods in paths.items():
        for method, op in methods.items():
            if isinstance(op, dict):
                original_id = op.get('operationId')
                if original_id:
                    op_id_count[original_id].append((path, method.upper()))

    duplicates = {oid: locs for oid, locs in op_id_count.items() if len(locs) > 1}

    counter = defaultdict(int)
    for path, methods in paths.items():
        for method, op in methods.items():
            if not isinstance(op, dict):
                continue
            op_id = op.get('operationId')
            if not op_id:
                continue

            counter[op_id] += 1
            new_id = sanitize_go_identifier(op_id)

            # Add suffix for duplicates
            if counter[op_id] > 1 or op_id != new_id:
                suffix = sanitize_go_identifier(path.strip('/').replace('/', '_'))
                new_id = f"{new_id}_{suffix}" if suffix else new_id

            # === NEW: Enforce Claude 64-char limit (minimal addition) ===
            new_id = shorten_tool_name(new_id)

            op['operationId'] = new_id
            print(f"   {method} {path}  →  {new_id}  ({len(new_id)} chars)")

    # === Save ===
    if output_file is None:
        output_file = input_file.replace('.yaml', '_go_safe.yaml')

    with open(output_file, 'w', encoding='utf-8') as f:
        yaml.dump(spec, f,
                  sort_keys=False,
                  allow_unicode=True,
                  width=120,
                  default_flow_style=False)

    print(f"\n🎉 Fixed file saved as: {output_file}")
    print("   → Next: Run mcpgen on this file")
    return output_file


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python fix_openapi.py <input.yaml> [output.yaml]")
        print("       --no-strip-examples")
        sys.exit(1)

    strip = '--no-strip-examples' not in ' '.join(sys.argv)
    args = [a for a in sys.argv[1:] if not a.startswith('--')]

    input_file = args[0]
    output_file = args[1] if len(args) > 1 else None

    fix_openapi(input_file, output_file, strip_examples=strip)