// Package antigravity provides lifecycle methods (Validate, Clean) for the Antigravity CLI adapter.
package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// antigravityProbeTimeout is the last-resort ceiling on the best-effort
// readiness probe.
//
// It must stay above processprobe.DefaultWaitDelay. The drain grace for an
// inherited pipe has to fit inside the ceiling, or a probe that actually needs
// the grace fails instead of answering — and a best-effort degradation
// swallows that failure silently, flipping the result. The ordering is pinned
// by antigravity_lifecycle_bounds_test.go.
const antigravityProbeTimeout = processprobe.DefaultWaitDelay + 2*time.Second

// antigravityRequiredRoutes are the auto routes a usable installation must
// expose. They are checked on the native plugin surface because that is the
// tree `agy` discovers, and on the legacy Gemini tree because Gemini Code
// Assist installations still read it.
var antigravityRequiredRoutes = []string{"auto-plan", "auto-go", "auto-fix", "auto-sync", "auto-review"}

// Validate checks the validity of installed files.
func (a *Adapter) Validate(ctx context.Context) ([]adapter.ValidationError, error) {
	return a.validateWithin(ctx, antigravityProbeTimeout)
}

// validateWithin is Validate with an explicit ceiling on the best-effort
// readiness probe. That ceiling is the last-resort bound: the caller deadline
// and processprobe's inherited-pipe bound both return long before it. Tests
// widen the ceiling to tell those bounds apart without timing the machine.
func (a *Adapter) validateWithin(
	ctx context.Context,
	probeTimeout time.Duration,
) ([]adapter.ValidationError, error) {
	var errs []adapter.ValidationError

	data, err := os.ReadFile(filepath.Join(a.root, "GEMINI.md"))
	if err != nil {
		return append(errs, adapter.ValidationError{
			File:    "GEMINI.md",
			Message: "GEMINI.md를 읽을 수 없음",
			Level:   "error",
		}), nil
	}
	if !strings.Contains(string(data), markerBegin) {
		errs = append(errs, adapter.ValidationError{
			File:    "GEMINI.md",
			Message: "AUTOPUS 마커 섹션이 없음",
			Level:   "warning",
		})
	}

	errs = append(errs, a.validateNativePluginSurface()...)
	for _, route := range antigravityRequiredRoutes {
		legacy := filepath.Join(a.root, ".gemini", "skills", "autopus", route, "SKILL.md")
		if _, err := os.Stat(legacy); os.IsNotExist(err) {
			errs = append(errs, adapter.ValidationError{
				File:    legacy,
				Message: fmt.Sprintf("SKILL.md가 없음: %s", route),
				Level:   "error",
			})
		}
	}

	return append(errs, a.validateCLICapabilities(ctx, probeTimeout)...), nil
}

// validateNativePluginSurface checks what `agy` actually loads: a manifest
// that parses and names the bundle, plus the route skills inside it.
func (a *Adapter) validateNativePluginSurface() []adapter.ValidationError {
	var errs []adapter.ValidationError

	manifestPath := filepath.Join(a.root, filepath.FromSlash(antigravityPluginDir), "plugin.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return append(errs, adapter.ValidationError{
			File:    antigravityPluginDir + "/plugin.json",
			Message: "Antigravity 워크스페이스 플러그인 매니페스트가 없음",
			Level:   "error",
		})
	}
	var manifest antigravityPluginManifest
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Name == "" {
		errs = append(errs, adapter.ValidationError{
			File:    antigravityPluginDir + "/plugin.json",
			Message: "plugin.json을 파싱할 수 없거나 name이 비어 있음",
			Level:   "error",
		})
	}

	for _, route := range antigravityRequiredRoutes {
		skill := filepath.Join(
			a.root, filepath.FromSlash(antigravityPluginDir), "skills", route, "SKILL.md")
		if _, err := os.Stat(skill); os.IsNotExist(err) {
			errs = append(errs, adapter.ValidationError{
				File:    filepath.ToSlash(filepath.Join(antigravityPluginDir, "skills", route, "SKILL.md")),
				Message: fmt.Sprintf("플러그인 스킬이 없어 /%s 슬래시 커맨드가 생기지 않음", route),
				Level:   "error",
			})
		}
	}
	return errs
}

// validateCLICapabilities reports components the installed CLI cannot load.
// An absent binary is not a project defect, so no finding is produced for it —
// the adapter refuses to claim or deny support it did not observe.
func (a *Adapter) validateCLICapabilities(
	ctx context.Context,
	probeTimeout time.Duration,
) []adapter.ValidationError {
	if _, err := exec.LookPath(cliBinary); err != nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	report := ProbeAntigravityReadiness(ctx, AntigravityReadinessOptions{Timeout: probeTimeout})
	unsupported := report.Unsupported()
	if len(unsupported) == 0 {
		return nil
	}
	reasons := make(map[string]bool, len(unsupported))
	ids := make([]string, 0, len(unsupported))
	for _, capability := range unsupported {
		ids = append(ids, capability.ID)
		reasons[capability.Reason] = true
	}
	// A probe that could not observe the CLI says nothing about the project.
	if reasons["probe_failed"] || reasons["output_oversized"] {
		return nil
	}
	sort.Strings(ids)
	version := report.Version
	if version == "" {
		version = "unknown"
	}
	return []adapter.ValidationError{{
		File: antigravityPluginDir,
		Message: fmt.Sprintf(
			"설치된 %s %s에서 확인되지 않은 플러그인 구성요소: %s (검증된 최소 버전 %s)",
			cliBinary, version, strings.Join(ids, ", "), antigravityVerifiedFloor),
		Level: "warning",
	}}
}
