\#!/usr/bin/env bash

set -euo pipefail

INPUT_FILE="internal/api/mcp/generated/server.go"

if [[ ! -f "$INPUT_FILE" ]]; then
    echo "File not found: $INPUT_FILE"
    exit 1
fi

TMP_FILE="$(mktemp)"

awk '
BEGIN {
    in_addtool_block = 0
    captured = ""
    injected = 0
}

/^[[:space:]]*s\.AddTool\(/ {
    in_addtool_block = 1
}

{
    if (in_addtool_block) {
        if ($0 ~ /^[[:space:]]*s\.AddTool\(/) {
            captured = captured $0 "\n"
            next
        } else {
            in_addtool_block = 0
        }
    }
}

/^[[:space:]]*return s/ {
    print "\t// Register all tools"
    print ""
    print "\tRegisterAllTools(s)"
    print ""
    print $0
    print "}"
    print ""

    print "func RegisterAllTools(s *server.MCPServer) {"
    print ""
    print "\t// Register all tools"
    print ""

    printf "%s", captured

    print "}"

    injected = 1
    next
}

{
    if (!injected) {
        print
    }
}
' "$INPUT_FILE" > "$TMP_FILE"

mv "$TMP_FILE" "$INPUT_FILE"

echo "Refactored MCP tool registration in: $INPUT_FILE"