package apiv1

import "testing"

func TestNormalizeRestoreScope(t *testing.T) {
	cases := map[string]string{
		"":      "both",
		"work":  "work",
		"WORK":  "work",
		"life":  "life",
		"LiFe":  "life",
		"other": "both",
	}
	for in, want := range cases {
		if got := normalizeRestoreScope(in); got != want {
			t.Fatalf("normalizeRestoreScope(%q)=%q want %q", in, got, want)
		}
	}
}

func TestScopeAllowsMode(t *testing.T) {
	if !scopeAllowsMode("both", "work") || !scopeAllowsMode("both", "life") {
		t.Fatalf("both scope should allow all modes")
	}
	if !scopeAllowsMode("work", "work") || scopeAllowsMode("work", "life") {
		t.Fatalf("work scope mismatch")
	}
	if !scopeAllowsMode("life", "life") || scopeAllowsMode("life", "work") {
		t.Fatalf("life scope mismatch")
	}
}

func TestResolveRecoveredAccountName(t *testing.T) {
	existing := map[string]struct{}{
		"cash":             {},
		"cash (recovered)": {},
	}
	got := resolveRecoveredAccountName(existing, "Cash")
	if got != "Cash (Recovered 2)" {
		t.Fatalf("unexpected recovered name: %s", got)
	}
	if _, ok := existing["cash (recovered 2)"]; !ok {
		t.Fatalf("expected new name to be reserved")
	}
}
