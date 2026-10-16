package adapter

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// codexWorkerSandboxMode is the OS sandbox every worker Codex task runs in.
// A worker worktree is only a directory, so it does not isolate anything by
// itself; workspace-write keeps the task's writes inside its working
// directory (plus the temp roots Codex adds) and blocks the rest of the disk.
const codexWorkerSandboxMode = "workspace-write"

// codexGitCommonDirTimeout bounds the git lookup that runs while the command
// is built, so a stuck repository cannot hang task dispatch.
const codexGitCommonDirTimeout = 5 * time.Second

// codexGitCommonDir resolves the repository's shared git directory for a
// worktree. Worker tasks must commit (the commit postcondition), and in a
// linked worktree the commit writes objects and refs into that shared
// directory, which lies outside the worktree. It is a variable so tests can
// pin it without a repository.
var codexGitCommonDir = func(workDir string) (string, bool) {
	if strings.TrimSpace(workDir) == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), codexGitCommonDirTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", workDir,
		"rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", false
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" || !filepath.IsAbs(dir) {
		return "", false
	}
	return filepath.Clean(dir), true
}

// codexWorkerSandboxArgs returns the sandbox flags for a worker task that
// carries no evidence-run options. Network stays available because worker
// tasks install dependencies; the sandbox limits where the task can write.
func codexWorkerSandboxArgs(workDir string) []string {
	args := []string{
		"--sandbox", codexWorkerSandboxMode,
		"-c", "sandbox_workspace_write.network_access=true",
	}
	if dir, ok := codexGitCommonDir(workDir); ok {
		args = append(args, "--add-dir", dir)
	}
	return args
}
