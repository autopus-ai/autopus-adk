package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// providerReadinessProbeTimeout bounds every status probe (REQ-10).
const providerReadinessProbeTimeout = 5 * time.Second

const readinessReasonProbeFailed = "probe_failed"

// providerReadinessStatus is the Readiness Contract outcome of one provider.
type providerReadinessStatus string

const (
	providerReadinessReady    providerReadinessStatus = "ready"
	providerReadinessNotReady providerReadinessStatus = "not_ready"
	providerReadinessUnknown  providerReadinessStatus = "unknown"
	providerReadinessSkipped  providerReadinessStatus = "skipped"
)

// providerReadinessResult never carries raw probe output.
type providerReadinessResult struct {
	Provider string
	Status   providerReadinessStatus
	// Reason is the not_ready state or the unknown reason.
	Reason string
	// Remedy is the operator command for a not_ready provider.
	Remedy string
	// Warnings are advisory lines built from counts and names, never from probe text.
	Warnings []string
}

// Token renders ready, not_ready(<state>), unknown(<reason>), or skipped.
func (result providerReadinessResult) Token() string {
	if result.Reason == "" {
		return string(result.Status)
	}
	return string(result.Status) + "(" + result.Reason + ")"
}

// ompAgentDirRemedyNote tells the operator to log in under the review's OMP agent dir.
const ompAgentDirRemedyNote = " (same PI_CODING_AGENT_DIR as this review)"

// RunRemedy renders `run "<command>"`, keeping a remedy note outside the quotes.
func (result providerReadinessResult) RunRemedy() string {
	if result.Remedy == "" {
		return ""
	}
	command, note := result.Remedy, ""
	if trimmed, found := strings.CutSuffix(result.Remedy, ompAgentDirRemedyNote); found {
		command, note = trimmed, ompAgentDirRemedyNote
	}
	return `run "` + command + `"` + note
}

// PreflightLine renders the redacted stderr line `preflight: <provider> <status>`.
func (result providerReadinessResult) PreflightLine() string {
	line := "preflight: " + result.Provider + " " + result.Token()
	if remedy := result.RunRemedy(); remedy != "" {
		line += " - " + remedy
	}
	return redactReadinessText(line)
}

// providerReadinessOptions configures one preflight run.
type providerReadinessOptions struct {
	// Skip runs no probe and reports every provider as skipped (--skip-provider-readiness).
	Skip bool
	// Env is the probe environment; nil inherits the current process environment.
	Env []string
	// Dir is the probe working directory; empty inherits the current one.
	Dir string
}

func (options providerReadinessOptions) environment() []string {
	if options.Env == nil {
		return os.Environ()
	}
	return options.Env
}

// readinessProbeKind names one Readiness Contract status command. A CLI kind is
// also the bare executable name it runs; the OMP kind runs the canonical path.
type readinessProbeKind string

const (
	readinessProbeClaude readinessProbeKind = "claude"
	readinessProbeCodex  readinessProbeKind = "codex"
)

// readinessStatusArgs holds the only arguments a status probe may carry.
var readinessStatusArgs = map[readinessProbeKind][]string{
	readinessProbeClaude: {"auth", "status", "--json"},
	readinessProbeCodex:  {"login", "status"},
	readinessProbeOMP:    {"usage", "--json", "--redact"},
}

// readinessKindFor maps a provider onto its status command; false means none exists.
func readinessKindFor(provider orchestra.ProviderConfig) (readinessProbeKind, bool) {
	if provider.Backend == config.ProviderBackendOMP {
		return readinessProbeOMP, true
	}
	switch provider.Name {
	case "claude":
		return readinessProbeClaude, true
	case "codex":
		return readinessProbeCodex, true
	default:
		return "", false
	}
}

// probeProviderReadiness classifies every provider through status commands
// only; it makes no model call. Results follow the input order, and providers
// that share a status command share one probe execution.
func probeProviderReadiness(
	ctx context.Context,
	providers []orchestra.ProviderConfig,
	options providerReadinessOptions,
) []providerReadinessResult {
	results := make([]providerReadinessResult, len(providers))
	if options.Skip {
		for index, provider := range providers {
			results[index] = providerReadinessResult{Provider: provider.Name, Status: providerReadinessSkipped}
		}
		return results
	}
	env := options.environment()
	evidence := runReadinessProbes(ctx, providers, env, options.Dir)
	for index, provider := range providers {
		results[index] = classifyProviderReadiness(provider, evidence, env)
		results[index].Provider = provider.Name
	}
	return results
}

