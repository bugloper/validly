package role

import "testing"

func TestIsRole(t *testing.T) {
	for local, want := range map[string]bool{
		"support":      true,
		"Admin":        true,
		"sales+leads":  true,
		"noreply":      true,
		"no-reply":     true,
		"postmaster":   true,
		"jane":         false,
		"supporter":    false,
		"jane.support": false,
		"":             false,
	} {
		if got := IsRole(local); got != want {
			t.Errorf("IsRole(%q) = %v", local, got)
		}
	}
}
