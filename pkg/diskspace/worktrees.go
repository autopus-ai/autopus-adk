package diskspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// BaseUnknown is the base of a repository where no ref of the ladder resolves.
const BaseUnknown = "unknown"

// Reasons a linked worktree's clean or merged state is unknown.
const (
	ReasonPathMissing     = "path_missing"
	ReasonStatusFailed    = "status_failed"
	ReasonNoBase          = "no_base"
	ReasonMergeBaseFailed = "merge_base_failed"
)

// cargoTargetMaxDepth bounds how deep a Cargo target directory is looked for.
const cargoTargetMaxDepth = 6

// mergeBaseLadder is the base a linked worktree is measured against; the
// first ref that resolves wins (REQ-DISK-WT-01).
var mergeBaseLadder = []struct{ name, ref string }{
	{"origin/HEAD", "refs/remotes/origin/HEAD"},
	{"origin/main", "refs/remotes/origin/main"},
	{"main", "refs/heads/main"},
	{"master", "refs/heads/master"},
}

// harnessWorktreeDirs are where the harness creates worktrees, relative to
// the main worktree.
var harnessWorktreeDirs = []string{".autopus/worktrees", ".claude/worktrees", ".worktrees"}

// Tristate is an answer git gave as true or false, or could not give.
type Tristate int

// Tristate values.
const (
	Unknown Tristate = iota
	True
	False
)

// MarshalJSON writes true, false, or "unknown".
func (t Tristate) MarshalJSON() ([]byte, error) {
	switch t {
	case True:
		return []byte("true"), nil
	case False:
		return []byte("false"), nil
	default:
		return []byte(`"unknown"`), nil
	}
}

// UnmarshalJSON reads what MarshalJSON writes.
func (t *Tristate) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true":
		*t = True
	case "false":
		*t = False
	case `"unknown"`:
		*t = Unknown
	default:
		return fmt.Errorf("tristate %s is not true, false, or \"unknown\"", data)
	}
	return nil
}

// Worktree is one entry of `git worktree list --porcelain -z`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Bare     bool
	Main     bool
	Locked   bool
	Prunable bool
	Harness  bool
	// Clean and Merged classify a linked worktree; the main worktree is
	// listed but never classified or sized. Reason says why one is unknown.
	Clean            Tristate
	Merged           Tristate
	Reason           string
	Base             string
	Bytes            int64
	CargoTargetBytes int64
	// NodeModulesBytes totals the node_modules directories in the worktree.
	NodeModulesBytes int64
	// LockReason is the reason recorded when the worktree was locked.
	LockReason string
	// Suggestion is the removal command an operator may paste for a merged,
	// clean, unlocked, unprunable linked worktree, or the notice that
	// replaces it for a path with control characters. Nothing runs it.
	Suggestion string
}

// WorktreeScan is the worktrees consumer. Root.Path is the main worktree.
type WorktreeScan struct {
	Root  Root
	Base  string
	Items []Worktree
}

// ScanWorktrees lists every worktree of the repository holding projectDir and
// classifies the linked ones (REQ-DISK-WT-01). It only reads: no worktree,
// branch, or target directory is removed, pruned, locked, or modified, and git
// runs with --no-optional-locks so not even the index is refreshed
// (REQ-DISK-WT-02). The -z records keep paths with spaces, quotes, or
// newlines intact.
func ScanWorktrees(ctx context.Context, run Runner, projectDir string) WorktreeScan {
	out, err := git(ctx, run, projectDir, "worktree", "list", "--porcelain", "-z")
	if errors.Is(err, exec.ErrNotFound) {
		return WorktreeScan{Root: Root{Reason: ReasonGitUnavailable, Message: "git is not on PATH"}}
	}
	items := parseWorktreePorcelain(out)
	if err != nil || len(items) == 0 {
		return WorktreeScan{Root: Root{Reason: ReasonNotAGitRepository, Message: "the project is not in a git repository"}}
	}
	items[0].Main = true
	base, baseRef := resolveBase(ctx, run, projectDir)
	for i := 1; i < len(items); i++ {
		classifyWorktree(ctx, run, items[0].Path, base, baseRef, &items[i])
	}
	return WorktreeScan{Root: Root{Path: items[0].Path}, Base: base, Items: items}
}

// Linked returns the linked worktrees.
func (s WorktreeScan) Linked() []Worktree {
	var linked []Worktree
	for _, item := range s.Items {
		if !item.Main {
			linked = append(linked, item)
		}
	}
	return linked
}

// Bytes and CargoTargetBytes total the linked worktrees.
func (s WorktreeScan) Bytes() (total, cargo int64) {
	for _, item := range s.Linked() {
		total += item.Bytes
		cargo += item.CargoTargetBytes
	}
	return total, cargo
}

// NodeModulesBytes totals the node_modules of the linked worktrees.
func (s WorktreeScan) NodeModulesBytes() (total int64) {
	for _, item := range s.Linked() {
		total += item.NodeModulesBytes
	}
	return total
}

