package omp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The acceptance tests read emitted frontmatter, never a session: this oracle
// drives the installed binary so an undelivered rule cannot pass unnoticed.
const (
	ompRulesLiveEnvVar   = "AUTOPUS_OMP_RULES_LIVE"
	ompRulesLiveDeadline = 90 * time.Second
)

// ompRuleRoute is the mechanism that carries a rule into a session. The domain
// list is printed into the system prompt, TTSR registrations are visible only to
// `omp ttsr list` (they inject on a tool call), always-applied rules as a body.
type ompRuleRoute string

const (
	ompRouteDomainList  ompRuleRoute = "domain-rules"
	ompRouteTTSR        ompRuleRoute = "ttsr"
	ompRouteAlwaysApply ompRuleRoute = "always-applied"
)

// ompRulesLiveIgnorePatterns mirrors the rule-surface entries `auto init` writes;
// internal/cli/init.go owns the shipped list. Measured against omp 17.3.5: the
// filename glob `.omp/rules/autopus-*.md` drops every matching rule from
// discovery (domain list 9 -> 0, TTSR 3 -> 0) while the directory pattern
// `.omp/rules/` suppresses nothing, and dropping the pattern fails doctor's
// unignored-runtime-file hygiene, so the ignore file is under test here too.
var ompRulesLiveIgnorePatterns = []string{".omp/rules/", ".omp/agents/", ".omp/config.yml"}

type ompRuleExpectation struct {
	route ompRuleRoute
	// fingerprint is the longest body line: measured, a body reaches the prompt
	// only when the rule is always applied.
	fingerprint string
}

func TestOMPRules_LiveSessionReceivesEverySourceRule(t *testing.T) {
	if os.Getenv(ompRulesLiveEnvVar) != "1" {
		t.Skipf("set %s=1 to probe rule delivery with the installed omp binary", ompRulesLiveEnvVar)
	}
	executable, err := exec.LookPath(cliBinary)
	if err != nil {
		t.Skip("installed omp binary is unavailable")
	}
	if _, err = exec.LookPath("git"); err != nil {
		t.Skip("git is required so the shipped .gitignore binds during the probe")
	}

	workspace := generateOMPOnly(t)
	prepareOMPRulesLiveGitignore(t, workspace)
	profile := t.TempDir()
	overlay := filepath.Join(profile, "rules-live-config.yml")
	require.NoError(t, os.WriteFile(overlay, []byte("skills: {}\n"), 0o600))
	writeOMPLiveModelConfig(t, profile, ompClosedProxy)
	env, err := isolatedOMPLiveEnv(profile, overlay)
	require.NoError(t, err)
	expected, identifiers := expectedOMPRuleRoutes(t, workspace)
	require.Len(t, identifiers, 4, "content/rules must hold exactly 4 rule sources")
	prompt := dumpOMPLiveSystemPrompt(t, executable, workspace, overlay, env)
	listed, bodies := ompDomainRuleListing(t, prompt)
	registered := registeredOMPTTSRRules(t, executable, workspace, env)

	reached, problems := make([]string, 0, len(identifiers)), make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		rule := expected[identifier]
		routes := make([]ompRuleRoute, 0, 3)
		if listed[identifier] {
			routes = append(routes, ompRouteDomainList)
		}
		if registered[identifier] {
			routes = append(routes, ompRouteTTSR)
		}
		if strings.Contains(bodies, rule.fingerprint) {
			routes = append(routes, ompRouteAlwaysApply)
		}
		switch {
		case len(routes) == 0:
			problems = append(problems, fmt.Sprintf("%s: no route reached it, expected %s", identifier, rule.route))
			continue
		case len(routes) > 1:
			problems = append(problems, fmt.Sprintf("%s: registered on %v at once", identifier, routes))
		case routes[0] != rule.route:
			problems = append(problems, fmt.Sprintf("%s: expected %s, session used %s", identifier, rule.route, routes[0]))
		}
		reached = append(reached, identifier)
	}
	require.Empty(t, problems, "live rule delivery diverged:\n%s", strings.Join(problems, "\n"))
	assert.ElementsMatch(t, identifiers, reached,
		"the union of the three session routes must equal the generated rule set")
	t.Logf("live rule delivery: %d/%d rules reached the session", len(reached), len(identifiers))
}

