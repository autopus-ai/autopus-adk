package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/langgate"
)

// languageStopInput is the part of the Claude Code Stop hook payload the
// language check reads.
type languageStopInput struct {
	Cwd                  string `json:"cwd"`
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

func newVerifyLanguageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "language",
		Short: "Stop hook: send a reply back when it is not in language.ai_responses",
		Long: "Reads the Claude Code Stop hook payload on stdin and checks the final reply against " +
			"language.ai_responses in autopus.yaml. Prose written in another language blocks the stop once, " +
			"asking for the reply again; code, inline code, and URLs are not counted. " +
			"Set AUTOPUS_LANGUAGE_STOP=off or hooks.language_stop: false to disable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := json.Marshal(runVerifyLanguage(cmd.InOrStdin()))
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}

func runVerifyLanguage(stdin io.Reader) stopHookOutput {
	if strings.EqualFold(os.Getenv("AUTOPUS_LANGUAGE_STOP"), "off") {
		return stopHookOutput{}
	}
	var in languageStopInput
	_ = json.NewDecoder(stdin).Decode(&in)
	// A reply that is already the answer to this hook is let through, so a
	// misjudged reply costs one retry, never a loop.
	if in.StopHookActive || strings.TrimSpace(in.LastAssistantMessage) == "" {
		return stopHookOutput{}
	}
	dir := in.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	cfg := nearestConfig(dir)
	if cfg == nil || !cfg.Hooks.IsLanguageStopEnabled() {
		return stopHookOutput{}
	}
	lang := cfg.Language.AIResponses
	if langgate.Check(lang, in.LastAssistantMessage) {
		return stopHookOutput{}
	}
	name := langgate.Name(lang)
	return stopHookOutput{
		Decision: "block",
		Reason: fmt.Sprintf("autopus.yaml sets language.ai_responses to %s, but your last reply was not written in %s. "+
			"Send that reply again in %s; keep code, commands, paths, and identifiers as they are. "+
			"Write every later reply in %s too, including short progress updates.", lang, name, name, name),
	}
}

// nearestConfig loads autopus.yaml from dir or its closest ancestor that has
// one, without normalizing the file on disk.
func nearestConfig(dir string) *config.HarnessConfig {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "autopus.yaml")); err == nil {
			cfg, err := config.LoadPreview(dir)
			if err != nil {
				return nil
			}
			return cfg
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}
