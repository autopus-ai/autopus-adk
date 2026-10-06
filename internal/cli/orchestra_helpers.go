package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/design"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// buildFileContents reads each file and returns the formatted contents. A
// single unreadable entry aborts the whole batch (GitHub issue #37): silently
// embedding a "읽기 실패" marker produced prompts that paired missing content
// with the topic-isolation instruction, so providers could neither read the
// file nor fall back to disk — every run ended in "리뷰 불가".
func buildFileContents(files []string) (string, error) {
	var sb strings.Builder
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("파일 읽기 실패: %s: %w", f, err)
		}
		fmt.Fprintf(&sb, "--- %s ---\n```\n%s\n```\n\n", f, string(content))
	}
	return sb.String(), nil
}

// buildReviewPrompt builds the review prompt, including file contents if provided.
func buildReviewPrompt(files []string) (string, error) {
	effectiveCfg, err := loadEffectiveHarnessConfigForFlags(globalFlags{})
	if err != nil {
		effectiveCfg = effectiveHarnessConfig{Config: config.DefaultFullConfig("."), ConfigDir: "."}
	}
	return buildReviewPromptWithEffectiveConfig(files, effectiveCfg)
}

func buildReviewPromptWithEffectiveConfig(files []string, effectiveCfg effectiveHarnessConfig) (string, error) {
	if len(files) == 0 {
		return "현재 프로젝트의 코드를 리뷰해주세요. 품질, 가독성, 잠재적 버그를 중심으로 분석하세요.", nil
	}
	contents, err := buildFileContents(files)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("다음 파일들을 코드 리뷰해주세요:\n\n")
	sb.WriteString(contents)
	if section := buildReviewDesignContextWithEffectiveConfig(files, effectiveCfg); section != "" {
		sb.WriteString("\n")
		sb.WriteString(section)
		sb.WriteString("\n")
	}
	sb.WriteString("품질, 가독성, 잠재적 버그를 중심으로 분석하세요.")
	return sb.String(), nil
}

// @AX:NOTE [AUTO]: Review design context is appended only for UI-related files and remains untrusted prompt evidence.
func buildReviewDesignContextWithEffectiveConfig(files []string, effectiveCfg effectiveHarnessConfig) string {
	cfg := effectiveCfg.Config
	if !design.AnyUIRelatedFile(files, cfg.Design.UIFileGlobs) {
		return "Design context: skipped (non-ui changes)\n"
	}
	if !cfg.Design.InjectOnReview {
		return "Design context: skipped (disabled)\n"
	}
	ctx, err := loadEffectiveDesignContext(effectiveCfg, design.Options{
		Enabled:         cfg.Design.Enabled,
		Paths:           cfg.Design.Paths,
		MaxContextLines: cfg.Design.MaxContextLines,
		UIFileGlobs:     cfg.Design.UIFileGlobs,
	})
	if err != nil {
		return fmt.Sprintf("Design context: skipped (%v)\n", err)
	}
	if !ctx.Found {
		return fmt.Sprintf("Design context: skipped (not configured)\n%s", ctx.DiagnosticsSummary())
	}
	var sb strings.Builder
	sb.WriteString(ctx.PromptSection())
	sb.WriteString("\nReview UI diffs against this context for palette-role drift, typography hierarchy, component guardrails, layout/responsive regressions, and source-of-truth mismatch.\n")
	return sb.String()
}

// buildSecurePrompt builds the security analysis prompt, including file contents if provided.
func buildSecurePrompt(files []string) (string, error) {
	if len(files) == 0 {
		return "현재 프로젝트의 보안 취약점을 분석해주세요. OWASP Top 10을 기준으로 검토하세요.", nil
	}
	contents, err := buildFileContents(files)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("다음 파일들의 보안 취약점을 분석해주세요:\n\n")
	sb.WriteString(contents)
	sb.WriteString("OWASP Top 10을 기준으로 검토하세요.")
	return sb.String(), nil
}

// flagStringIfChanged returns the flag value only if the flag was explicitly set.
// Returns empty string when using default (not changed).
func flagStringIfChanged(cmd *cobra.Command, name, value string) string {
	if cmd.Flags().Changed(name) {
		return value
	}
	return ""
}

// flagStringSliceIfChanged returns the flag value only if the flag was explicitly set.
// Returns nil when using default (not changed).
func flagStringSliceIfChanged(cmd *cobra.Command, name string, value []string) []string {
	if cmd.Flags().Changed(name) {
		return value
	}
	return nil
}

// resolveRounds returns the effective debate round count.
// Default: 2 for debate strategy when --rounds not specified, 1 for others.
func resolveRounds(strategy string, rounds int) int {
	if rounds > 0 {
		return rounds
	}
	if strategy == "debate" {
		return 2
	}
	return 0
}

// buildProviderConfigs converts provider names to ProviderConfig slice.
// This is the hardcoded fallback used when config is unavailable.
// @AX:NOTE: [AUTO] hardcoded provider registry — add new providers here and in agenticArgs when expanding provider support
func buildProviderConfigs(names []string) []orchestra.ProviderConfig {
	return buildProviderConfigsForRuntime(names, "", "")
}

