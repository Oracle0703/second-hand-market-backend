package main

import (
	"strings"
	"testing"
)

func TestDatabaseConfigFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		driver     string
		dsn        string
		wantDriver string
		wantDSN    string
		wantError  string
	}{
		{
			name:      "requires driver",
			wantError: "DB_DRIVER is required",
		},
		{
			name:      "requires dsn",
			driver:    "sqlite",
			wantError: "DB_DSN is required",
		},
		{
			name:      "rejects unsupported driver",
			driver:    "unknown",
			dsn:       "synthetic",
			wantError: "unsupported DB_DRIVER",
		},
		{
			name:       "accepts synthetic in-memory sqlite",
			driver:     "sqlite",
			dsn:        "file:seed-test?mode=memory&cache=shared",
			wantDriver: "sqlite",
			wantDSN:    "file:seed-test?mode=memory&cache=shared",
		},
		{
			name:       "trims explicit configuration",
			driver:     " mysql ",
			dsn:        " synthetic-dsn ",
			wantDriver: "mysql",
			wantDSN:    "synthetic-dsn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DB_DRIVER", tt.driver)
			t.Setenv("DB_DSN", tt.dsn)

			driver, dsn, err := databaseConfigFromEnv()
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("databaseConfigFromEnv() error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("databaseConfigFromEnv() unexpected error: %v", err)
			}
			if driver != tt.wantDriver || dsn != tt.wantDSN {
				t.Fatalf(
					"databaseConfigFromEnv() = (%q, %q), want (%q, %q)",
					driver,
					dsn,
					tt.wantDriver,
					tt.wantDSN,
				)
			}
		})
	}
}
