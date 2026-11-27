package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/insajin/autopus-adk/pkg/langgate"
)

// noteTranscriptTail bounds the read of the note check, which runs after
// every tool call: the note it judges was written just before that call.
const noteTranscriptTail = 512 << 10

// languageNoteInput is the part of the PostToolUse payload the check reads.
type languageNoteInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
}

// runLanguageNote returns a reminder when the latest progress note of the
// turn is not in language.ai_responses, or "". The Stop hook catches such a
// note only when the turn ends, after the agent has kept writing in the
// same language for every note since; this catches the first one. It never
// blocks, and it reminds once per note.
func runLanguageNote(stdin io.Reader) string {
	if strings.EqualFold(os.Getenv("AUTOPUS_LANGUAGE_STOP"), "off") {
		return ""
	}
	var in languageNoteInput
	_ = json.NewDecoder(stdin).Decode(&in)
	dir := in.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}
	cfg := nearestConfig(dir)
	if cfg == nil || !cfg.Hooks.IsLanguageStopEnabled() || !langgate.Supported(cfg.Language.AIResponses) {
		return ""
	}
	replies := turnRepliesTail(in.TranscriptPath, noteTranscriptTail)
	if len(replies) == 0 {
		return ""
	}
	last := replies[len(replies)-1]
	lang := cfg.Language.AIResponses
	if strings.TrimSpace(last) == "" || langgate.Check(lang, last) {
		return ""
	}
	sum := sha256.Sum256([]byte(last))
	key := hex.EncodeToString(sum[:8])
	if readStopState(in.SessionID, "langnote") == key {
		return ""
	}
	writeStopState(in.SessionID, "langnote", key)
	name := langgate.Name(lang)
	return fmt.Sprintf("autopus.yaml sets language.ai_responses to %s, and the note you just wrote is not in %s: %q. "+
		"The user reads notes as you write them. Write the next note, and every later one, in %s; keep code, "+
		"commands, paths, and identifiers as they are.", lang, name, excerpt(last), name)
}

// writeLanguageNote prints the PostToolUse output that adds the reminder to
// the agent's context, or nothing.
func writeLanguageNote(w io.Writer, reminder string) error {
	if reminder == "" {
		return nil
	}
	return json.NewEncoder(w).Encode(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName": "PostToolUse", "additionalContext": reminder,
	}})
}
