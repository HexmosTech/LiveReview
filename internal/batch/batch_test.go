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

func validQuizJSON() string {
	return `[
		{"type":"core_objective","question":"Q1?","options":["a","b","c","d"],"correctIndex":0,"explanation":"e1"},
		{"type":"blast_radius","question":"Q2?","options":["a","b","c","d"],"correctIndex":1,"explanation":"e2"},
		{"type":"trade_off","question":"Q3?","options":["a","b","c","d"],"correctIndex":2,"explanation":"e3"},
		{"type":"edge_case","question":"Q4?","options":["a","b","c","d"],"correctIndex":3,"explanation":"e4"},
		{"type":"reviewer_confidence","question":"Q5?","options":["a","b","c","d"],"correctIndex":0,"explanation":"e5"}
	]`
}

func TestSplitSummaryAndQuiz(t *testing.T) {
	t.Run("no delimiter falls back to whole response as summary", func(t *testing.T) {
		raw := "# Some Summary\n\nJust markdown, no quiz section at all."
		summary, quiz := splitSummaryAndQuiz(raw, nil)
		if summary != raw {
			t.Errorf("summary = %q, want unchanged raw response", summary)
		}
		if quiz != nil {
			t.Errorf("quiz = %v, want nil", quiz)
		}
	})

	t.Run("well-formed delimited quiz splits and parses", func(t *testing.T) {
		raw := "# Summary Title\n\nSome body text.\n\n" + quizJSONStart + "\n" + validQuizJSON() + "\n" + quizJSONEnd
		summary, quiz := splitSummaryAndQuiz(raw, nil)
		if summary != "# Summary Title\n\nSome body text." {
			t.Errorf("summary = %q, want markdown before the delimiter", summary)
		}
		if len(quiz) != 5 {
			t.Fatalf("len(quiz) = %d, want 5", len(quiz))
		}
		if quiz[0].Type != "core_objective" || quiz[0].CorrectIndex != 0 {
			t.Errorf("quiz[0] = %+v, unexpected", quiz[0])
		}
	})

	t.Run("start delimiter without end delimiter falls back to no quiz", func(t *testing.T) {
		raw := "# Summary\n\nBody.\n\n" + quizJSONStart + "\n" + validQuizJSON()
		summary, quiz := splitSummaryAndQuiz(raw, nil)
		if summary != "# Summary\n\nBody." {
			t.Errorf("summary = %q, want markdown before the delimiter", summary)
		}
		if quiz != nil {
			t.Errorf("quiz = %v, want nil when end delimiter is missing", quiz)
		}
	})

	t.Run("malformed JSON between delimiters falls back to no quiz", func(t *testing.T) {
		raw := "# Summary\n\nBody.\n\n" + quizJSONStart + "\nnot json at all\n" + quizJSONEnd
		summary, quiz := splitSummaryAndQuiz(raw, nil)
		if summary != "# Summary\n\nBody." {
			t.Errorf("summary = %q, want markdown before the delimiter", summary)
		}
		if quiz != nil {
			t.Errorf("quiz = %v, want nil for malformed JSON", quiz)
		}
	})

	t.Run("wrong shape (4 questions instead of 5) falls back to no quiz", func(t *testing.T) {
		short := `[
			{"type":"core_objective","question":"Q1?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"blast_radius","question":"Q2?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"trade_off","question":"Q3?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"edge_case","question":"Q4?","options":["a","b","c","d"],"correctIndex":0}
		]`
		raw := "# Summary\n\nBody.\n\n" + quizJSONStart + "\n" + short + "\n" + quizJSONEnd
		_, quiz := splitSummaryAndQuiz(raw, nil)
		if quiz != nil {
			t.Errorf("quiz = %v, want nil for only 4 questions", quiz)
		}
	})

	t.Run("wrong option count falls back to no quiz", func(t *testing.T) {
		bad := `[
			{"type":"core_objective","question":"Q1?","options":["a","b","c"],"correctIndex":0},
			{"type":"blast_radius","question":"Q2?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"trade_off","question":"Q3?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"edge_case","question":"Q4?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"reviewer_confidence","question":"Q5?","options":["a","b","c","d"],"correctIndex":0}
		]`
		raw := "# Summary\n\nBody.\n\n" + quizJSONStart + "\n" + bad + "\n" + quizJSONEnd
		_, quiz := splitSummaryAndQuiz(raw, nil)
		if quiz != nil {
			t.Errorf("quiz = %v, want nil when a question has 3 options instead of 4", quiz)
		}
	})

	t.Run("out-of-range correctIndex falls back to no quiz", func(t *testing.T) {
		bad := `[
			{"type":"core_objective","question":"Q1?","options":["a","b","c","d"],"correctIndex":4},
			{"type":"blast_radius","question":"Q2?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"trade_off","question":"Q3?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"edge_case","question":"Q4?","options":["a","b","c","d"],"correctIndex":0},
			{"type":"reviewer_confidence","question":"Q5?","options":["a","b","c","d"],"correctIndex":0}
		]`
		raw := "# Summary\n\nBody.\n\n" + quizJSONStart + "\n" + bad + "\n" + quizJSONEnd
		_, quiz := splitSummaryAndQuiz(raw, nil)
		if quiz != nil {
			t.Errorf("quiz = %v, want nil when correctIndex is out of range", quiz)
		}
	})
}
