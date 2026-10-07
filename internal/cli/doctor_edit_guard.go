package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/editguard"
)

const doctorEditGuardCheckPrefix = "doctor.edit_guard."

// editGuardSurface is the generated file that carries an enforced lane's guard
// registration. event names the hook event of a JSON hook file; the OpenCode
// plugin carries its registration as the EDIT_GUARD literal instead.
type editGuardSurface struct {
	path, event string
}

var editGuardSurfaces = map[string]editGuardSurface{
	editguard.PlatformClaudeCode: {".claude/settings.json", "PreToolUse"},
	editguard.PlatformCodex:      {".codex/hooks.json", "PreToolUse"},
	editguard.PlatformGemini:     {".gemini/settings.json", "BeforeTool"},
	editguard.PlatformOpenCode:   {".opencode/plugins/autopus-hooks.js", ""},
}

// openCodeEditGuardLine starts the generated plugin line that holds the guard
// registration as a JSON object, or null without one.
const openCodeEditGuardLine = "const EDIT_GUARD = "

// editGuardDiagnosis is one doctor row: an installed platform's lane, its
// matrix state, and whether the generated surface registers the guard.
type editGuardDiagnosis struct {
	platform, lane, surface string
	state                   editguard.EnforcementState
	enabled, registered     bool
	status, detail          string
	warningCode             string
}

func (d editGuardDiagnosis) id() string {
	if d.lane != "" {
		return doctorEditGuardCheckPrefix + d.lane
	}
	return doctorEditGuardCheckPrefix + d.platform
}

func (d editGuardDiagnosis) fields() map[string]string {
	fields := map[string]string{
		"platform":     d.platform,
		"matrix_state": string(d.state),
		"registered":   strconv.FormatBool(d.registered),
		"edit_guard":   "enabled",
	}
	if !d.enabled {
		fields["edit_guard"] = "disabled"
	}
	if d.lane != "" {
		fields["lane"] = d.lane
	}
	if d.surface != "" {
		fields["surface"] = d.surface
	}
	return fields
}

// checkEditGuardText reports, per installed platform, whether the edit guard
// is registered and the platform's enforcement matrix state
// (SPEC-EDITGUARD-001 REQ-EG-23). It is advisory: a missing registration is
// worth naming, but it never fails harness health.
func checkEditGuardText(w io.Writer, dir string, cfg *config.HarnessConfig) {
	diagnoses := diagnoseEditGuard(dir, cfg)
	if len(diagnoses) == 0 {
		return
	}
	tui.SectionHeader(w, "Edit Guard")
	for _, d := range diagnoses {
		switch d.status {
		case "pass":
			tui.OK(w, d.detail)
		case "warn":
			tui.SKIP(w, d.detail)
		default:
			tui.Info(w, d.detail)
		}
	}
}

func (r *doctorJSONReport) collectEditGuardChecks(dir string, cfg *config.HarnessConfig) {
	for _, d := range diagnoseEditGuard(dir, cfg) {
		severity := "info"
		if d.status == "warn" {
			severity = "warning"
			r.status = jsonStatusWarn
			r.warnings = append(r.warnings, jsonMessage{Code: d.warningCode, Message: d.detail})
		}
		r.checks = append(r.checks, jsonCheck{
			ID:       d.id(),
			Severity: severity,
			Status:   d.status,
			Detail:   d.detail,
			Fields:   d.fields(),
		})
	}
}

// diagnoseEditGuard returns one row per lane of every installed platform, in
// autopus.yaml order. The matrix decides each lane's state; only an enforced
// lane is expected to carry the guard, and only while hooks.edit_guard is on.
func diagnoseEditGuard(dir string, cfg *config.HarnessConfig) []editGuardDiagnosis {
	if cfg == nil {
		return nil
	}
	enabled := cfg.Hooks.IsEditGuardEnabled()
	var diagnoses []editGuardDiagnosis
	for _, platform := range cfg.Platforms {
		lanes := editGuardLanesOf(platform)
		if len(lanes) == 0 {
			diagnoses = append(diagnoses, editGuardDiagnosis{
				platform: platform, state: editguard.NotEnforced, enabled: enabled, status: "skip",
				detail: platform + ": guard not registered, no edit-guard lane (matrix: none)",
			})
			continue
		}
		for _, lane := range lanes {
			diagnoses = append(diagnoses, diagnoseEditGuardLane(dir, platform, lane, enabled))
		}
	}
	return diagnoses
}

