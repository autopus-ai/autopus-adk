package omp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompModelSelectorRPCWorkers bounds how many resolution OMP processes run at
// once. Every bundled agent is an independent process; the bound survives the
// cutover so a future profile with more selectors keeps the doctor's shared
// probe deadline.
const ompModelSelectorRPCWorkers = 4

var ompModelRPCStateRequest = []byte(`{"id":"autopus-model-state","type":"get_state"}` + "\n")

type ompModelSelectorRPCStateRunner interface {
	RunWithInput(context.Context, string, []byte, ...string) ([]byte, error)
}

// SafeOMPModelSelectorRPCArgs accepts only provider-free selector resolution
// sessions. The model argument is the same concrete provider/model[:thinking]
// selector written to task.agentModelOverrides, never a role alias: after the
// native cutover Autopus owns no OMP model role.
func SafeOMPModelSelectorRPCArgs(args []string) bool {
	if len(args) != 9 || args[0] != "--config" || !filepath.IsAbs(args[1]) ||
		strings.ContainsAny(args[1], "\x00\r\n") || args[2] != "--model" ||
		!safeOMPModelRPCSelector(args[3]) ||
		args[4] != "--mode" || args[5] != "rpc" || args[6] != "--no-tools" ||
		args[7] != "--no-skills" || args[8] != "--no-extensions" {
		return false
	}
	return true
}

// safeOMPModelRPCSelector accepts exactly `provider/model:thinking`, the form
// model-resolver.ts documents as an exact selector with a thinking suffix.
func safeOMPModelRPCSelector(value string) bool {
	selector, thinking, err := splitOMPProjectedSelector(value)
	return err == nil && validateOMPProjectedSelector(selector, thinking) == nil
}

// readOMPModelAgentSelectorsViaRPC resolves every bundled agent's configured
// selector in a live OMP session. It proves the selector OMP will apply for
// that agent is reachable and lands on the exact provider, model, and thinking
// level Autopus wrote; it does not claim the spawn-time override binding was
// exercised, which no CLI surface can observe.
func readOMPModelAgentSelectorsViaRPC(
	ctx context.Context,
	runner ompModelSelectorRPCStateRunner,
	configPath string,
	expected map[string]string,
) (map[string]string, error) {
	agents := make([]string, 0, len(expected))
	for agent := range expected {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		resolved = make(map[string]string, len(agents))
		slots    = make(chan struct{}, ompModelSelectorRPCWorkers)
	)
	for _, agent := range agents {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		slots <- struct{}{}
		go func(agent string) {
			defer wg.Done()
			defer func() { <-slots }()
			selector, err := readOMPModelAgentSelectorViaRPC(ctx, runner, configPath, agent, expected[agent])
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				return
			}
			resolved[agent] = selector
		}(agent)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return resolved, nil
}

func readOMPModelAgentSelectorViaRPC(
	ctx context.Context,
	runner ompModelSelectorRPCStateRunner,
	configPath, agent, want string,
) (string, error) {
	args := []string{
		"--config", configPath, "--model", want, "--mode", "rpc",
		"--no-tools", "--no-skills", "--no-extensions",
	}
	output, err := runner.RunWithInput(ctx, cliBinary, ompModelRPCStateRequest, args...)
	if err != nil {
		return "", fmt.Errorf("activation agent readback %s: %w", agent, err)
	}
	selector, err := parseOMPModelRPCState(output)
	if err != nil {
		return "", fmt.Errorf("activation agent readback %s: %w", agent, err)
	}
	if selector != want {
		return "", fmt.Errorf("activation agent readback mismatch: %s", agent)
	}
	return selector, nil
}

func parseOMPModelRPCState(output []byte) (string, error) {
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		if !bytes.Contains(line, []byte(`"command":"get_state"`)) ||
			!bytes.Contains(line, []byte(`"id":"autopus-model-state"`)) {
			continue
		}
		if rejectDuplicateOMPModelReceiptJSON(line) != nil {
			return "", fmt.Errorf("RPC state contains duplicate fields")
		}
		var frame struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Command string `json:"command"`
			Success bool   `json:"success"`
			Data    struct {
				Model struct {
					Provider string `json:"provider"`
					ID       string `json:"id"`
				} `json:"model"`
				Thinking string `json:"thinkingLevel"`
			} `json:"data"`
		}
		if json.Unmarshal(line, &frame) != nil || frame.ID != "autopus-model-state" ||
			frame.Type != "response" || frame.Command != "get_state" || !frame.Success ||
			!safeOMPModelToken(frame.Data.Model.Provider) || !safeOMPModelToken(frame.Data.Model.ID) ||
			!config.IsOMPNativeThinkingLevel(frame.Data.Thinking) {
			return "", fmt.Errorf("RPC state is invalid")
		}
		return frame.Data.Model.Provider + "/" + frame.Data.Model.ID + ":" + frame.Data.Thinking, nil
	}
	return "", fmt.Errorf("RPC state response is missing")
}
