package ocr

import (
	"strings"
	"testing"
)

func TestLimitedJSONPreservesLargeIntegerCents(t *testing.T) {
	data := `{"amount_cents":9007199254740993,"confidence":0.75}`
	var raw map[string]any
	if err := decodeLimitedJSONResponse(strings.NewReader(data), int64(len(data)), &raw); err != nil {
		t.Fatalf("decode exact-limit JSON: %v", err)
	}
	suggestion := parseSuggestion(raw)
	if suggestion.AmountCents == nil || *suggestion.AmountCents != 9007199254740993 {
		t.Fatalf("amount cents = %v, want exact large integer", suggestion.AmountCents)
	}
	if suggestion.Confidence != 0.75 {
		t.Fatalf("confidence = %v, want 0.75", suggestion.Confidence)
	}
}

func TestLimitedJSONRejectsOneByteOverLimit(t *testing.T) {
	data := `{"amount_cents":123}`
	var raw map[string]any
	if err := decodeLimitedJSONResponse(strings.NewReader(data), int64(len(data)-1), &raw); err == nil {
		t.Fatal("response one byte over the limit was accepted")
	}
}

func TestSuggestionRejectsInexactFloatInteger(t *testing.T) {
	suggestion := parseSuggestion(map[string]any{"amount_cents": float64(9007199254740992)})
	if suggestion.AmountCents != nil {
		t.Fatalf("inexact float cents unexpectedly accepted: %d", *suggestion.AmountCents)
	}
}
