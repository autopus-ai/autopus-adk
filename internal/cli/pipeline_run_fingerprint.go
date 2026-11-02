package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// The fingerprints a completed run records and status recomputes
// (SPEC-STATUSEVIDENCE-001 REQ-SEV-BASE-01, REQ-SEV-BASE-02). A tree the
// fingerprint cannot hash in bounded time, or whose changes git would hide,
// is reported as an error, never as unchanged.
const (
	treeFingerprintMaxPaths = 5000
	treeFingerprintMaxBytes = int64(1) << 30
)

var (
	errTreeTooLarge     = errors.New("too many changed files to fingerprint")
	errTreeHidesChanges = errors.New("git hides changes to tracked files")
)

// verificationSpecDocuments are the SPEC documents a run is bound to, the same
// four the pipeline snapshot hashes.
var verificationSpecDocuments = []string{"spec.md", "plan.md", "acceptance.md", "research.md"}

// specRequirementsFingerprint hashes the SPEC documents without their status
// lines: review promotion and sync rewrite those without changing a
// requirement, and a status edit must not make verified code look stale.
func specRequirementsFingerprint(specDir string) (string, error) {
	hash := sha256.New()
	for _, name := range verificationSpecDocuments {
		body, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && name != "spec.md" {
				continue
			}
			return "", err
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(withoutStatusLines(string(body))))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// withoutStatusLines drops the status lines of a document's header, which is
// everything before its first H2 heading, frontmatter included.
func withoutStatusLines(body string) string {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	header := true
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			header = false
		}
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if header && (strings.HasPrefix(trimmed, "**status**") || strings.HasPrefix(trimmed, "status:")) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// workingTreeDigests returns a digest for every path whose working-tree
// content differs from base or that is untracked and not ignored, keyed by
// its path from the repository root. Renames are listed as both sides, an
// executable-bit change counts even where core.fileMode is off, and a tracked
// file whose changes git would skip makes the tree unverifiable. Committing
// the tree leaves the result unchanged; any later content change alters it.
func workingTreeDigests(dir, base string) (map[string]string, error) {
	top, err := gitText(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	hidden, err := hiddenTrackedPaths(top)
	if err != nil {
		return nil, err
	}
	if len(hidden) > 0 {
		return nil, fmt.Errorf("%w: %d marked assume-unchanged or skip-worktree: %s",
			errTreeHidesChanges, len(hidden), summarizeStatusPaths(hidden))
	}
	diffArgs := []string{"diff", "--name-only", "--no-renames", "-z", base, "--"}
	if runtime.GOOS != "windows" {
		diffArgs = append([]string{"-c", "core.fileMode=true"}, diffArgs...)
	}
	changed, err := gitBytes(top, diffArgs...)
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", shortCommit(base), err)
	}
	untracked, err := gitBytes(top, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, raw := range bytes.Split(append(changed, untracked...), []byte{0}) {
		if rel := string(raw); rel != "" && !harnessStatePath(rel) {
			set[rel] = true
		}
	}
	if len(set) > treeFingerprintMaxPaths {
		return nil, fmt.Errorf("%w: %d paths", errTreeTooLarge, len(set))
	}
	budget := treeFingerprintMaxBytes
	digests := make(map[string]string, len(set))
	for rel := range set {
		digest, err := pathDigest(top, rel, &budget)
		if err != nil {
			return nil, err
		}
		digests[rel] = digest
	}
	return digests, nil
}

// hiddenTrackedPaths lists tracked paths outside .autopus whose changes git
// diff would not report: assume-unchanged (a lowercase tag in ls-files -v)
// and skip-worktree (S). Status never clears these flags; it reports them.
func hiddenTrackedPaths(top string) ([]string, error) {
	out, err := gitBytes(top, "ls-files", "-v", "-z")
	if err != nil {
		return nil, err
	}
	var hidden []string
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) < 3 || entry[1] != ' ' {
			continue
		}
		tag, rel := entry[0], string(entry[2:])
		if !harnessStatePath(rel) && ((tag >= 'a' && tag <= 'z') || tag == 'S') {
			hidden = append(hidden, rel)
		}
	}
	sort.Strings(hidden)
	return hidden, nil
}

// harnessStatePath reports a path inside any .autopus directory.
func harnessStatePath(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if part == ".autopus" {
			return true
		}
	}
	return false
}

func pathDigest(top, rel string, budget *int64) (string, error) {
	full := filepath.Join(top, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return "deleted", nil
	}
	if err != nil {
		return "", err
	}
	mode := info.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		target, err := os.Readlink(full)
		if err != nil {
			return "", err
		}
		return "symlink:" + digestBytes([]byte(target)), nil
	case mode.IsDir():
		// A submodule: its checked-out commit stands for its content.
		if head, err := gitText(full, "rev-parse", "HEAD"); err == nil {
			return "commit:" + head, nil
		}
		return "dir", nil
	case !mode.IsRegular():
		return "special:" + mode.Type().String(), nil
	}
	if *budget -= info.Size(); *budget < 0 {
		return "", fmt.Errorf("%w: changed files exceed %d bytes", errTreeTooLarge, treeFingerprintMaxBytes)
	}
	file, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if mode&0o111 != 0 {
		digest += ":x"
	}
	return digest, nil
}

func treeFingerprint(paths map[string]string) string {
	keys := make([]string, 0, len(paths))
	for rel := range paths {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, rel := range keys {
		_, _ = hash.Write([]byte(rel + "\x00" + paths[rel] + "\x00"))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gitHeadAt(dir string) (string, error) {
	return gitText(dir, "rev-parse", "--verify", "HEAD^{commit}")
}

func gitText(dir string, args ...string) (string, error) {
	out, err := gitBytes(dir, args...)
	return strings.TrimSpace(string(out)), err
}

// gitBytes runs a read-only git command. GIT_OPTIONAL_LOCKS=0 keeps git from
// refreshing the index as a side effect of reading it.
func gitBytes(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		name := gitSubcommand(args)
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("git %s: %s", name, strings.SplitN(msg, "\n", 2)[0])
		}
		return nil, fmt.Errorf("git %s: %w", name, err)
	}
	return out, nil
}

// gitSubcommand names the subcommand of a git argument list, past any
// leading -c settings.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

func shortCommit(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
