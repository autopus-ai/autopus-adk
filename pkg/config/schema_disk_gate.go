package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DiskGateConf is hooks.disk_gate. It is written either as a boolean
// (`disk_gate: false`) or as a mapping that also extends the heavy-command
// list:
//
//	disk_gate:
//	  enabled: true
//	  extra_commands: ["make release", "bazel build"]
//
// Each extra command is a word prefix matched against every simple command of
// a shell line, after env assignments and wrappers such as env or time.
type DiskGateConf struct {
	Enabled       *bool    `yaml:"enabled,omitempty"`
	ExtraCommands []string `yaml:"extra_commands,omitempty"`
}

// diskGatePlain is DiskGateConf without its YAML methods.
type diskGatePlain DiskGateConf

// UnmarshalYAML accepts the boolean and the mapping form, the mapping
// decoded strictly so a misspelt key still fails the load.
func (g *DiskGateConf) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var enabled bool
		if err := node.Decode(&enabled); err != nil {
			return fmt.Errorf("hooks.disk_gate must be true, false, or a mapping with enabled and extra_commands")
		}
		*g = DiskGateConf{Enabled: &enabled}
		return nil
	}
	var plain diskGatePlain
	if err := decodeNodeKnownFields(node, &plain); err != nil {
		return err
	}
	*g = DiskGateConf(plain)
	return nil
}

// MarshalYAML writes the boolean form when no extra command is set.
func (g DiskGateConf) MarshalYAML() (any, error) {
	if len(g.ExtraCommands) == 0 && g.Enabled != nil {
		return *g.Enabled, nil
	}
	return diskGatePlain(g), nil
}

func (g *DiskGateConf) validate() error {
	if g == nil {
		return nil
	}
	for _, command := range g.ExtraCommands {
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("hooks.disk_gate.extra_commands entries must not be empty")
		}
	}
	return nil
}

// IsDiskGateEnabled reports whether the disk gate PreToolUse hook is
// registered. Unset means on: the gate costs one statfs per shell call and
// only blocks while free space is below the floor.
func (h HooksConf) IsDiskGateEnabled() bool {
	return h.DiskGate == nil || h.DiskGate.Enabled == nil || *h.DiskGate.Enabled
}

// DiskGateExtraCommands is hooks.disk_gate.extra_commands.
func (h HooksConf) DiskGateExtraCommands() []string {
	if h.DiskGate == nil {
		return nil
	}
	return h.DiskGate.ExtraCommands
}
