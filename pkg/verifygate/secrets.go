package verifygate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/securityscan"
)

// maxUntrackedBytes caps how much of one new file is read; a larger file is
// almost always generated output or data, not hand-written configuration.
const maxUntrackedBytes = 1 << 20

// SecretHit is one added line that looks like a credential. It names the
// place and the rule only; the value itself is never kept.
type SecretHit struct {
	File string
	Line int
	Rule string
}

func (h SecretHit) String() string { return fmt.Sprintf("%s:%d (%s)", h.File, h.Line, h.Rule) }

// fingerprint is the gitleaks fingerprint form for an uncommitted finding.
func (h SecretHit) fingerprint() string { return fmt.Sprintf("%s:%s:%d", h.File, h.Rule, h.Line) }

// AddedSecrets scans the lines the work tree adds against HEAD (tracked
// edits plus untracked files that are not gitignored) with the built-in
// secret patterns. Harness directories, binary files, lines with an allow
// marker, and .gitleaksignore fingerprints are skipped. It returns nil when
// dir is not in a git work tree.
func AddedSecrets(ctx context.Context, dir string) []SecretHit {
	top, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	root := strings.TrimSpace(string(top))
	var hits []SecretHit
	if diff, err := gitOutput(ctx, root, "-c", "core.quotePath=false", "diff", "--unified=0", "--no-color",
		"--no-ext-diff", "--no-renames", "HEAD"); err == nil {
		hits = append(hits, diffSecrets(diff)...)
	}
	if out, err := gitOutput(ctx, root, "ls-files", "--others", "--exclude-standard", "-z"); err == nil {
		for _, f := range withoutHarnessPaths(strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")) {
			if f != "" {
				hits = append(hits, fileSecrets(root, f)...)
			}
		}
	}
	ignored := gitleaksIgnored(root)
	return slices.DeleteFunc(hits, func(h SecretHit) bool {
		fp := h.fingerprint()
		return slices.ContainsFunc(ignored, func(entry string) bool { return entry == fp || strings.HasSuffix(entry, ":"+fp) })
	})
}

// diffSecrets reads `git diff --unified=0` output and matches each added line.
func diffSecrets(diff []byte) []SecretHit {
	var (
		hits   []SecretHit
		file   string
		line   int
		inHunk bool
	)
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 64*1024), maxUntrackedBytes)
	for scanner.Scan() {
		text := scanner.Text()
		switch {
		case strings.HasPrefix(text, "diff --git "):
			file, inHunk = "", false
		case !inHunk && strings.HasPrefix(text, "+++ "):
			file = diffPath(strings.TrimPrefix(text, "+++ "))
			if len(withoutHarnessPaths([]string{file})) == 0 {
				file = ""
			}
		case strings.HasPrefix(text, "@@ "):
			inHunk, line = true, hunkStart(text)
		case inHunk && strings.HasPrefix(text, "+"):
			if file != "" {
				for _, rule := range securityscan.MatchSecrets(text[1:]) {
					hits = append(hits, SecretHit{File: file, Line: line, Rule: rule})
				}
			}
			line++
		}
	}
	return hits
}

// diffPath turns a `+++ b/path` header into the repository path; /dev/null
// (a deletion) yields "".
func diffPath(header string) string {
	header = strings.Trim(header, "\"")
	if !strings.HasPrefix(header, "b/") {
		return ""
	}
	return strings.TrimPrefix(header, "b/")
}

// hunkStart reads the new-file start line from `@@ -a,b +c,d @@`.
func hunkStart(header string) int {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 0
	}
	start, _, _ := strings.Cut(strings.TrimPrefix(fields[2], "+"), ",")
	n, _ := strconv.Atoi(start)
	return n
}

// fileSecrets matches every line of an untracked text file.
func fileSecrets(root, rel string) []SecretHit {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxUntrackedBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return nil
	}
	var hits []SecretHit
	for i, text := range strings.Split(string(data), "\n") {
		for _, rule := range securityscan.MatchSecrets(text) {
			hits = append(hits, SecretHit{File: rel, Line: i + 1, Rule: rule})
		}
	}
	return hits
}

// gitleaksIgnored reads the fingerprints in the root .gitleaksignore.
func gitleaksIgnored(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, ".gitleaksignore"))
	if err != nil {
		return nil
	}
	var entries []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}
	return entries
}

// SecretHitsFingerprint identifies a set of hits regardless of order, so
// the gate can tell a hit it already reported from a new one.
func SecretHitsFingerprint(hits []SecretHit) string {
	keys := make([]string, 0, len(hits))
	for _, h := range hits {
		keys = append(keys, h.fingerprint())
	}
	slices.Sort(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(sum[:])
}
