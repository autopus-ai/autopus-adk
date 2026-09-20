package agentprobe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/codexruntime"
)

// CodexOptions configures an explicitly requested provider-consuming probe.
// The collector uses native app-server events, never assistant self-reports.
type CodexOptions struct {
	Executable string
	Model      string
	WorkingDir string
	Timeout    time.Duration
}

// RunCodex probes this invocation's actual native lifecycle. Missing native
// notifications remain unknown; process termination never becomes cleanup proof.
func RunCodex(ctx context.Context, opts CodexOptions) (Evidence, error) {
	if opts.Executable == "" {
		opts.Executable = "codex"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 90 * time.Second
	}
	if opts.Timeout > 5*time.Minute {
		return Evidence{}, fmt.Errorf("Codex probe timeout exceeds five minutes")
	}
	cwd, err := os.MkdirTemp("", "autopus-codex-lifecycle-")
	if err != nil {
		return Evidence{}, err
	}
	// Freeze the owned temporary path before an explicit working directory replaces cwd.
	defer func(temporary string) { _ = os.RemoveAll(temporary) }(cwd)
	if opts.WorkingDir != "" {
		cwd, err = filepath.Abs(opts.WorkingDir)
		if err != nil {
			return Evidence{}, err
		}
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return Evidence{}, err
	}
	e := Evidence{Version: 1, Platform: "codex", RunID: "probe-" + hex.EncodeToString(nonce), SupervisorID: "unobserved", Challenge: "probe-" + hex.EncodeToString(nonce), RuntimeVersion: "unknown", Events: []Event{}}
	if raw, ok := codexruntime.ProbeVersion(ctx, opts.Executable, 5*time.Second); ok {
		fields := strings.Fields(raw)
		for _, field := range fields {
			if token(field) && strings.Contains(field, ".") {
				e.RuntimeVersion = field
				break
			}
		}
	}
	// Reserve a bounded cleanup interval beyond the behavior deadline, while
	// retaining the caller's cancellation authority over all child processes.
	processCtx, stop := context.WithTimeout(ctx, opts.Timeout+10*time.Second)
	defer stop()
	rpc, err := startCodexRPC(processCtx, opts.Executable, cwd)
	if err != nil {
		return e, err
	}
	defer rpc.close()
	behavior, cancel := context.WithTimeout(processCtx, opts.Timeout)
	defer cancel()
	return runCodexLifecycle(behavior, processCtx, rpc, e, opts, cwd)
}

func runCodexLifecycle(ctx, cleanupParent context.Context, rpc codexRPC, e Evidence, opts CodexOptions, cwd string) (Evidence, error) {
	if _, err := rpc.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "autopus_lifecycle_probe", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}); err != nil {
		return e, codexDiagnostic("initialize_failed", err)
	}
	if err := rpc.notify("initialized", map[string]any{}); err != nil {
		return e, err
	}
	params := map[string]any{"cwd": cwd, "approvalPolicy": "never", "sandbox": "read-only", "ephemeral": false, "developerInstructions": "This is a bounded native subagent lifecycle probe. Do not edit files, use network tools, or invoke skills. Follow the exact requested subagent protocol only."}
	if opts.Model != "" {
		params["model"] = opts.Model
	}
	raw, err := rpc.call(ctx, "thread/start", params)
	if err != nil {
		return e, codexDiagnostic("thread_start_failed", err)
	}
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &started) != nil || !token(started.Thread.ID) {
		return e, fmt.Errorf("Codex thread/start missing native thread ID")
	}
	e.SupervisorID = started.Thread.ID
	capture := newCodexCapture(e)
	behaviorErr := runCodexCases(ctx, rpc, capture)
	cleanupCtx, stop := context.WithTimeout(cleanupParent, 8*time.Second)
	defer stop()
	cleanupErr := cleanupCodexProbe(cleanupCtx, rpc, capture)
	if behaviorErr != nil {
		return capture.evidence, behaviorErr
	}
	if cleanupErr != nil {
		return capture.evidence, codexDiagnostic("cleanup_unverified", cleanupErr)
	}
	return capture.evidence, nil
}

