package cli

import (
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// escapeFlags are the `auto learn record --type escape` inputs.
type escapeFlags struct {
	missedBy string
	paths    []string
	keywords []string
	commit   string
	provider string
}

func (f *escapeFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.missedBy, "missed-by", "", "escape: the check that passed while the defect was present; at most 512 bytes after secret redaction")
	cmd.Flags().StringArrayVar(&f.paths, "when-path", nil, "escape: a changed-path glob a future check applies under (repeatable)")
	cmd.Flags().StringArrayVar(&f.keywords, "when-keyword", nil, "escape: a string in an added diff line a future check applies under (repeatable)")
	cmd.Flags().StringVar(&f.commit, "commit", "", "escape: commit the lesson belongs to (default: HEAD)")
	cmd.Flags().StringVar(&f.provider, "provider", "", "escape: agent that wrote the lesson (default: $AUTOPUS_PROVIDER, else detected from the session environment; unknown lets any provider refute)")
}

// toEscape returns the escape fields for an escape record, or nil otherwise.
func (f *escapeFlags) toEscape(entryType string) *learn.Escape {
	if entryType != string(learn.EntryTypeEscape) {
		return nil
	}
	commit := f.commit
	if commit == "" {
		commit = headCommit()
	}
	provider := f.provider
	if provider == "" {
		provider = detectAgentProvider(os.Environ())
	}
	return &learn.Escape{
		MissedBy: f.missedBy, WhenPaths: f.paths, WhenKeywords: f.keywords,
		Commit: commit, Provider: provider,
	}
}

// headCommit is HEAD of the working directory's repository, or "" outside one.
func headCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// detectAgentProvider names the agent CLI this process runs under, from the
// variables each CLI sets for its child processes, or "" when unknown. An
// unknown drafter only widens who may refute; it never narrows it.
func detectAgentProvider(environ []string) string {
	has := func(prefix string) bool {
		for _, kv := range environ {
			if strings.HasPrefix(kv, prefix) {
				return true
			}
		}
		return false
	}
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "AUTOPUS_PROVIDER="); ok && v != "" {
			return v
		}
	}
	// Only variables a CLI sets for its own child processes count: a user can
	// export CODEX_HOME or similar in any shell.
	switch {
	case has("CLAUDECODE="):
		return "claude"
	case has("GEMINI_CLI="):
		return "gemini"
	case has("CODEX_SANDBOX=") || has("CODEX_SANDBOX_NETWORK_DISABLED="):
		return "codex"
	case has("OPENCODE=") || has("OPENCODE_CLIENT="):
		return "opencode"
	}
	return ""
}
