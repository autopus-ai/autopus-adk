// Package config는 autopus.yaml 설정 스키마와 로더를 제공한다.
package config

import "fmt"

// Mode는 설치 모드를 나타낸다.
type Mode string

const (
	ModeFull Mode = "full"
)

// LanguageConf는 프로젝트 언어 설정이다.
type LanguageConf struct {
	Comments    string `yaml:"comments"`     // 코드 주석 언어 (en, ko, ja, zh)
	Commits     string `yaml:"commits"`      // 커밋 메시지 언어
	AIResponses string `yaml:"ai_responses"` // AI 응답 언어
}

// QualityPreset defines a named quality configuration with agent mappings.
type QualityPreset struct {
	Description string            `yaml:"description,omitempty"`
	Agents      map[string]string `yaml:"agents,omitempty"`
}

// QualityConf holds quality preset definitions and provider-specific defaults.
type QualityConf struct {
	Default               string                   `yaml:"default,omitempty"`
	Providers             map[string]string        `yaml:"providers,omitempty"`
	SupervisorModelPolicy string                   `yaml:"supervisor_model_policy,omitempty"`
	Presets               map[string]QualityPreset `yaml:"presets,omitempty"`
}

// SkillsConf holds configuration for the skills activation system.
type SkillsConf struct {
	// AutoActivate enables automatic skill activation (default true).
	AutoActivate bool `yaml:"auto_activate"`
	// MaxActiveSkills limits the number of concurrently active skills (default 5).
	MaxActiveSkills int `yaml:"max_active_skills"`
	// SharedSurface refines shared publication in explicit full compiler mode.
	// full: publish the selected library; auto: core on mixed Codex+OpenCode;
	// core: publish only the core shared skill set.
	SharedSurface string `yaml:"shared_surface,omitempty"`
	// CategoryWeights maps category names to priority weights for skill selection.
	CategoryWeights map[string]int `yaml:"category_weights,omitempty"`
	// Compiler defaults to a compact core; bundles or full mode opt into more skills.
	Compiler SkillCompilerConf `yaml:"compiler,omitempty"`
}

// IssueReportConf is the auto issue reporter configuration.
type IssueReportConf struct {
	Repo             string   `yaml:"repo,omitempty"`
	Labels           []string `yaml:"labels,omitempty"`
	AutoSubmit       bool     `yaml:"auto_submit,omitempty"`
	RateLimitMinutes int      `yaml:"rate_limit_minutes,omitempty"`
}

// HarnessConfig는 autopus.yaml의 최상위 설정 구조이다.
type HarnessConfig struct {
	Mode             Mode                 `yaml:"mode"`
	ProjectName      string               `yaml:"project_name"`
	Platforms        []string             `yaml:"platforms"`
	IsolateRules     bool                 `yaml:"isolate_rules,omitempty"`
	Stack            string               `yaml:"stack,omitempty"`     // detected stack: go, typescript, python, rust
	Framework        string               `yaml:"framework,omitempty"` // detected framework: nextjs, django, gin, etc.
	Language         LanguageConf         `yaml:"language,omitempty"`
	Architecture     ArchitectureConf     `yaml:"architecture"`
	Lore             LoreConf             `yaml:"lore"`
	Spec             SpecConf             `yaml:"spec"`
	Methodology      MethodologyConf      `yaml:"methodology,omitempty"`
	Hooks            HooksConf            `yaml:"hooks"`
	Session          SessionConf          `yaml:"session,omitempty"`
	Orchestra        OrchestraConf        `yaml:"orchestra,omitempty"`
	Quality          QualityConf          `yaml:"quality,omitempty"`
	Codex            CodexConf            `yaml:"codex,omitempty"`
	RoleModelPolicy  RoleModelPolicyConf  `yaml:"role_model_policy,omitempty"`
	OMPContextPolicy OMPContextPolicyConf `yaml:"omp_context_policy,omitempty"`
	Skills           SkillsConf           `yaml:"skills,omitempty"`
	Verify           VerifyConf           `yaml:"verify,omitempty"`
	Design           DesignConf           `yaml:"design,omitempty"`
	Constraints      ConstraintConf       `yaml:"constraints,omitempty"`
	Context          ContextConf          `yaml:"context,omitempty"`
	Features         FeaturesConf         `yaml:"features,omitempty"`
	IssueReport      IssueReportConf      `yaml:"issue_report,omitempty"`
	Profiles         ProfilesConf         `yaml:"profiles,omitempty"`
	UsageProfile     UsageProfile         `yaml:"usage_profile,omitempty"` // developer (default) or fullstack
	Hints            HintsConf            `yaml:"hints,omitempty"`
	Workflow         WorkflowConf         `yaml:"workflow,omitempty"`
	Runtime          RuntimeConf          `yaml:"-"`
}

// FeaturesConf holds feature-flag namespaces.
type FeaturesConf struct {
	CC21 CC21FeaturesConf `yaml:"cc21,omitempty"`
}

// CC21FeaturesConf holds Claude Code 2.1 integration flags.
type CC21FeaturesConf struct {
	Enabled                 bool   `yaml:"enabled"`
	EffortEnabled           bool   `yaml:"effort_enabled,omitempty"`
	MonitorEnabled          bool   `yaml:"monitor_enabled,omitempty"`
	TaskCreatedEnabled      bool   `yaml:"task_created_enabled,omitempty"`
	InitialPromptEnabled    bool   `yaml:"initial_prompt_enabled,omitempty"`
	TaskCreatedMode         string `yaml:"task_created_mode,omitempty"`
	MonitorPatternTimeoutMS int    `yaml:"monitor_pattern_timeout_ms,omitempty"`
}

