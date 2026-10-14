package config

// Security re-check: a load that fails must not print the value of a
// placeholder. The decoder quotes the key or value it rejects, Validate
// quotes the value it rejects, and the retired-key notice names the paths it
// pruned, so a file could otherwise make a CI log print any variable of the
// environment. Load, ParseConfig, and LoadLoreSection spell such a value as
// the placeholder that put it there, and refuse an explicit tag on a
// placeholder, since a tag makes the decoder convert or retype the value.

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	spellSecret = "sk-proj-Spell123secret"
	spellHead   = "mode: full\nproject_name: spell\nplatforms: [claude-code]\n"
)

func writeSpellConfig(t *testing.T, text string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), []byte(text), 0o644))
	return dir
}

func assertSpelled(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), want)
	assert.NotContains(t, err.Error(), "Spell123")
	assert.NotContains(t, err.Error(), "sk-proj", "the decoder cuts a long value to its first seven bytes")
}

func TestLoad_SpellsThePlaceholderOfAValueTheErrorWouldQuote(t *testing.T) {
	t.Setenv("SPELL_SECRET", spellSecret)
	t.Setenv("SPELL_SHORT", "Zq9xWv")
	policy := "role_model_policy:\n  version: v1\n"
	for name, row := range map[string]struct{ text, want string }{
		"unknown root key": {spellHead + "${SPELL_SECRET}: x\n",
			"line 4: field ${SPELL_SECRET} not found in type config.HarnessConfig"},
		"unknown nested key": {spellHead + "orchestra:\n  providers:\n    claude:\n      binary: claude\n      ${SPELL_SECRET}: x\n",
			"line 8: orchestra.providers.claude: field ${SPELL_SECRET} not found in type config.ProviderEntry"},
		"value of the wrong type": {spellHead + "workflow:\n  coverage_threshold: ${SPELL_SECRET}\n",
			"line 5: workflow.coverage_threshold: cannot unmarshal !!str `${SPELL_SECRET}` into int"},
		"short value of the wrong type": {spellHead + "workflow:\n  coverage_threshold: ${SPELL_SHORT}\n",
			"line 5: workflow.coverage_threshold: cannot unmarshal !!str `${SPELL_SHORT}` into int"},
		"block scalar of the wrong type": {spellHead + "workflow:\n  coverage_threshold: |\n    ${SPELL_SECRET}\n",
			"line 5: workflow.coverage_threshold: cannot unmarshal !!str `${SPELL_SECRET}\\n` into int"},
		"alias of the wrong type": {spellHead + "future_extension:\n  a: &a ${SPELL_SECRET}\nworkflow:\n  coverage_threshold: *a\n",
			"line 5: future_extension.a: cannot unmarshal !!str `${SPELL_SECRET}` into int"},
		"complex key of an override": {spellHead + "profiles:\n  executor:\n    override:\n      a:\n        b:\n" +
			"          ? [${SPELL_SECRET}]\n          : v\n", "invalid map key: []interface {}{${SPELL_SECRET}}"},
		"root profile": {spellHead + policy + "  profile: ${SPELL_SECRET}\n",
			"validate config: role_model_policy.profile_unknown: ${SPELL_SECRET}"},
		"root family": {spellHead + policy + "  profile: balanced\n  family: ${SPELL_SECRET}\n",
			"validate config: role_model_policy.family_invalid: ${SPELL_SECRET}"},
		"policy version": {spellHead + "role_model_policy:\n  version: ${SPELL_SECRET}\n  profile: balanced\n",
			"validate config: role_model_policy.policy_version_unknown: ${SPELL_SECRET}"},
		"mode": {strings.Replace(spellHead, "mode: full", "mode: ${SPELL_SECRET}", 1),
			"validate config: invalid mode ${SPELL_SECRET}: must be 'full'"},
		"part of a platform": {strings.Replace(spellHead, "[claude-code]", "[pre-${SPELL_SECRET}]", 1),
			"validate config: invalid platform pre-${SPELL_SECRET}"},
		"provider key printed bare": {spellHead + "orchestra:\n  providers:\n    ${SPELL_SECRET}:\n      backend: bogus\n      binary: x\n",
			"validate config: provider_backend_invalid: ${SPELL_SECRET}"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeSpellConfig(t, row.text))

			assertSpelled(t, err, row.want)
		})
	}
}

