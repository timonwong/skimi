package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestRunEditCreatesMissingConfigAndRunsEditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "skills.yaml")
	var stdout bytes.Buffer

	var calls []struct {
		name string
		args []string
	}
	err := runEdit(path, editorEnv{
		editor: "printf 'packages:\\n' >",
		goos:   "darwin",
		stdout: &stdout,
		runner: commandRunnerFunc(func(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			calls = append(calls, struct {
				name string
				args []string
			}{name: name, args: append([]string(nil), args...)})
			return realCommandRunner{}.Run(name, args, stdin, stdout, stderr)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(calls) < 1 {
		t.Fatal("expected editor command call")
	}
	if calls[0].name != "sh" {
		t.Fatalf("command name = %q, want sh", calls[0].name)
	}
	if len(calls[0].args) != 4 || calls[0].args[0] != "-c" || calls[0].args[2] != "skimi-editor" || calls[0].args[3] != path {
		t.Fatalf("command args = %#v", calls[0].args)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "packages:\n" {
		t.Fatalf("config content = %q", content)
	}
	if !strings.Contains(stripANSI(stdout.String()), "+packages:") {
		t.Fatalf("stdout diff missing added line:\n%s", stdout.String())
	}
}

func TestRunEditPrintsUnchangedMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.yaml")
	if err := os.WriteFile(path, []byte("packages: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer

	err := runEdit(path, editorEnv{
		editor: "true",
		goos:   "linux",
		stdout: &stdout,
		runner: realCommandRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := stdout.String(); !strings.Contains(got, "No changes.") {
		t.Fatalf("stdout = %q, want unchanged message", got)
	}
}

func TestRunEditUsesWindowsCommandProcessorForEditor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.yaml")
	if err := os.WriteFile(path, []byte("packages: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var gotName string
	var gotArgs []string

	err := runEdit(path, editorEnv{
		editor: "notepad",
		goos:   "windows",
		stdout: &stdout,
		runner: commandRunnerFunc(func(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotName != "cmd" {
		t.Fatalf("command name = %q, want cmd", gotName)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "/C" || gotArgs[1] != `notepad "`+path+`"` {
		t.Fatalf("command args = %#v", gotArgs)
	}
	if got := stdout.String(); !strings.Contains(got, "No changes.") {
		t.Fatalf("stdout = %q, want unchanged message", got)
	}
}

func TestRunEditUsesSystemOpenerWhenEditorIsUnset(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		wantName string
	}{
		{name: "macOS", goos: "darwin", wantName: "open"},
		{name: "Linux", goos: "linux", wantName: "xdg-open"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EDITOR", "")
			path := filepath.Join(t.TempDir(), "skills.yaml")
			var stdout bytes.Buffer
			var gotName string
			var gotArgs []string

			err := runEdit(path, editorEnv{
				goos:   tt.goos,
				stdout: &stdout,
				runner: commandRunnerFunc(func(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
					gotName = name
					gotArgs = append([]string(nil), args...)
					return nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}

			if gotName != tt.wantName {
				t.Fatalf("command name = %q, want %q", gotName, tt.wantName)
			}
			if len(gotArgs) != 1 || gotArgs[0] != path {
				t.Fatalf("command args = %#v, want config path", gotArgs)
			}
			if got := stdout.String(); !strings.Contains(got, "Opened "+path) {
				t.Fatalf("stdout = %q, want opened path", got)
			}
		})
	}
}

func TestRunEditReturnsUnsupportedOpener(t *testing.T) {
	t.Setenv("EDITOR", "")
	err := runEdit(filepath.Join(t.TempDir(), "skills.yaml"), editorEnv{
		goos: "windows",
		runner: commandRunnerFunc(func(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			return errors.New("unexpected command")
		}),
	})
	if err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

func TestPrintConfigDiffTreatsGitDiffExitOneAsSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("git diff --no-index exit behavior is shell-platform dependent")
	}
	dir := t.TempDir()
	before := filepath.Join(dir, "before")
	after := filepath.Join(dir, "after")
	if err := os.WriteFile(before, []byte("packages: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(after, []byte("packages:\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	err := printConfigDiff(before, after, &stdout, io.Discard, realCommandRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stripANSI(stdout.String()), "+packages:") {
		t.Fatalf("stdout diff missing added line:\n%s", stdout.String())
	}
}

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}
