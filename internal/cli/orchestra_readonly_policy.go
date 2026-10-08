package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// readOnlyPolicyOptions tunes the read-only projection for the execution environment.
type readOnlyPolicyOptions struct {
	// OutsideRepo marks that providers start from an isolated non-repository
	// directory; Codex exec refuses such a cwd without --skip-git-repo-check.
	OutsideRepo bool
	// Confined marks a band local-patch request (SPEC-SIGMABAND-002 REQ-03):
	// it admits only a claude without a Backend and adds --output-format
	// stream-json, --verbose, and --restricted to its projection.
	Confined bool
}

// errReadOnlyUnconfined is wrapped by every refusal of a confined projection:
// only a subprocess claude takes --restricted (Provider Contract item 3).
var errReadOnlyUnconfined = errors.New("only a subprocess claude can be confined")

// readOnlyOrchestraCommand reports whether a command must run its providers
// read-only: planning and brainstorming produce advisory text only, so no
// provider may touch the caller's worktree (issue #108).
func readOnlyOrchestraCommand(commandName string) bool {
	return commandName == "plan" || commandName == "brainstorm"
}

// applyCommandReadOnlyPolicy projects provider argv for read-only commands and
// returns other commands' providers untouched.
func applyCommandReadOnlyPolicy(commandName string, providers []orchestra.ProviderConfig, opts readOnlyPolicyOptions) ([]orchestra.ProviderConfig, error) {
	if !readOnlyOrchestraCommand(commandName) {
		return providers, nil
	}
	return applyReadOnlyProviderPolicy(providers, opts)
}

// readOnlyNativeBinaries maps each provider with a read-only projection to the
// only binary whose flags the projection understands.
var readOnlyNativeBinaries = map[string]string{"claude": "claude", "codex": "codex", "gemini": "agy"}

// checkReadOnlyProvider runs every check of the read-only policy without
// projecting, so callers can reject a provider before any configured binary
// executes. OMP-backed providers pass, as in the shared projection.
func checkReadOnlyProvider(provider orchestra.ProviderConfig) *readOnlyPolicyViolation {
	if provider.Backend == config.ProviderBackendOMP {
		return nil
	}
	return validateReadOnlyProvider(provider)
}

// checkConfinedProvider refuses what a confined projection cannot confine,
// before every other check: the shared projection passes OMP providers as-is.
func checkConfinedProvider(provider orchestra.ProviderConfig) error {
	if provider.Backend != "" {
		return fmt.Errorf("read-only provider policy: provider %q on backend %q: %w", provider.Name, provider.Backend, errReadOnlyUnconfined)
	}
	if provider.Name != "claude" {
		return fmt.Errorf("read-only provider policy: provider %q: %w", provider.Name, errReadOnlyUnconfined)
	}
	return nil
}

// applyReadOnlyProviderPolicy returns provider configs whose native argv
// enforce read-only execution and whose SandboxMode records that evidence.
// OMP-backed providers are accepted as-is: that backend only exposes the
// read/grep/glob tool allowlist and fails closed at session start otherwise.
// Caller-owned configs and slices are not mutated. A confined refusal wraps
// errReadOnlyUnconfined; other rejections are *readOnlyPolicyViolation errors.
func applyReadOnlyProviderPolicy(providers []orchestra.ProviderConfig, opts readOnlyPolicyOptions) ([]orchestra.ProviderConfig, error) {
	projected := make([]orchestra.ProviderConfig, len(providers))
	for index, provider := range providers {
		if opts.Confined {
			if err := checkConfinedProvider(provider); err != nil {
				return nil, err
			}
		}
		if violation := checkReadOnlyProvider(provider); violation != nil {
			return nil, violation
		}
		if provider.Backend == config.ProviderBackendOMP {
			provider.SandboxMode = orchestra.SandboxModeReadOnly
			projected[index] = provider
			continue
		}

		provider.Args = append([]string(nil), provider.Args...)
		provider.ResultReadyPatterns = append([]string(nil), provider.ResultReadyPatterns...)
		provider.FastFailPatterns = append([]orchestra.FastFailRule(nil), provider.FastFailPatterns...)

		// checkReadOnlyProvider admitted only names in readOnlyNativeBinaries,
		// and each of them has a case here.
		switch provider.Name {
		case "claude":
			provider.Args = projectClaudeReadOnlyArgs(provider.Args, opts.Confined)
		case "codex":
			provider.Args = projectCodexReadOnlyArgs(provider.Args, opts.OutsideRepo)
		case "gemini":
			provider.Args = projectGeminiReadOnlyArgs(provider.Args)
		}
		provider.SandboxMode = orchestra.SandboxModeReadOnly
		if provider.Name == "gemini" {
			// RFP-3: agy blocks writes by headless auto-deny, not by plan mode,
			// and agy settings can widen it, so the receipt stays unverified.
			provider.SandboxMode = orchestra.SandboxModeUnverified
		}
		projected[index] = provider
	}
	return projected, nil
}

