package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DefaultVersionStopMaxMajorBehind is how many release lines behind the
// latest stable a newly added or bumped pin may be before the stop blocks.
// Zero: the gate only looks at pins this turn adds or changes, so there is no
// migration cost to weigh, and a whole line behind on a fresh choice is the
// symptom of a version remembered from training data. A deliberate pin still
// passes with one `version-pin:` line.
const DefaultVersionStopMaxMajorBehind = 0

// VersionStopConf is hooks.version_stop. It is written either as a boolean
// (`version_stop: false`) or as a mapping:
//
//	version_stop:
//	  enabled: true
//	  max_major_behind: 1
type VersionStopConf struct {
	Enabled        *bool `yaml:"enabled,omitempty"`
	MaxMajorBehind *int  `yaml:"max_major_behind,omitempty"`
}

// versionStopPlain is VersionStopConf without its YAML methods.
type versionStopPlain VersionStopConf

// UnmarshalYAML accepts the boolean and the mapping form, the mapping
// decoded strictly so a misspelt key still fails the load.
func (v *VersionStopConf) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var enabled bool
		if err := node.Decode(&enabled); err != nil {
			return fmt.Errorf("hooks.version_stop must be true, false, or a mapping with enabled and max_major_behind")
		}
		*v = VersionStopConf{Enabled: &enabled}
		return nil
	}
	var plain versionStopPlain
	if err := decodeNodeKnownFields(node, &plain); err != nil {
		return err
	}
	*v = VersionStopConf(plain)
	return nil
}

// MarshalYAML writes the boolean form when no threshold is set.
func (v VersionStopConf) MarshalYAML() (any, error) {
	if v.MaxMajorBehind == nil && v.Enabled != nil {
		return *v.Enabled, nil
	}
	return versionStopPlain(v), nil
}

func (v *VersionStopConf) validate() error {
	if v != nil && v.MaxMajorBehind != nil && *v.MaxMajorBehind < 0 {
		return fmt.Errorf("hooks.version_stop.max_major_behind must be >= 0")
	}
	return nil
}

// IsVersionStopEnabled reports whether the verification Stop hook checks
// newly added or bumped version pins. Unset means on; it runs only where the
// verification Stop hook itself runs.
func (h HooksConf) IsVersionStopEnabled() bool {
	return h.VersionStop == nil || h.VersionStop.Enabled == nil || *h.VersionStop.Enabled
}

// VersionStopMaxMajorBehind is hooks.version_stop.max_major_behind, or
// DefaultVersionStopMaxMajorBehind when unset.
func (h HooksConf) VersionStopMaxMajorBehind() int {
	if h.VersionStop == nil || h.VersionStop.MaxMajorBehind == nil {
		return DefaultVersionStopMaxMajorBehind
	}
	return *h.VersionStop.MaxMajorBehind
}
