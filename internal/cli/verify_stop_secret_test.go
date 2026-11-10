package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// stopFakeToken is GitHub-token-shaped, assembled at run time so no file in
// the repository holds a literal a secret scanner would flag.
func stopFakeToken() string { return "gh" + "p_" + strings.Repeat("aZ3kP9qR2mX7", 3) }

func secretEdit() string {
	return "package stopgate\n\nconst token = \"" + stopFakeToken() + "\"\n\nfunc Add(a, b int) int { return a + b }\n"
}

// A passing change that adds a credential is sent back once, naming the
// place and the rule but never the value; the same hit then lets the
// session end with a warning instead of looping.
func TestVerifyStop_SecretBlocksOnceWithoutTheValue(t *testing.T) {
	session := "secret-once"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, secretEdit())

	got := runStop(t, dir, session)
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "add.go:3 (github-pat)")
	assert.Contains(t, got.Reason, "autopus:allow-secret")
	assert.NotContains(t, got.Reason, stopFakeToken())

	again := runStop(t, dir, session)
	assert.Empty(t, again.Decision, "the same hit blocks only once")
	assert.Contains(t, again.SystemMessage, "add.go:3 (github-pat)")
	assert.NotContains(t, again.SystemMessage, stopFakeToken())
}

// A credential in a file no toolchain covers is still caught: the check
// does not depend on there being a build to run.
func TestVerifyStop_SecretInAConfigFile(t *testing.T) {
	session := "secret-config"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("ci:\n  token: "+stopFakeToken()+"\n"), 0o644))

	got := runStop(t, dir, session)
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "deploy.yaml:2 (github-pat)")
}

func TestVerifyStop_SecretOffByMarkerEnvOrConfig(t *testing.T) {
	marked := "package stopgate\n\nconst token = \"" + stopFakeToken() + "\" // autopus:allow-secret\n\nfunc Add(a, b int) int { return a + b }\n"
	off := false
	for name, row := range map[string]struct {
		edit  string
		setup func(t *testing.T, dir string)
	}{
		"marker": {marked, func(*testing.T, string) {}},
		"env":    {secretEdit(), func(t *testing.T, _ string) { t.Setenv("AUTOPUS_SECRET_STOP", "off") }},
		"config": {secretEdit(), func(t *testing.T, dir string) {
			cfg := config.DefaultFullConfig("demo")
			cfg.Hooks.SecretStop = &off
			require.NoError(t, config.Save(dir, cfg))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			session := "secret-off-" + name
			t.Cleanup(func() { resetStopState(session) })
			dir := newStopRepo(t, row.edit)
			row.setup(t, dir)
			assert.Equal(t, stopHookOutput{}, runStop(t, dir, session))
		})
	}
}
