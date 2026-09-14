package omp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

type ompSurfaceSet struct {
	file     string
	label    string
	expected map[string]bool
	actual   map[string]bool
}

// validateInstalledSurface checks the native path set and the integrity of
// every compiler-owned file. The base config is optional for a plain install.
// @AX:WARN [AUTO]: installed-surface validation contains more than eight conditional branches.
// @AX:REASON [AUTO]: cancellation, config loading, generated-surface reconstruction, native path comparison, manifest integrity, and sensitive permissions converge here.
func (a *Adapter) validateInstalledSurface(ctx context.Context) ([]adapter.ValidationError, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	findings, incomplete := a.validateBaseSurface()
	if incomplete {
		return findings, nil
	}

	cfg, err := config.LoadPreview(a.root)
	if err != nil {
		return findings, err
	}
	modelCtx := ctx
	if modelCtx == nil {
		modelCtx = context.Background()
	}
	_, err = a.prepareModelIntegration(modelCtx, cfg)
	if err != nil {
		return findings, err
	}
	rules, err := a.prepareRuleMappings()
	if err != nil {
		return findings, err
	}
	workflow, err := a.prepareWorkflowSkillMappings(cfg)
	if err != nil {
		return findings, err
	}
	extended, err := a.prepareExtendedSkillMappings(cfg)
	if err != nil {
		return findings, err
	}
	commands, err := a.prepareCommandMappings(cfg)
	if err != nil {
		return findings, err
	}
	contextMappings, err := prepareOMPContextBridgeMappings(cfg)
	if err != nil {
		return findings, err
	}

	if finding := compareOMPSurfaceSet(ompRuleSurfaceSet(a.root, rules)); finding != nil {
		findings = append(findings, *finding)
	}
	expected := make([]adapter.FileMapping, 0, len(rules)+len(workflow)+len(extended)+len(commands)+len(contextMappings))
	expected = append(expected, rules...)
	expected = append(expected, workflow...)
	expected = append(expected, extended...)
	expected = append(expected, commands...)
	expected = append(expected, contextMappings...)
	findings = append(findings, a.validateOMPExpectedMappings(expected)...)
	findings = append(findings, a.validateOMPManifestIntegrity(expected)...)
	findings = append(findings, a.validateOMPSensitivePermissions()...)
	return findings, nil
}

func (a *Adapter) validateBaseSurface() ([]adapter.ValidationError, bool) {
	checks := []struct{ path, message string }{
		{filepath.Join(".omp", "skills"), "omp skill 디렉터리가 없음"},
		{filepath.Join(".omp", "commands"), "omp command 디렉터리가 없음"},
		{filepath.FromSlash(ompRuleDir), "omp rule 디렉터리가 없음"},
	}
	var findings []adapter.ValidationError
	for _, check := range checks {
		info, err := os.Lstat(filepath.Join(a.root, check.path))
		if os.IsNotExist(err) {
			findings = append(findings, adapter.ValidationError{
				File: check.path, Message: check.message, Level: "error",
			})
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			findings = append(findings, adapter.ValidationError{
				File: check.path, Message: "expected real directory", Level: "error",
			})
		}
	}
	return findings, len(findings) > 0
}

func (a *Adapter) validateOMPExpectedMappings(mappings []adapter.FileMapping) []adapter.ValidationError {
	var findings []adapter.ValidationError
	workspace, err := openOMPRootedWorkspace(a.root)
	if err != nil {
		for _, mapping := range mappings {
			path := filepath.ToSlash(mapping.TargetPath)
			findings = append(findings, ompIntegrityFinding(path, "managed workspace is unavailable"))
		}
		return findings
	}
	defer func() { _ = workspace.Close() }()
	for _, mapping := range mappings {
		path := filepath.ToSlash(mapping.TargetPath)
		if err := adapter.RejectSymlinkComponents(a.root, path); err != nil {
			findings = append(findings,
				ompIntegrityFinding(path, "managed path must be a regular file, not a symlink"))
			continue
		}
		data, _, err := workspace.readFile(path, int64(len(mapping.Content))+1)
		if err != nil {
			findings = append(findings,
				ompIntegrityFinding(path, "managed regular file is unreadable or changed"))
			continue
		}
		if adapter.Checksum(string(data)) != mapping.Checksum {
			findings = append(findings, adapter.ValidationError{
				File: path, Message: "managed content checksum mismatch", Level: "error",
			})
		}
	}
	return findings
}

