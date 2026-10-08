package config

// HealthBandConf is the optional health_band namespace read by
// `auto react band` (SPEC-SIGMABAND-001, SPEC-SIGMABAND-002). Every field is
// omitempty so a config at defaults never writes its key: older binaries
// decode autopus.yaml strictly and reject a key they do not declare, so a
// binary without SPEC-SIGMABAND-002 rejects a file that sets allow_local_patch
// or local_patch_provider. Unknown keys inside the namespace, such as
// allow_draft_pr of the retired draft PR path, stay rejected by strict
// decoding.
type HealthBandConf struct {
	// DiagnosisProvider names the provider band diagnosis selects first; empty
	// keeps the default order (orchestra.judge, then the first provider name).
	// It is not validated here: an unknown or unsupported name is a runtime
	// "unavailable" diagnosis, never a config load failure.
	DiagnosisProvider string `yaml:"diagnosis_provider,omitempty"`
	// AllowLocalPatch turns the tier-3 local patch flow on (SPEC-SIGMABAND-002
	// REQ-01); false keeps band diagnosis-only, as SPEC-SIGMABAND-001 defines.
	AllowLocalPatch bool `yaml:"allow_local_patch,omitempty"`
	// LocalPatchProvider names, while AllowLocalPatch is true, the provider
	// band runs as a CLI subprocess for every diagnosis and patch request,
	// whatever its orchestra backend (SPEC-SIGMABAND-002 REQ-15); band never
	// reads it while the flag is false. Like DiagnosisProvider it is not
	// validated here: a name that cannot be confined is a runtime
	// unavailable(provider_unconfined), never a config load failure.
	LocalPatchProvider string `yaml:"local_patch_provider,omitempty"`
}
