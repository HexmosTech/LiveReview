#!/usr/bin/env python3
"""
Fix OpenAPI spec:
1. Remove Go-style @ annotation lines from description fields.
2. Re-order path parameters to match their order in the URL path template.
   (typed sometimes emits parameters in the wrong order.)
"""
import re
import sys


def fix_annotation_descriptions(lines):
    """Strip @Annotation lines from description literal blocks."""
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
    return result


def fix_path_param_order(lines):
    """
    Re-order 'in: path' parameter blocks within each operation so their
    order matches the order the path template variables appear in the URL.

    Example: /api/v1/orgs/{org_id}/tools/{tool_id}
      => parameters should appear as org_id first, tool_id second.
    """
    content = "".join(lines)

    # Match each top-level path entry in the YAML (2-space indent)
    path_block_re = re.compile(
        r'^  (/[^\n]*):\n((?:(?!^  /).*\n)*)',
        re.MULTILINE,
    )

    def get_param_name(item):
        name_m = re.search(r'name:\s*(\S+)', item)
        return name_m.group(1) if name_m else None

    def is_path_param(item):
        return 'in: path' in item

    def fix_params_in_method_block(method_text, template_params):
        # Find the parameters: list inside this method block
        params_section_re = re.compile(
            r'(      parameters:\n)((?:(?:        - .*\n|          .*\n))*)',
        )
        m = params_section_re.search(method_text)
        if not m:
            return method_text

        params_header = m.group(1)
        params_body = m.group(2)

        # Split individual parameter items (each starting with "        - ")
        param_items = re.findall(
            r'(        - .*\n(?:          .*\n)*)',
            params_body,
        )
        if len(param_items) < 2:
            return method_text

        path_items = {}
        non_path_items = []
        for p in param_items:
            if is_path_param(p):
                name = get_param_name(p)
                if name:
                    path_items[name] = p
            else:
                non_path_items.append(p)

        if len(path_items) < 2:
            return method_text

        # Build ordered path params according to template_params order
        ordered_path = []
        for tp in template_params:
            if tp in path_items:
                ordered_path.append(path_items[tp])
        # Append any path params not in the template (shouldn't happen, but safe)
        for name, item in path_items.items():
            if name not in template_params:
                ordered_path.append(item)

        new_params_body = "".join(ordered_path + non_path_items)
        if new_params_body == params_body:
            return method_text

        return method_text[:m.start()] + params_header + new_params_body + method_text[m.end():]

    def reorder_params_in_block(path_template, block_text):
        # Extract the ordered list of {param} names from the URL template
        template_params = re.findall(r'\{([^}]+)\}', path_template)
        if len(template_params) < 2:
            return block_text  # Nothing to reorder

        # Split block_text into per-HTTP-method sub-blocks
        method_split_re = re.compile(
            r'(    (?:get|put|post|delete|patch|options|head):\n(?:(?!    (?:get|put|post|delete|patch|options|head):).*\n)*)',
        )
        parts = method_split_re.split(block_text)

        result_parts = []
        for part in parts:
            result_parts.append(fix_params_in_method_block(part, template_params))
        return "".join(result_parts)

    def replace_block(match):
        path_template = match.group(1)
        block_text = match.group(2)
        fixed = reorder_params_in_block(path_template, block_text)
        return f"  {path_template}:\n{fixed}"

    fixed_content = path_block_re.sub(replace_block, content)
    return fixed_content.splitlines(keepends=True)


def fix_openapi_spec(filepath):
    with open(filepath, 'r') as f:
        lines = f.readlines()

    lines = fix_annotation_descriptions(lines)
    lines = fix_path_param_order(lines)

    with open(filepath, 'w') as f:
        f.writelines(lines)

    print(f"Fixed OpenAPI spec in {filepath}")


if __name__ == '__main__':
    filepath = sys.argv[1] if len(sys.argv) > 1 else 'docs/openapi.yaml'
    fix_openapi_spec(filepath)
