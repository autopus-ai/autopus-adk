//go:build darwin

package harneval

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// sandboxed runs argv under grader.sb from the checkout, able to write only
// below grade, with exactly env. Like grader.sandbox_argv it refuses a root,
// module cache, or toolchain root that is not canonical or that is or holds
// a home or a temporary root, and a grade root holding the caches, the
// toolchain, or the checkout.
func (b *armBuild) sandboxed(grade, modcache string, env []string, dir string, timeout time.Duration, argv ...string) error {
	homes := []string{b.account, b.codexHome, b.owner, "/Users", "/private/var/folders", "/private/tmp", "/private/var/tmp"}
	for role, root := range map[string]string{"sandbox root": grade, "module cache": modcache, "toolchain root": b.goroot} {
		if info, err := os.Stat(root); err != nil || !info.IsDir() || canonical(root) != root {
			return fmt.Errorf("%s must be an existing canonical directory: %s", role, root)
		}
		for _, home := range homes {
			if home != "" && within(root, home) {
				return fmt.Errorf("%s must not be or contain a home or temporary root: %s", role, root)
			}
		}
	}
	for _, inner := range []string{modcache, b.goroot, b.root} {
		if within(grade, inner) {
			return fmt.Errorf("sandbox root must not contain the caches, toolchain or checkout: %s", grade)
		}
	}
	args := []string{"-f", filepath.Join(b.root, graderProfilePath), "-D", "GRADE_ROOT=" + grade, "-D", "MODCACHE=" + modcache,
		"-D", "GOROOT=" + b.goroot, "-D", "ACCOUNT_HOME=" + b.account, "-D", "CODEX_HOME_DIR=" + b.codexHome}
	_, err := b.output(dir, env, timeout, sandboxExecPath, append(args, argv...)...)
	return err
}

// output runs one trusted or sandboxed process in its own process group with
// exactly env and returns its stdout. On timeout, and once it has exited,
// whatever is left in its group is killed.
func (b *armBuild) output(dir string, env []string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = dir, append([]string{}, env...), &stdout, &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if err != nil {
		// %v, not %w: a child's exit status must not become the CLI exit code.
		return "", fmt.Errorf("%s: %v: %s", filepath.Base(name), err, tail(stderr.String()))
	}
	return stdout.String(), nil
}

// rejectLinkedComponents refuses a symlink at any existing component of rel
// below base, as workspace._relative does.
func rejectLinkedComponents(base, rel string) error {
	current := base
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink", current)
		}
	}
	return nil
}

// removeTree removes root, first making every directory writable: the
// session module cache is read-only.
func removeTree(root string) error {
	_ = filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(current, 0o700)
		}
		return nil
	})
	return os.RemoveAll(root)
}

// envValue is the last value of name in env.
func envValue(env []string, name string) string {
	value := ""
	for _, entry := range env {
		if key, rest, found := strings.Cut(entry, "="); found && key == name {
			value = rest
		}
	}
	return value
}

// canonical resolves every link of an existing path; a missing path is
// only cleaned, as os.path.realpath leaves it.
func canonical(name string) string {
	if name == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(name); err == nil {
		return resolved
	}
	return filepath.Clean(name)
}

// within reports whether inner is outer or lies below it.
func within(outer, inner string) bool {
	return inner == outer || strings.HasPrefix(inner, strings.TrimSuffix(outer, "/")+"/")
}

// tail is the last 400 characters of a process's whitespace-folded stderr.
func tail(text string) string {
	folded := strings.Join(strings.Fields(text), " ")
	if len(folded) > 400 {
		return folded[len(folded)-400:]
	}
	return folded
}
