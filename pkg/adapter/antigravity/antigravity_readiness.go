// Package antigravity provides read-only Antigravity CLI capability detection.
package antigravity

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// antigravityVerifiedFloor is the lowest `agy` release on which every plugin
// component this adapter emits was observed to load. It was measured with
// `agy plugin validate` against a fixture carrying plugin.json, skills/,
// agents/, commands/ and rules/ — the CLI reported skills, agents and commands
// as processed, and accepted the manifest and rule files. Raising this constant
// requires a new observation, never a documentation reading: the published
// plugin guide for 1.2.x omits `commands/` entirely, yet 1.1.26 converts those
// files into skills.
const antigravityVerifiedFloor = "1.1.26"

// antigravityCapabilityFloors maps each generated native component to the
// release where it was verified. The IDs are stable: doctor and Validate
// surface them verbatim.
var antigravityCapabilityFloors = []struct {
	id         string
	minVersion string
}{
	{"plugin.manifest", antigravityVerifiedFloor},
	{"plugin.skills", antigravityVerifiedFloor},
	{"plugin.commands", antigravityVerifiedFloor},
	{"plugin.agents", antigravityVerifiedFloor},
	{"plugin.rules", antigravityVerifiedFloor},
	{"workspace.hooks", antigravityVerifiedFloor},
}

// AntigravityProbeRunner makes the readiness subprocess injectable and bounded
// by the context supplied for that individual probe.
type AntigravityProbeRunner interface {
	Run(ctx context.Context, executable string, args ...string) ([]byte, error)
}

// AntigravityReadinessOptions configures a single readiness probe.
type AntigravityReadinessOptions struct {
	Executable string
	Runner     AntigravityProbeRunner
	Timeout    time.Duration
	MaxOutput  int
}

// AntigravityCapabilityResult reports one native component's support state.
type AntigravityCapabilityResult struct {
	ID         string `json:"id"`
	Supported  bool   `json:"supported"`
	MinVersion string `json:"min_version"`
	Reason     string `json:"reason"`
}

// AntigravityReadinessReport is the whole observation for one CLI binary.
type AntigravityReadinessReport struct {
	Executable   string                        `json:"executable"`
	Version      string                        `json:"version,omitempty"`
	Capabilities []AntigravityCapabilityResult `json:"capabilities"`
}

// Unsupported returns the capabilities the probe could not confirm. An empty
// slice means every emitted component was observed as loadable.
func (r AntigravityReadinessReport) Unsupported() []AntigravityCapabilityResult {
	out := make([]AntigravityCapabilityResult, 0, len(r.Capabilities))
	for _, capability := range r.Capabilities {
		if !capability.Supported {
			out = append(out, capability)
		}
	}
	return out
}

// ProbeAntigravityReadiness reads the installed CLI version and nothing else.
// It never starts a conversation, never installs or imports a plugin, and never
// writes to the user's global configuration, so it is safe to run from Validate
// and from doctor.
// @AX:ANCHOR [AUTO]: preserve the version-only Antigravity readiness boundary.
// @AX:REASON [AUTO]: Validate and doctor depend on capability detection that cannot mutate global CLI state or consume provider quota.
func ProbeAntigravityReadiness(
	ctx context.Context,
	opts AntigravityReadinessOptions,
) AntigravityReadinessReport {
	opts = normalizeAntigravityReadinessOptions(opts)
	report := AntigravityReadinessReport{
		Executable:   opts.Executable,
		Capabilities: make([]AntigravityCapabilityResult, 0, len(antigravityCapabilityFloors)),
	}

	output, reason := runAntigravityVersionProbe(ctx, opts)
	if reason != "" {
		return antigravityUniformReport(report, reason)
	}
	observed, ok := parseAntigravityVersion(string(output))
	if !ok {
		return antigravityUniformReport(report, "version_unparsed")
	}
	report.Version = formatAntigravityVersion(observed)

	for _, floor := range antigravityCapabilityFloors {
		minimum, _ := parseAntigravityVersion(floor.minVersion)
		supported := compareAntigravityVersions(observed, minimum) >= 0
		capabilityReason := "verified_min_version"
		if !supported {
			capabilityReason = "below_min_version"
		}
		report.Capabilities = append(report.Capabilities, AntigravityCapabilityResult{
			ID:         floor.id,
			Supported:  supported,
			MinVersion: floor.minVersion,
			Reason:     capabilityReason,
		})
	}
	return report
}

func antigravityUniformReport(
	report AntigravityReadinessReport,
	reason string,
) AntigravityReadinessReport {
	for _, floor := range antigravityCapabilityFloors {
		report.Capabilities = append(report.Capabilities, AntigravityCapabilityResult{
			ID:         floor.id,
			MinVersion: floor.minVersion,
			Reason:     reason,
		})
	}
	return report
}

func normalizeAntigravityReadinessOptions(opts AntigravityReadinessOptions) AntigravityReadinessOptions {
	if opts.Executable == "" {
		opts.Executable = cliBinary
	}
	// The ceiling must stay above processprobe.DefaultWaitDelay: the drain
	// grace for an inherited pipe has to fit inside it, or a probe that needs
	// the grace is reported as a failure instead of an answer.
	if opts.Timeout <= 0 {
		opts.Timeout = processprobe.DefaultWaitDelay + 3*time.Second
	}
	// @AX:NOTE [AUTO]: 4 KiB bounds a version banner with room for a deprecation notice.
	if opts.MaxOutput <= 0 {
		opts.MaxOutput = 4 * 1024
	}
	if opts.Runner == nil {
		opts.Runner = commandAntigravityProbeRunner{maxOutput: opts.MaxOutput}
	}
	return opts
}

func runAntigravityVersionProbe(
	parent context.Context,
	opts AntigravityReadinessOptions,
) ([]byte, string) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()

	output, err := opts.Runner.Run(ctx, opts.Executable, "--version")
	if err != nil {
		return nil, "probe_failed"
	}
	if len(output) > opts.MaxOutput {
		return nil, "output_oversized"
	}
	return output, ""
}

type commandAntigravityProbeRunner struct{ maxOutput int }

func (r commandAntigravityProbeRunner) Run(
	ctx context.Context,
	executable string,
	args ...string,
) ([]byte, error) {
	binary, err := exec.LookPath(executable)
	if err != nil {
		return nil, err
	}
	return processprobe.OutputLimited(exec.CommandContext(ctx, binary, args...), r.maxOutput)
}

// parseAntigravityVersion accepts the bare `1.1.26` banner `agy --version`
// prints, tolerating a `v` prefix and a missing patch component.
func parseAntigravityVersion(raw string) ([3]int, bool) {
	field := strings.TrimSpace(raw)
	if index := strings.IndexAny(field, " \t\n\r"); index >= 0 {
		field = field[:index]
	}
	field = strings.TrimPrefix(field, "v")
	if field == "" {
		return [3]int{}, false
	}
	parts := strings.Split(field, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, false
	}
	var parsed [3]int
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return [3]int{}, false
		}
		parsed[i] = value
	}
	return parsed, true
}

func compareAntigravityVersions(left, right [3]int) int {
	for i := range left {
		switch {
		case left[i] > right[i]:
			return 1
		case left[i] < right[i]:
			return -1
		}
	}
	return 0
}

func formatAntigravityVersion(version [3]int) string {
	return strconv.Itoa(version[0]) + "." + strconv.Itoa(version[1]) + "." + strconv.Itoa(version[2])
}
