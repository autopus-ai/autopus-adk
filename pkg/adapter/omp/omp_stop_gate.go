package omp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/langgate"
)

// ompStopGateTarget is the project extension omp loads from .omp/extensions.
const ompStopGateTarget = ".omp/extensions/autopus-stop-gate.ts"

//go:embed stop_gate/autopus-stop-gate.ts
var ompStopGateSource string

// ompStopGateTimeoutLine is the extension's kill timer as embedded, and
// ompStopGateDefaultTimeout its value; ompStopGateSpawnArgs the arguments it
// runs each check with.
const (
	ompStopGateTimeoutLine    = "const TIMEOUT_MS = 10 * 60 * 1000"
	ompStopGateDefaultTimeout = 10 * time.Minute
	ompStopGateSpawnArgs      = `["verify", check]`
)

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
//
// The OMP context policy opt-in owns .omp/extensions as a locked set: its
// managed RPC runtime admits only the bridge and the pipeline route
// (SPEC-OMP-004), and a gate that sends follow-ups would change what that
// runtime runs. The gate is left out there.
func prepareStopGateMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	checks := ompStopGateChecks(cfg)
	if len(checks) == 0 {
		return nil, nil
	}
	if _, _, selected, err := cfg.OMPContextPolicy.SelectedOMPContextProfile(); err != nil || selected {
		return nil, err
	}
	list, err := json.Marshal(checks)
	if err != nil {
		return nil, err
	}
	rendered := strings.Replace(ompStopGateSource, "__AUTOPUS_STOP_CHECKS__", string(list), 1)
	// A set hooks.verify_stop_budget is passed to `auto verify stop` as
	// --budget (the gate alone may only lower it), and the kill timer
	// outlasts it the way the Claude Code hook timeout does, so the two move
	// together. Unset keeps the file as it always was.
	if budget, err := cfg.Hooks.VerifyStopBudgetDuration(); err == nil && cfg.Hooks.VerifyStopBudget != "" {
		rendered = strings.Replace(rendered, ompStopGateSpawnArgs, fmt.Sprintf(
			`["verify", check, ...(check === "stop" ? ["--budget", %q] : [])]`, budget.String()), 1)
		if timeout := config.VerifyStopHookTimeout(budget); timeout > ompStopGateDefaultTimeout {
			rendered = strings.Replace(rendered, ompStopGateTimeoutLine,
				fmt.Sprintf("const TIMEOUT_MS = %d * 1000", int(timeout/time.Second)), 1)
		}
	}
	return []adapter.FileMapping{{
		TargetPath:      ompStopGateTarget,
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        adapter.Checksum(rendered),
		Content:         []byte(rendered),
	}}, nil
}
