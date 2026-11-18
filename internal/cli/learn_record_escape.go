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
	cmd.Flags().StringVar(&f.provider, "provider", "", "escape: agent that wrote the lesson (default: $AUTOPUS_PROVIDER, else claude)")
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
		provider = os.Getenv("AUTOPUS_PROVIDER")
	}
	if provider == "" {
		provider = "claude"
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
