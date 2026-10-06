package rulecond_test

// SPEC-EDITGUARD-001 REQ-EG-09: fix-lock state is created and read only through
// the os.Root component-wise descent the sticky counter state uses.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

func runtimeProject(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOpenRuntimeStateDir_CreatesTheDirectoryAndContainsWrites(t *testing.T) {
	t.Parallel()
	root := runtimeProject(t)
	state, err := rulecond.OpenRuntimeStateDir(root, "fix-locks")
	if err != nil {
		t.Fatalf("OpenRuntimeStateDir: %v", err)
	}
	defer func() { _ = state.Close() }()
	if err := state.WriteFile("probe.json", []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".autopus", "runtime", "fix-locks", "probe.json"))
	if err != nil || string(got) != "{}" {
		t.Fatalf("write landed elsewhere: %q, %v", got, err)
	}
	again, err := rulecond.OpenRuntimeStateDir(root, "fix-locks")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = again.Close()
}

// A committed symlink at any component is refused, never followed, and the
// read-side lookup reports it as unusable rather than as absent.
func TestRuntimeStateDir_SymlinkedComponent_IsRefusedWithoutWritingThroughIt(t *testing.T) {
	t.Parallel()
	for _, component := range []string{".autopus", ".autopus/runtime", ".autopus/runtime/fix-locks"} {
		root := runtimeProject(t)
		outside := runtimeProject(t)
		link := filepath.Join(root, filepath.FromSlash(component))
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if state, err := rulecond.OpenRuntimeStateDir(root, "fix-locks"); err == nil {
			_ = state.Close()
			t.Errorf("%s: OpenRuntimeStateDir followed the link", component)
		}
		state, err := rulecond.LookupRuntimeStateDir(root, "fix-locks")
		if err == nil {
			_ = state.Close()
			t.Errorf("%s: LookupRuntimeStateDir followed the link", component)
		} else if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: a refused link reads as absent state", component)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Errorf("%s: something was created in the link target: %v", component, entries)
		}
	}
}

func TestLookupRuntimeStateDir_AbsentState_IsNotExistAndCreatesNothing(t *testing.T) {
	t.Parallel()
	root := runtimeProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".autopus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := rulecond.LookupRuntimeStateDir(root, "fix-locks"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".autopus", "runtime")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lookup created state: %v", err)
	}
	if _, err := rulecond.OpenRuntimeStateDir(root, "fix-locks"); err != nil {
		t.Fatal(err)
	}
	state, err := rulecond.LookupRuntimeStateDir(root, "fix-locks")
	if err != nil {
		t.Fatalf("lookup of existing state: %v", err)
	}
	_ = state.Close()
}

func TestRuntimeStateDir_NonDirectoryComponentOrBadName_IsRefused(t *testing.T) {
	t.Parallel()
	root := runtimeProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".autopus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".autopus", "runtime"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rulecond.OpenRuntimeStateDir(root, "fix-locks"); err == nil {
		t.Error("a regular file at .autopus/runtime was descended")
	}
	if _, err := rulecond.LookupRuntimeStateDir(root, "fix-locks"); err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lookup err = %v, want an unusable-state error", err)
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "nul\x00"} {
		if _, err := rulecond.OpenRuntimeStateDir(runtimeProject(t), name); err == nil {
			t.Errorf("name %q was accepted", name)
		}
	}
	if _, err := rulecond.OpenRuntimeStateDir(filepath.Join(root, "missing"), "fix-locks"); err == nil {
		t.Error("a missing project root was accepted")
	}
}
