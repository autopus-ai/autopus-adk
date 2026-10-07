package editguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// rootStages caches the stages of one project root; each loads on first use,
// so manifests are read only for namespace targets (REQ-EG-24).
type rootStages struct {
	root      string
	fold      bool
	locks     *lockView
	manifests *manifestStage
	source    *bool
	state     *stateDirs
}

func (ev *evaluation) stagesOf(target Target) *rootStages {
	stages, ok := ev.roots[target.Root]
	if !ok {
		stages = &rootStages{root: target.Root, fold: target.CaseInsensitive}
		ev.roots[target.Root] = stages
	}
	return stages
}

func (s *rootStages) lockView(ev *evaluation) *lockView {
	if s.locks == nil {
		view := loadLockView(s.root, s.fold, ev.now)
		s.locks = &view
		ev.note(view.fault)
	}
	return s.locks
}

func (s *rootStages) manifestStage() *manifestStage {
	if s.manifests == nil {
		stage := loadManifestStage(s.root, s.fold)
		s.manifests = &stage
	}
	return s.manifests
}

func (s *rootStages) sourceRepo() bool {
	if s.source == nil {
		source := isSourceRepo(s.root)
		s.source = &source
	}
	return *s.source
}

// guardState reports whether target is the lock store or a root manifest
// (REQ-EG-09): by its folded key, or by directory identity, so a spelling the
// key fold does not know still names the same directories (H1).
func (s *rootStages) guardState(target Target) bool {
	return isGuardState(target.Key) || s.namesStateDir(target)
}

// isGuardState reports the lock store and everything at or below a manifest
// name of the manifest directory, so no edit can plant a directory where a
// manifest belongs (H2).
func isGuardState(key string) bool {
	if key == FixLocksDir || strings.HasPrefix(key, FixLocksDir+"/") {
		return true
	}
	rest, ok := strings.CutPrefix(key, manifestDir+"/")
	if !ok {
		return false
	}
	name, _, _ := strings.Cut(rest, "/")
	return strings.HasSuffix(name, manifestSuffix)
}

// stateDirs are the guard-state directories of one root as they are now; a
// missing one is nil.
type stateDirs struct {
	manifests, fixLocks fs.FileInfo
}

func (s *rootStages) stateDirs() *stateDirs {
	if s.state == nil {
		s.state = &stateDirs{}
		if info, err := os.Stat(filepath.Join(s.root, manifestDir)); err == nil && info.IsDir() {
			s.state.manifests = info
		}
		if info, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(FixLocksDir))); err == nil && info.IsDir() {
			s.state.fixLocks = info
		}
	}
	return s.state
}

// namesStateDir walks the target's path up to the root: a component that is
// the lock store, or one whose parent is the manifest directory and whose name
// folds to a manifest name, makes the target guard state.
func (s *rootStages) namesStateDir(target Target) bool {
	dirs := s.stateDirs()
	if dirs.manifests == nil {
		return false
	}
	for p := target.Abs(); p != s.root; {
		parent := filepath.Dir(p)
		if parent == p {
			return false
		}
		if dirs.fixLocks != nil {
			if info, err := os.Stat(p); err == nil && os.SameFile(info, dirs.fixLocks) {
				return true
			}
		}
		if strings.HasSuffix(FoldKey(filepath.Base(p), s.fold), manifestSuffix) {
			if info, err := os.Stat(parent); err == nil && os.SameFile(info, dirs.manifests) {
				return true
			}
		}
		p = parent
	}
	return false
}
