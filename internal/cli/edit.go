package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

type commandRunner interface {
	Run(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

type commandRunnerFunc func(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error

func (f commandRunnerFunc) Run(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return f(name, args, stdin, stdout, stderr)
}

type realCommandRunner struct{}

func (realCommandRunner) Run(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

type editorEnv struct {
	editor string
	goos   string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	runner commandRunner
}

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open skills.yaml in your editor",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(globalConfigFile, editorEnv{})
		},
	}
}

func runEdit(configPath string, env editorEnv) error {
	env = env.withDefaults()

	if err := ensureConfigFile(configPath); err != nil {
		return err
	}

	if env.editor == "" {
		opener, err := systemOpener(env.goos)
		if err != nil {
			return err
		}
		if err := env.runner.Run(opener, []string{configPath}, env.stdin, env.stdout, env.stderr); err != nil {
			return fmt.Errorf("open config: %w", err)
		}
		fmt.Fprintf(env.stdout, "Opened %s\n", configPath)
		return nil
	}

	before, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config before edit: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "skimi-edit-*")
	if err != nil {
		return fmt.Errorf("create diff temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintf(env.stderr, "warning: remove diff temp dir: %v\n", err)
		}
	}()

	beforePath := filepath.Join(tmpDir, "skills.yaml.before")
	if err := os.WriteFile(beforePath, before, 0o644); err != nil {
		return fmt.Errorf("write config snapshot: %w", err)
	}

	args := []string{"-c", env.editor + ` "$1"`, "skimi-editor", configPath}
	if err := env.runner.Run("sh", args, env.stdin, env.stdout, env.stderr); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config after edit: %w", err)
	}
	if bytes.Equal(before, after) {
		fmt.Fprintln(env.stdout, "No changes.")
		return nil
	}

	return printConfigDiff(beforePath, configPath, env.stdout, env.stderr, env.runner)
}

func (env editorEnv) withDefaults() editorEnv {
	if env.editor == "" {
		env.editor = os.Getenv("EDITOR")
	}
	if env.goos == "" {
		env.goos = runtime.GOOS
	}
	if env.stdin == nil {
		env.stdin = os.Stdin
	}
	if env.stdout == nil {
		env.stdout = os.Stdout
	}
	if env.stderr == nil {
		env.stderr = os.Stderr
	}
	if env.runner == nil {
		env.runner = realCommandRunner{}
	}
	return env
}

func ensureConfigFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	return f.Close()
}

func systemOpener(goos string) (string, error) {
	switch goos {
	case "darwin":
		return "open", nil
	case "linux":
		return "xdg-open", nil
	default:
		return "", fmt.Errorf("unsupported platform %q: set EDITOR to edit config", goos)
	}
}

func printConfigDiff(beforePath, afterPath string, stdout, stderr io.Writer, runner commandRunner) error {
	err := runner.Run("git", []string{"diff", "--no-ext-diff", "--no-index", "--color=always", beforePath, afterPath}, nil, stdout, stderr)
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("show config diff: %w", err)
}
