#!/usr/bin/env python3
"""
Fix OpenAPI spec by removing Go-style @ annotation lines
(e.g. @Summary, @Tags, @Router) from description fields.
Only retains the @Description line content (without the prefix).
Detects such patterns generically — no hardcoded strings.
"""
import sys


def fix_openapi_spec(filepath):
    with open(filepath, 'r') as f:
        lines = f.readlines()

    result = []
    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.lstrip()

        # Detect YAML literal block scalar for description
        if stripped.startswith('description:') and ('|' in stripped):
            desc_indent = len(line) - len(line.lstrip())
            block_indent = desc_indent + 2

            block_lines = []
            j = i + 1
            while j < len(lines):
                if lines[j].startswith(' ' * block_indent) and lines[j].strip():
                    block_lines.append(lines[j])
                    j += 1
                elif not lines[j].strip():
                    block_lines.append(lines[j])
                    j += 1
                else:
                    break

            has_annotations = any(
                l.strip().startswith('@') for l in block_lines if l.strip()
            )

            if has_annotations:
                desc_text = ""
                for bl in block_lines:
                    bl_stripped = bl.strip()
                    if bl_stripped.startswith('@Description'):
                        desc_text = bl_stripped[len('@Description'):].strip()
                        break

                if desc_text:
                    result.append(f'{" " * desc_indent}description: "{desc_text}"\n')
                    i = j
                    continue

        result.append(line)
        i += 1

    with open(filepath, 'w') as f:
        f.writelines(result)

    print(f"Fixed OpenAPI spec in {filepath}")


if __name__ == '__main__':
    filepath = sys.argv[1] if len(sys.argv) > 1 else 'docs/openapi.yaml'
    fix_openapi_spec(filepath)
