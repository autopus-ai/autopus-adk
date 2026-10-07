package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

const pipelineOMPEmptyAssistantOutput = "OMP pipeline RPC returned empty assistant output"

type ompReviewBackend struct {
	projectDir string
}

var _ orchestra.ExecutionBackend = (*ompReviewBackend)(nil)
var _ orchestra.FreshExecutionBackend = (*ompReviewBackend)(nil)

func newOMPReviewBackend(projectDir string) orchestra.ExecutionBackend {
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}
	return &ompReviewBackend{projectDir: projectDir}
}

func (backend *ompReviewBackend) Name() string { return config.ProviderBackendOMP }

func (*ompReviewBackend) FreshExecutionPerRequest() bool { return true }

func (backend *ompReviewBackend) Execute(ctx context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	selector := req.Config.Model
	model, thinking, ok := orchestra.SplitModelSelector(selector)
	if !ok {
		return nil, fmt.Errorf("OMP review provider %q requires a valid model selector", req.Provider)
	}
	tools, err := normalizeOMPReviewTools(req.Config.Tools)
	if err != nil {
		return nil, fmt.Errorf("OMP review provider %q: %w", req.Provider, err)
	}
	timeout := ompReviewTimeout(req.Timeout)
	maxTime := strconv.Itoa(ompReviewTimeoutSeconds(timeout))
	toolsCSV := strings.Join(tools, ",")
	fmt.Fprintf(os.Stderr, "OMP 리뷰 세션: provider=%s model=%s tools=%s\n", req.Provider, selector, toolsCSV)

	started := time.Now()
	response := &orchestra.ProviderResponse{
		Provider: req.Provider, ExecutedBackend: config.ProviderBackendOMP,
		Role: req.Role, ModelFamily: orchestra.ModelFamilyForSelector(selector),
	}
	executeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	session := ompReviewSession{
		projectDir: backend.projectDir, timeout: timeout, model: model, thinking: thinking,
		prompt: req.Prompt, tools: tools, toolsCSV: toolsCSV, maxTime: maxTime, unsetEnv: req.Config.UnsetEnv,
	}
	var output string
	var executionErr, cleanupErr error
	for attempt := 1; ; attempt++ {
		output, executionErr, cleanupErr = session.run(executeCtx)
		if !ompReviewShouldRetry(executeCtx, executionErr, cleanupErr, attempt) {
			break
		}
		fmt.Fprintf(os.Stderr, "OMP 리뷰 세션 재시도: provider=%s attempt=%d/%d reason=%v\n",
			req.Provider, attempt+1, ompReviewMaxAttempts, executionErr)
		if err := ompReviewBackoff(executeCtx, attempt); err != nil {
			break
		}
	}
	if executeCtx.Err() != nil {
		return finishOMPReviewFailure(
			executeCtx, response, started, errors.Join(executionErr, cleanupErr, executeCtx.Err()),
		)
	}
	if executionErr != nil && executionErr.Error() == pipelineOMPEmptyAssistantOutput && cleanupErr == nil {
		response.Duration = time.Since(started)
		response.EmptyOutput = true
		return response, nil
	}
	if err := errors.Join(executionErr, cleanupErr); err != nil {
		return finishOMPReviewFailure(executeCtx, response, started, err)
	}
	response.Output = output
	response.EmptyOutput = strings.TrimSpace(output) == ""
	response.Duration = time.Since(started)
	return response, nil
}

