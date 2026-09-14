package config

const (
	CodexFrontierModel           = CodexAstraModel
	CodexCodingModel             = CodexSolModel
	CodexStandardModel           = CodexTerraModel
	CodexMiniModel               = CodexLunaModel
	CodexSparkModel              = CodexLunaModel
	CodexFallbackModel           = CodexLegacyModel
	CodexOrchestraTimeoutSeconds = 420
	// ClaudeOrchestraTimeoutSeconds covers opus reasoning that routinely runs
	// 3–6 minutes on spec review workloads. Exceeds the 240s global timeout to
	// prevent the cutoff reported in issue #55.
	ClaudeOrchestraTimeoutSeconds = 480
	// Gemini review through agy can exceed the 240s global timeout on structured
	// SPEC review prompts, so it needs the same per-provider execution budget.
	GeminiOrchestraTimeoutSeconds = 480
)

// DefaultCodexProviderEntry returns the canonical Codex orchestra provider entry.
func DefaultCodexProviderEntry() ProviderEntry {
	return CodexProviderEntryForQuality(QualityConf{Default: "balanced"})
}

// CodexProviderEntryForQuality returns a managed Codex provider for a quality mode.
func CodexProviderEntryForQuality(quality QualityConf) ProviderEntry {
	profile := quality.CodexOrchestraProfile()
	return ProviderEntry{
		Binary:      "codex",
		ModelPolicy: ProviderModelPolicyQuality,
		// SPEC-ORCH-021 REQ-014/015: `exec --sandbox workspace-write` (no deprecated --full-auto);
		// reasoning effort aligned to autopus.yaml. Pane argv stays interactive (no leading `exec`).
		Args:          []string{"exec", "--json", "--sandbox", "workspace-write", "-m", profile.Model, "-c", `model_reasoning_effort="` + profile.Effort + `"`},
		PaneArgs:      []string{"-m", profile.Model, "-c", `model_reasoning_effort="` + profile.Effort + `"`},
		PromptViaArgs: false,
		Subprocess: SubprocessProvConf{
			SchemaFlag: "--output-schema",
			Timeout:    CodexOrchestraTimeoutSeconds,
		},
	}
}