// ProfilesConf holds profile configuration for agents.
type ProfilesConf struct {
	Executor ExecutorProfileConf `yaml:"executor,omitempty"`
	Test     TestProfileConf     `yaml:"test,omitempty"`
}

// ExecutorProfileConf holds executor profile settings.
type ExecutorProfileConf struct {
	Default   string                            `yaml:"default,omitempty"`
	CustomDir string                            `yaml:"custom_dir,omitempty"`
	Override  map[string]map[string]interface{} `yaml:"override,omitempty"`
}

// ArchitectureConf는 ARCHITECTURE.md 설정이다.
type ArchitectureConf struct {
	AutoGenerate bool     `yaml:"auto_generate"`
	Enforce      bool     `yaml:"enforce"`
	Layers       []string `yaml:"layers"`
	// MaxFileLines is a project-specific code-line ceiling; zero is advisory only.
	MaxFileLines int `yaml:"max_file_lines,omitempty"`
}

// LoreConf는 Lore Decision Knowledge 설정이다.
type LoreConf struct {
	Enabled            bool     `yaml:"enabled"`
	AutoInject         bool     `yaml:"auto_inject"`
	RequiredTrailers   []string `yaml:"required_trailers"`
	StaleThresholdDays int      `yaml:"stale_threshold_days"`
}

// MethodologyConf는 방법론 설정이다 (Full 전용).
type MethodologyConf struct {
	Mode       string `yaml:"mode"`
	Enforce    bool   `yaml:"enforce"`
	ReviewGate bool   `yaml:"review_gate"`
}

// HooksConf는 훅 설정이다.
type HooksConf struct {
	PreCommitArch  bool            `yaml:"pre_commit_arch"`
	PreCommitLore  bool            `yaml:"pre_commit_lore"`
	ReactCIFailure bool            `yaml:"react_ci_failure"`
	ReactReview    bool            `yaml:"react_review"`
	Permissions    PermissionsConf `yaml:"permissions,omitempty"`
	// StickyCadence is the SPEC-STICKYRULE-001 prompt interval between sticky
	// rule re-injections. Read it through StickyCadence, never directly.
	StickyCadence int `yaml:"sticky_cadence,omitempty"`
}

// PermissionsConf는 코딩 CLI 권한 설정이다.
type PermissionsConf struct {
	// ExtraAllow는 autopus.yaml에서 사용자가 추가하는 allow 규칙이다.
	ExtraAllow []string `yaml:"extra_allow,omitempty"`
	// ExtraDeny는 autopus.yaml에서 사용자가 추가하는 deny 규칙이다.
	ExtraDeny []string `yaml:"extra_deny,omitempty"`
}

// SessionConf는 세션 연속성 설정이다 (Full 전용).
type SessionConf struct {
	HandoffEnabled   bool   `yaml:"handoff_enabled"`
	ContinueFile     string `yaml:"continue_file"`
	MaxContextTokens int    `yaml:"max_context_tokens"`
}

// ConstraintConf is the anti-pattern constraint configuration.
type ConstraintConf struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`
}

// Validate는 설정의 유효성을 검증한다.
// @AX:WARN [AUTO]: harness validation has cyclomatic complexity 15.
// @AX:REASON [AUTO]: gocyclo reports 15 across mode, platform, context, role-model, and provider configuration invariants.
func (c *HarnessConfig) Validate() error {
	if c.Mode != ModeFull {
		return fmt.Errorf("invalid mode %q: must be 'full'", c.Mode)
	}
	if c.ProjectName == "" {
		return fmt.Errorf("project_name is required")
	}
	if len(c.Platforms) == 0 {
		return fmt.Errorf("at least one platform is required")
	}
	for _, p := range c.Platforms {
		if !isValidPlatform(p) {
			return fmt.Errorf("invalid platform %q", p)
		}
	}
	if err := c.validateProviderBackends(); err != nil {
		return err
	}
	if c.Quality.Default != "" {
		if _, ok := c.Quality.Presets[c.Quality.Default]; !ok {
			return fmt.Errorf("quality.default %q is not defined in quality.presets", c.Quality.Default)
		}
	}
	if c.Features.CC21.TaskCreatedMode != "" {
		switch c.Features.CC21.TaskCreatedMode {
		case "warn", "enforce":
		default:
			return fmt.Errorf("features.cc21.task_created_mode %q is invalid", c.Features.CC21.TaskCreatedMode)
		}
	}
	if err := c.validateModelSelectionRoleAndOMPContextPolicy(); err != nil {
		return err
	}
	if err := c.validateSkillsConfig(); err != nil {
		return err
	}
	if c.Architecture.MaxFileLines < 0 {
		return fmt.Errorf("architecture.max_file_lines must be >= 0")
	}
	if c.Design.MaxContextLines < 0 {
		return fmt.Errorf("design.max_context_lines must be >= 0")
	}
	if !c.UsageProfile.IsValid() {
		return fmt.Errorf("invalid usage_profile %q: must be 'developer' or 'fullstack'", c.UsageProfile)
	}
	if err := c.Workflow.Validate(); err != nil {
		return err
	}
	if err := c.Verify.Validate(); err != nil {
		return err
	}
	if err := c.Codex.Validate(); err != nil {
		return err
	}
	return nil
}

// IsFullMode는 Full 모드 여부를 반환한다. 항상 true를 반환한다.
func (c *HarnessConfig) IsFullMode() bool {
	return c.Mode == ModeFull
}

var validPlatforms = map[string]bool{
	"claude-code":     true,
	"codex":           true,
	"antigravity-cli": true,
	"opencode":        true,
	"cursor":          true,
	"omp":             true,
}

func isValidPlatform(p string) bool {
	return validPlatforms[p]
}
