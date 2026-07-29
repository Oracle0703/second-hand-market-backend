package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRunReportsFixedDatabaseIdentityResult(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var stdout strings.Builder
		var stderr strings.Builder

		exitCode := run(func() error { return nil }, &stdout, &stderr)

		if exitCode != 0 {
			t.Fatalf("exit code = %d", exitCode)
		}
		if stdout.String() != "DATABASE_IDENTITY PASS\n" {
			t.Fatalf("stdout = %q", stdout.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("failure_is_redacted", func(t *testing.T) {
		const sentinel = "verify-database-secret-sentinel"
		var stdout strings.Builder
		var stderr strings.Builder

		exitCode := run(func() error { return errors.New(sentinel) }, &stdout, &stderr)

		if exitCode == 0 {
			t.Fatal("failure returned exit code 0")
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q", stdout.String())
		}
		if stderr.String() != "DATABASE_IDENTITY FAIL\n" {
			t.Fatalf("stderr = %q", stderr.String())
		}
		if strings.Contains(stderr.String(), sentinel) {
			t.Fatalf("stderr leaked protected error: %q", stderr.String())
		}
	})
}

func TestVerifyDatabaseRejectsNonRemoteTarget(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DB_TARGET", "local")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", "file:verify-database?mode=memory&cache=shared")

	err := verifyDatabase()
	if err == nil || !strings.Contains(err.Error(), "DB_TARGET") {
		t.Fatalf("expected remote target rejection, got %v", err)
	}
}
