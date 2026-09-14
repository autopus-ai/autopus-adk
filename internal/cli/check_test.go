package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

// TestCheckCmd_NoFlags verifies that running `check` with no flags exits cleanly
// when there are no oversized files and the last commit (if any) is valid.
func TestCheckCmd_NoFlags(t *testing.T) {
	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--dir", t.TempDir()})

	// In a temp dir there are no .go files, so arch passes.
	// There are no commits, so lore is skipped.
	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCheckCmd_QuietSuppressesOutput verifies that --quiet suppresses the banner.
func TestCheckCmd_QuietSuppressesOutput(t *testing.T) {
	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--arch", "--quiet", "--dir", t.TempDir()})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(buf.String(), "Autopus") {
		t.Errorf("quiet mode should suppress the banner, got: %s", buf.String())
	}
}

// TestCheckCmd_ArchFailsOnOversizedFile checks the explicit project ceiling.
func TestCheckCmd_ArchFailsOnOversizedFile(t *testing.T) {
	dir := t.TempDir()
	configureFileSizeLimit(t, dir, 300)

	// Create a .go file with 301 lines.
	var sb strings.Builder
	sb.WriteString("package dummy\n")
	for i := 0; i < 300; i++ {
		sb.WriteString(fmt.Sprintf("var _ = %d\n", i))
	}
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--arch", "--quiet", "--dir", dir})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
}

// TestCheckCmd_ArchPassesOnSmallFile verifies that a file under 200 lines passes cleanly.
func TestCheckCmd_ArchPassesOnSmallFile(t *testing.T) {
	dir := t.TempDir()

	content := "package dummy\n// small file\n"
	if err := os.WriteFile(filepath.Join(dir, "small.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--arch", "--quiet", "--dir", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCheckCmd_ArchSkipsGeneratedFiles verifies that generated files are not checked.
func TestCheckCmd_ArchSkipsGeneratedFiles(t *testing.T) {
	dir := t.TempDir()

	// Write a 301-line generated file — should be skipped.
	var sb strings.Builder
	sb.WriteString("package dummy\n")
	for i := 0; i < 300; i++ {
		sb.WriteString(fmt.Sprintf("var _ = %d\n", i))
	}
	for _, name := range []string{"foo_generated.go", "bar_gen.go", "baz.pb.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--arch", "--quiet", "--dir", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("generated files should be skipped, got: %v", err)
	}
}

// TestCheckCmd_LoreSkipsOnExperimentBranch verifies that lore check is skipped
// when the current branch has the experiment/ prefix.
func TestCheckCmd_LoreSkipsOnExperimentBranch(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo on an experiment branch.
	for _, args := range [][]string{
		{"init"},
		{"checkout", "-b", "experiment/XLOOP-test"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create a non-Lore commit (experiment format).
	dummyFile := filepath.Join(dir, "dummy.go")
	if err := os.WriteFile(dummyFile, []byte("package dummy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "dummy.go"},
		{"commit", "-m", "experiment(XLOOP-test): iteration 1 - test change"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--lore", "--dir", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("lore check should pass on experiment branch, got: %v", err)
	}
	if !strings.Contains(buf.String(), "experiment branch") {
		t.Errorf("expected experiment branch skip message, got: %s", buf.String())
	}
}

// TestCheckCmd_WarnOnlyExitsZero verifies that --warn-only causes the check to
// exit 0 even when violations are found.
func TestCheckCmd_WarnOnlyExitsZero(t *testing.T) {
	dir := t.TempDir()
	configureFileSizeLimit(t, dir, 300)

	// Create a .go file with 301 lines.
	var sb strings.Builder
	sb.WriteString("package dummy\n")
	for i := 0; i < 300; i++ {
		sb.WriteString(fmt.Sprintf("var _ = %d\n", i))
	}
	if err := os.WriteFile(filepath.Join(dir, "big.go"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--arch", "--quiet", "--warn-only", "--dir", dir})

	err := root.Execute()
	if err != nil {
		t.Fatalf("--warn-only should exit 0 even with violations, got: %v", err)
	}

	// Violations should still be printed
	if !strings.Contains(buf.String(), "big.go") {
		t.Errorf("violations should still be printed in warn-only mode, got: %s", buf.String())
	}
}

// TestCheckCmd_LoreSkipsWhenNoCommits verifies that lore check passes gracefully
// in a directory without any git history.
func TestCheckCmd_LoreSkipsWhenNoCommits(t *testing.T) {
	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"check", "--lore", "--dir", t.TempDir()})

	if err := root.Execute(); err != nil {
		t.Fatalf("lore check should pass when there are no commits, got: %v", err)
	}
}

func TestCheckCmd_CC21_EffortFlagOverridesEnv(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "low")

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--effort", "xhigh", "check", "--cc21", "--dir", t.TempDir()})

	if err := root.Execute(); err != nil {
		t.Fatalf("cc21 check should pass with explicit --effort override, got: %v", err)
	}
	if !strings.Contains(buf.String(), "effective effort=xhigh (source=flag)") {
		t.Fatalf("expected effort override output, got: %s", buf.String())
	}
}

func TestCheckCmd_CC21_TaskCreatedModeFlagOverridesEnvAndConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("demo")
	cfg.Features.CC21.TaskCreatedMode = "warn"
	if err := config.Save(dir, cfg); err != nil {
		t.Fatalf("config save failed: %v", err)
	}

	t.Setenv("TASKCREATED_MODE", "warn")

	root := newTestRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--task-created-mode", "enforce", "check", "--cc21", "--dir", dir})

	if err := root.Execute(); err != nil {
		t.Fatalf("cc21 check should pass with explicit --task-created-mode override, got: %v", err)
	}
	if !strings.Contains(buf.String(), "effective mode=enforce (source=flag)") {
		t.Fatalf("expected task-created override output, got: %s", buf.String())
	}
}

func configureFileSizeLimit(t *testing.T, dir string, limit int) {
	t.Helper()
	cfg := config.DefaultFullConfig("size-policy")
	cfg.Architecture.MaxFileLines = limit
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
}
