package config

import (
	"os"
	"testing"
)

func TestLoadAllowPrivateBaseURLFromEnv(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "canonical env var",
			key:  "NEXUSROUTER_SECURITY__ALLOW_PRIVATE_BASE_URL",
		},
		{
			name: "legacy missing underscore env var",
			key:  "NEXUSROUTER_SECURITY__ALLOW_PRIVATE_BASEURL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, "NEXUSROUTER_SECURITY__ALLOW_PRIVATE_BASE_URL")
			unsetEnv(t, "NEXUSROUTER_SECURITY__ALLOW_PRIVATE_BASEURL")
			t.Setenv(tt.key, "true")

			cfg, err := Load("")
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if !cfg.Security.AllowPrivateBaseURL {
				t.Fatalf("AllowPrivateBaseURL = false, want true")
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}
