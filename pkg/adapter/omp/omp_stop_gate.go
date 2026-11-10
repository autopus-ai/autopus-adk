package omp

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/langgate"
)

// ompStopGateTarget is the project extension omp loads from .omp/extensions.
const ompStopGateTarget = ".omp/extensions/autopus-stop-gate.ts"

//go:embed stop_gate/autopus-stop-gate.ts
var ompStopGateSource string

// ompStopGateChecks lists the `auto verify` checks the extension runs, with
// the same switches that decide the Claude Code Stop hooks.
func ompStopGateChecks(cfg *config.HarnessConfig) []string {
	var checks []string
	if cfg.Hooks.IsVerifyStopEnabled() {
		checks = append(checks, "stop")
	}
	if cfg.Hooks.IsLanguageStopEnabled() && langgate.Supported(cfg.Language.AIResponses) {
		checks = append(checks, "language")
	}
	return checks
}

// prepareStopGateMappings renders the extension that gives omp sessions the
// Claude Code Stop hooks. With every check switched off it writes nothing, so
// a later update prunes a file an earlier one wrote.
func prepareStopGateMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	checks := ompStopGateChecks(cfg)
	if len(checks) == 0 {
		return nil, nil
	}
	list, err := json.Marshal(checks)
	if err != nil {
		return nil, err
	}
	rendered := strings.Replace(ompStopGateSource, "__AUTOPUS_STOP_CHECKS__", string(list), 1)
	return []adapter.FileMapping{{
		TargetPath:      ompStopGateTarget,
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        adapter.Checksum(rendered),
		Content:         []byte(rendered),
	}}, nil
}
