package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
)

func loadConfigForUpdatePreview(dir string) (*config.HarnessConfig, bool, error) {
	return config.LoadPreviewWithMetadata(dir)
}

func prepareUpdatePreviewConfig(
	dir string,
	cfg *config.HarnessConfig,
	platformNamesMigrated bool,
	designConfigMissing bool,
	yesFlag bool,
	interactive bool,
) (*config.HarnessConfig, []string, error) {
	previewCfg, err := cloneHarnessConfig(cfg)
	if err != nil {
		return nil, nil, err
	}

	var reasons []string
	if platformNamesMigrated {
		reasons = appendConfigPreviewReason(reasons, "legacy platform names would be normalized in autopus.yaml")
	}
	if designConfigMissing && previewCfg.Design.Enabled {
		reasons = appendConfigPreviewReason(reasons, "design defaults would be persisted in autopus.yaml")
	}
	if retired := retiredConfigKeysInFile(dir); len(retired) > 0 {
		reasons = appendConfigPreviewReason(reasons,
			"retired orchestra keys would be removed from autopus.yaml: "+terminalSafe(strings.Join(retired, ", ")))
	}

	if changed, reason := previewLanguagePreview(previewCfg, yesFlag, interactive); changed {
		reasons = appendConfigPreviewReason(reasons, reason)
	}
	if changed, reason := previewIsolateRules(dir, previewCfg, yesFlag, interactive); changed {
		reasons = appendConfigPreviewReason(reasons, reason)
	}

	return previewCfg, reasons, nil
}

func previewLanguagePreview(cfg *config.HarnessConfig, yesFlag bool, interactive bool) (bool, string) {
	if cfg == nil || yesFlag {
		return false, ""
	}

	missing := missingLanguageFields(cfg)
	if len(missing) == 0 {
		return false, ""
	}

	if interactive {
		return true, "interactive apply may persist language settings for " + strings.Join(missing, ", ")
	}

	for _, field := range missing {
		switch field {
		case "comments":
			cfg.Language.Comments = "en"
		case "commits":
			cfg.Language.Commits = "en"
		case "ai_responses":
			cfg.Language.AIResponses = "en"
		}
	}

	return true, "non-interactive apply would persist default language settings for " + strings.Join(missing, ", ")
}

func previewIsolateRules(dir string, cfg *config.HarnessConfig, yesFlag bool, interactive bool) (bool, string) {
	if cfg == nil || cfg.IsolateRules {
		return false, ""
	}

	conflicts := detect.CheckParentRuleConflicts(dir)
	if len(conflicts) == 0 {
		return false, ""
	}

	if yesFlag || !interactive {
		cfg.IsolateRules = true
		return true, "parent rule conflicts would set isolate_rules: true during apply"
	}

	return true, "interactive apply may persist isolate_rules: true after parent-rule confirmation"
}

func missingLanguageFields(cfg *config.HarnessConfig) []string {
	if cfg == nil {
		return nil
	}

	var fields []string
	if strings.TrimSpace(cfg.Language.Comments) == "" {
		fields = append(fields, "comments")
	}
	if strings.TrimSpace(cfg.Language.Commits) == "" {
		fields = append(fields, "commits")
	}
	if strings.TrimSpace(cfg.Language.AIResponses) == "" {
		fields = append(fields, "ai_responses")
	}
	return fields
}

func appendConfigPreviewItem(items []previewItem, dir string, reasons []string) []previewItem {
	if len(reasons) == 0 {
		return items
	}

	kind := "update"
	if _, err := os.Stat(filepath.Join(dir, "autopus.yaml")); os.IsNotExist(err) {
		kind = "create"
	}

	return append(items, previewItem{
		Path:     "autopus.yaml",
		Kind:     kind,
		Category: "config",
		Reason:   strings.Join(reasons, "; "),
	})
}

func appendConfigPreviewReason(reasons []string, reason string) []string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reasons
	}
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
