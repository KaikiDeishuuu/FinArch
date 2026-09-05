package service

import (
	"strings"
	"testing"
)

func TestNormalizeAndValidateUsername(t *testing.T) {
	t.Run("trims surrounding whitespace", func(t *testing.T) {
		got, err := normalizeAndValidateUsername("  张三  ")
		if err != nil {
			t.Fatalf("normalizeAndValidateUsername() error = %v", err)
		}
		if got != "张三" {
			t.Fatalf("username = %q", got)
		}
	})

	t.Run("accepts maximum Unicode length", func(t *testing.T) {
		want := strings.Repeat("界", maxUsernameRunes)
		got, err := normalizeAndValidateUsername(want)
		if err != nil {
			t.Fatalf("normalizeAndValidateUsername() error = %v", err)
		}
		if got != want {
			t.Fatalf("username changed: got %q", got)
		}
	})

	for name, value := range map[string]string{
		"blank":      " \t\n ",
		"too long":   strings.Repeat("界", maxUsernameRunes+1),
		"line break": "alice\nbob",
		"tab":        "alice\tbob",
		"NUL":        "alice\x00bob",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeAndValidateUsername(value); err == nil {
				t.Fatalf("normalizeAndValidateUsername(%q) unexpectedly succeeded", value)
			}
		})
	}
}