func TestParseConfigAndLoadLoreSection_SpellThePlaceholderOfAQuotedValue(t *testing.T) {
	t.Setenv("SPELL_SECRET", spellSecret)
	lore := spellHead + "lore:\n  stale_threshold_days: ${SPELL_SECRET}\n"
	_, parseErr := ParseConfig([]byte(lore))
	_, loreErr := LoadLoreSection(writeSpellConfig(t, lore))
	_, profileErr := ParseConfig([]byte(spellHead + "role_model_policy:\n  version: v1\n  profile: ${SPELL_SECRET}\n"))

	for _, err := range []error{parseErr, loreErr} {
		assertSpelled(t, err, "line 5: lore.stale_threshold_days: cannot unmarshal !!str `${SPELL_SECRET}` into int")
	}
	assertSpelled(t, profileErr, "validate config: role_model_policy.profile_unknown: ${SPELL_SECRET}")
}

func TestLoad_ReportsARetiredPathWithItsPlaceholder(t *testing.T) {
	t.Setenv("SPELL_SECRET", spellSecret)
	var reported []string
	previous := SetRetiredKeyReporter(func(paths []string) { reported = append(reported, paths...) })
	t.Cleanup(func() { SetRetiredKeyReporter(previous) })

	_, err := Load(writeSpellConfig(t, spellHead+"orchestra:\n  providers:\n    ${SPELL_SECRET}:\n      binary: x\n      pane_args: [y]\n"))

	require.NoError(t, err)
	assert.Equal(t, []string{"orchestra.providers.${SPELL_SECRET}.pane_args"}, reported)
}

// A value the file spells itself is no secret of the environment, so an
// error that quotes it keeps it: here the platform, not the placeholder,
// is what Validate rejects.
func TestLoad_KeepsAValueTheFileSpellsItself(t *testing.T) {
	t.Setenv("SPELL_BINARY", "bogus-cli")
	text := strings.Replace(spellHead, "[claude-code]", "[bogus-cli]", 1) +
		"orchestra:\n  providers:\n    claude:\n      binary: ${SPELL_BINARY}\n"

	_, err := Load(writeSpellConfig(t, text))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `validate config: invalid platform "bogus-cli"`)
}

func TestLoad_RefusesAnExplicitTagOnAPlaceholder(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(spellSecret))
	t.Setenv("SPELL_SECRET", spellSecret)
	t.Setenv("SPELL_BASE64", encoded)
	for name, row := range map[string]struct{ text, want string }{
		"binary value": {strings.Replace(spellHead, "mode: full", "mode: !!binary ${SPELL_BASE64}", 1),
			"config.placeholder_tag_unsupported: ${SPELL_BASE64} at mode carries the tag !!binary"},
		"int value": {spellHead + "workflow:\n  coverage_threshold: !!int ${SPELL_SECRET}\n",
			"config.placeholder_tag_unsupported: ${SPELL_SECRET} at workflow.coverage_threshold carries the tag !!int"},
		"binary key": {spellHead + "orchestra:\n  providers:\n    !!binary ${SPELL_BASE64}: {binary: x}\n",
			"config.placeholder_tag_unsupported: ${SPELL_BASE64} at orchestra.providers.${SPELL_BASE64} carries the tag !!binary"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeSpellConfig(t, row.text))

			assertSpelled(t, err, row.want)
			assert.NotContains(t, err.Error(), encoded)
		})
	}
}

func TestLoad_KeepsAStringTagOnAPlaceholder(t *testing.T) {
	t.Setenv("SPELL_NAME", "tagged")

	cfg, err := Load(writeSpellConfig(t, strings.Replace(spellHead, "project_name: spell", "project_name: !!str ${SPELL_NAME}", 1)))

	require.NoError(t, err)
	assert.Equal(t, "tagged", cfg.ProjectName)
}