func (a *Adapter) validateOMPManifestIntegrity(expectedMappings []adapter.FileMapping) []adapter.ValidationError {
	manifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil || manifest == nil {
		return []adapter.ValidationError{{
			File:    filepath.Join(".autopus", adapterName+"-manifest.json"),
			Message: "managed manifest unavailable", Level: "error",
		}}
	}
	expected := make(map[string]bool, len(expectedMappings))
	for _, mapping := range expectedMappings {
		expected[filepath.ToSlash(mapping.TargetPath)] = true
	}
	var findings []adapter.ValidationError
	for path, entry := range manifest.Files {
		path = filepath.ToSlash(path)
		extensible := strings.HasPrefix(path, ".omp/agents/") ||
			strings.HasPrefix(path, ".omp/commands/") ||
			strings.HasPrefix(path, ".omp/skills/")
		if extensible && !expected[path] {
			findings = append(findings, ompIntegrityFinding(path, "stale managed path is not part of the expected surface"))
			continue
		}
		if err := adapter.RejectSymlinkComponents(a.root, path); err != nil {
			findings = append(findings, ompIntegrityFinding(path, "managed path must be a regular file, not a symlink"))
			continue
		}
		fullPath := filepath.Join(a.root, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if err != nil || !info.Mode().IsRegular() {
			findings = append(findings, ompIntegrityFinding(path, "managed path must be a regular file"))
			continue
		}
		if path == configFile || entry.Policy == adapter.OverwriteMarker {
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil || adapter.Checksum(string(data)) != entry.Checksum {
			findings = append(findings, ompIntegrityFinding(path, "managed content checksum mismatch"))
		}
	}
	return findings
}

func (a *Adapter) validateOMPSensitivePermissions() []adapter.ValidationError {
	paths := []string{
		configFile,
		DefaultOMPModelOverlayPath,
		OMPModelReceiptRelativePath,
		OMPModelProjectOwnershipRelativePath,
	}
	var findings []adapter.ValidationError
	for _, path := range paths {
		fullPath := filepath.Join(a.root, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			findings = append(findings, ompIntegrityFinding(path, "sensitive config must be a regular file"))
			continue
		}
		if info.Mode().Perm()&0o077 != 0 {
			findings = append(findings, ompIntegrityFinding(path, "sensitive config permission must be owner-only"))
		}
	}
	return findings
}

func ompIntegrityFinding(path, message string) adapter.ValidationError {
	return adapter.ValidationError{File: filepath.ToSlash(path), Message: message, Level: "error"}
}

func mappingSurfaceSet(root, prefix, label string, mappings []adapter.FileMapping) ompSurfaceSet {
	expected := make(map[string]bool, len(mappings))
	for _, mapping := range mappings {
		rel, err := filepath.Rel(filepath.FromSlash(prefix), mapping.TargetPath)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			expected[filepath.ToSlash(rel)] = true
		}
	}
	return ompSurfaceSet{
		file: filepath.FromSlash(prefix), label: label, expected: expected,
		actual: collectOMPRegularFiles(filepath.Join(root, filepath.FromSlash(prefix))),
	}
}

func ompRuleSurfaceSet(root string, rules []adapter.FileMapping) ompSurfaceSet {
	set := mappingSurfaceSet(root, ompRuleDir, "rules", rules)
	for name := range set.actual {
		if !strings.HasPrefix(name, ompRuleFilePrefix) {
			delete(set.actual, name)
		}
	}
	return set
}

func collectOMPRegularFiles(root string) map[string]bool {
	files := make(map[string]bool)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil {
			files[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	return files
}

func compareOMPSurfaceSet(set ompSurfaceSet) *adapter.ValidationError {
	missing, extra := ompSetDiff(set.expected, set.actual), ompSetDiff(set.actual, set.expected)
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	return &adapter.ValidationError{
		File: set.file,
		Message: fmt.Sprintf("%s expected=%d got=%d missing=[%s] extra=[%s]",
			set.label, len(set.expected), len(set.actual), strings.Join(missing, ","), strings.Join(extra, ",")),
		Level: "error",
	}
}

func ompSetDiff(left, right map[string]bool) []string {
	var values []string
	for value := range left {
		if !right[value] {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}
