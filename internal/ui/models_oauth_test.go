package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestProviderSettingsRows_OAuthProvidersDoNotSurfaceAPIKeyPrefix(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.EnsureProviderPresets()
	rows := ProviderSettingsRows(cfg)
	for _, provider := range []string{"codex", "xai-sub"} {
		var row *ProviderSettingsView
		for i := range rows {
			if rows[i].Provider == provider {
				row = &rows[i]
				break
			}
		}
		if row == nil {
			t.Fatalf("missing settings row for %q", provider)
		}
		if row.AuthKind != "oauth" {
			t.Fatalf("%q AuthKind = %q, want oauth", provider, row.AuthKind)
		}
		if row.NeedsAPIKey {
			t.Fatalf("%q NeedsAPIKey = true, want false", provider)
		}
		// Unsigned-in oauth rows must not show a fake API-key prefix from env/config.
		if row.KeyPrefix != "" && row.KeyPrefix != "(signed in)" && row.KeyPrefix != "(env token)" {
			t.Fatalf("%q KeyPrefix = %q, want empty or oauth status", provider, row.KeyPrefix)
		}
	}
}