package auth

import (
	"strings"
	"testing"
)

func TestIsSafeAdministratorPassword(t *testing.T) {
	for _, password := range []string{
		"short",
		legacyDefaultAdminPassword,
		strings.Repeat("a", 73),
	} {
		if IsSafeAdministratorPassword(password) {
			t.Fatal("expected administrator password to be rejected")
		}
	}
	if !IsSafeAdministratorPassword("DifferentAdmin@2026") {
		t.Fatal("expected non-default administrator password to be accepted")
	}
}
