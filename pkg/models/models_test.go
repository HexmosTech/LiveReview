package models

import (
	"encoding/json"
	"testing"
)

func TestReviewComment_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		wantFile string
		wantTool string
		wantLine int
	}{
		{
			name:     "snake_case",
			jsonStr:  `{"file_path":"src/index.js","tool_name":"eslint","line":10}`,
			wantFile: "src/index.js",
			wantTool: "eslint",
			wantLine: 10,
		},
		{
			name:     "PascalCase",
			jsonStr:  `{"FilePath":"src/auth.py","ToolName":"bandit","Line":25}`,
			wantFile: "src/auth.py",
			wantTool: "bandit",
			wantLine: 25,
		},
		{
			name:     "camelCase",
			jsonStr:  `{"filePath":"src/utils.go","toolName":"gitleaks","line":5}`,
			wantFile: "src/utils.go",
			wantTool: "gitleaks",
			wantLine: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rc ReviewComment
			if err := json.Unmarshal([]byte(tc.jsonStr), &rc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rc.FilePath != tc.wantFile {
				t.Errorf("got FilePath %q, want %q", rc.FilePath, tc.wantFile)
			}
			if rc.ToolName != tc.wantTool {
				t.Errorf("got ToolName %q, want %q", rc.ToolName, tc.wantTool)
			}
			if rc.Line != tc.wantLine {
				t.Errorf("got Line %d, want %d", rc.Line, tc.wantLine)
			}
		})
	}
}

