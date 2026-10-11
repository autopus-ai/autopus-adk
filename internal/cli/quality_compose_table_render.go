package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeTableLayout is the table as text cells before padding. Every cell is
// already terminal-safe.
type composeTableLayout struct {
	header []string
	rows   [][]string
	footer []string
}

// render writes the table. With before, every cell whose raw value differs
// from before's cell for the same agent and column carries a trailing `*`;
// before must come from the same binary and the same snapshot.
func (t composeTable) render(w io.Writer, lang string, before *composeTable) error {
	return writeComposeTableLayout(w, t.layout(lang, before))
}

func (t composeTable) layout(lang string, before *composeTable) composeTableLayout {
	labels := composeTableLabelsFor(lang)
	layout := composeTableLayout{header: []string{labels.group, labels.agent, labels.tier}}
	for _, platform := range t.platforms {
		layout.header = append(layout.header, t.platformHeader(platform, labels))
	}
	previous := make(map[string]composeTableRow)
	if before != nil {
		for _, row := range before.rows {
			previous[row.agent] = row
		}
	}
	for _, row := range t.rows {
		label := composeRoleGroups[row.group].Label(lang)
		old, compared := previous[row.agent]
		mark := func(text string, changed bool) string {
			if compared && changed {
				return text + "*"
			}
			return text
		}
		cells := []string{label, row.agent, mark(composeTableTierText(row.tier, labels), row.tier.tier != old.tier.tier)}
		for _, platform := range t.platforms {
			cells = append(cells, mark(composeTableCellText(row, platform, labels), composeTableCellChanged(row, old, platform)))
		}
		layout.rows = append(layout.rows, cells)
	}
	layout.footer = t.footer(labels, before != nil)
	return escapeComposeTableLayout(layout)
}

func (t composeTable) platformHeader(platform string, labels composeTableLabels) string {
	switch platform {
	case "claude-code":
		return composeTableOverrideHeader("Claude Code", t.claudeOverride, labels)
	case "codex":
		return composeTableOverrideHeader("Codex", t.codexOverride, labels)
	case "omp":
		if t.ompRoutes {
			return "OMP (" + labels.ompRequested + ")"
		}
		return "OMP"
	case "opencode":
		return "OpenCode"
	default:
		return "Antigravity"
	}
}

func composeTableOverrideHeader(name, override string, labels composeTableLabels) string {
	if override == "" {
		return name
	}
	return name + " (" + fmt.Sprintf(labels.override, override) + ")"
}

func (t composeTable) footer(labels composeTableLabels, marked bool) []string {
	var footer []string
	if t.shows("codex") {
		if t.catalogRead {
			footer = append(footer, fmt.Sprintf(labels.catalogRead, t.codexFallbacks()))
		} else {
			footer = append(footer, labels.catalogUnread)
		}
		if t.supervisorManaged {
			footer = append(footer, fmt.Sprintf(labels.supervisor,
				composeTableCodexText(composeTableCodexCell{resolution: t.supervisor}, labels)))
		}
		if t.codexRefused() {
			footer = append(footer, labels.codexFails)
		}
	}
	if !t.usesAgentModels() {
		footer = append(footer, labels.noAgentModels)
	}
	if marked {
		footer = append(footer, labels.changedLegend)
	}
	return footer
}

func composeTableCellText(row composeTableRow, platform string, labels composeTableLabels) string {
	switch platform {
	case "claude-code":
		return strings.TrimPrefix(row.claude.model, "claude-") + " · " + composeTableEffortText(row.claude.effort)
	case "codex":
		return composeTableCodexText(row.codex, labels)
	case "omp":
		return composeTableOMPText(row.omp, labels)
	default:
		return labels.platformDefault
	}
}

// composeTableCellChanged compares raw values: what the generator writes, not how
// the cell reads. OpenCode and Antigravity bind no model, so they never change.
func composeTableCellChanged(row, old composeTableRow, platform string) bool {
	switch platform {
	case "claude-code":
		return row.claude != old.claude
	case "codex":
		if row.codex.refused || old.codex.refused {
			return row.codex.refused != old.codex.refused
		}
		return row.codex.resolution.Effective != old.codex.resolution.Effective
	case "omp":
		return row.omp != old.omp
	default:
		return false
	}
}

