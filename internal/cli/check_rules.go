package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/linecount"
	"github.com/insajin/autopus-adk/pkg/lore"
)

const (
	// warnLineLimit is the line count that triggers a warning.
	warnLineLimit = 200
	// advisoryLineLimit reports large source files without imposing a project policy.
	advisoryLineLimit = 300
)

// loreValidTypes defines allowed Lore commit type prefixes.
var loreValidTypes = []string{
	"feat(", "fix(", "refactor(", "test(", "docs(", "chore(", "perf(",
}

// loreSignOff is the required Lore sign-off line.
const loreSignOff = "🐙 Autopus <noreply@autopus.co>"

// skipDirs contains directory names that should be skipped during arch walk.
// These include version control, tooling worktrees, and dependency directories.
var skipDirs = map[string]bool{
	"vendor":       true,
	".git":         true,
	"node_modules": true,
}

// checkArch reports source size and enforces only an explicit project ceiling.
// Read/count/configuration errors remain blocking even when size is advisory.
func checkArch(dir string, out io.Writer, quiet, stagedOnly bool) bool {
	if !quiet {
		tui.SectionHeader(out, "arch: file size (code lines)")
	}
	cfg, err := config.LoadPreview(dir)
	if err != nil {
		tui.Error(out, fmt.Sprintf("arch policy: %v", err))
		return false
	}
	limit := cfg.Architecture.MaxFileLines

	if stagedOnly {
		return checkArchStaged(dir, out, quiet, limit)
	}
	return checkArchWalk(dir, out, quiet, limit)
}

// isGeneratedGoFile reports whether a file name matches generated file patterns.
func isGeneratedGoFile(name string) bool {
	return strings.HasSuffix(name, "_generated.go") ||
		strings.HasSuffix(name, "_gen.go") ||
		strings.HasSuffix(name, ".pb.go")
}

// countLines applies the shared source-line policy to worktree content.
func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return countSourceLines(path, data)
}

// countSourceLines reports the physical count when it already sits under the
// warn threshold, and lexes for comment-only lines only when the physical size
// could still trip a threshold. Lexing costs about a megabyte per second; the
// byte scan is effectively free.
func countSourceLines(name string, data []byte) (int, error) {
	if physical := linecount.Physical(data); physical <= warnLineLimit {
		return physical, nil
	}
	counts, err := linecount.Source(name, data)
	return counts.Counted, err
}

// checkLore verifies that the most recent commit uses Lore format.
// It checks for a valid type prefix and the required sign-off line.
// Experiment branches (experiment/*) are exempt from Lore format.
// Returns false if the format is invalid or git is unavailable.
func checkLore(dir string, out io.Writer, quiet bool) bool {
	if !quiet {
		tui.SectionHeader(out, "lore: commit format")
	}

	// Experiment branches use a simplified commit format, not Lore.
	if isExperimentBranch(dir) {
		if !quiet {
			tui.SKIP(out, "experiment branch — Lore format not required")
		}
		return true
	}

	msg, err := lastCommitMessage(dir)
	if err != nil {
		// No commits yet or git unavailable — skip silently.
		if !quiet {
			tui.Info(out, "no commits found, skipping lore check")
		}
		return true
	}

	validType := hasValidLoreType(msg)
	hasSignOff := strings.Contains(msg, loreSignOff)
	validTrailers := validateLoreMessage(msg, dir, out)

	if validType && hasSignOff && validTrailers {
		if !quiet {
			tui.OK(out, "last commit follows Lore format")
		}
		return true
	}

	if !validType {
		tui.FAIL(out, "last commit missing valid Lore type prefix (e.g. feat(...): ...)")
	}
	if !hasSignOff {
		tui.FAIL(out, fmt.Sprintf("last commit missing sign-off: %s", loreSignOff))
	}
	return validType && hasSignOff && validTrailers
}

// checkLoreFromFile validates a commit message read from the given file path.
// This is used by the commit-msg git hook where the message file is passed as $1.
func checkLoreFromFile(msgFile string, out io.Writer, quiet bool) bool {
	if !quiet {
		tui.SectionHeader(out, "lore: commit format (message file)")
	}

	data, err := os.ReadFile(msgFile)
	if err != nil {
		tui.Error(out, fmt.Sprintf("cannot read message file: %v", err))
		return false
	}

	msg := strings.TrimSpace(string(data))
	if msg == "" {
		tui.FAIL(out, "empty commit message")
		return false
	}

	validType := hasValidLoreType(msg)
	hasSignOff := strings.Contains(msg, loreSignOff)
	configDir := filepath.Dir(msgFile)
	if filepath.Base(configDir) == ".git" {
		configDir = filepath.Dir(configDir)
	}
	validTrailers := validateLoreMessage(msg, configDir, out)

	if validType && hasSignOff && validTrailers {
		if !quiet {
			tui.OK(out, "commit message follows Lore format")
		}
		return true
	}

	if !validType {
		tui.FAIL(out, "commit message missing valid Lore type prefix (e.g. feat(...): ...)")
	}
	if !hasSignOff {
		tui.FAIL(out, fmt.Sprintf("commit message missing sign-off: %s", loreSignOff))
	}
	return validType && hasSignOff && validTrailers
}

// lastCommitMessage returns the full body of the most recent commit.
func lastCommitMessage(dir string) (string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%B")
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	msg := strings.TrimSpace(buf.String())
	if msg == "" {
		return "", fmt.Errorf("empty commit message")
	}
	return msg, nil
}

// hasValidLoreType reports whether msg starts with a recognised Lore type prefix.
func hasValidLoreType(msg string) bool {
	for _, t := range loreValidTypes {
		if strings.HasPrefix(msg, t) {
			return true
		}
	}
	return false
}

// isExperimentBranch reports whether the current branch has the experiment/ prefix.
func isExperimentBranch(dir string) bool {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(buf.String()), "experiment/")
}

func validateLoreMessage(msg, dir string, out io.Writer) bool {
	// 전체 설정이 아니라 lore 블록만 읽는다. 검증 대상은 커밋 메시지이고,
	// 설정 유효성은 auto doctor 가 소유한다. 예전에는 config.Load 를 불러
	// 전체 Validate 를 통과해야 했으므로 lore 와 무관한 필드 하나가 커밋
	// 훅을 브릭했다.
	loreConf, err := config.LoadLoreSection(dir)
	if err != nil {
		tui.Error(out, fmt.Sprintf("cannot load lore config: %v", err))
		return false
	}

	loreConfig := lore.LoreConfig{
		RequiredTrailers:   append([]string(nil), loreConf.RequiredTrailers...),
		StaleThresholdDays: loreConf.StaleThresholdDays,
	}

	errs := lore.Validate(msg, loreConfig)
	if len(errs) == 0 {
		return true
	}

	for _, err := range errs {
		tui.FAIL(out, fmt.Sprintf("lore trailer invalid [%s]: %s", err.Field, err.Message))
	}
	return false
}
