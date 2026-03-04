package test

import (
	"testing"

	"finarch/internal/domain/service"
)

func TestGenerateProfessionalNickname_Deterministic(t *testing.T) {
	a := service.GenerateNicknameForTest("u@example.com:tester", 0)
	b := service.GenerateNicknameForTest("u@example.com:tester", 0)
	if a != b {
		t.Fatalf("expected deterministic nickname, got %q vs %q", a, b)
	}
}

func TestGenerateProfessionalNickname_AttemptChanges(t *testing.T) {
	a := service.GenerateNicknameForTest("u@example.com:tester", 0)
	b := service.GenerateNicknameForTest("u@example.com:tester", 1)
	if a == b {
		t.Fatalf("expected fallback attempt to change nickname")
	}
}

func TestGenerateProfessionalNickname_LengthBound(t *testing.T) {
	name := service.GenerateNicknameForTest("long-seed", 9)
	if len(name) > 24 {
		t.Fatalf("nickname too long: %d", len(name))
	}
}
