package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

var orchestraInjectSubmitDelay = 500 * time.Millisecond

// newLegacyOrchestraInjectCmd creates the "orchestra inject" subcommand.
// Retired (SPEC-PANERM-001): the command tree registers the stub of
// orchestra_retired.go instead; this body stays until the pane backend deletion.
// Sends a prompt to a specific provider's pane in an existing session.
func newLegacyOrchestraInjectCmd() *cobra.Command {
	var (
		sessionID    string
		provider     string
		workspaceRef string
	)

	cmd := &cobra.Command{
		Use:   "inject <prompt>",
		Short: "세션의 특정 pane에 프롬프트를 주입한다",
		Long:  "yield-rounds로 생성된 세션의 프로바이더 pane에 SendLongText로 프롬프트를 전송합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if sessionID == "" {
				return fmt.Errorf("--session-id is required")
			}
			if provider == "" {
				return fmt.Errorf("--provider is required")
			}
			return runOrchestraInjectWithWorkspace(cmd, sessionID, provider, workspaceRef, args[0])
		},
	}

	cmd.Flags().StringVar(&sessionID, "session-id", "", "세션 ID")
	cmd.Flags().StringVar(&provider, "provider", "", "프롬프트를 보낼 프로바이더 이름")
	cmd.Flags().StringVar(&workspaceRef, "workspace-ref", "", "legacy cmux 세션의 원 workspace ref")
	_ = cmd.MarkFlagRequired("session-id")
	_ = cmd.MarkFlagRequired("provider")

	return cmd
}

// runOrchestraInjectWithWorkspace loads a session, restores its terminal context,
// and sends the prompt to the requested provider pane.
func runOrchestraInjectWithWorkspace(cmd *cobra.Command, sessionID, provider, workspaceRef, prompt string) error {
	session, err := orchestra.LoadSession(sessionID)
	if err != nil {
		return fmt.Errorf("session %s not found: %w", sessionID, err)
	}

	paneID, ok := session.Panes[provider]
	if !ok {
		available := make([]string, 0, len(session.Panes))
		for name := range session.Panes {
			available = append(available, name)
		}
		return fmt.Errorf("provider %q not found in session (available: %v)", provider, available)
	}

	term, err := orchestra.ResolveSessionTerminalWithWorkspace(
		session, orchestraSessionTerminalDetector(), workspaceRef,
	)
	if err != nil {
		return fmt.Errorf("restore session %s terminal: %w", sessionID, err)
	}

	ctx := cmd.Context()

	// Send the prompt via SendLongText
	if err := term.SendLongText(ctx, terminal.PaneID(paneID), prompt); err != nil {
		return fmt.Errorf("SendLongText to %s failed: %w", provider, err)
	}

	// Send Enter to submit the prompt
	time.Sleep(orchestraInjectSubmitDelay)
	if err := term.SendCommand(ctx, terminal.PaneID(paneID), "\n"); err != nil {
		fmt.Fprintf(os.Stderr, "[inject] SendCommand (Enter) failed: %v — prompt may need manual submit\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Injected prompt to %s (pane %s)\n", provider, paneID)
	return nil
}
