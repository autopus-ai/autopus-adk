package config

// Security re-check NEW-1: the loader expands ${NAME} in the raw text of
// autopus.yaml before YAML parses it, so a value could add keys, items, or a
// whole block, or turn a scalar into an alias, an anchor, a tagged value, or
// a merge key, even on one line. Load, ParseConfig, and LoadLoreSection
// refuse a value that does more than fill the scalar it stands in, and the
// refusal names the variable, never the value.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	shapeSecret = "sk-proj-Shape123secret"
	shapeHead   = "mode: full\nproject_name: shape\nplatforms: [claude-code]\n"
	shapeArgs   = "orchestra:\n  providers:\n    claude:\n      binary: claude\n" +
		"      args: [\"--print\", \"${SHAPE_VALUE}\"]\n"
	shapeBinary  = "orchestra:\n  providers:\n    claude:\n      binary: ${SHAPE_VALUE}\n"
	shapeChanged = "config.placeholder_shape_changed: expanding ${SHAPE_VALUE} changes the YAML shape of autopus.yaml at "
	shapeBroken  = "config.placeholder_shape_changed: expanding ${SHAPE_VALUE} breaks the YAML of autopus.yaml;"
	shapeBreak   = "config.placeholder_line_break: ${SHAPE_VALUE} expands to a value with a line break"
	shapeNoParse = "config.placeholder_shape_changed: autopus.yaml does not parse with ${SHAPE_VALUE} unexpanded"
)

func writeShapeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), []byte(shapeHead+body), 0o644))
	return dir
}

func TestLoad_RefusesAPlaceholderValueThatReshapesTheFile(t *testing.T) {
	policy := "role_model_policy:\n  version: v1\n  profile: balanced\n  family: ${SHAPE_VALUE}\n"
	for name, row := range map[string]struct{ value, body, want string }{
		"line feed":              {"openai\n  agents: {executor: {}}\n" + shapeSecret, policy, shapeBreak},
		"carriage return":        {shapeSecret + "\rx", shapeBinary, shapeBreak},
		"unicode line separator": {shapeSecret + " x", shapeBinary, shapeBreak},
		"comma in a flow mapping": {"openai, agents: {executor: {candidates: [{selector: " + shapeSecret + "}]}}",
			"role_model_policy: {version: v1, profile: balanced, family: ${SHAPE_VALUE}}\n", shapeChanged + "role_model_policy;"},
		"quote break in a flow mapping": {shapeSecret + `", config_mode: "overlay`,
			"role_model_policy: {version: v1, profile: balanced, family: \"${SHAPE_VALUE}\"}\n", shapeChanged + "role_model_policy;"},
		"quote break in a flow sequence": {shapeSecret + `", "--dangerously-skip-permissions`, shapeArgs,
			shapeChanged + "orchestra.providers.claude.args;"},
		"flow mapping for a scalar": {"{binary: " + shapeSecret + "}", shapeBinary, shapeChanged + "orchestra.providers.claude.binary;"},
		"alias for a scalar": {"*base", "future_extension:\n  base: &base claude\n" + shapeBinary,
			shapeChanged + "orchestra.providers.claude.binary;"},
		"alias to no anchor":   {"*" + shapeSecret, shapeBinary, shapeBroken},
		"anchor on a scalar":   {"&a " + shapeSecret, shapeBinary, shapeChanged + "orchestra.providers.claude.binary;"},
		"tag on a scalar":      {"!!str " + shapeSecret, shapeBinary, shapeChanged + "orchestra.providers.claude.binary;"},
		"merge key from a key": {"<<", "orchestra:\n  providers:\n    ${SHAPE_VALUE}: {binary: x}\n", shapeChanged + "orchestra.providers;"},
		"colliding keys": {"zqprovider", "orchestra:\n  providers:\n    zqprovider: {binary: a}\n    ${SHAPE_VALUE}: {binary: b}\n",
			shapeChanged + "orchestra.providers;"},
		"value that breaks the YAML": {shapeSecret + ": x", shapeBinary, shapeBroken},
		"placeholder in an alias name": {"zqanchor", "future_extension:\n  a: &zqanchor x\n  b: *${SHAPE_VALUE}\n",
			shapeNoParse + " (yaml: unknown anchor '${SHAPE_VALUE}' referenced)"},
		"placeholder the YAML cannot parse around": {shapeSecret, "role_model_policy:\n  version: v1\n  agents:\n" +
			"    executor:\n      candidates:\n        - selector: [${SHAPE_VALUE}\n", shapeNoParse},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SHAPE_VALUE", row.value)

			_, err := Load(writeShapeConfig(t, row.body))

			require.Error(t, err)
			assert.Contains(t, err.Error(), row.want)
			assert.NotContains(t, err.Error(), row.value)
			assert.NotContains(t, err.Error(), shapeSecret)
		})
	}
}

func TestParseConfigAndLoadLoreSection_RefuseAReshapingPlaceholder(t *testing.T) {
	t.Setenv("SHAPE_VALUE", shapeSecret+`", "--dangerously-skip-permissions`)
	dir := writeShapeConfig(t, shapeArgs)
	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	require.NoError(t, err)

	_, parseErr := ParseConfig(data)
	_, loreErr := LoadLoreSection(dir)

	for _, err := range []error{parseErr, loreErr} {
		require.Error(t, err)
		assert.Contains(t, err.Error(), shapeChanged+"orchestra.providers.claude.args;")
		assert.NotContains(t, err.Error(), shapeSecret)
	}
}

// A value may carry YAML punctuation, a number, or a map key, and an unset
// variable stays literal text: none of them changes the shape of the file.
func TestLoad_KeepsPlaceholdersThatOnlyFillTheirScalar(t *testing.T) {
	t.Setenv("SHAPE_VALUE", "a: [b], {c} # d")
	t.Setenv("SHAPE_NUMBER", "90")
	t.Setenv("SHAPE_KEY", "custom")
	dir := writeShapeConfig(t, "# note: ${SHAPE_VALUE}\nworkflow:\n  coverage_threshold: ${SHAPE_NUMBER}\n"+
		"orchestra:\n  providers:\n    ${SHAPE_KEY}:\n      binary: \"${SHAPE_VALUE}\"\n"+
		"      args: [\"--model\", \"${SHAPE_UNSET_VARIABLE}\"]\n")

	cfg, err := Load(dir)

	require.NoError(t, err)
	assert.Equal(t, 90, cfg.Workflow.CoverageThreshold)
	assert.Equal(t, "a: [b], {c} # d", cfg.Orchestra.Providers["custom"].Binary)
	assert.Equal(t, []string{"--model", "${SHAPE_UNSET_VARIABLE}"}, cfg.Orchestra.Providers["custom"].Args)
}

func TestLoad_KeepsARootFamilyPlaceholderInAFlowPolicy(t *testing.T) {
	t.Setenv("OMP_FAMILY", "openai")
	dir := writePlaceholderPolicy(t, "role_model_policy: {version: v1, profile: balanced, family: ${OMP_FAMILY}}\n")

	cfg, err := LoadPreview(dir)

	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.RoleModelPolicy.Family)
}
