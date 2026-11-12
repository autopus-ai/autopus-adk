package cli

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reviewArgsRowFor is the CLI Model Contract row of a provider binary, matched
// by its file name.
func reviewArgsRowFor(binary string) reviewArgsRow {
	switch strings.TrimSuffix(filepath.Base(binary), ".exe") {
	case "claude":
		return reviewClaudeRow
	case "codex":
		return reviewCodexRow
	case "agy":
		return reviewAgyRow
	}
	return reviewArgsRow{kind: reviewRowOther}
}

// reviewCLIModel is one model a CLI row offers: the value written, the
// efforts it takes (none for agy ids, which carry theirs), and
// for agy the group it belongs to and its display name.
type reviewCLIModel struct {
	ID      string
	Efforts []string
	Base    string
	Suffix  string
	Display string
}

// reviewClaudeModels is the claude row: the Claude tab's model of every tier,
// with the low–max efforts for every model, Haiku 5.5 included (REQ-RVM-21).
func reviewClaudeModels() []reviewCLIModel {
	models := make([]reviewCLIModel, 0, 4)
	for _, tier := range []string{"fable", "opus", "sonnet", "haiku"} {
		models = append(models, reviewCLIModel{ID: config.ClaudeModelForTier(tier), Efforts: config.QualityEfforts()})
	}
	return models
}

// reviewCodexSlug is a Codex catalog slug the tab offers: one that reads as
// a model name, never as a flag, a path climb, or control characters.
var reviewCodexSlug = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)

// reviewCodexModels is the codex row: the run's Codex catalog snapshot models
// that report at least one of low–max, by name, each with those levels in
// ladder order (REQ-RVM-22). A model whose levels miss the ladder entirely,
// or whose slug does not read as a model name, is left out. It reports false
// for a snapshot it cannot read.
func reviewCodexModels(snapshot driftGenerationSnapshot) ([]reviewCLIModel, bool) {
	if !snapshot.codexCatalogFixed || len(snapshot.codexCatalog) == 0 {
		return nil, false
	}
	catalog, err := config.ParseCodexModelCatalog(snapshot.codexCatalog)
	if err != nil {
		return nil, false
	}
	var models []reviewCLIModel
	for _, entry := range catalog.Models {
		if !reviewCodexSlug.MatchString(entry.Slug) {
			continue
		}
		model := reviewCLIModel{ID: entry.Slug}
		for _, level := range config.QualityEfforts() {
			if slices.ContainsFunc(entry.SupportedReasoningLevels, func(reported config.CodexCatalogReasoningLevel) bool {
				return reported.Effort == level
			}) {
				model.Efforts = append(model.Efforts, level)
			}
		}
		if len(model.Efforts) > 0 && !slices.ContainsFunc(models, func(m reviewCLIModel) bool { return m.ID == model.ID }) {
			models = append(models, model)
		}
	}
	slices.SortFunc(models, func(a, b reviewCLIModel) int { return strings.Compare(a.ID, b.ID) })
	return models, true
}

// reviewMigratedConfig is a detached copy of cfg after
// config.MigrateOrchestraConfig: the args a CLI provider runs with, which the
// tab decides current models on (REQ-RVM-25). `auto quality` loads without
// that migration, so its own config can hold args the runtime never runs.
func reviewMigratedConfig(cfg *config.HarnessConfig) *config.HarnessConfig {
	migrated, err := cloneHarnessConfig(cfg)
	if err != nil || migrated == nil {
		return cfg
	}
	if _, err := config.MigrateOrchestraConfig(migrated); err != nil {
		return cfg
	}
	return migrated
}

// reviewCLIKeepText names a CLI value in a keep label: `<model> · <effort>`,
// or the binary when no model flag is set.
func reviewCLIKeepText(binary string, value reviewCLIValue) string {
	if value.Model == "" {
		return binary
	}
	if value.Effort == "" {
		return value.Model
	}
	return value.Model + " · " + value.Effort
}

// reviewCLICellText is the after table's model cell of a CLI provider:
// `<binary> <model> · <effort>`, or the binary alone.
func reviewCLICellText(binary string, value reviewCLIValue) string {
	if value.Model == "" {
		return binary
	}
	return binary + " " + reviewCLIKeepText(binary, value)
}
