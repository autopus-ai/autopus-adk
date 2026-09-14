package config

// OrchestraConf는 다중 모델 오케스트레이션 설정이다 (Full 전용).
type OrchestraConf struct {
	Enabled            bool                     `yaml:"enabled"`
	DefaultStrategy    string                   `yaml:"default_strategy"`
	TimeoutSeconds     int                      `yaml:"timeout_seconds"`
	Judge              string                   `yaml:"judge,omitempty"`               // global default judge provider for debate
	ConsensusThreshold float64                  `yaml:"consensus_threshold,omitempty"` // global consensus threshold
	Providers          map[string]ProviderEntry `yaml:"providers,omitempty"`
	Commands           map[string]CommandEntry  `yaml:"commands,omitempty"`
	Subprocess         SubprocessConf           `yaml:"subprocess,omitempty"` // global subprocess settings
}

// SubprocessConf holds global subprocess execution settings.
type SubprocessConf struct {
	Enabled       bool   `yaml:"enabled"`                  // enable subprocess mode globally
	MaxConcurrent int    `yaml:"max_concurrent,omitempty"` // max parallel subprocess executions (default 3)
	WorkDir       string `yaml:"work_dir,omitempty"`       // working directory for subprocess execution
	Rounds        int    `yaml:"rounds,omitempty"`         // default debate rounds (default 1)
}

// ProviderEntry는 프로바이더 실행 설정이다.
type ProviderEntry struct {
	Backend          string             `yaml:"backend,omitempty"`
	Model            string             `yaml:"model,omitempty"`
	Tools            []string           `yaml:"tools,flow,omitempty"`
	Binary           string             `yaml:"binary"`
	Args             []string           `yaml:"args,flow"`
	PaneArgs         []string           `yaml:"pane_args,flow,omitempty"`
	ModelPolicy      string             `yaml:"model_policy,omitempty"`
	PromptViaArgs    bool               `yaml:"prompt_via_args,omitempty"`
	InteractiveInput string             `yaml:"interactive_input,omitempty"`
	WorkingPatterns  []string           `yaml:"working_patterns,flow,omitempty"`
	Subprocess       SubprocessProvConf `yaml:"subprocess,omitempty"` // per-provider subprocess overrides
}

// SubprocessProvConf holds per-provider subprocess execution overrides.
type SubprocessProvConf struct {
	SchemaFlag   string `yaml:"schema_flag,omitempty"`   // CLI flag name for JSON schema (e.g., "--schema")
	StdinMode    string `yaml:"stdin_mode,omitempty"`    // how to pass prompt: "pipe" (default) or "file"
	OutputFormat string `yaml:"output_format,omitempty"` // expected output format: "json" (default) or "text"
	Timeout      int    `yaml:"timeout,omitempty"`       // per-provider timeout override in seconds
}

// CommandEntry는 커맨드별 오케스트레이션 설정이다.
type CommandEntry struct {
	Strategy           string   `yaml:"strategy"`
	Providers          []string `yaml:"providers,flow"`
	Judge              string   `yaml:"judge,omitempty"`               // provider name to act as debate judge
	ConsensusThreshold float64  `yaml:"consensus_threshold,omitempty"` // per-command consensus threshold override
}
