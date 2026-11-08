package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func languageProject(t *testing.T, lang string, languageStop *bool) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("demo")
	cfg.Language.AIResponses = lang
	cfg.Hooks.LanguageStop = languageStop
	require.NoError(t, config.Save(dir, cfg))
	sub := filepath.Join(dir, "pkg", "x")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	return sub
}

func languageInput(t *testing.T, cwd, message string, active bool) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(languageStopInput{Cwd: cwd, LastAssistantMessage: message, StopHookActive: active})
	require.NoError(t, err)
	return strings.NewReader(string(data))
}

// The project's autopus.yaml is found from a subdirectory, and an English
// reply under ai_responses: ko is sent back once with the reason.
func TestRunVerifyLanguage_BlocksAReplyOutsideTheConfiguredLanguage(t *testing.T) {
	cwd := languageProject(t, "ko", nil)
	english := "Waiting for the internal/cli result while the suite runs."

	out := runVerifyLanguage(languageInput(t, cwd, english, false))
	assert.Equal(t, "block", out.Decision)
	assert.Contains(t, out.Reason, "Korean")

	assert.Empty(t, runVerifyLanguage(languageInput(t, cwd, "internal/cli 결과를 기다리는 중입니다.", false)).Decision)
	assert.Empty(t, runVerifyLanguage(languageInput(t, cwd, english, true)).Decision, "the retry is never blocked again")

	t.Setenv("AUTOPUS_LANGUAGE_STOP", "off")
	assert.Empty(t, runVerifyLanguage(languageInput(t, cwd, english, false)).Decision)
}

func TestRunVerifyLanguage_LetsThroughWhenNothingToCheck(t *testing.T) {
	english := "Waiting for the internal/cli result while the suite runs."
	off := false
	for name, cwd := range map[string]string{
		"english":  languageProject(t, "en", nil),
		"disabled": languageProject(t, "ko", &off),
	} {
		assert.Empty(t, runVerifyLanguage(languageInput(t, cwd, english, false)).Decision, name)
	}
	assert.Empty(t, runVerifyLanguage(languageInput(t, t.TempDir(), english, false)).Decision, "no autopus.yaml")
}