func upsertClaudeEffortArg(args []string, effort string) []string {
	if effort == "" {
		if args == nil {
			return nil
		}
		return append([]string{}, args...)
	}
	result := make([]string, 0, len(args)+2)
	found := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--effort":
			if !found {
				result = append(result, "--effort", effort)
				found = true
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		case strings.HasPrefix(args[i], "--effort="):
			if !found {
				result = append(result, "--effort="+effort)
				found = true
			}
		default:
			result = append(result, args[i])
		}
	}
	if !found {
		result = append(result, "--effort", effort)
	}
	return result
}

func codexEffortForRuntime(effort string) string {
	if effort == "ultracode" {
		return config.CodexEffortXHigh
	}
	return effort
}

func buildProviderConfigsForRuntime(names []string, quality, effort string) []orchestra.ProviderConfig {
	qualityConf := config.QualityConf{Default: strings.TrimSpace(quality)}
	codexEntry := config.CodexProviderEntryForQuality(qualityConf)
	if effort = strings.TrimSpace(effort); effort != "" {
		profile := qualityConf.CodexOrchestraProfile()
		profile.Effort = codexEffortForRuntime(effort)
		codexEntry = config.ApplyCodexProviderProfile(codexEntry, profile)
	}
	claudeEntry := config.DefaultClaudeProviderEntry()
	knownProviders := map[string]orchestra.ProviderConfig{
		"claude": {Name: "claude", Binary: claudeEntry.Binary, ModelFamily: "anthropic", Args: upsertClaudeEffortArg(claudeEntry.Args, effort), PaneArgs: upsertClaudeEffortArg(claudeEntry.PaneArgs, effort), PromptViaArgs: false},
		// SPEC-ORCH-021 REQ-014/015: codex subprocess uses `exec --sandbox workspace-write`
		// (no deprecated --full-auto) with reasoning effort aligned to autopus.yaml; pane argv
		// stays interactive (no leading `exec`). SchemaFlag carries the structured schema.
		"codex": providerConfigFromEntry("codex", codexEntry, ""),
		// SPEC-ORCH-021 REQ-014/015: gemini (`agy`) --print is a STRING flag taking the prompt
		// as its value. Pass the prompt in the empty "" slot via PromptViaArgs (injectPromptArg
		// replaces "" with the prompt) → `agy --print "<prompt>"`. Pane argv must be interactive,
		// so it carries no --print.
		"gemini": {Name: "gemini", Binary: "agy", ModelFamily: "google", Args: []string{"--print", ""}, PaneArgs: []string{}, PromptViaArgs: true, StartupTimeout: defaultProviderStartupTimeout("gemini"), OutputFormat: "text"},
	}

	var result []orchestra.ProviderConfig
	for _, name := range names {
		if p, ok := knownProviders[name]; ok {
			result = append(result, p)
		} else {
			result = append(result, orchestra.ProviderConfig{
				Name:        name,
				Binary:      name,
				ModelFamily: providerModelFamily(name),
				Args:        []string{},
			})
		}
	}
	return result
}

func providerConfigFromEntry(name string, entry config.ProviderEntry, interactiveInput string) orchestra.ProviderConfig {
	binary := entry.Binary
	modelFamily := providerModelFamily(name)
	var tools []string
	if entry.Backend == config.ProviderBackendOMP {
		tools = entry.EffectiveTools()
		if binary == "" {
			binary = config.ProviderBackendOMP
		}
		if modelFamily == "" {
			modelFamily = orchestra.ModelFamilyForSelector(entry.Model)
		}
	}
	return orchestra.ProviderConfig{
		Name:             name,
		Backend:          entry.Backend,
		Model:            entry.Model,
		Tools:            tools,
		Binary:           binary,
		ModelFamily:      modelFamily,
		Args:             append([]string(nil), entry.Args...),
		PaneArgs:         append([]string(nil), entry.PaneArgs...),
		ModelPolicy:      entry.ModelPolicy,
		PromptViaArgs:    entry.PromptViaArgs,
		InteractiveInput: interactiveInput,
		StartupTimeout:   resolveProviderStartupTimeout(name),
		ExecutionTimeout: resolveProviderExecutionTimeout(entry),
		WorkingPatterns:  resolveWorkingPatterns(name, entry.WorkingPatterns),
		SchemaFlag:       entry.Subprocess.SchemaFlag,
		StdinMode:        entry.Subprocess.StdinMode,
		OutputFormat:     entry.Subprocess.OutputFormat,
	}
}

// defaultProviders returns the hardcoded default provider list.
func defaultProviders() []string {
	return []string{"claude", "codex", "gemini"}
}

func defaultProviderStartupTimeout(name string) time.Duration {
	switch name {
	case "gemini":
		return 20 * time.Second
	default:
		return 0
	}
}

// resolveAndValidateThreshold validates the threshold flag and resolves the final value.
func resolveAndValidateThreshold(orchConf *config.OrchestraConf, configErr error, commandName string, threshold float64) (float64, error) {
	if err := validateThreshold(threshold); err != nil {
		return 0, err
	}
	var resolved float64
	if configErr != nil || orchConf == nil {
		if threshold > 0 {
			resolved = threshold
		} else {
			resolved = 0.66
		}
	} else {
		resolved = resolveThreshold(orchConf, commandName, threshold)
	}
	if err := validateThreshold(resolved); err != nil {
		return 0, fmt.Errorf("resolved threshold invalid: %w", err)
	}
	return resolved, nil
}