func composeTableTierText(cell composeTableTierCell, labels composeTableLabels) string {
	switch cell.source {
	case composeTableTierDefault:
		return fmt.Sprintf(labels.tierDefault, cell.tier)
	case composeTableTierInvalid:
		return fmt.Sprintf(labels.tierInvalid, cell.tier)
	default:
		return cell.tier
	}
}

// composeTableCodexText applies the Codex notation of the Table Cell Contract to
// one resolution.
func composeTableCodexText(cell composeTableCodexCell, labels composeTableLabels) string {
	if cell.refused {
		return labels.refused
	}
	resolution := cell.resolution
	if resolution.Effective.Model == "" {
		return labels.codexDefault
	}
	text := strings.TrimPrefix(resolution.Effective.Model, "gpt-") + " · " + composeTableEffortText(resolution.Effective.Effort)
	switch resolution.Reason {
	case config.CodexResolutionCatalogUnknown:
		if resolution.Fallback {
			return text + " " + labels.noCatalog
		}
		return text + " " + labels.unverified
	case config.CodexResolutionModelUnavailable, config.CodexResolutionEffortUnavailable:
		return text + " " + labels.substituted
	default:
		return text
	}
}

func composeTableOMPText(cell composeTableOMPCell, labels composeTableLabels) string {
	switch {
	case cell.unresolved:
		return labels.routeError
	case cell.native == "":
		return labels.platformDefault
	}
	model := cell.selector
	if _, bare, found := strings.Cut(model, "/"); found {
		model = bare
	}
	model = strings.TrimPrefix(strings.TrimPrefix(model, "claude-"), "gpt-")
	if cell.thinking == "" {
		return cell.native + " · " + model
	}
	return cell.native + " · " + model + ":" + cell.thinking
}

func composeTableEffortText(effort string) string {
	if effort == "" {
		return "-"
	}
	return effort
}

// escapeComposeTableLayout makes every cell terminal-safe. Preset names,
// custom OMP selectors, and the catalog come from files the operator may not
// have written, so none of them may move the cursor or recolor the screen.
func escapeComposeTableLayout(layout composeTableLayout) composeTableLayout {
	escape := func(cells []string) []string {
		for i, cell := range cells {
			cells[i] = terminalSafe(cell)
		}
		return cells
	}
	layout.header = escape(layout.header)
	for i := range layout.rows {
		layout.rows[i] = escape(layout.rows[i])
	}
	layout.footer = escape(layout.footer)
	return layout
}

// writeComposeTableLayout pads every column to its widest cell, measured in
// terminal cells so Korean text stays aligned, with two spaces between
// columns and no trailing blanks.
func writeComposeTableLayout(w io.Writer, layout composeTableLayout) error {
	widths := make([]int, len(layout.header))
	lines := append([][]string{layout.header}, layout.rows...)
	for _, cells := range lines {
		for i, cell := range cells {
			widths[i] = max(widths[i], composeTableTextWidth(cell))
		}
	}
	var b strings.Builder
	for _, cells := range lines {
		for i, cell := range cells {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(cell)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-composeTableTextWidth(cell)))
			}
		}
		b.WriteByte('\n')
	}
	for _, line := range layout.footer {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// composeTableTextWidth counts terminal cells: East Asian wide runes (Hangul, CJK,
// fullwidth forms) take two, everything else one. Ambiguous-width runes such
// as `·` count as narrow, which is how terminals render them by default.
func composeTableTextWidth(s string) int {
	width := 0
	for _, r := range s {
		width++
		if r >= 0x1100 && r <= 0x115F || r >= 0x2E80 && r <= 0xA4CF && r != 0x303F ||
			r >= 0xAC00 && r <= 0xD7A3 || r >= 0xF900 && r <= 0xFAFF || r >= 0xFE30 && r <= 0xFE4F ||
			r >= 0xFF00 && r <= 0xFF60 || r >= 0xFFE0 && r <= 0xFFE6 {
			width++
		}
	}
	return width
}