func runCodexCases(ctx context.Context, rpc codexRPC, c *codexCapture) error {
	prompt := "Spawn exactly one native subagent. Ask that subagent to return only this exact token: " + c.evidence.Challenge + ". Wait for its native result. Do not fabricate the result yourself. Do not close/archive the child. Then finish your own turn."
	if err := startCodexProbeTurn(ctx, rpc, c, prompt); err != nil {
		return err
	}
	for !c.rootTurnDone || c.activityReadPending {
		if c.rootTurnDone && c.activityReadPending {
			if err := codexReadRetryPause(ctx); err != nil {
				return codexDiagnostic("native_activity_visibility_timeout", err)
			}
			if err := observeCodexActivities(ctx, rpc, c, "success"); err != nil {
				return codexDiagnostic("native_success_activity_failed", err)
			}
			continue
		}
		m, err := rpc.next(ctx)
		if err != nil {
			return codexDiagnostic("native_protocol_receive_failed", err)
		}
		if err := c.observe(m, "success"); err != nil {
			return codexDiagnostic("native_event_observation_failed", err)
		}
		if codexNeedsActivityRefresh(c, m) {
			if err := observeCodexActivities(ctx, rpc, c, "success"); err != nil {
				return codexDiagnostic("native_success_activity_failed", err)
			}
		}
	}
	if !c.successResult {
		if c.rootTurnStatus == "failed" {
			return codexDiagnostic("supervisor_turn_failed", nil)
		}
		if c.rootTurnStatus == "interrupted" {
			return codexDiagnostic("supervisor_turn_interrupted", nil)
		}
		if c.children["success"] == "" {
			return codexDiagnostic("native_spawn_unobserved", nil)
		}
		return codexDiagnostic("native_result_unobserved", nil)
	}
	prompt = "Spawn exactly one additional native subagent. Ask it to run a local sleep for 60 seconds and then return DONE. It must not write files or use the network. Do not interrupt or close it yourself; an external probe will interrupt it. Wait for its result using native subagent tools."
	if err := startCodexProbeTurn(ctx, rpc, c, prompt); err != nil {
		return err
	}
	for c.children["cancel"] == "" && (!c.rootTurnDone || c.activityReadPending) {
		if c.rootTurnDone && c.activityReadPending {
			if err := codexReadRetryPause(ctx); err != nil {
				return codexDiagnostic("native_activity_visibility_timeout", err)
			}
			if err := observeCodexActivities(ctx, rpc, c, "cancel"); err != nil {
				return codexDiagnostic("native_cancel_activity_failed", err)
			}
			continue
		}
		m, err := rpc.next(ctx)
		if err != nil {
			return codexDiagnostic("native_protocol_receive_failed", err)
		}
		if err := c.observe(m, "cancel"); err != nil {
			return codexDiagnostic("native_event_observation_failed", err)
		}
		if codexNeedsActivityRefresh(c, m) {
			if err := observeCodexActivities(ctx, rpc, c, "cancel"); err != nil {
				return codexDiagnostic("native_cancel_activity_failed", err)
			}
		}
	}
	if c.children["cancel"] == "" {
		return codexDiagnostic("cancellation_worker_unobserved", nil)
	}
	if err := interruptCodexChild(ctx, rpc, c); err != nil {
		return codexDiagnostic("native_cancellation_unverified", err)
	}
	return nil
}

func startCodexProbeTurn(ctx context.Context, rpc codexRPC, c *codexCapture, prompt string) error {
	raw, err := rpc.call(ctx, "turn/start", map[string]any{"threadId": c.evidence.SupervisorID, "input": []map[string]string{{"type": "text", "text": prompt}}})
	if err != nil {
		return codexDiagnostic("native_turn_start_failed", err)
	}
	var result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(raw, &result) != nil || !token(result.Turn.ID) {
		return fmt.Errorf("Codex turn/start missing native turn ID")
	}
	c.rootTurn = result.Turn.ID
	c.rootTurnDone = false
	return nil
}
