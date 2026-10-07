package config

// HealthBandConf is the optional health_band namespace read by
// `auto react band` (SPEC-SIGMABAND-001). Every field is omitempty so a config
// at defaults never writes the key: older binaries decode autopus.yaml
// strictly and would reject it. Unknown keys inside the namespace, such as
// allow_draft_pr (SPEC-SIGMABAND-002), stay rejected by strict decoding.
type HealthBandConf struct {
	// DiagnosisProvider names the provider band diagnosis selects first; empty
	// keeps the default order (orchestra.judge, then the first provider name).
	// It is not validated here: an unknown or unsupported name is a runtime
	// "unavailable" diagnosis, never a config load failure.
	DiagnosisProvider string `yaml:"diagnosis_provider,omitempty"`
}