// prepareOMPRulesLiveGitignore makes the workspace a repository and installs the
// shipped ignore patterns, then fails when the rule surface is glob-matched (the
// shape that silently kills discovery) or is not matched at all.
func prepareOMPRulesLiveGitignore(t *testing.T, workspace string) {
	t.Helper()
	// Measured against omp 17.3.5: rule discovery consults .gitignore only inside a
	// repository, so without `git init` the ignore file is inert here.
	out, err := exec.Command("git", "-C", workspace, "init", "-q").CombinedOutput()
	require.NoError(t, err, "git init: %s", out)
	path := filepath.Join(workspace, ".gitignore")
	require.NoFileExists(t, path, "Generate must not own .gitignore; `auto init` writes it")
	content := strings.Join(ompRulesLiveIgnorePatterns, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	covered := false
	for _, pattern := range ompRulesLiveIgnorePatterns {
		if !strings.Contains(pattern, ompRuleDir) {
			continue
		}
		require.NotContains(t, pattern, "*",
			"%q matches rule files by glob, which hides them from omp discovery", pattern)
		require.True(t, strings.HasSuffix(pattern, "/"),
			"%q must ignore the rule surface as a directory, not as files", pattern)
		covered = true
	}
	require.True(t, covered,
		"%q must stay ignored or doctor reports unignored runtime files", ompRuleDir+"/")
}

// expectedOMPRuleRoutes derives each generated rule's route from its emitted
// frontmatter, keyed by the identifier omp uses (file name without extension).
// Nothing is hardcoded per rule, so a policy change moves the expectation.
func expectedOMPRuleRoutes(t *testing.T, workspace string) (map[string]ompRuleExpectation, []string) {
	t.Helper()
	ruleDir := filepath.Join(workspace, filepath.FromSlash(ompRuleDir))
	sources := sourceRuleNames(t)
	expectations := make(map[string]ompRuleExpectation, len(sources))
	identifiers := make([]string, 0, len(sources))
	for _, source := range sources {
		fileName := ompRuleFilePrefix + source
		raw, err := os.ReadFile(filepath.Join(ruleDir, fileName))
		require.NoError(t, err, "rule %s must be emitted before the live probe", fileName)
		frontmatter, body := splitEmittedFrontmatter(t, string(raw))
		fields := make(map[string]any)
		require.NoError(t, yaml.Unmarshal([]byte(frontmatter), &fields), "rule %s frontmatter", fileName)
		route := ompRouteDomainList
		switch {
		case fields["condition"] != nil || fields["astCondition"] != nil || fields["scope"] != nil:
			route = ompRouteTTSR
		case fields["alwaysApply"] == true:
			route = ompRouteAlwaysApply
		}
		// The longest non-fenced line collides least with unrelated prompt text.
		fingerprint := ""
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "```") && len(trimmed) > len(fingerprint) {
				fingerprint = trimmed
			}
		}
		require.NotEmpty(t, fingerprint, "rule %s needs a body line to fingerprint", fileName)
		identifier := strings.TrimSuffix(fileName, ".md")
		expectations[identifier] = ompRuleExpectation{route: route, fingerprint: fingerprint}
		identifiers = append(identifiers, identifier)
	}
	sort.Strings(identifiers)
	return expectations, identifiers
}

