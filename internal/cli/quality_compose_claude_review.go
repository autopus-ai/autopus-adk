package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// claudeReviewProvider returns the claude orchestra provider with its args as
// autopus.yaml in dir writes them, without environment expansion, and the
// model those args pass with `--model`. Like the review tab (REQ-RVM-26) it
// never shows an expanded value: a model with a placeholder, args the raw
// document cannot read as written, orchestra off, a backend-routed provider,
// or args without a model give nothing to show, and so does a dir whose
// autopus.yaml cannot be read. Only a session without a project directory,
// one assembled from fakes, reads the config itself.
func claudeReviewProvider(dir string, cfg *config.HarnessConfig) (config.ProviderEntry, string, bool) {
	if cfg == nil || !cfg.Orchestra.Enabled {
		return config.ProviderEntry{}, "", false
	}
	entry, ok := cfg.Orchestra.Providers["claude"]
	if !ok {
		return config.ProviderEntry{}, "", false
	}
	if dir != "" {
		raw, viewed := readReviewRaw(dir).Providers["claude"]
		if !viewed {
			return config.ProviderEntry{}, "", false
		}
		entry.Args = raw.Args
	}
	model, ok := config.ClaudeProviderModel(entry)
	if !ok || strings.Contains(model, "${") {
		return config.ProviderEntry{}, "", false
	}
	return entry, model, true
}

// claudeReviewModelLine is the one line that names the claude review
// provider model wherever the Claude Code preset is shown, since the preset
// never changes it. It becomes a warning when the effective Claude Code
// preset tops out at another tier than that model, such as an all-opus
// preset next to claude-fable-5-1. It is "" when there is no model to name.
func claudeReviewModelLine(dir string, cfg *config.HarnessConfig, text composeText) string {
	_, model, ok := claudeReviewProvider(dir, cfg)
	if !ok {
		return ""
	}
	top := cfg.Quality.ClaudeTopTier()
	if tier, known := config.ClaudeModelTier(model); known && top != "" && tier != top {
		preset := cfg.Quality.EffectiveMode(config.QualityProviderClaude)
		return fmt.Sprintf(text.ClaudeReviewDiffers, terminalSafe(preset), top, terminalSafe(model))
	}
	return fmt.Sprintf(text.ClaudeReview, terminalSafe(model))
}

// writeClaudeReviewModelLine prints claudeReviewModelLine when it has one.
func writeClaudeReviewModelLine(out io.Writer, dir string, cfg *config.HarnessConfig, text composeText) {
	if line := claudeReviewModelLine(dir, cfg, text); line != "" {
		fmt.Fprintln(out, line)
	}
}

// writeClaudeReviewFormerDefaultNotice prints update's one notice line for a
// claude review provider still on a former shipped default model. The
// migration no longer moves that model, because a deliberate choice of the
// same argv cannot be told apart from an untouched old default; this line
// tells the operator instead, with the current default and the command that
// changes it.
func writeClaudeReviewFormerDefaultNotice(out io.Writer, dir string, cfg *config.HarnessConfig, text composeText) {
	entry, _, ok := claudeReviewProvider(dir, cfg)
	if !ok {
		return
	}
	model, former := config.ClaudeProviderFormerDefault(entry)
	current, _ := config.ClaudeProviderModel(config.DefaultClaudeProviderEntry())
	if former {
		fmt.Fprintf(out, text.ClaudeReviewFormer+"\n", terminalSafe(model), current)
	}
}
