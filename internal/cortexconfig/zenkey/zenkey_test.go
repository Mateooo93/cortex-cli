package zenkey

import "testing"

func TestKeyDecryptsEmbeddedCredential(t *testing.T) {
	if !Available() {
		t.Fatal("expected embedded zen key")
	}
	got := Key()
	if got == "" {
		t.Fatal("Key() returned empty string")
	}
	if len(got) < 20 {
		t.Fatalf("Key() too short: len=%d", len(got))
	}
	if got[:3] != "sk-" {
		t.Fatalf("Key() prefix = %q, want sk-", got[:3])
	}
	if Key() != got {
		t.Fatal("Key() not cached consistently")
	}
}