// dumpOMPLiveSystemPrompt runs one isolated RPC session and returns the /dump
// text. /dump answers from local state, so no provider is ever dialled.
func dumpOMPLiveSystemPrompt(t *testing.T, executable, workspace, overlay string, env []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), ompRulesLiveDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--mode", "rpc", "--no-session",
		"--cwd", workspace, "--model", "s7dummy/"+ompLiveModel, "--config", overlay,
		"--no-lsp", "--no-pty", "--max-time", "60s")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	hardenOMPRulesLiveCommand(t, cmd, workspace, env)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() {
		_ = stdin.Close()
		_ = terminateOMPRPCProcessGroup(cmd)
		_ = cmd.Wait()
	}()
	stream, done := scanOMPRPCFrames(stdout)
	encoder := json.NewEncoder(stdin)
	ready, requested := false, false
	for {
		frame, readErr := nextOMPRPCFrame(ctx, stream, done)
		if readErr != nil {
			t.Fatalf("live /dump probe failed: %v (stderr=%s)", readErr, printableOMPFrame(stderr.Bytes()))
		}
		ready = ready || rpcFrameType(frame) == "ready"
		if ready && !requested {
			// Retry and compaction would both perturb the transcript /dump reads.
			require.NoError(t, encoder.Encode(map[string]any{"type": "set_auto_retry", "enabled": false}))
			require.NoError(t, encoder.Encode(map[string]any{"type": "set_auto_compaction", "enabled": false}))
			require.NoError(t, encoder.Encode(map[string]any{
				"id": "rules-live-dump", "type": "prompt", "message": "/dump",
			}))
			requested = true
		}
		if rpcFrameType(frame) != "command_output" {
			continue
		}
		var payload struct {
			Output string `json:"output"`
			Text   string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(frame, &payload))
		if text := payload.Output + payload.Text; text != "" {
			return text
		}
	}
}

// ompDomainRuleListing parses the <domain-rules> block, whose entries render as
// "- name (globs): description", and returns the listed identifiers plus the
// prompt with that block cut out: a synthesized description repeats the rule
// title, so a body fingerprint may only be searched outside the listing.
func ompDomainRuleListing(t *testing.T, prompt string) (map[string]bool, string) {
	t.Helper()
	start := strings.Index(prompt, "<domain-rules>")
	end := strings.Index(prompt, "</domain-rules>")
	require.True(t, start >= 0 && end > start,
		"the system prompt must carry a <domain-rules> block; none was rendered")
	listed := make(map[string]bool, 16)
	for _, line := range strings.Split(prompt[start:end], "\n") {
		entry := strings.TrimPrefix(strings.TrimSpace(line), "- ")
		identifier := strings.TrimSpace(strings.SplitN(strings.SplitN(entry, ":", 2)[0], "(", 2)[0])
		if strings.HasPrefix(identifier, ompRuleFilePrefix) {
			listed[identifier] = true
		}
	}
	return listed, prompt[:start] + prompt[end:]
}

// registeredOMPTTSRRules asks the binary which trigger-carrying rules it loaded.
func registeredOMPTTSRRules(t *testing.T, executable, workspace string, env []string) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), ompRulesLiveDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "ttsr", "list", "--json")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	hardenOMPRulesLiveCommand(t, cmd, workspace, env)
	stdout, err := cmd.Output()
	if err != nil {
		_ = terminateOMPRPCProcessGroup(cmd)
		require.NoError(t, err, "ttsr inspection failed: %s", printableOMPFrame(stderr.Bytes()))
	}
	var entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	require.NoError(t, json.Unmarshal(stdout, &entries), "ttsr list must emit JSON")
	registered := make(map[string]bool, 4)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name, ompRuleFilePrefix) {
			continue // built-in rule shipped by omp itself
		}
		require.True(t, strings.HasSuffix(filepath.ToSlash(entry.Path), ompRuleDir+"/"+entry.Name+".md"),
			"rule %s must be loaded from the probed rule surface", entry.Name)
		registered[entry.Name] = true
	}
	return registered
}

// hardenOMPRulesLiveCommand pins a probe to the isolated profile, which already
// routes every proxy to a closed port. The OS sandbox is darwin-only extra
// hardening; the process group stops a hung child outliving the test.
func hardenOMPRulesLiveCommand(t *testing.T, cmd *exec.Cmd, workspace string, env []string) {
	t.Helper()
	cmd.Dir = workspace
	cmd.Env = env
	cmd.WaitDelay = ompRPCWaitDelay
	if err := configureOMPRPCNetworkSandbox(cmd, ompClosedProxy); err != nil {
		require.NotEqual(t, "darwin", runtime.GOOS, "darwin must support the rpc network sandbox")
	}
	if err := configureOMPRPCProcessGroup(cmd); err != nil {
		t.Logf("isolated process group unavailable: %v", err)
	}
}
