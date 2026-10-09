package cli

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Local Patch Provider Contract (SPEC-SIGMABAND-002 REQ-03, REQ-15). While
// health_band.allow_local_patch is true it replaces the selection and the
// working directory of 001's Provider Read-Only Contract for every
// diagnosis and every patch request: the provider is always a subprocess
// claude confined by --restricted to a band worktree, and anything else is
// unavailable(provider_unconfined). Orchestra commands keep their backend;
// only band reads local_patch_provider, and band never runs an OMP entry.

// bandClaudeModelIDPattern is the model rule's form of a full claude ID.
var bandClaudeModelIDPattern = regexp.MustCompile(`^claude-[a-z0-9][a-z0-9.-]*$`)

// bandConfinedKeepEnv is the allowlist of a confined request's environment
// (Provider Contract item 8): the process, locale, terminal, and XDG
// variables, the claude configuration directory and its headless OAuth
// token, ANTHROPIC_API_KEY for an API-key-only deployment, the proxy
// variables in both letter cases, and the TLS trust variables. Every other
// inherited variable is dropped, so the markers, socket, and token of an
// agent session that runs band (CLAUDECODE, CLAUDE_CODE_*, ORCA_*), an IDE
// or MCP bridge, NODE_OPTIONS, an SSH agent, and every other ANTHROPIC_*
// variable, such as a base URL that would send the key elsewhere, never
// reach the confined claude.
var bandConfinedKeepEnv = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "LC_*", "TERM", "TZ", "XDG_*",
	"CLAUDE_CONFIG_DIR", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
}

// bandConfinedProvider selects, projects, and runs the confined provider.
type bandConfinedProvider struct {
	harness *config.HarnessConfig
	// Seams: the shared read-only projection and the single-provider runner.
	project func([]orchestra.ProviderConfig, readOnlyPolicyOptions) ([]orchestra.ProviderConfig, error)
	run     func(context.Context, orchestra.OrchestraConfig, orchestra.ProviderConfig, string) (*orchestra.ProviderResponse, error)
	timeout time.Duration
}

func newBandConfinedProvider(harness *config.HarnessConfig) bandConfinedProvider {
	return bandConfinedProvider{
		harness: harness, project: applyReadOnlyProviderPolicy, run: orchestra.RunSingleProvider, timeout: healthband.ProviderTimeout,
	}
}

// resolve applies Provider Contract items 1–3 to the configuration, once per
// request: a projection is never projected again, because the confined
// flags it adds are outside the argv allowlist. It returns the projected
// provider or the unavailable reason: provider_unconfined for every
// selection that does not end at a confined subprocess claude, and
// provider_policy_rejected for an argv item outside the claude allowlist.
func (c bandConfinedProvider) resolve() (orchestra.ProviderConfig, string) {
	entry, ok := c.subprocessEntry()
	if !ok {
		return orchestra.ProviderConfig{}, bandProviderUnconfined
	}
	projected, err := c.project([]orchestra.ProviderConfig{providerConfigFromEntry("claude", entry)}, readOnlyPolicyOptions{Confined: true})
	switch {
	case errors.Is(err, errReadOnlyUnconfined):
		return orchestra.ProviderConfig{}, bandProviderUnconfined
	case err != nil:
		return orchestra.ProviderConfig{}, bandProviderPolicyRejected
	case len(projected) != 1 || !bandConfinedControls(projected[0]):
		return orchestra.ProviderConfig{}, bandProviderUnconfined
	}
	return projected[0], ""
}

