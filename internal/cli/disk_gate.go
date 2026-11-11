package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskgate"
	"github.com/insajin/autopus-adk/pkg/diskspace"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// diskGateInput is the part of a Claude Code PreToolUse payload the gate
// reads. A subagent's tool call carries the same fields plus agent_id, so
// the gate covers subagents without telling them apart.
type diskGateInput struct {
	ToolName  string `json:"tool_name"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

type diskGateDecision struct {
	Event    string `json:"hookEventName"`
	Decision string `json:"permissionDecision"`
	Reason   string `json:"permissionDecisionReason"`
}

func newDiskGateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gate",
		Short: "PreToolUse hook: deny heavy shell commands while free space is below disk.min_free_gb",
		Long: "Reads a Claude Code PreToolUse payload on stdin. When the Bash command runs a build, test, install, " +
			"container build, or `git worktree add` (see hooks.disk_gate.extra_commands to add more) and free space on " +
			"the file system of the payload's cwd is below disk.min_free_gb, prints a deny decision telling the agent to " +
			"pause and run `auto disk status`. Every `auto` command passes, `auto disk lease -- <command>` included. " +
			"It costs one statfs and walks nothing. It always exits 0 and prints nothing when it allows. " +
			"AUTOPUS_DISK_GATE=off or hooks.disk_gate: false turns it off.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if reason := diskGateReason(cmd.InOrStdin()); reason != "" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetEscapeHTML(false)
				return encoder.Encode(map[string]diskGateDecision{"hookSpecificOutput": {
					Event: rulecond.EventPreToolUse, Decision: "deny", Reason: reason}})
			}
			return nil
		},
	}
}

// diskGateReason returns the deny reason, or "" to allow. Anything it cannot
// read, measure, or parse allows: the gate must never stop work by failing.
func diskGateReason(stdin io.Reader) string {
	if strings.EqualFold(os.Getenv("AUTOPUS_DISK_GATE"), "off") {
		return ""
	}
	var in diskGateInput
	if json.NewDecoder(stdin).Decode(&in) != nil || (in.ToolName != "" && in.ToolName != "Bash") {
		return ""
	}
	if strings.TrimSpace(in.ToolInput.Command) == "" {
		return ""
	}
	dir := in.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	disk, hooks := config.DiskConf{}.Effective(), config.HooksConf{}
	if harness, err := config.LoadPreview(dir); err == nil {
		disk, hooks = harness.Disk.Effective(), harness.Hooks
	}
	if !hooks.IsDiskGateEnabled() {
		return ""
	}
	verdict := diskgate.Classify(in.ToolInput.Command, hooks.DiskGateExtraCommands()...)
	if !verdict.Heavy {
		return ""
	}
	space, err := diskFree(dir)
	if err != nil || space.AvailBytes >= disk.FloorBytes() {
		return ""
	}
	return terminalSafe(fmt.Sprintf("Disk gate: %s GiB free on %s, below the floor of %s GiB (disk.min_free_gb), so this "+
		"`%s` step was not started. Pause heavy work: run `auto disk status` to see which consumer grew, then free space "+
		"you own with `auto disk reclaim --apply` and `auto disk trim --apply`, or wait for other sessions to finish. "+
		"To run it anyway under a machine-wide reservation, use `auto disk lease --need <GiB> -- <command>`. "+
		"If you lead other agents, tell each of them to pause heavy work too.",
		diskspace.GiBString(space.AvailBytes), diskspace.DisplayPath(space.Mount), diskspace.GiBString(disk.FloorBytes()), verdict.Match))
}
