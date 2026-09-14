package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// backupCopies returns every backed-up path (relative to the backup root) found
// under .autopus/backup/.
func backupCopies(t *testing.T, root string) map[string]bool {
	t.Helper()
	base := filepath.Join(root, ".autopus", "backup")
	found := make(map[string]bool)
	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			return relErr
		}
		// Strip the timestamped backup session directory.
		parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
		if len(parts) == 2 {
			found[parts[1]] = true
			return nil
		}
		found[filepath.ToSlash(rel)] = true
		return nil
	})
	require.NoError(t, err)
	return found
}

// TestOMPAcceptance_S13_UserOwnedOMPSurfacePreserved covers REQ-017. Ownership
// inside `.omp/rules` is per file, not per directory: only the `autopus-`
// prefixed files the manifest records belong to the ADK, everything else in that
// directory is the user's.
func TestOMPAcceptance_S13_UserOwnedOMPSurfacePreserved(t *testing.T) {
	dir := generateOMPOnly(t)

	userRule := filepath.Join(dir, ompRuleDir, "mine.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(userRule), 0o755))
	require.NoError(t, os.WriteFile(userRule, []byte("# user rule\n"), 0o644))
	userRulesMD := filepath.Join(dir, ".omp", "RULES.md")
	require.NoError(t, os.WriteFile(userRulesMD, []byte("# sticky rules\n"), 0o644))

	managedRule := filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md")
	require.FileExists(t, managedRule, "the managed rule must exist before Clean")

	// User edits one managed command; another stays untouched.
	editedCommand := filepath.Join(dir, ".omp", "commands", "auto.md")
	require.NoError(t, os.WriteFile(editedCommand, []byte("# hand edited\n"), 0o644))
	untouchedCommand := filepath.Join(dir, ".omp", "commands", "auto-plan.md")
	require.FileExists(t, untouchedCommand)

	// `auto platform remove omp` drops omp from the platform list before Clean.
	remaining := config.DefaultFullConfig("omp-acceptance")
	remaining.Platforms = []string{"claude-code"}
	require.NoError(t, config.Save(dir, remaining))

	require.NoError(t, NewWithRoot(dir).Clean(context.Background()))

	survivingRule, err := os.ReadFile(userRule)
	require.NoError(t, err, ".omp/rules/mine.md must survive")
	assert.Equal(t, "# user rule\n", string(survivingRule),
		"a user file sharing the rule directory survives byte-identically")
	survivingRulesMD, err := os.ReadFile(userRulesMD)
	require.NoError(t, err, ".omp/RULES.md must survive")
	assert.Equal(t, "# sticky rules\n", string(survivingRulesMD))
	assert.NoFileExists(t, managedRule,
		"a manifest-recorded .omp/rules/autopus-*.md must be removed")
	assert.DirExists(t, filepath.Join(dir, ".omp"), ".omp/ must not be removed wholesale")
	assert.DirExists(t, filepath.Join(dir, ompRuleDir),
		".omp/rules/ survives while it still holds a user file")

	backups := backupCopies(t, dir)
	assert.True(t, backups[".omp/commands/auto.md"],
		"a user-edited managed file must be backed up before removal, found backups: %v", backups)
	assert.False(t, backups[".omp/commands/auto-plan.md"],
		"an unmodified managed file must be removed without a backup")

	assert.NoFileExists(t, editedCommand)
	assert.NoFileExists(t, untouchedCommand)
}

func TestOMPAcceptance_UserBaseConfigSurvivesUpdateUnchanged(t *testing.T) {
	dir := generateOMPOnly(t)
	cfgPath := filepath.Join(dir, configFile)
	original := []byte("disabledProviders:\n  - anthropic\n")
	require.NoError(t, os.WriteFile(cfgPath, original, 0o600))

	cfg := config.DefaultFullConfig("omp-acceptance")
	cfg.Platforms = []string{"omp"}
	_, err := NewWithRoot(dir).Update(context.Background(), cfg)
	require.NoError(t, err)

	updated, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, original, updated)
	assert.NotContains(t, manifestPaths(t, dir), configFile)
}
