package cli

import (
	"bytes"
	"testing"
)

func TestExecuteVersion(t *testing.T) {
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetArgs(nil)
	})

	if err := Execute("1.2.3"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := output.String(), "skimi version 1.2.3\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
