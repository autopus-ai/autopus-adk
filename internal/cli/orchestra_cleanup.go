package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/terminal"
)

var orchestraSessionTerminalDetector = terminal.DetectTerminal
var orchestraSessionUpdater = orchestra.UpdateSession
var orchestraSessionRemover = orchestra.RemoveSession

// newLegacyOrchestraCleanupCmd creates the cleanup subcommand for orchestra.
// Retired (SPEC-PANERM-001): the command tree registers the stub of
// orchestra_retired.go instead; this body stays until the pane backend deletion.
// Loads a persisted session, kills all panes, and removes the session file.
func newLegacyOrchestraCleanupCmd() *cobra.Command {
	var sessionID string
	var workspaceRef string

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "yield-rounds 세션의 pane을 정리하고 세션 파일을 삭제한다",
		Long:  "지정된 세션 ID로 저장된 yield-rounds 세션의 모든 pane을 종료하고 세션 파일을 제거합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sessionID == "" {
				return fmt.Errorf("--session-id is required")
			}
			return runOrchestraCleanupWithWorkspace(cmd.Context(), sessionID, workspaceRef)
		},
	}

	cmd.Flags().StringVar(&sessionID, "session-id", "", "cleanup 대상 세션 ID (필수)")
	cmd.Flags().StringVar(&workspaceRef, "workspace-ref", "", "legacy cmux 세션의 원 workspace ref")
	_ = cmd.MarkFlagRequired("session-id")

	return cmd
}

// runOrchestraCleanup loads the session, kills panes, and removes the session file.
// Idempotent: returns nil if session file is already missing.
func runOrchestraCleanup(ctx context.Context, sessionID string) error {
	return runOrchestraCleanupWithWorkspace(ctx, sessionID, "")
}

func runOrchestraCleanupWithWorkspace(ctx context.Context, sessionID, workspaceRef string) error {
	session, err := orchestra.LoadSession(sessionID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "[cleanup] session %s already absent\n", sessionID)
			return nil
		}
		return fmt.Errorf("load cleanup session %s: %w", sessionID, err)
	}
	if len(session.Panes) == 0 {
		if workspaceRef != "" {
			if _, err := orchestra.ResolveSessionTerminalWithWorkspace(
				session, orchestraSessionTerminalDetector(), workspaceRef,
			); err != nil {
				return fmt.Errorf("validate cleanup workspace override: %w", err)
			}
		}
		if err := orchestraSessionRemover(sessionID); err != nil {
			return fmt.Errorf("remove empty cleanup session %s: %w", sessionID, err)
		}
		fmt.Fprintf(os.Stderr, "[cleanup] session %s: no panes, session file removed\n", sessionID)
		return nil
	}
	term, err := orchestra.ResolveSessionTerminalWithWorkspace(
		session, orchestraSessionTerminalDetector(), workspaceRef,
	)
	if err != nil {
		return fmt.Errorf("restore cleanup session %s terminal: %w", sessionID, err)
	}
	providers := make([]string, 0, len(session.Panes))
	for provider := range session.Panes {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	closed := 0
	remaining := make(map[string]string, len(session.Panes))
	for provider, paneID := range session.Panes {
		remaining[provider] = paneID
	}
	var closeErrors []error
	for _, provider := range providers {
		paneID := session.Panes[provider]
		if closeErr := term.Close(ctx, paneID); closeErr != nil {
			closeErrors = append(closeErrors, fmt.Errorf("%s pane %s: %w", provider, paneID, closeErr))
			continue
		}
		closed++
		delete(remaining, provider)
	}
	if len(closeErrors) > 0 {
		if closed > 0 {
			session.Panes = remaining
			if updateErr := orchestraSessionUpdater(*session); updateErr != nil {
				return fmt.Errorf("cleanup session %s: closed %d/%d panes but failed to persist retry set; original session retained: %w",
					sessionID, closed, len(providers), errors.Join(errors.Join(closeErrors...), updateErr))
			}
		}
		return fmt.Errorf("cleanup session %s: closed %d/%d panes; session retained for retry: %w",
			sessionID, closed, len(providers), errors.Join(closeErrors...))
	}
	session.Panes = map[string]string{}
	if err := orchestraSessionUpdater(*session); err != nil {
		return fmt.Errorf("persist cleaned session %s tombstone: %w", sessionID, err)
	}
	if err := orchestraSessionRemover(sessionID); err != nil {
		return fmt.Errorf("remove cleaned session %s: %w", sessionID, err)
	}
	fmt.Fprintf(os.Stderr, "[cleanup] session %s: %d panes closed, session file removed\n", sessionID, closed)
	return nil
}
