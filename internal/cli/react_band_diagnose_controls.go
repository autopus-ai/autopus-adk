package cli

import (
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// bandReadOnlyControls checks, fail-closed, the required controls of the
// Provider Read-Only Contract item 3 on a projected provider.
func bandReadOnlyControls(provider orchestra.ProviderConfig) bool {
	if provider.Backend == config.ProviderBackendOMP {
		tools := slices.Clone(provider.Tools)
		slices.Sort(tools)
		return provider.SandboxMode == orchestra.SandboxModeReadOnly && slices.Equal(slices.Compact(tools), []string{"glob", "grep", "read"})
	}
	args := provider.Args
	switch provider.Name {
	case "claude":
		return slices.Equal(bandFlagValues(args, "--permission-mode"), []string{"plan"}) &&
			slices.Equal(bandFlagValues(args, "--tools"), []string{claudeReadOnlyTools}) &&
			slices.Contains(args, "--tools="+claudeReadOnlyTools)
	case "codex":
		return slices.Equal(bandFlagValues(args, "--sandbox"), []string{"read-only"}) && len(bandFlagValues(args, "-s")) == 0
	case "gemini":
		return slices.Equal(bandFlagValues(args, "--mode"), []string{"plan"}) && slices.Contains(args, "--sandbox")
	}
	return false
}

// bandFlagValues returns every value of a value flag before a "--"
// separator, in separated or inline form; a separated flag owns the next
// item, as in the projection.
func bandFlagValues(args []string, flag string) []string {
	var values []string
	for index := 0; index < len(args) && args[index] != "--"; index++ {
		if value, inline := strings.CutPrefix(args[index], flag+"="); inline {
			values = append(values, value)
		} else if args[index] == flag && index+1 < len(args) {
			index++
			values = append(values, args[index])
		}
	}
	return values
}