func validateReadOnlyProvider(provider orchestra.ProviderConfig) *readOnlyPolicyViolation {
	binary, known := readOnlyNativeBinaries[provider.Name]
	if !known {
		return &readOnlyPolicyViolation{Provider: provider.Name, Item: provider.Name, Kind: readOnlyUnsupportedProvider}
	}
	if provider.Binary != binary {
		return &readOnlyPolicyViolation{
			Provider: provider.Name, Field: readOnlyFieldBinary, Item: binary, Value: provider.Binary, Kind: readOnlyNativeBinary,
		}
	}
	if violation := validateReadOnlyProviderArgv(provider.Name, readOnlyFieldArgs, provider.Args); violation != nil {
		return violation
	}
	if !readOnlySchemaFlagAllowed(provider.Name, provider.SchemaFlag) {
		return &readOnlyPolicyViolation{
			Provider: provider.Name, Field: readOnlyFieldSchemaFlag, Item: provider.SchemaFlag, Kind: readOnlyUnsupportedSchemaFlag,
		}
	}
	return nil
}

// readOnlySchemaFlagAllowed admits only schema flags that cannot widen the
// provider: the subprocess backend appends the flag and a schema path after
// the projected argv, so any other flag would bypass the projection.
func readOnlySchemaFlagAllowed(provider, schemaFlag string) bool {
	flag := strings.TrimSpace(schemaFlag)
	return flag == "" || (provider == "codex" && flag == "--output-schema")
}

func validateReadOnlyProviderArgv(provider, field string, args []string) *readOnlyPolicyViolation {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if dangerousProviderArg(arg) {
			return &readOnlyPolicyViolation{Provider: provider, Field: field, Item: arg, Kind: readOnlyUnsafeArgv}
		}
		if readOnlyProviderBoolArg(provider, arg) {
			continue
		}
		flag, value, inline := strings.Cut(arg, "=")
		if !readOnlyProviderValueArg(provider, flag) {
			return &readOnlyPolicyViolation{Provider: provider, Field: field, Item: arg, Kind: readOnlyUnsupportedArgv}
		}
		if !inline {
			if index+1 >= len(args) {
				return &readOnlyPolicyViolation{Provider: provider, Field: field, Item: arg, Kind: readOnlyIncompleteArgv}
			}
			index++
			value = args[index]
		}
		if !validReadOnlyProviderArgValue(provider, flag, value, !inline) {
			return &readOnlyPolicyViolation{
				Provider: provider, Field: field, Item: flag, Value: value, Inline: inline, Kind: readOnlyUnsafeValue,
			}
		}
	}
	return nil
}

func readOnlyProviderBoolArg(provider, arg string) bool {
	switch provider {
	case "claude":
		switch arg {
		case "--print", "-p", "--no-session-persistence", "--disable-slash-commands", "--safe-mode", "--strict-mcp-config":
			return true
		}
	case "codex":
		switch arg {
		case "exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check":
			return true
		}
	case "gemini":
		return arg == "--sandbox" || arg == "--disable-slash-commands"
	}
	return false
}

