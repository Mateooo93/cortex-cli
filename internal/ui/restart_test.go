package ui

import "testing"

func TestTakePendingRestart_EmptyByDefault(t *testing.T) {
	if _, _, ok := TakePendingRestart(); ok {
		t.Fatal("expected no pending restart by default")
	}
}

func TestTakePendingRestart_RoundTrip(t *testing.T) {
	setPendingRestart("/tmp/cortex", []string{"--config-dir", "/tmp/cfg"})
	exe, args, ok := TakePendingRestart()
	if !ok || exe != "/tmp/cortex" || len(args) != 2 || args[0] != "--config-dir" {
		t.Fatalf("TakePendingRestart() = (%q, %v, %v)", exe, args, ok)
	}
	if _, _, ok := TakePendingRestart(); ok {
		t.Fatal("expected pending restart to be consumed")
	}
}