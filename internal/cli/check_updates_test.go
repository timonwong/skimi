package cli

import "testing"

func TestUpdateHintUsesAllFlag(t *testing.T) {
	got := updateHint()
	want := "Run `skimi update --all` to apply updates."
	if got != want {
		t.Fatalf("updateHint() = %q, want %q", got, want)
	}
}
