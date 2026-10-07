package config

// SPEC-PANERM-001 S14 (REQ-11, INV-09): the codex and claude default-entry
// detection returns the same upgrade decision as B for every A34 default entry.
//
// Each row is one orchestra provider entry as an autopus.yaml at B could hold
// it, group K keys included, loaded and migrated through the public seam
// (Load, then MigrateOrchestraConfig). The atB column was recorded by running
// this table at B (pkg/config non-test code unchanged since c447badc), so it is
// independent of the code under test. The only intended differences are the
// entries whose sole customization is pane_args: B treated them as user-owned,
// and with pane_args retired nothing distinguishes them from the default entry
// they otherwise equal, so they now upgrade like it (CHANGELOG, S14).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type defaultEntryDecision struct {
	Policy  string
	Args    []string
	Changed bool
}

type defaultEntryRow struct {
	name     string
	platform string
	provider string
	// c1Entry names the C1 provider whose entry the row uses; entry is the
	// row's own YAML when c1Entry is empty.
	c1Entry string
	entry   string
	// legacyQuality drops quality.supervisor_model_policy, which A34 writes
	// as "inherit" and which pre-A34 files did not carry.
	legacyQuality bool
	atB           defaultEntryDecision
	// after is the decision once pane_args is retired; nil means "as at B".
	after *defaultEntryDecision
}

const (
	s14HistoricalCodexArgs   = `[exec, --sandbox, workspace-write, -m, gpt-5.5, -c, model_reasoning_effort="xhigh"]`
	s14HistoricalCodexPane   = `[-m, gpt-5.5, -c, model_reasoning_effort="xhigh"]`
	s14V05066CodexArgs       = `[exec, --sandbox, workspace-write, -m, gpt-5.5]`
	s14V05066CodexPane       = `[-m, gpt-5.5]`
	s14V05066CodexSubprocess = "subprocess:\n  schema_flag: --output-schema\n  timeout: 420\n"
)

var (
	s14ManagedCodexArgs = []string{"exec", "--json", "--sandbox", "workspace-write", "-m", CodexAstraModel,
		"-c", `model_reasoning_effort="max"`}
	s14ClaudeDefaultArgs = []string{"--print", "--model", ClaudeFableModel, "--effort", ClaudeOrchestraEffort}
)

