package learnings

import "testing"

func TestNullableStringPtr(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		if got := nullableStringPtr(""); got != nil {
			t.Fatalf("expected nil pointer for empty string")
		}
	})

	t.Run("non-empty string returns pointer", func(t *testing.T) {
		const input = "repo-1"
		got := nullableStringPtr(input)
		if got == nil {
			t.Fatalf("expected non-nil pointer")
		}
		if *got != input {
			t.Fatalf("expected %q, got %q", input, *got)
		}
	})
}

func TestNullableInt64Ptr(t *testing.T) {
	t.Run("zero returns nil", func(t *testing.T) {
		if got := nullableInt64Ptr(0); got != nil {
			t.Fatalf("expected nil pointer for zero")
		}
	})

	t.Run("positive value returns pointer", func(t *testing.T) {
		const input int64 = 42
		got := nullableInt64Ptr(input)
		if got == nil {
			t.Fatalf("expected non-nil pointer")
		}
		if *got != input {
			t.Fatalf("expected %d, got %d", input, *got)
		}
	})
}