// editGuardLanesOf returns the matrix lanes the platform's adapter generates:
// the antigravity-cli adapter also writes the Gemini CLI settings.
func editGuardLanesOf(platform string) []editguard.Lane {
	var lanes []editguard.Lane
	if platform == "antigravity-cli" {
		if gemini, ok := editguard.LaneFor(editguard.PlatformGemini); ok {
			lanes = append(lanes, gemini)
		}
	}
	if lane, ok := editguard.LaneFor(platform); ok {
		lanes = append(lanes, lane)
	}
	return lanes
}

func diagnoseEditGuardLane(dir, platform string, lane editguard.Lane, enabled bool) editGuardDiagnosis {
	d := editGuardDiagnosis{platform: platform, lane: lane.Platform, state: lane.State, enabled: enabled, status: "skip"}
	matrix := fmt.Sprintf("(matrix: %s)", lane.State)
	surface, ok := editGuardSurfaces[lane.Platform]
	if lane.State != editguard.Enforced || !ok {
		d.detail = fmt.Sprintf("%s: guard not registered by design %s", lane.Name, matrix)
		return d
	}
	d.surface = surface.path
	registered, err := editGuardRegistered(dir, lane.Platform, surface)
	d.registered = registered
	switch {
	case err != nil:
		d.status, d.warningCode = "warn", "edit_guard_unreadable"
		d.detail = fmt.Sprintf("%s: cannot read %s: %v %s", lane.Name, surface.path, err, matrix)
	case registered && enabled:
		d.status = "pass"
		d.detail = fmt.Sprintf("%s: guard registered in %s %s", lane.Name, surface.path, matrix)
	case registered:
		d.status, d.warningCode = "warn", "edit_guard_not_retracted"
		d.detail = fmt.Sprintf("%s: guard still registered in %s although hooks.edit_guard is false; run 'auto update' %s",
			lane.Name, surface.path, matrix)
	case enabled:
		d.status, d.warningCode = "warn", "edit_guard_not_registered"
		d.detail = fmt.Sprintf("%s: guard not registered in %s; run 'auto update' %s", lane.Name, surface.path, matrix)
	default:
		d.detail = fmt.Sprintf("%s: guard not registered, hooks.edit_guard is false %s", lane.Name, matrix)
	}
	return d
}

// editGuardRegistered reads the lane's generated surface. A missing file holds
// no registration.
func editGuardRegistered(dir, lanePlatform string, surface editGuardSurface) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(surface.path)))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	command := content.EditGuardCommand(lanePlatform)
	if surface.event == "" {
		return openCodePluginRegistersGuard(data, command)
	}
	return hookFileRegistersGuard(data, surface.event, command)
}

// hookFileRegistersGuard reports whether a JSON hook file holds, under event,
// a handler that runs command inside the exit-masking command line the hook
// generator registers (REQ-EG-11).
func hookFileRegistersGuard(data []byte, event, command string) (bool, error) {
	var doc struct {
		Hooks map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	wrapped := "out=$(" + command + ")"
	entries, _ := doc.Hooks[event].([]any)
	for _, entry := range entries {
		object, _ := entry.(map[string]any)
		handlers, _ := object["hooks"].([]any)
		for _, handler := range handlers {
			fields, _ := handler.(map[string]any)
			line, _ := fields["command"].(string)
			if strings.HasPrefix(strings.TrimSpace(line), wrapped) {
				return true, nil
			}
		}
	}
	return false, nil
}

// openCodePluginRegistersGuard reports whether the plugin's EDIT_GUARD literal
// spawns command; null, or a plugin without the literal, registers nothing.
func openCodePluginRegistersGuard(data []byte, command string) (bool, error) {
	for _, line := range strings.Split(string(data), "\n") {
		literal, ok := strings.CutPrefix(strings.TrimSpace(line), openCodeEditGuardLine)
		if !ok {
			continue
		}
		var spec *struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSuffix(literal, ";")), &spec); err != nil {
			return false, fmt.Errorf("EDIT_GUARD is not a JSON literal: %w", err)
		}
		return spec != nil && strings.Join(append([]string{spec.Command}, spec.Args...), " ") == command, nil
	}
	return false, nil
}