func defaultEntryRows() []defaultEntryRow {
	historicalCodexArgs := []string{"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel,
		"-c", `model_reasoning_effort="xhigh"`}
	v05066CodexArgs := []string{"exec", "--sandbox", "workspace-write", "-m", CodexLegacyModel}
	managed := defaultEntryDecision{Policy: ProviderModelPolicyQuality, Args: s14ManagedCodexArgs, Changed: true}
	return []defaultEntryRow{
		// The A34 default entries of C1: B leaves each one as it is.
		{name: "c1-claude", platform: "claude-code", provider: "claude", c1Entry: "claude",
			atB: defaultEntryDecision{Args: s14ClaudeDefaultArgs}},
		{name: "c1-codex", platform: "codex", provider: "codex", c1Entry: "codex",
			atB: defaultEntryDecision{Policy: ProviderModelPolicyQuality, Args: s14ManagedCodexArgs}},
		{name: "c1-gemini", platform: "antigravity-cli", provider: "gemini", c1Entry: "gemini",
			atB: defaultEntryDecision{Args: []string{"--print", ""}}},
		// Historical shipped defaults: B upgrades each onto the managed policy.
		{name: "historical-canonical-codex", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nargs: " + s14HistoricalCodexArgs + "\npane_args: " + s14HistoricalCodexPane + "\n",
			atB:   managed},
		{name: "v05066-auto-pinned-codex", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nmodel_policy: pinned\nargs: " + s14V05066CodexArgs + "\npane_args: " + s14V05066CodexPane +
				"\n" + s14V05066CodexSubprocess,
			atB: managed},
		{name: "v05066-auto-pinned-codex-under-a34-quality", platform: "codex", provider: "codex",
			entry: "binary: codex\nmodel_policy: pinned\nargs: " + s14V05066CodexArgs + "\npane_args: " + s14V05066CodexPane +
				"\n" + s14V05066CodexSubprocess,
			atB: defaultEntryDecision{Policy: ProviderModelPolicyPinned, Args: v05066CodexArgs}},
		{name: "historical-claude-opus-high", platform: "claude-code", provider: "claude", legacyQuality: true,
			entry: "binary: claude\nargs: [--print, --model, opus, --effort, high]\n" +
				"pane_args: [--print, --model, opus, --effort, high]\n",
			atB: defaultEntryDecision{Args: s14ClaudeDefaultArgs, Changed: true}},
		// The intended changes: the only customization is a retired pane key.
		// B rewrote the pane argv alone; there is no pane argv left to rewrite.
		{name: "claude-historical-pane-args-only", platform: "claude-code", provider: "claude", legacyQuality: true,
			entry: "binary: claude\nargs: [--print, --model, my-model]\npane_args: [-p, --model, opus, --effort, max]\n" +
				"subprocess:\n  timeout: 480\n",
			atB:   defaultEntryDecision{Args: []string{"--print", "--model", "my-model"}, Changed: true},
			after: &defaultEntryDecision{Args: []string{"--print", "--model", "my-model"}}},
		// B kept these as user-owned pinned entries; they now equal a default.
		{name: "historical-canonical-codex-custom-pane-args", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nargs: " + s14HistoricalCodexArgs + "\npane_args: [-m, my-model]\n",
			atB:   defaultEntryDecision{Policy: ProviderModelPolicyPinned, Args: historicalCodexArgs, Changed: true},
			after: &managed},
		{name: "v05066-auto-pinned-codex-custom-pane-args", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nmodel_policy: pinned\nargs: " + s14V05066CodexArgs + "\npane_args: [-m, my-model]\n" +
				s14V05066CodexSubprocess,
			atB:   defaultEntryDecision{Policy: ProviderModelPolicyPinned, Args: v05066CodexArgs},
			after: &managed},
		{name: "unmarked-codex-pane-args-only", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\npane_args: [-m, my-model]\n",
			atB:   defaultEntryDecision{Policy: ProviderModelPolicyPinned, Changed: true},
			after: &managed},
		{name: "unmarked-codex-working-patterns-only", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nworking_patterns: [thinking]\n",
			atB:   defaultEntryDecision{Policy: ProviderModelPolicyPinned, Changed: true},
			after: &managed},
		{name: "unmarked-codex-interactive-input-only", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\ninteractive_input: stdin\n",
			atB:   defaultEntryDecision{Policy: ProviderModelPolicyPinned, Changed: true},
			after: &managed},
		// A model the user chose stays, pane keys or not (fix-round item 7,
		// atB recorded at 60f92ea5). So does an entry B's update already
		// pinned: only the v0.50.66 auto-pin above moves after a B update.
		{name: "user-pinned-codex-model-with-pane-args", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nmodel_policy: pinned\nargs: [exec, --sandbox, workspace-write, -m, my-own-model]\n" +
				"pane_args: [-m, my-own-model]\n",
			atB: defaultEntryDecision{Policy: ProviderModelPolicyPinned,
				Args: []string{"exec", "--sandbox", "workspace-write", "-m", "my-own-model"}}},
		{name: "unmarked-user-codex-args-with-pane-args", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nargs: [exec, --sandbox, workspace-write, -m, my-own-model]\npane_args: [-m, my-own-model]\n",
			atB: defaultEntryDecision{Policy: ProviderModelPolicyPinned,
				Args: []string{"exec", "--sandbox", "workspace-write", "-m", "my-own-model"}, Changed: true}},
		{name: "historical-canonical-codex-custom-pane-args-after-b-update", platform: "codex", provider: "codex",
			legacyQuality: true,
			entry:         "binary: codex\nmodel_policy: pinned\nargs: " + s14HistoricalCodexArgs + "\npane_args: [-m, my-model]\n",
			atB:           defaultEntryDecision{Policy: ProviderModelPolicyPinned, Args: historicalCodexArgs}},
		{name: "unmarked-codex-pane-args-only-after-b-update", platform: "codex", provider: "codex", legacyQuality: true,
			entry: "binary: codex\nmodel_policy: pinned\npane_args: [-m, my-model]\n",
			atB:   defaultEntryDecision{Policy: ProviderModelPolicyPinned}},
	}
}

// s14Workspace writes C1 reduced to one platform and one provider entry, with
// no orchestra commands, so MigrateOrchestraConfig reports only that entry.
func s14Workspace(t *testing.T, row defaultEntryRow) string {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(readLegacyPaneFixture(t, "c1.yaml"), &doc))
	root := doc.Content[0]
	orchestra := s14MapValue(t, root, "orchestra")
	providers := s14MapValue(t, orchestra, "providers")

	var entry *yaml.Node
	if row.c1Entry != "" {
		entry = s14MapValue(t, providers, row.c1Entry)
	} else {
		var parsed yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(row.entry), &parsed), row.name)
		entry = parsed.Content[0]
	}
	s14SetMapValue(t, root, "platforms", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq",
		Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: row.platform}}})
	s14SetMapValue(t, orchestra, "providers", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map",
		Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: row.provider}, entry}})
	s14DeleteMapKey(orchestra, "commands")
	if row.legacyQuality {
		s14DeleteMapKey(s14MapValue(t, root, "quality"), "supervisor_model_policy")
	}

	data, err := yaml.Marshal(&doc)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), data, 0o644))
	return dir
}

func s14MapValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	require.Failf(t, "missing key", "%q", key)
	return nil
}

func s14SetMapValue(t *testing.T, mapping *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	require.Failf(t, "missing key", "%q", key)
}

func s14DeleteMapKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func TestPanermS14_DefaultEntryUpgradeDecisionsMatchB(t *testing.T) {
	t.Parallel()
	for _, row := range defaultEntryRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(s14Workspace(t, row))
			require.NoError(t, err)
			changed, err := MigrateOrchestraConfig(cfg)
			require.NoError(t, err)
			entry := cfg.Orchestra.Providers[row.provider]
			got := defaultEntryDecision{Policy: entry.ModelPolicy, Args: entry.Args, Changed: changed}

			want := row.atB
			if row.after != nil {
				want = *row.after
			}
			assert.Equal(t, want, got)
		})
	}
}
