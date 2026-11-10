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
	TranscriptPath       string `json:"transcript_path"`
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

func newVerifyLanguageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "language",
		Short: "Stop hook: send a reply back when it is not in language.ai_responses",
		Long: "Reads the Claude Code Stop hook payload on stdin and checks the final reply against " +
			"language.ai_responses in autopus.yaml, together with every progress note written earlier in the turn. " +
			"Prose written in another language blocks the stop once, " +
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
	replies := turnReplies(in.TranscriptPath)
	if n := len(replies); n == 0 || replies[n-1] != in.LastAssistantMessage {
		replies = append(replies, in.LastAssistantMessage)
	}
	failed := -1
	for i, reply := range replies {
		if !langgate.Check(lang, reply) {
			failed = i
			break
		}
	}
	if failed < 0 {
		return stopHookOutput{}
	}
	name := langgate.Name(lang)
	if failed == len(replies)-1 {
		return stopHookOutput{
			Decision: "block",
			Reason: fmt.Sprintf("autopus.yaml sets language.ai_responses to %s, but your last reply was not written in %s. "+
				"Send that reply again in %s; keep code, commands, paths, and identifiers as they are. "+
				"Write every later reply in %s too, including short progress updates.", lang, name, name, name),
		}
	}
	return stopHookOutput{
		Decision: "block",
		Reason: fmt.Sprintf("autopus.yaml sets language.ai_responses to %s, but a progress note earlier in this turn "+
			"was not written in %s: %q. The user read it as written. Tell the user in %s, in one or two sentences, "+
			"what that note said; keep code, commands, paths, and identifiers as they are. "+
			"Write every later reply in %s too, including short progress updates.",
			lang, name, excerpt(replies[failed]), name, name),
	}
}

// excerpt shortens a reply to its first line for the block reason.
func excerpt(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if r := []rune(line); len(r) > 80 {
		return string(r[:80]) + "..."
	}
	return line
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