// subprocessEntry is items 1–2: health_band.local_patch_provider, trimmed,
// when set (only claude can be confined, so another name is final and
// unconfined), else 001's selection when it names a claude entry without a
// backend. Rule 1 takes a backend-less claude entry as is, else the shipped
// default under the model rule.
func (c bandConfinedProvider) subprocessEntry() (config.ProviderEntry, bool) {
	if c.harness == nil {
		return config.ProviderEntry{}, false
	}
	entry, configured := c.harness.Orchestra.Providers["claude"]
	if name := strings.TrimSpace(c.harness.HealthBand.LocalPatchProvider); name != "" {
		if name != "claude" {
			return config.ProviderEntry{}, false
		}
		if configured && entry.Backend == "" {
			return entry, true
		}
		return bandDefaultClaudeEntry(entry, configured), true
	}
	if selectBandProvider(c.harness) != "claude" || !configured || entry.Backend != "" {
		return config.ProviderEntry{}, false
	}
	return entry, true
}

// bandDefaultClaudeEntry is config.DefaultClaudeProviderEntry under the
// model rule: a backend entry's anthropic/claude-* model selector gives the
// --model value without its thinking suffix; every other case keeps the
// default model. The OMP entry's binary and tools are never read.
func bandDefaultClaudeEntry(routed config.ProviderEntry, configured bool) config.ProviderEntry {
	entry := config.DefaultClaudeProviderEntry()
	if !configured || routed.Backend == "" {
		return entry
	}
	model, _, ok := orchestra.SplitModelSelector(routed.Model)
	provider, id, _ := strings.Cut(model, "/")
	if !ok || provider != "anthropic" || !bandClaudeModelIDPattern.MatchString(id) {
		return entry
	}
	args := slices.Clone(entry.Args)
	if at := slices.Index(args, "--model"); at >= 0 && at+1 < len(args) {
		args[at+1] = id
	}
	entry.Args = args
	return entry
}

// bandConfinedControls checks fail-closed that a projection is a confined
// subprocess claude: 001's read-only controls plus --restricted, --verbose,
// and --output-format stream-json.
func bandConfinedControls(provider orchestra.ProviderConfig) bool {
	return provider.Backend == "" && provider.Name == "claude" && bandReadOnlyControls(provider) &&
		slices.Contains(provider.Args, "--restricted") && slices.Contains(provider.Args, "--verbose") &&
		slices.Equal(bandFlagValues(provider.Args, "--output-format"), []string{"stream-json"})
}

// bandRequestedModel is the last --model value of an argv, "" without one.
func bandRequestedModel(args []string) string {
	values := bandFlagValues(args, "--model")
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// request runs one confined request with workDir, a band worktree, as the
// only working directory, with only the variables of bandConfinedKeepEnv
// (001's bandProviderUnsetEnv and GIT_* stay on the unset list as a second
// guard), and with no backend route registered, the one path that honors
// KeepEnv. The stream is bounded at bandProviderStreamBytes while the
// provider runs. The fast-fail substring rules are off: stream-json carries
// tool results, which are repository text. It returns the parsed stream and
// 001's REQ-12 reason of a failed run.
func (c bandConfinedProvider) request(ctx context.Context, provider orchestra.ProviderConfig, workDir, prompt, kind string) (bandConfinedReply, string) {
	provider.WorkDir, provider.ExecutionTimeout = workDir, c.timeout
	provider.KeepEnv = bandConfinedKeepEnv
	provider.UnsetEnv = append(slices.Clone(bandProviderUnsetEnv), "GIT_*")
	provider.MaxOutputBytes = bandProviderStreamBytes + 1
	provider.FastFailPatterns = []orchestra.FastFailRule{}
	cfg := orchestra.OrchestraConfig{
		Providers: []orchestra.ProviderConfig{provider}, TimeoutSeconds: int((c.timeout + time.Second - 1) / time.Second),
		WorkingDir: workDir, ProviderWorkDir: workDir, ReadOnly: true,
	}
	response, err := c.run(ctx, cfg, provider, prompt)
	var reply bandConfinedReply
	if response != nil && strings.TrimSpace(response.Output) != "" {
		reply = parseBandStream(response.Output, kind, bandRequestedModel(provider.Args))
	}
	if reason := bandRunFailure(response, err); reason != "" {
		return reply, reason
	}
	if !reply.streamed {
		return reply, bandProviderEmptyOutput
	}
	return reply, ""
}