func executeOMPReviewRPC(
	ctx context.Context,
	process *pipelineOMPProcess,
	model string,
	thinking string,
	prompt string,
	allowedTools []string,
) (string, error) {
	protocol := newPipelineOMPRPCProtocol(process)
	if err := protocol.initialize(ctx); err != nil {
		return "", err
	}
	// argv restricts built-in tools only; MCP, custom, and extension tools
	// arrive through discovery. The session's own tool dump is the proof that
	// nothing outside the allowlist is reachable, so verify it before any
	// prompt is sent.
	if err := verifyOMPReviewToolSet(ctx, protocol, allowedTools); err != nil {
		return "", err
	}
	provider, modelID, ok := strings.Cut(model, "/")
	if !ok || !safePipelineOMPToken(provider) || !safePipelineOMPToken(modelID) {
		return "", errors.New("invalid OMP review model selector")
	}
	if thinking != "" && !safePipelineOMPToken(thinking) {
		return "", errors.New("invalid OMP review thinking level")
	}
	if _, err := protocol.call(ctx, pipelineOMPRPCCommand{
		Type: "set_model", Provider: provider, ModelID: modelID,
	}, false); err != nil {
		return "", err
	}
	if thinking != "" {
		if err := protocol.setThinkingLevel(ctx, thinking); err != nil {
			return "", err
		}
	}
	output, err := protocol.execute(ctx, "", prompt)
	// The pinned model is a contract: a runtime that answered with another
	// provider/model (fallback, alias drift) must not be reported as this one,
	// and a drifted turn's later failure (empty text, RPC error) must not
	// mask the drift. The identity is settled at agent_end, so check it first.
	if identity := protocol.lastTurn; identity.Provider != "" || identity.Model != "" {
		if identity.Provider != provider || identity.Model != modelID {
			return "", fmt.Errorf("OMP review session executed model mismatch: want %s/%s got %s/%s",
				provider, modelID, identity.Provider, identity.Model)
		}
	}
	if err != nil {
		return "", err
	}
	return output, nil
}

func finishOMPReviewFailure(
	ctx context.Context,
	response *orchestra.ProviderResponse,
	started time.Time,
	err error,
) (*orchestra.ProviderResponse, error) {
	response.Duration = time.Since(started)
	response.Error = err.Error()
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		response.TimedOut = true
		return response, nil
	}
	response.ExitCode = 1
	return response, err
}

func normalizeOMPReviewTools(input []string) ([]string, error) {
	if len(input) == 0 {
		input = config.OMPReviewToolAllowlist
	}
	tools := append([]string(nil), input...)
	for _, tool := range tools {
		if tool != "read" && tool != "grep" && tool != "glob" {
			return nil, fmt.Errorf("tool %q is not read-only", tool)
		}
	}
	slices.Sort(tools)
	return slices.Compact(tools), nil
}

func selectRoutedBackend(cfg orchestra.OrchestraConfig) orchestra.ExecutionBackend {
	base := orchestra.SelectBackend(cfg)
	if !orchestraConfigUsesOMP(cfg) {
		return base
	}
	return orchestra.NewRoutedBackend(base, map[string]orchestra.ExecutionBackend{
		config.ProviderBackendOMP: newOMPReviewBackend(cfg.WorkingDir),
	})
}

func orchestraConfigUsesOMP(cfg orchestra.OrchestraConfig) bool {
	for _, provider := range cfg.Providers {
		if provider.Backend == config.ProviderBackendOMP {
			return true
		}
	}
	return cfg.JudgeConfig != nil && cfg.JudgeConfig.Backend == config.ProviderBackendOMP
}

// verifyOMPReviewToolSet fails closed when the live session exposes any tool
// outside the allowlist.
func verifyOMPReviewToolSet(ctx context.Context, protocol *pipelineOMPRPCProtocol, allowed []string) error {
	data, err := protocol.call(ctx, pipelineOMPRPCCommand{Type: "get_state"}, false)
	if err != nil {
		return err
	}
	var state struct {
		DumpTools []struct {
			Name string `json:"name"`
		} `json:"dumpTools"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return errors.New("OMP review session tool dump is malformed")
	}
	if state.DumpTools == nil {
		return errors.New("OMP review session did not report its tool set")
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	var unexpected []string
	for _, tool := range state.DumpTools {
		if _, ok := allowedSet[tool.Name]; !ok {
			unexpected = append(unexpected, tool.Name)
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return fmt.Errorf("OMP review session exposed tools outside the allowlist: %s", strings.Join(unexpected, ","))
	}
	return nil
}
