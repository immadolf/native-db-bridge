package cli

import "testing"

func TestCommandNames(t *testing.T) {
	got := CommandNames()
	want := []string{"serve", "healthcheck", "install-service", "uninstall-service"}
	if len(got) != len(want) {
		t.Fatalf("len(CommandNames())=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CommandNames()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
