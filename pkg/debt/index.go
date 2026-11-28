package debt

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// indexPath caches what each file contributes, keyed by its size and
// modification time, so a stop reads only the files that changed.
const indexPath = ".autopus/runtime/debt/index.gob"

// win is one window of dupWindow code lines: its hash and the file lines
// it spans (1-based).
type win struct {
	H          uint64
	Start, End int
}

type fileEntry struct {
	Size, Mod int64
	Product   bool
	Markers   []Item
	Wins      []win // one per code-line position; H is 0 for a window too small to count
	Physical  int
}

type index struct {
	Version int
	Files   map[string]*fileEntry
	dirty   bool
}

// indexVersion changes whenever what an entry holds changes.
const indexVersion = 2

// maxScanBytes skips files too large to be hand-written code (bundles,
// fixtures): one would blow the stop's time budget on its own.
const maxScanBytes = 2 << 20

// thirdParty are directories of code a project carries but does not write.
var thirdParty = []string{"vendor/", "node_modules/", "third_party/", "third-party/", "external/"}

func isThirdParty(f string) bool {
	for _, d := range thirdParty {
		if strings.HasPrefix(f, d) || strings.Contains(f, "/"+d) {
			return true
		}
	}
	return false
}

func loadIndex(root string) *index {
	x := &index{Version: indexVersion, Files: map[string]*fileEntry{}}
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(indexPath)))
	if err != nil {
		return x
	}
	defer f.Close()
	var got index
	if gob.NewDecoder(f).Decode(&got) == nil && got.Version == indexVersion && got.Files != nil {
		return &got
	}
	return x
}

func (x *index) save(root string) {
	if !x.dirty {
		return
	}
	dst := filepath.Join(root, filepath.FromSlash(indexPath))
	if os.MkdirAll(filepath.Dir(dst), 0o755) != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", dst, os.Getpid())
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	if gob.NewEncoder(f).Encode(x) != nil || f.Close() != nil {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Rename(tmp, dst)
}

// ErrIncomplete means the scan ran out of its time before every changed
// file was read; the files read so far are kept for the next scan.
var ErrIncomplete = errors.New("debt: scan did not finish within its time; the next one continues")

// refresh brings the index in line with root's work tree and returns the
// product files in order. Past deadline (zero means none) it stops reading
// files and returns ErrIncomplete.
func (x *index) refresh(ctx context.Context, root string, deadline time.Time) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("debt: git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	live := map[string]bool{}
	var files []string
	incomplete := false
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" || live[f] {
			continue
		}
		live[f] = true
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxScanBytes || isThirdParty(f) {
			if _, had := x.Files[f]; had {
				delete(x.Files, f)
				x.dirty = true
			}
			continue
		}
		e := x.Files[f]
		if e == nil || e.Size != fi.Size() || e.Mod != fi.ModTime().UnixNano() {
			if !deadline.IsZero() && time.Now().After(deadline) {
				incomplete = true
				continue
			}
			e = scanFile(root, f, fi)
			x.Files[f] = e
			x.dirty = true
		}
		if e.Product {
			files = append(files, f)
		}
	}
	for f := range x.Files {
		if !live[f] {
			delete(x.Files, f)
			x.dirty = true
		}
	}
	sort.Strings(files)
	if incomplete {
		return files, ErrIncomplete
	}
	return files, nil
}

func scanFile(root, f string, fi os.FileInfo) *fileEntry {
	e := &fileEntry{Size: fi.Size(), Mod: fi.ModTime().UnixNano()}
	product := verifygate.IsProductSource(root, f)
	if !product && !verifygate.IsMaintainedCode(root, f) {
		return e
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
	if err != nil {
		return e
	}
	e.Product = true
	lines := strings.Split(string(data), "\n")
	e.Markers = markers(f, lines)
	if !product {
		return e // markers only: no size limit or duplicate measure for this language
	}
	e.Physical = len(lines)
	text, at := codeLines(lines)
	for i := 0; i+dupWindow <= len(text); i++ {
		w := win{Start: at[i] + 1, End: at[i+dupWindow-1] + 1}
		joined := strings.Join(text[i:i+dupWindow], "\n")
		if len(joined) >= dupMinChars {
			h := fnv.New64a()
			_, _ = h.Write([]byte(joined))
			w.H = h.Sum64() | 1 // never 0, which marks a small window
		}
		e.Wins = append(e.Wins, w)
	}
	return e
}
