package main

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed hooks/prepare-commit-msg.sh hooks/commit-msg.sh hooks/post-commit.sh
var hookTemplatesFS embed.FS

const (
	hookMarkerBeginPlaceholder       = "__LRC_MARKER_BEGIN__"
	hookMarkerEndPlaceholder         = "__LRC_MARKER_END__"
	hookVersionPlaceholder           = "__LRC_VERSION__"
	hookCommitMessageFilePlaceholder = "__LRC_COMMIT_MESSAGE_FILE__"
	hookPushRequestFilePlaceholder   = "__LRC_PUSH_REQUEST_FILE__"
)

func renderHookTemplate(path string, replacements map[string]string) string {
	content, err := hookTemplatesFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("failed to load hook template %s: %v", path, err))
	}

	rendered := string(content)
	for placeholder, value := range replacements {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}

	return rendered
}
