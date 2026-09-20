package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// Validate checks the validity of installed OpenCode files.
func (a *Adapter) Validate(_ context.Context) ([]adapter.ValidationError, error) {
	var errs []adapter.ValidationError

	data, err := os.ReadFile(filepath.Join(a.root, "AGENTS.md"))
	if err != nil {
		errs = append(errs, adapter.ValidationError{File: "AGENTS.md", Message: "AGENTS.md를 읽을 수 없음", Level: "error"})
		return errs, nil
	}
	content := string(data)
	if !strings.Contains(content, markerBegin) || !strings.Contains(content, markerEnd) {
		errs = append(errs, adapter.ValidationError{File: "AGENTS.md", Message: "AUTOPUS 마커 섹션이 없음", Level: "warning"})
	}

	checks := []struct {
		path    string
		message string
		level   string
	}{
		{configFile, "opencode.json이 없음", "error"},
		{filepath.Join(".opencode", "commands", "auto.md"), "OpenCode router command가 없음", "error"},
		{filepath.Join(".opencode", "agents"), "OpenCode agent 디렉터리가 없음", "error"},
		{filepath.Join(".opencode", "rules", "autopus"), "OpenCode rule 디렉터리가 없음", "error"},
		{filepath.Join(".opencode", "plugins", "autopus-hooks.js"), "OpenCode hook plugin이 없음", "warning"},
		{filepath.Join(".agents", "skills", "auto", "SKILL.md"), "Autopus router skill이 없음", "warning"},
	}
	for _, check := range checks {
		if _, statErr := os.Stat(filepath.Join(a.root, check.path)); os.IsNotExist(statErr) {
			errs = append(errs, adapter.ValidationError{File: check.path, Message: check.message, Level: check.level})
		}
	}

	configDoc, err := readJSONObject(filepath.Join(a.root, configFile))
	if err == nil {
		for _, plugin := range managedPluginPaths(nil) {
			if managedPluginRegistered(configDoc, plugin, a.isV2(), a.root) {
				continue
			}
			errs = append(errs, adapter.ValidationError{
				File:    configFile,
				Message: fmt.Sprintf("OpenCode hook plugin 등록 누락: %s", plugin),
				Level:   "warning",
			})
		}
	}

	a.validateContext7Rule(&errs)

	return errs, nil
}

// Clean removes files created by this adapter.
func (a *Adapter) Clean(_ context.Context) error {
	manifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil {
		return err
	}
	if manifest != nil {
		for path := range manifest.Files {
			if path == "AGENTS.md" || strings.HasPrefix(path, filepath.Join(".git", "hooks")) {
				continue
			}
			if removeErr := os.RemoveAll(filepath.Join(a.root, path)); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("%s 제거 실패: %w", path, removeErr)
			}
		}
		_ = os.Remove(filepath.Join(a.root, ".autopus", adapterName+"-manifest.json"))
	}

	if err := os.RemoveAll(filepath.Join(a.root, ".opencode")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf(".opencode 제거 실패: %w", err)
	}
	for _, spec := range workflowSpecs {
		if err := os.RemoveAll(filepath.Join(a.root, ".agents", "skills", spec.Name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf(".agents/skills/%s 제거 실패: %w", spec.Name, err)
		}
	}

	agentsPath := filepath.Join(a.root, "AGENTS.md")
	data, readErr := os.ReadFile(agentsPath)
	if readErr == nil {
		cleaned := removeMarkerSection(string(data))
		if cleaned == "" {
			_ = os.Remove(agentsPath)
		} else {
			if err := os.WriteFile(agentsPath, []byte(cleaned), 0644); err != nil {
				return fmt.Errorf("AGENTS.md 정리 실패: %w", err)
			}
		}
	}

	return nil
}

func (a *Adapter) validateContext7Rule(errs *[]adapter.ValidationError) {
	ruleRel := filepath.Join(".opencode", "rules", "autopus", "context7-docs.md")
	data, err := os.ReadFile(filepath.Join(a.root, ruleRel))
	if err != nil {
		if os.IsNotExist(err) {
			*errs = append(*errs, adapter.ValidationError{
				File:    ruleRel,
				Message: "OpenCode Context7 규칙 파일이 없음",
				Level:   "warning",
			})
			return
		}
		*errs = append(*errs, adapter.ValidationError{
			File:    ruleRel,
			Message: "OpenCode Context7 규칙 파일을 읽을 수 없음",
			Level:   "warning",
		})
		return
	}

	content := string(data)
	if !strings.Contains(content, "Context7 MCP") || !strings.Contains(content, "web search") {
		*errs = append(*errs, adapter.ValidationError{
			File:    ruleRel,
			Message: "OpenCode Context7 규칙에 web fallback 계약이 없음",
			Level:   "warning",
		})
	}
}