func git(ctx context.Context, run Runner, dir string, args ...string) ([]byte, error) {
	return run(ctx, dir, nil, "git", append([]string{"--no-optional-locks"}, args...)...)
}

func parseWorktreePorcelain(out []byte) []Worktree {
	var items []Worktree
	for _, field := range strings.Split(string(out), "\x00") {
		key, value, _ := strings.Cut(field, " ")
		if key == "worktree" {
			items = append(items, Worktree{Path: value})
			continue
		}
		if len(items) == 0 || field == "" {
			continue
		}
		cur := &items[len(items)-1]
		switch key {
		case "HEAD":
			cur.Head = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			cur.Detached = true
		case "bare":
			cur.Bare = true
		case "locked":
			cur.Locked = true
			cur.LockReason = value
		case "prunable":
			cur.Prunable = true
		}
	}
	return items
}

func resolveBase(ctx context.Context, run Runner, dir string) (name, ref string) {
	for _, candidate := range mergeBaseLadder {
		if _, err := git(ctx, run, dir, "rev-parse", "--verify", "--quiet", candidate.ref+"^{commit}"); err == nil {
			return candidate.name, candidate.ref
		}
	}
	return BaseUnknown, ""
}

func classifyWorktree(ctx context.Context, run Runner, mainPath, base, baseRef string, wt *Worktree) {
	wt.Harness = isHarnessWorktree(mainPath, wt.Path)
	wt.Base = base
	if info, err := os.Lstat(wt.Path); err != nil || !info.IsDir() {
		wt.Reason = ReasonPathMissing
		return
	}
	status, err := git(ctx, run, wt.Path, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	switch {
	case err != nil:
		wt.Reason = ReasonStatusFailed
	case len(status) == 0:
		wt.Clean = True
	default:
		wt.Clean = False
	}
	if baseRef == "" {
		wt.setReason(ReasonNoBase)
	} else {
		_, err := git(ctx, run, wt.Path, "merge-base", "--is-ancestor", "HEAD", baseRef)
		switch exitCode(err) {
		case 0:
			wt.Merged = True
		case 1:
			wt.Merged = False
		default:
			wt.setReason(ReasonMergeBaseFailed)
		}
	}
	wt.Bytes, wt.CargoTargetBytes, wt.NodeModulesBytes = worktreeUsage(wt.Path)
	if wt.Clean == True && wt.Merged == True && !wt.Locked && !wt.Prunable {
		wt.Suggestion = removalSuggestion(wt.Path, base, wt.Bytes)
	}
}

func (wt *Worktree) setReason(reason string) {
	if wt.Reason == "" {
		wt.Reason = reason
	}
}

// worktreeUsage sizes a worktree, the Cargo target directories in it (a
// directory named target, at most cargoTargetMaxDepth deep, whose parent
// holds a regular Cargo.toml), and its node_modules directories (the first
// one on each path, at the same depth bound). A target or node_modules found
// is not entered again, so nothing is counted twice.
func worktreeUsage(path string) (total, cargo, node int64) {
	var targets, modules []string
	var ownBytes int64
	total, err := walkAllocated(path, func(rel string, depth int, info fs.FileInfo, st fileStat) bool {
		if !info.IsDir() || depth > cargoTargetMaxDepth {
			return true
		}
		dir := filepath.Join(path, filepath.FromSlash(rel))
		switch info.Name() {
		case "node_modules":
			modules = append(modules, dir)
		case "target":
			manifest, err := os.Lstat(filepath.Join(filepath.Dir(dir), "Cargo.toml"))
			if err != nil || !manifest.Mode().IsRegular() {
				return true
			}
			targets = append(targets, dir)
		default:
			return true
		}
		ownBytes += st.alloc
		return false
	})
	if err != nil {
		return 0, 0, 0
	}
	for _, target := range targets {
		if size, err := walkAllocated(target, nil); err == nil {
			cargo += size
		}
	}
	for _, module := range modules {
		if size, err := walkAllocated(module, nil); err == nil {
			node += size
		}
	}
	return total - ownBytes + cargo + node, cargo, node
}

// pipelineWorktreeName is the name pkg/pipeline's WorktreeManager gives the
// worktrees it creates directly under the system temp dir (os.MkdirTemp with
// the prefix autopus-worktree-), SPEC-DISK-002 REQ-DISK2-REG-04.
var pipelineWorktreeName = regexp.MustCompile(`^autopus-worktree-[0-9]+$`)

func isHarnessWorktree(mainPath, path string) bool {
	mainPath, path = canonicalOrSelf(mainPath), canonicalOrSelf(path)
	if pipelineWorktreeName.MatchString(filepath.Base(path)) && filepath.Dir(path) == canonicalOrSelf(os.TempDir()) {
		return true
	}
	for _, dir := range harnessWorktreeDirs {
		if strings.HasPrefix(path, filepath.Join(mainPath, filepath.FromSlash(dir))+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
