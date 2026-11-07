package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderCodexConfig(t *testing.T, dir string, cfg *config.HarnessConfig, options ...Option) string {
	t.Helper()
	files, err := NewWithRoot(dir, options...).prepareConfigFile(cfg)
	require.NoError(t, err)
	require.Len(t, files, 1)
	return string(files[0].Content)
}

func codexSection(body, name string) string {
	_, after, ok := strings.Cut(body, "["+name+"]\n")
	if !ok {
		return ""
	}
	section, _, _ := strings.Cut(after, "\n[")
	return section
}

// A release verified to recognise agents.max_concurrent_threads_per_session
// must get the ceiling there, and the comment must record that the namespace
// came from a verified release rather than from documentation alone.
func TestPrepareConfig_VerifiedReleaseUsesDocumentedAgentsNamespace(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("concurrency")
	cfg.Codex.Agents.MaxConcurrentThreads = 8

	body := renderCodexConfig(t, t.TempDir(), cfg, WithCLIVersion("codex-cli 0.153.4\n"))

	assert.Equal(t, "max_concurrent_threads_per_session = 8\n", codexSection(body, "agents"))
	assert.Contains(t, codexSection(body, "features.multi_agent_v2"), "enabled = true")
	assert.NotContains(t, codexSection(body, "features.multi_agent_v2"),
		"max_concurrent_threads_per_session")
	assert.Contains(t, body, "verified to recognise")
}

// A release nobody verified still gets the documented table: no evidence puts
// the ceiling anywhere else, and a version number is not evidence that an
// older release wants the undocumented feature table. The comment carries the
// assumption instead of the version deciding silently.
func TestPrepareConfig_UnverifiedReleaseKeepsDocumentedNamespaceAndNamesTheAssumption(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("concurrency")
	cfg.Codex.Agents.MaxConcurrentThreads = 6

	body := renderCodexConfig(t, t.TempDir(), cfg, WithCLIVersion("codex-cli 0.149.1\n"))

	assert.Equal(t, "max_concurrent_threads_per_session = 6\n", codexSection(body, "agents"))
	assert.NotContains(t, codexSection(body, "features.multi_agent_v2"),
		"max_concurrent_threads_per_session")
	assert.Contains(t, body, "not one Autopus verified")
}

// Without a readable version the file follows current documentation and says
// so, so nobody reads the namespace as a detected fact.
func TestPrepareConfig_UnknownReleaseUsesDocumentedNamespaceAndNamesTheAssumption(t *testing.T) {
	t.Parallel()

	body := renderCodexConfig(t, t.TempDir(), config.DefaultFullConfig("concurrency"),
		WithCLIVersion(""))

	assert.Contains(t, body, "[agents]\nmax_concurrent_threads_per_session = 4")
	assert.Contains(t, body, "version unknown")
	assert.Contains(t, body, "coordinator thread is not counted")
}

// A file an older Autopus wrote carries the ceiling in the undocumented
// features.multi_agent_v2 table. Regeneration must relocate it instead of
// leaving two tables that can disagree.
func TestPrepareConfig_MigratesLegacyCeilingToAgentsNamespace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := `approval_policy = "on-request"

[features]
goals = true

[features.multi_agent_v2]
enabled = true
max_concurrent_threads_per_session = 4
`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, codexConfigRelPath), []byte(existing), 0o644))

	cfg := config.DefaultFullConfig("concurrency")
	cfg.Codex.Agents.MaxConcurrentThreads = 12
	body := renderCodexConfig(t, dir, cfg, WithCLIVersion("codex-cli 0.153.4\n"))

	assert.NotContains(t, codexSection(body, "features.multi_agent_v2"),
		"max_concurrent_threads_per_session")
	assert.Contains(t, codexSection(body, "agents"), "max_concurrent_threads_per_session = 12")
}

// max_threads is the documented legacy alias of the generated key, so a file
// carrying both would hold two ceilings with no documented winner. The count
// the project declared must survive regeneration as the only ceiling in the
// file, and the alias the old Autopus template wrote must not survive beside
// it and shadow the generated key.
func TestPrepareConfig_LegacyAliasNeverSurvivesBesideTheGeneratedCeiling(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := `approval_policy = "on-request"

[features]
goals = true

[agents]
max_threads = 6
max_depth = 1

[features.multi_agent_v2]
enabled = true
max_concurrent_threads_per_session = 6
`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, codexConfigRelPath), []byte(existing), 0o644))

	cfg := config.DefaultFullConfig("concurrency")
	cfg.Codex.Agents.MaxConcurrentThreads = 8
	body := renderCodexConfig(t, dir, cfg, WithCLIVersion("codex-cli 0.153.4\n"))

	assert.NotContains(t, body, "max_threads")
	assert.NotContains(t, body, "max_depth")
	assert.NotContains(t, codexSection(body, "features.multi_agent_v2"),
		"max_concurrent_threads_per_session")
	assert.Equal(t, 1, strings.Count(body, "max_concurrent_threads_per_session = "))
	assert.Contains(t, codexSection(body, "agents"), "max_concurrent_threads_per_session = 8")
}

// A per-role [agents.<name>] table is user config and survives; only the
// managed scalar and the header Autopus created are removed.
func TestClean_RemovesManagedAgentsCeilingAndKeepsUserRoles(t *testing.T) {
	t.Parallel()
	body := `[agents]
max_concurrent_threads_per_session = 8

[agents.reviewer]
description = "user role"
`
	cleaned := removeAutopusCodexConfig(body)

	assert.NotContains(t, cleaned, "max_concurrent_threads_per_session")
	assert.NotContains(t, cleaned, "[agents]\n")
	assert.Contains(t, cleaned, "[agents.reviewer]")
	assert.Contains(t, cleaned, `description = "user role"`)
}

