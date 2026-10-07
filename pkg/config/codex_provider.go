package config

import (
	"slices"
	"strings"
)

const (
	ProviderModelPolicyQuality = "quality"
	ProviderModelPolicyPinned  = "pinned"
)

var historicalCanonicalCodexArgs = []string{
	"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel,
	"-c", `model_reasoning_effort="xhigh"`,
}

var v05066AutoPinnedCodexArgs = []string{
	"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel,
}

// ApplyCodexProviderProfile changes only model policy arguments. Other provider
// flags remain in their original order.
func ApplyCodexProviderProfile(entry ProviderEntry, profile CodexProfile) ProviderEntry {
	entry.Args = applyCodexProfileArgs(entry.Args, profile)
	return entry
}

// applyCodexProfileArgs rewrites the model and effort arguments of a codex
// subprocess argv and keeps exactly one --json right after a leading exec.
func applyCodexProfileArgs(args []string, profile CodexProfile) []string {
	if len(args) == 0 {
		args = []string{"exec", "--sandbox", "workspace-write"}
	}
	managedArgs, suffix := splitCodexManagedArgs(args)

	next := make([]string, 0, len(args)+4)
	modelFound := false
	effortFound := false
	jsonFound := false
	for i := 0; i < len(managedArgs); i++ {
		switch managedArgs[i] {
		case "--json":
			if !jsonFound {
				next = append(next, managedArgs[i])
				jsonFound = true
			}
		case "-m", "--model":
			if i+1 < len(managedArgs) {
				if profile.Model != "" {
					next = append(next, managedArgs[i], profile.Model)
				}
				modelFound = true
				i++
			} else {
				next = append(next, managedArgs[i])
			}
		case "-c", "--config":
			if i+1 < len(managedArgs) && isCodexReasoningEffortAssignment(managedArgs[i+1]) {
				if profile.Effort != "" {
					next = append(next, managedArgs[i], codexReasoningEffortAssignment(profile.Effort))
				}
				effortFound = true
				i++
			} else {
				next = append(next, managedArgs[i])
			}
		default:
			if strings.HasPrefix(managedArgs[i], "--model=") {
				if profile.Model != "" {
					next = append(next, "--model="+profile.Model)
				}
				modelFound = true
			} else if isCodexReasoningEffortLongOption(managedArgs[i]) {
				if profile.Effort != "" {
					next = append(next, codexReasoningEffortLongOption(profile.Effort))
				}
				effortFound = true
			} else if isCodexReasoningEffortAssignment(managedArgs[i]) {
				if profile.Effort != "" {
					next = append(next, codexReasoningEffortAssignment(profile.Effort))
				}
				effortFound = true
			} else {
				next = append(next, managedArgs[i])
			}
		}
	}
	if profile.Model != "" && !modelFound {
		next = append(next, "-m", profile.Model)
	}
	if profile.Effort != "" && !effortFound {
		next = append(next, "-c", codexReasoningEffortAssignment(profile.Effort))
	}
	if !jsonFound {
		at := 0
		if len(next) > 0 && next[0] == "exec" {
			at = 1
		}
		next = append(next, "")
		copy(next[at+1:], next[at:])
		next[at] = "--json"
	}
	return append(next, suffix...)
}

// ResolveCodexProviderProfile applies catalog capability fallback to a managed provider.
func ResolveCodexProviderProfile(entry ProviderEntry, catalogJSON []byte) (ProviderEntry, CodexProfileResolution) {
	requested := codexProfileFromArgs(entry.Args)
	resolution := ResolveCodexProfile(requested, catalogJSON)
	if entry.ModelPolicy != ProviderModelPolicyQuality {
		resolution.Effective = requested
		resolution.Fallback = false
		resolution.Reason = CodexResolutionSupported
		return entry, resolution
	}
	return ApplyCodexProviderProfile(entry, resolution.Effective), resolution
}

func codexProfileFromArgs(args []string) CodexProfile {
	var profile CodexProfile
	managedArgs, _ := splitCodexManagedArgs(args)
	for i := 0; i < len(managedArgs); i++ {
		switch managedArgs[i] {
		case "-m", "--model":
			if i+1 < len(managedArgs) {
				profile.Model = managedArgs[i+1]
				i++
			}
		case "-c", "--config":
			if i+1 < len(managedArgs) && isCodexReasoningEffortAssignment(managedArgs[i+1]) {
				profile.Effort = codexReasoningEffortValue(managedArgs[i+1])
				i++
			}
		default:
			if strings.HasPrefix(managedArgs[i], "--model=") {
				profile.Model = strings.TrimPrefix(managedArgs[i], "--model=")
			} else if isCodexReasoningEffortLongOption(managedArgs[i]) {
				profile.Effort = codexReasoningEffortValue(strings.TrimPrefix(managedArgs[i], "--config="))
			} else if isCodexReasoningEffortAssignment(managedArgs[i]) {
				profile.Effort = codexReasoningEffortValue(managedArgs[i])
			}
		}
	}
	return profile
}

func splitCodexManagedArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

func isCodexReasoningEffortAssignment(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "model_reasoning_effort=")
}

func isCodexReasoningEffortLongOption(value string) bool {
	return strings.HasPrefix(value, "--config=") &&
		isCodexReasoningEffortAssignment(strings.TrimPrefix(value, "--config="))
}

func codexReasoningEffortAssignment(effort string) string {
	return `model_reasoning_effort="` + effort + `"`
}

func codexReasoningEffortLongOption(effort string) string {
	return "--config=" + codexReasoningEffortAssignment(effort)
}

func codexReasoningEffortValue(assignment string) string {
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(assignment), "model_reasoning_effort="))
	return strings.Trim(value, `"'`)
}

// The default-entry detectors below compare only the keys a load keeps. A
// shipped default also carried pane argv, but that key is retired and ignored
// on load, so an entry whose only difference was its pane argv is now the
// default entry it otherwise equals (SPEC-PANERM-001 S14).
func isHistoricalCanonicalCodexProvider(entry ProviderEntry) bool {
	return entry.Binary == "codex" &&
		slices.Equal(entry.Args, historicalCanonicalCodexArgs)
}

func isV05066AutoPinnedCodexProvider(entry ProviderEntry) bool {
	return entry.ModelPolicy == ProviderModelPolicyPinned &&
		entry.Binary == "codex" &&
		slices.Equal(entry.Args, v05066AutoPinnedCodexArgs) &&
		!entry.PromptViaArgs &&
		entry.Subprocess == (SubprocessProvConf{
			SchemaFlag: "--output-schema",
			Timeout:    CodexOrchestraTimeoutSeconds,
		})
}

func migrateCodexProviderModelPolicy(entry ProviderEntry, quality QualityConf) (ProviderEntry, bool) {
	switch entry.ModelPolicy {
	case ProviderModelPolicyPinned:
		if quality.SupervisorModelPolicy == "" && isV05066AutoPinnedCodexProvider(entry) {
			entry.ModelPolicy = ProviderModelPolicyQuality
			entry = ApplyCodexProviderProfile(entry, quality.CodexOrchestraProfile())
			return entry, true
		}
		return entry, false
	case ProviderModelPolicyQuality:
		updated := ApplyCodexProviderProfile(entry, quality.CodexOrchestraProfile())
		return updated, !providerEntryEqual(entry, updated)
	case "":
		if isHistoricalCanonicalCodexProvider(entry) {
			entry.ModelPolicy = ProviderModelPolicyQuality
			entry = ApplyCodexProviderProfile(entry, quality.CodexOrchestraProfile())
			return entry, true
		}
		entry.ModelPolicy = ProviderModelPolicyPinned
		return entry, true
	default:
		return entry, false
	}
}

func providerEntryEqual(left, right ProviderEntry) bool {
	return left.Binary == right.Binary &&
		left.ModelPolicy == right.ModelPolicy &&
		left.PromptViaArgs == right.PromptViaArgs &&
		slices.Equal(left.Args, right.Args) &&
		left.Subprocess == right.Subprocess
}

// shouldRestoreProviderDefaults reports whether a provider entry should be
// replaced by the shipped defaults. An entry that names an execution backend
// is explicit configuration even though it carries no CLI args, so it is
// never restored.
func shouldRestoreProviderDefaults(providerName string, entry ProviderEntry, exists bool) bool {
	if !exists {
		return true
	}
	if entry.Backend != "" || len(entry.Args) != 0 {
		return false
	}
	if providerName != "codex" {
		return true
	}
	switch entry.ModelPolicy {
	case ProviderModelPolicyQuality:
		return true
	case ProviderModelPolicyPinned:
		return false
	default:
		return isZeroLikeUnmarkedCodexProvider(entry)
	}
}

func classifyCustomUnmarkedEmptyCodexProvider(providerName string, entry ProviderEntry) (ProviderEntry, bool) {
	if providerName != "codex" || entry.Backend != "" || len(entry.Args) != 0 || entry.ModelPolicy != "" || isZeroLikeUnmarkedCodexProvider(entry) {
		return entry, false
	}
	entry.ModelPolicy = ProviderModelPolicyPinned
	return entry, true
}

func classifyStoredCustomUnmarkedEmptyCodexProvider(
	providers map[string]ProviderEntry,
	providerName string,
) (ProviderEntry, bool, bool) {
	entry, exists := providers[providerName]
	classified, changed := classifyCustomUnmarkedEmptyCodexProvider(providerName, entry)
	if changed {
		providers[providerName] = classified
	}
	return classified, exists, changed
}

func isZeroLikeUnmarkedCodexProvider(entry ProviderEntry) bool {
	return entry.ModelPolicy == "" &&
		len(entry.Args) == 0 &&
		(entry.Binary == "" || entry.Binary == "codex") &&
		!entry.PromptViaArgs &&
		entry.Subprocess == (SubprocessProvConf{})
}
