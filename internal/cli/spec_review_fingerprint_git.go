package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/spec"
)

// specReviewArtifactNames are the files a review writes into a SPEC
// directory; they never count as code changes.
var specReviewArtifactNames = map[string]bool{
	"review.md": true, "review-findings.json": true, "review-receipt.json": true,
	"review-session.json": true, ".review-session.lock": true,
}

// specReviewGitBaseline is the code half of the fingerprint: the HEAD commit
// and a digest of every changed or untracked file outside the reviewed SPEC
// directory (status, path, content digest), excluding review artifacts under
// .autopus/specs. Reviewers read the repository, so any such change can
// change their answer (REQ-RR-REUSE-01). Tests replace it.
var specReviewGitBaseline = func(projectRoot, specDir string) (string, string, error) {
	top, err := specReviewGit(projectRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", errors.New("not a git work tree")
	}
	top = strings.TrimSpace(top)
	head, err := specReviewGit(top, "rev-parse", "HEAD")
	if err != nil {
		return "", "", errors.New("no HEAD commit")
	}
	status, err := specReviewGit(top, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return "", "", fmt.Errorf("git status failed: %w", err)
	}
	return strings.TrimSpace(head), specReviewWorktreeDigest(top, specDir, status), nil
}

func specReviewGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.String(), err
}

// specReviewWorktreeDigest digests porcelain v1 -z entries. A rename entry is
// followed by its source path, which is consumed with it.
func specReviewWorktreeDigest(top, specDir, status string) string {
	specRel := ""
	if absSpec, err := filepath.Abs(specDir); err == nil {
		if resolved, err := filepath.EvalSymlinks(absSpec); err == nil {
			absSpec = resolved
		}
		if resolvedTop, err := filepath.EvalSymlinks(top); err == nil {
			top = resolvedTop
		}
		if rel, err := filepath.Rel(top, absSpec); err == nil {
			specRel = filepath.ToSlash(rel)
		}
	}
	entries := strings.Split(status, "\x00")
	var lines []string
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		code, path := entry[:2], entry[3:]
		if code[0] == 'R' || code[0] == 'C' {
			i++
		}
		if specReviewIgnoredChange(path, specRel) {
			continue
		}
		content := "deleted"
		if data, err := os.ReadFile(filepath.Join(top, path)); err == nil {
			content = spec.SHA256Hex(data)
		}
		lines = append(lines, code+"\x00"+path+"\x00"+content)
	}
	sort.Strings(lines)
	return spec.SHA256Hex([]byte(strings.Join(lines, "\n")))
}

func specReviewIgnoredChange(path, specRel string) bool {
	if specRel != "" && specRel != "." && (path == specRel || strings.HasPrefix(path, specRel+"/")) {
		return true
	}
	return strings.Contains("/"+path, "/.autopus/specs/") && specReviewArtifactNames[filepath.Base(path)]
}