// DefaultFullConfig returns the default config for Full mode.
// @AX:NOTE: [AUTO] magic constants — model names (fable, opus, sonnet, haiku, Codex GPT), timeouts, and tier mappings are hardcoded below
func DefaultFullConfig(projectName string) *HarnessConfig {
	return &HarnessConfig{
		Mode:        ModeFull,
		ProjectName: projectName,
		Platforms:   []string{"claude-code"},
		Architecture: ArchitectureConf{
			AutoGenerate: true,
			Enforce:      true,
		},
		Lore: LoreConf{
			Enabled:            true,
			RequiredTrailers:   []string{"Constraint"},
			StaleThresholdDays: 90,
		},
		Spec: SpecConf{
			IDFormat:  "SPEC-{DOMAIN}-{NUMBER}",
			EARSTypes: []string{"ubiquitous", "event-driven", "unwanted", "optional", "complex"},
			ReviewGate: ReviewGateConf{
				Enabled:  true,
				Strategy: "debate",
				// SPEC-ORCH-021 REQ-016: include codex so it is not silently dropped from
				// structured review; matches the orchestra command provider set.
				Providers:          []string{"claude", "codex", "gemini"},
				Judge:              "claude",
				MaxRevisions:       new(2),
				AutoCollectContext: true,
				ContextMaxLines:    0,
				VerdictThreshold:   0.67,
				DocContextMaxLines: 200,
				// MinProviders 0 = derive the majority quorum from the configured
				// provider count (spec.DefaultMinProviders); REQ-RINT-QUORUM-05.
				MinProviders: 0,
			},
		},
		Methodology: MethodologyConf{
			Mode:       "tdd",
			Enforce:    true,
			ReviewGate: true,
		},
		Hooks: HooksConf{
			PreCommitArch:  true,
			PreCommitLore:  true,
			ReactCIFailure: true,
			ReactReview:    true,
		},
		Session: SessionConf{
			HandoffEnabled:   true,
			ContinueFile:     ".auto-continue.md",
			MaxContextTokens: 2000,
		},
		Orchestra: OrchestraConf{
			Enabled:         true,
			DefaultStrategy: "consensus",
			TimeoutSeconds:  240,
			Judge:           "claude",
			Providers: map[string]ProviderEntry{
				"claude": DefaultClaudeProviderEntry(),
				// SPEC-ORCH-021 REQ-014/015: prompt is the value of --print (injected into the ""
				// slot); pane argv carries no --print (interactive session).
				"gemini": {Binary: "agy", Args: []string{"--print", ""}, PaneArgs: []string{}, PromptViaArgs: true, InteractiveInput: "stdin", Subprocess: SubprocessProvConf{OutputFormat: "text", Timeout: GeminiOrchestraTimeoutSeconds}},
				"codex":  DefaultCodexProviderEntry(),
			},
			Commands: map[string]CommandEntry{
				"review":     {Strategy: "debate", Providers: []string{"claude", "codex", "gemini"}},
				"plan":       {Strategy: "consensus", Providers: []string{"claude", "codex", "gemini"}},
				"secure":     {Strategy: "consensus", Providers: []string{"claude", "codex", "gemini"}},
				"brainstorm": {Strategy: "debate", Providers: []string{"claude", "codex", "gemini"}},
			},
		},
		// Ultra keeps the shared tier ladder. Standard balanced placement
		// uses one role matrix across Claude, Codex, and OMP; custom native
		// agent tiers continue to use the tier ladder.
		Quality: QualityConf{
			Default:               "balanced",
			SupervisorModelPolicy: SupervisorModelPolicyInherit,
			Presets: map[string]QualityPreset{
				"ultra": {
					Description: "추론 코어 7개는 Fable, 나머지는 Opus. 최고 품질.",
					Agents: map[string]string{
						"architect": "fable", "debugger": "fable", "deep-worker": "fable",
						"planner": "fable", "reviewer": "fable", "security-auditor": "fable",
						"spec-writer": "fable",
						"annotator":   "opus", "devops": "opus", "executor": "opus",
						"explorer": "opus", "frontend-specialist": "opus",
						"perf-engineer": "opus", "tester": "opus",
						"ux-validator": "opus", "validator": "opus",
					},
				},
				"balanced": {
					Description: "기획·리뷰·디버깅은 최상위, 구현·검증은 경량 모델.",
					Agents:      defaultBalancedAgentTiers(),
				},
			},
		},
		// Written explicitly so the knob is visible in a fresh autopus.yaml;
		// a file that omits it keeps resolving to the same default.
		Codex: CodexConf{
			Agents: CodexAgentsConf{MaxConcurrentThreads: CodexAgentConcurrencyDefault},
		},
		Skills: SkillsConf{
			AutoActivate:    true,
			MaxActiveSkills: 5,
			Compiler:        SkillCompilerConf{Mode: SkillCompilerModeSplit},
			CategoryWeights: map[string]int{
				"security": 30,
				"quality":  20,
				"agentic":  15,
				"workflow": 10,
			},
		},
		Verify: VerifyConf{
			Enabled:         true,
			DefaultViewport: "desktop",
			AutoFix:         true,
			MaxFixAttempts:  2,
		},
		Design: DesignConf{
			Enabled:         true,
			MaxContextLines: 80,
			InjectOnReview:  true,
			InjectOnVerify:  true,
			ExternalImports: false,
		},
		Context: ContextConf{
			SignatureMap: true,
		},
		Features: FeaturesConf{
			CC21: CC21FeaturesConf{
				Enabled:                 false,
				EffortEnabled:           false,
				MonitorEnabled:          false,
				TaskCreatedEnabled:      false,
				InitialPromptEnabled:    false,
				TaskCreatedMode:         "warn",
				MonitorPatternTimeoutMS: 30000,
			},
		},
	}
}
