package batch

import (
	"testing"

	"github.com/livereview/pkg/models"
)

func TestIsBinaryFile(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Plain text",
			content:  "This is a plain text file with normal content.\nIt has multiple lines and some special chars like $@#%.",
			expected: false,
		},
		{
			name:     "Code file",
			content:  "package main\n\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}\n",
			expected: false,
		},
		{
			name:     "File with null byte",
			content:  "This file has a null byte \x00 in it.",
			expected: true,
		},
		{
			name:     "Binary content",
			content:  "\x7F\x45\x4C\x46\x02\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x3E\x00\x01\x00\x00\x00",
			expected: true,
		},
		{
			name:     "High non-printable ratio",
			content:  "Normal text with \x01\x02\x03\x04\x05\x06\x07\x08\x0B\x0C\x0E\x0F\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1A\x1B\x1C\x1D\x1E\x1F many control chars",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsBinaryFile(tt.content)
			if result != tt.expected {
				t.Errorf("IsBinaryFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldSkipFile(t *testing.T) {
	processor := DefaultBatchProcessor()

	tests := []struct {
		name     string
		filePath string
		content  string
		expected bool
	}{
		{
			name:     "Text file with text extension",
			filePath: "file.txt",
			content:  "This is a text file.",
			expected: false,
		},
		{
			name:     "Code file",
			filePath: "main.go",
			content:  "package main\n\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}\n",
			expected: false,
		},
		{
			name:     "Image file",
			filePath: "image.png",
			content:  "Some content that won't be checked",
			expected: true,
		},
		{
			name:     "Binary file with binary extension",
			filePath: "program.exe",
			content:  "\x7F\x45\x4C\x46\x02\x01\x01\x00",
			expected: true,
		},
		{
			name:     "Binary content with text extension",
			filePath: "suspicious.txt",
			content:  "\x7F\x45\x4C\x46\x02\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x3E\x00\x01\x00\x00\x00",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := &models.CodeDiff{
				FilePath: tt.filePath,
				Hunks: []models.DiffHunk{
					{
						Content: tt.content,
					},
				},
			}

			result := processor.shouldSkipFile(diff)
			if result != tt.expected {
				t.Errorf("shouldSkipFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}