// runReadinessProbes runs each distinct status command once, concurrently.
func runReadinessProbes(
	ctx context.Context, providers []orchestra.ProviderConfig, env []string, dir string,
) map[readinessProbeKind]readinessEvidence {
	evidence := make(map[readinessProbeKind]readinessEvidence)
	var mu sync.Mutex
	var probes sync.WaitGroup
	started := make(map[readinessProbeKind]bool)
	for _, provider := range providers {
		kind, ok := readinessKindFor(provider)
		if !ok || started[kind] {
			continue
		}
		started[kind] = true
		probes.Add(1)
		go func() {
			defer probes.Done()
			result := probeReadinessKind(ctx, kind, env, dir)
			mu.Lock()
			evidence[kind] = result
			mu.Unlock()
		}()
	}
	probes.Wait()
	return evidence
}

func probeReadinessKind(ctx context.Context, kind readinessProbeKind, env []string, dir string) readinessEvidence {
	if kind == readinessProbeOMP {
		command, err := ompReadinessCommand(env, dir)
		if err != nil {
			return readinessEvidence{Failure: readinessReasonProbeFailed}
		}
		return collectReadinessEvidence(ctx, command)
	}
	argv := append([]string{string(kind)}, readinessStatusArgs[kind]...)
	return collectReadinessEvidence(ctx, providerReadinessCommand{Argv: argv, Env: env, Dir: dir})
}

func classifyProviderReadiness(
	provider orchestra.ProviderConfig, evidence map[readinessProbeKind]readinessEvidence, env []string,
) providerReadinessResult {
	kind, ok := readinessKindFor(provider)
	switch {
	case !ok:
		return unknownReadiness("no_status_command")
	case kind == readinessProbeOMP:
		return classifyOMPReadiness(evidence[kind], ompReadinessFamily(provider.Model), env)
	case kind == readinessProbeCodex:
		return classifyCodexReadiness(evidence[kind], env)
	default:
		return classifyClaudeReadiness(evidence[kind], env)
	}
}

// codexCredentialEnv lists variables that authenticate codex without a login.
var codexCredentialEnv = []string{"CODEX_API_KEY", "OPENAI_API_KEY"}

const codexLoggedOutMarker = "Not logged in"

func classifyCodexReadiness(evidence readinessEvidence, env []string) providerReadinessResult {
	if evidence.Failure != "" {
		return unknownReadiness(evidence.Failure)
	}
	loggedOut := bytes.Contains(evidence.Stdout.Data, []byte(codexLoggedOutMarker)) ||
		bytes.Contains(evidence.Stderr.Data, []byte(codexLoggedOutMarker))
	switch {
	case evidence.ExitCode == 0:
		return providerReadinessResult{Status: providerReadinessReady}
	case evidence.ExitCode != 1 || !loggedOut:
		return unknownReadiness(readinessReasonProbeFailed)
	case readinessEnvHasAny(env, codexCredentialEnv):
		return unknownReadiness("env_credentials")
	default:
		return notReadyReadiness("logged_out", "codex login")
	}
}

// claudeCredentialEnv lists variables that authenticate claude without a login.
var claudeCredentialEnv = []string{
	"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN",
	"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX",
}

func classifyClaudeReadiness(evidence readinessEvidence, env []string) providerReadinessResult {
	if evidence.Failure != "" {
		return unknownReadiness(evidence.Failure)
	}
	loggedIn, parsed := claudeLoggedIn(evidence.Stdout.Data)
	switch {
	case !parsed:
		return unknownReadiness("unparsable")
	case loggedIn:
		return providerReadinessResult{Status: providerReadinessReady}
	case readinessEnvHasAny(env, claudeCredentialEnv):
		return unknownReadiness("env_credentials")
	default:
		return notReadyReadiness("logged_out", "claude auth login")
	}
}

// claudeLoggedIn reads only the exact boolean field loggedIn.
func claudeLoggedIn(stdout []byte) (loggedIn bool, parsed bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stdout, &fields); err != nil {
		return false, false
	}
	switch string(bytes.TrimSpace(fields["loggedIn"])) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func unknownReadiness(reason string) providerReadinessResult {
	return providerReadinessResult{Status: providerReadinessUnknown, Reason: reason}
}

func notReadyReadiness(state, remedy string) providerReadinessResult {
	return providerReadinessResult{Status: providerReadinessNotReady, Reason: state, Remedy: remedy}
}

// readinessEnvHasAny reports whether any key holds a non-empty value; values never leave this function.
func readinessEnvHasAny(env []string, keys []string) bool {
	for _, key := range keys {
		if readinessEnvValue(env, key) != "" {
			return true
		}
	}
	return false
}

// readinessEnvValue returns the last value of key, matching exec's duplicate handling.
func readinessEnvValue(env []string, key string) string {
	value := ""
	for _, entry := range env {
		if name, candidate, found := strings.Cut(entry, "="); found && name == key {
			value = candidate
		}
	}
	return value
}
