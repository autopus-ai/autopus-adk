package verifygate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TreeHash identifies what the gate judged: the HEAD commit, every entry of
// `git status` (deletions, renames and their sources, and the harness
// directories the plan leaves out, which tests may still read), each
// present path's mode, symlink target and contents (untracked files under the
// harness directories excepted), the Go environment, and
// extra (the hook's budget). Files git ignores are not included: hashing
// them would mean walking build output, so a stop that reuses a result says
// they were not compared. When git cannot list the tree the hash is unique
// to this call, so nothing recorded under it is ever reused.
func TreeHash(ctx context.Context, root, extra string) string {
	out, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return "unlisted-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	h := sha256.New()
	field(h, headCommit(ctx, root))
	field(h, EnvDigest(ctx, root))
	field(h, extra)
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		status, path := string(entry[:2]), string(entry[3:])
		if status == "??" && isHarnessPath(path) {
			// The harness writes untracked state there at every stop (QA
			// journeys and scenarios are deliberately not ignored); hashing
			// it would rerun, and reblock, a tree nothing else changed in.
			// Tracked harness files a test may read are still hashed.
			continue
		}
		field(h, status)
		field(h, path)
		if (status[0] == 'R' || status[0] == 'C') && i+1 < len(fields) {
			i++
			field(h, string(fields[i]))
		}
		pathState(h, filepath.Join(root, filepath.FromSlash(path)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pathState writes a path's mode and its symlink target or contents. A
// missing or unreadable path writes a marker no contents can produce.
func pathState(h hash.Hash, path string) {
	info, err := os.Lstat(path)
	if err != nil {
		field(h, "\x00missing")
		return
	}
	field(h, info.Mode().String())
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, _ := os.Readlink(path)
		field(h, target)
	case info.Mode().IsRegular():
		f, err := os.Open(path)
		if err != nil {
			field(h, "\x00unreadable")
			return
		}
		sum := sha256.New()
		_, err = io.Copy(sum, f)
		f.Close()
		if err != nil {
			field(h, "\x00unreadable")
			return
		}
		field(h, "content:"+hex.EncodeToString(sum.Sum(nil)))
	}
}

func isHarnessPath(path string) bool {
	return slices.ContainsFunc(harnessDirs, func(dir string) bool { return strings.HasPrefix(path, dir) })
}

// field writes one length-prefixed value, so no two sequences of values
// hash alike.
func field(h hash.Hash, v string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(v)))
	h.Write(n[:])
	_, _ = io.WriteString(h, v)
}

// EnvDigest names the Go environment a test result belongs to: the same
// tree under another Go version, platform or flags is another run. Other
// environment variables the tests read cannot be known, so a reused result
// is also limited by age. "" when there is no go.mod or no go.
func EnvDigest(ctx context.Context, root string) string {
	if !fileExists(filepath.Join(root, "go.mod")) {
		return ""
	}
	cmd := exec.CommandContext(ctx, "go", "env", "GOVERSION", "GOOS", "GOARCH", "GOFLAGS", "CGO_ENABLED", "GOEXPERIMENT")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(string(out))))
	return hex.EncodeToString(sum[:8])
}

// headCommit is the HEAD commit of root; "" when there is none.
func headCommit(ctx context.Context, root string) string {
	out, err := gitOutput(ctx, root, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