func TestClean_KeepsUserOwnedAgentsScalars(t *testing.T) {
	t.Parallel()
	body := `[agents]
max_concurrent_threads_per_session = 8
default_subagent_model = "user-model"
`
	cleaned := removeAutopusCodexConfig(body)

	assert.Contains(t, cleaned, "[agents]")
	assert.Contains(t, cleaned, `default_subagent_model = "user-model"`)
	assert.NotContains(t, cleaned, "max_concurrent_threads_per_session")
}

func TestInspectAgentConcurrency(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		body          string
		wantFound     bool
		wantValue     int
		wantNamespace string
		wantKey       string
		wantMalformed bool
	}{
		{
			name:      "documented namespace",
			body:      "[agents]\nmax_concurrent_threads_per_session = 8\n",
			wantFound: true, wantValue: 8, wantNamespace: "agents",
			wantKey: "max_concurrent_threads_per_session",
		},
		{
			name:      "legacy namespace",
			body:      "[features.multi_agent_v2]\nmax_concurrent_threads_per_session = 3\n",
			wantFound: true, wantValue: 3, wantNamespace: "features.multi_agent_v2",
			wantKey: "max_concurrent_threads_per_session",
		},
		{
			// The reference documents agents.max_threads as an alias of the
			// generated key, so a file setting only the alias does configure a
			// ceiling and must not read as unset.
			name:      "documented legacy alias alone",
			body:      "[agents]\nmax_threads = 6\n",
			wantFound: true, wantValue: 6, wantNamespace: "agents", wantKey: "max_threads",
		},
		{
			name:      "documented key outranks its alias",
			body:      "[agents]\nmax_concurrent_threads_per_session = 8\nmax_threads = 6\n",
			wantFound: true, wantValue: 8, wantNamespace: "agents",
			wantKey: "max_concurrent_threads_per_session",
		},
		{
			name:      "inline comment is not part of the value",
			body:      "[agents]\nmax_concurrent_threads_per_session = 5 # tuned\n",
			wantFound: true, wantValue: 5, wantNamespace: "agents",
			wantKey: "max_concurrent_threads_per_session",
		},
		{
			name:      "non-numeric value is reported rather than guessed",
			body:      "[agents]\nmax_concurrent_threads_per_session = \"many\"\n",
			wantFound: true, wantNamespace: "agents",
			wantKey: "max_concurrent_threads_per_session", wantMalformed: true,
		},
		{
			name: "unset",
			body: "[features]\ngoals = true\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o644))

			inspection, err := InspectAgentConcurrency(path)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFound, inspection.Found)
			assert.Equal(t, tc.wantValue, inspection.Value)
			assert.Equal(t, tc.wantNamespace, inspection.Namespace)
			assert.Equal(t, tc.wantKey, inspection.Key)
			assert.Equal(t, tc.wantMalformed, inspection.Malformed)
		})
	}
}

func TestInspectAgentConcurrency_MissingFileIsNotAnError(t *testing.T) {
	t.Parallel()

	inspection, err := InspectAgentConcurrency(filepath.Join(t.TempDir(), "absent.toml"))
	require.NoError(t, err)
	assert.False(t, inspection.Found)
}

// Every ceiling in the file must be reported with its own provenance, not
// just the winning one: the diagnostic turns the extras into the conflict it
// warns about, and dropping them would hide a second key that can change what
// Codex applies.
func TestInspectAgentConcurrency_ReportsEveryCeilingInPrecedenceOrder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[agents]\nmax_concurrent_threads_per_session = 8\nmax_threads = 6\n\n" +
		"[features.multi_agent_v2]\nmax_concurrent_threads_per_session = 2\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	inspection, err := InspectAgentConcurrency(path)
	require.NoError(t, err)

	assert.Equal(t, []AgentConcurrencySetting{
		{Namespace: "agents", Key: "max_concurrent_threads_per_session", Value: 8},
		{Namespace: "agents", Key: "max_threads", Value: 6},
		{Namespace: "features.multi_agent_v2", Key: "max_concurrent_threads_per_session", Value: 2},
	}, inspection.Settings)
	assert.Equal(t, 8, inspection.Value)
	assert.Equal(t, "max_concurrent_threads_per_session", inspection.Key)
}

func TestValidateConfig_AcceptsCeilingInEitherNamespace(t *testing.T) {
	t.Parallel()

	for _, namespace := range []string{"agents", "features.multi_agent_v2"} {
		t.Run(namespace, func(t *testing.T) {
			t.Parallel()
			content := "[features]\ngoals = true\nhooks = true\nshell_tool = true\nunified_exec = true\n\n" +
				"[features.multi_agent_v2]\nenabled = true\n\n[" + namespace + "]\n" +
				"max_concurrent_threads_per_session = 9\n"
			var errs []adapter.ValidationError
			validateCodexFeatureFlags(content, &errs)
			assert.Empty(t, errs)
		})
	}
}

func TestValidateConfig_RejectsOutOfRangeCeiling(t *testing.T) {
	t.Parallel()
	content := "[features]\ngoals = true\nhooks = true\nshell_tool = true\nunified_exec = true\n\n" +
		"[features.multi_agent_v2]\nenabled = true\n\n[agents]\n" +
		"max_concurrent_threads_per_session = 999\n"

	var errs []adapter.ValidationError
	validateCodexFeatureFlags(content, &errs)

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "유효 범위")
}