func readOnlyProviderValueArg(provider, flag string) bool {
	switch provider {
	case "claude":
		switch flag {
		case "--model", "--effort", "--output-format", "--max-budget-usd", "--permission-mode", "--tools":
			return true
		}
	case "codex":
		switch flag {
		case "--sandbox", "-s", "--model", "-m", "--color", "--config", "-c":
			return true
		}
	case "gemini":
		switch flag {
		case "--print", "-p", "--model", "--effort", "--mode", "--output-format", "--json-schema", "--print-timeout":
			return true
		}
	}
	return false
}

func validReadOnlyProviderArgValue(provider, flag, value string, separated bool) bool {
	if strings.ContainsAny(value, "\x00\r\n") || dangerousProviderArg(value) {
		return false
	}
	// A separated value that looks like a flag is ambiguous: the projection
	// keeps it as a flag while the provider CLI may consume it as the value,
	// so an unvalidated flag would pass or a policy flag would be swallowed.
	if separated && strings.HasPrefix(value, "-") {
		return false
	}
	if provider == "codex" && (flag == "-c" || flag == "--config") {
		return strings.HasPrefix(value, "model_reasoning_effort=")
	}
	if provider == "claude" && flag == "--tools" {
		return value == claudeReadOnlyTools
	}
	return true
}

// claudeReadOnlyTools is the only built-in tool set a read-only claude keeps.
const claudeReadOnlyTools = "Read,Grep,Glob"

func projectClaudeReadOnlyArgs(args []string, confined bool) []string {
	args = upsertArgValue(removeArgFlag(args, "--tools"), "--permission-mode", "plan")
	for _, flag := range []string{"--safe-mode", "--no-session-persistence", "--disable-slash-commands", "--strict-mcp-config"} {
		args = ensureBoolArg(args, flag)
	}
	if confined {
		// --restricted keeps file tools inside the working directory, and
		// stream-json, which --print emits only with --verbose, names the
		// model of every turn. Neither flag is in the argv allowlist, so only
		// the projection adds them; stream-json replaces a configured format.
		args = append(removeArgFlag(args, "--output-format"), "--output-format", "stream-json", "--verbose", "--restricted")
	}
	// The inline form goes last: separated --tools is variadic and swallows a
	// following positional prompt (RFP-1).
	return append(args, "--tools="+claudeReadOnlyTools)
}

func projectCodexReadOnlyArgs(args []string, outsideRepo bool) []string {
	// Drop the short sandbox alias first: codex exits 2 on duplicate sandbox
	// flags, so exactly one "--sandbox read-only" item may remain.
	args = upsertArgValue(removeArgFlag(args, "-s"), "--sandbox", "read-only")
	for _, flag := range []string{"--ephemeral", "--ignore-user-config", "--ignore-rules"} {
		args = ensureBoolArg(args, flag)
	}
	if outsideRepo {
		args = ensureBoolArg(args, "--skip-git-repo-check")
	}
	return args
}

func projectGeminiReadOnlyArgs(args []string) []string {
	args = upsertArgValue(args, "--mode", "plan")
	args = ensureBoolArg(args, "--sandbox")
	return ensureBoolArg(args, "--disable-slash-commands")
}

// removeArgFlag drops every occurrence of a value flag in separated or inline
// form. A separated flag owns the next item, as in the validator's pairing.
func removeArgFlag(args []string, flag string) []string {
	result := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == flag:
			index++
		case strings.HasPrefix(args[index], flag+"="):
			// The inline form carries its own value; dropping the item is enough.
		default:
			result = append(result, args[index])
		}
	}
	return result
}

func dangerousProviderArg(arg string) bool {
	arg = strings.ToLower(strings.TrimSpace(arg))
	if strings.Contains(arg, "danger") || strings.Contains(arg, "bypass") || strings.Contains(arg, "yolo") {
		return true
	}
	switch arg {
	case "-y", "--full-auto", "--no-sandbox", "--sandbox=false", "--sandbox=0":
		return true
	default:
		return false
	}
}
