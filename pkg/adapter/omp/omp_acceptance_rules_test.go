package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// generateOMPOnly runs Generate for an omp-only platform list and persists the
// config so that ownership re-evaluation in Clean reads the same platform set.
func generateOMPOnly(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("omp-acceptance")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, cfg))
	_, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	return dir
}

// splitEmittedFrontmatter separates the leading `---` block from the body.
func splitEmittedFrontmatter(t *testing.T, content string) (frontmatter, body string) {
	t.Helper()
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	require.GreaterOrEqual(t, end, 0, "frontmatter block must be terminated")
	return rest[:end], rest[end+len("\n---\n"):]
}

func sourceRuleNames(t *testing.T) []string {
	t.Helper()
	entries, err := contentfs.FS.ReadDir("rules")
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	return names
}

// TestOMPAcceptance_S1_RuleFrontmatterPassthrough covers REQ-002/REQ-003/REQ-004.
func TestOMPAcceptance_S1_RuleFrontmatterPassthrough(t *testing.T) {
	dir := generateOMPOnly(t)
	ruleDir := filepath.Join(dir, ompRuleDir)

	loreRaw, err := os.ReadFile(filepath.Join(ruleDir, ompRuleFilePrefix+"lore-commit.md"))
	require.NoError(t, err)
	lore := string(loreRaw)
	assert.True(t, strings.HasPrefix(lore, "---\n"),
		"autopus-lore-commit.md must open with a frontmatter delimiter, got first line %q",
		strings.SplitN(lore, "\n", 2)[0])

	loreFM, loreBody := splitEmittedFrontmatter(t, lore)
	assert.Contains(t, loreFM,
		"description: Lore commit format rules for structured, traceable commit messages")
	assert.NotContains(t, loreFM, "category:")
	assert.NotContains(t, loreFM, "name:")
	assert.NotEmpty(t, strings.TrimSpace(loreBody))

	// Every emitted rule carries a non-empty body (REQ-004) and reaches an omp
	// session: it is listed in <domain-rules> when it declares a description, or
	// registered with the TTSR engine when it declares a trigger key. A rule with
	// neither lands on disk and is never discovered.
	names := sourceRuleNames(t)
	assert.Len(t, names, 4, "content/rules must hold exactly 4 rule sources")
	for _, name := range names {
		data, readErr := os.ReadFile(filepath.Join(ruleDir, ompRuleFilePrefix+name))
		require.NoError(t, readErr, "rule %s must be emitted", name)
		fm, body := splitEmittedFrontmatter(t, string(data))
		hasContentLine := false
		for _, line := range strings.Split(body, "\n") {
			if strings.TrimSpace(line) != "" {
				hasContentLine = true
				break
			}
		}
		assert.True(t, hasContentLine, "rule %s body must hold a non-blank line", name)

		discoverable := strings.Contains(fm, "description:") ||
			strings.Contains(fm, "condition:") ||
			strings.Contains(fm, "astCondition:") ||
			strings.Contains(fm, "scope:")
		assert.True(t, discoverable,
			"rule %s must be discoverable by an omp session, frontmatter was %q", name, fm)
	}

	// The rule surface is one flat, prefixed namespace inside `.omp/rules`
	// (REQ-002). omp discovers rules non-recursively, so a subdirectory would
	// hide every rule it holds, and `.agents/rules/` is not an omp target at all.
	assert.NoDirExists(t, filepath.Join(dir, ".agents", "rules"),
		"omp must not create .agents/rules/")

	entries, err := os.ReadDir(ruleDir)
	require.NoError(t, err)
	emitted := make([]string, 0, len(entries))
	for _, e := range entries {
		require.False(t, e.IsDir(),
			"%q: .omp/rules must stay flat or omp finds nothing", e.Name())
		require.True(t, strings.HasPrefix(e.Name(), ompRuleFilePrefix),
			"%q must carry the %q ownership prefix", e.Name(), ompRuleFilePrefix)
		emitted = append(emitted, strings.TrimPrefix(e.Name(), ompRuleFilePrefix))
	}
	assert.ElementsMatch(t, names, emitted,
		"emitted rule files map 1:1 onto the content/rules sources")
}

// TestOMPAcceptance_S2_ConditionalFieldPassthrough covers REQ-003 trigger fields.
func TestOMPAcceptance_S2_ConditionalFieldPassthrough(t *testing.T) {
	conditional := `---
description: demo
condition: tool:bash
scope:
  - tool:edit
alwaysApply: false
interruptMode: prose-only
---

# Conditional Demo

Body line.
`
	out, err := pkgcontent.TransformRuleForOMP(conditional)
	require.NoError(t, err)

	fm, body := splitEmittedFrontmatter(t, out)
	require.NotEmpty(t, fm, "conditional-demo must retain a frontmatter block")

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(fm), &parsed))
	assert.Equal(t, "tool:bash", parsed["condition"])
	assert.Equal(t, []any{"tool:edit"}, parsed["scope"])
	assert.Equal(t, false, parsed["alwaysApply"])
	assert.Equal(t, "prose-only", parsed["interruptMode"])
	assert.Contains(t, body, "# Conditional Demo")

	plain := `# Plain Demo

Body line.
`
	plainOut, err := pkgcontent.TransformRuleForOMP(plain)
	require.NoError(t, err)
	plainFM, plainBody := splitEmittedFrontmatter(t, plainOut)
	assert.Equal(t, "description: Plain Demo", strings.TrimSpace(plainFM),
		"plain-demo gains only a synthesized description")
	assert.NotContains(t, plainOut, "condition:",
		"synthesis must not invent a trigger that would reroute the rule to TTSR")
	assert.NotContains(t, plainOut, "scope:")
	assert.NotContains(t, plainOut, "alwaysApply:")
	assert.Contains(t, plainBody, "# Plain Demo")
}

// TestOMPAcceptance_E1_UnrecognizedKeysOnly covers REQ-003 drop behavior and
// the synthesis that keeps such a rule discoverable. The original E1 contract
// (emit no frontmatter block) was falsified by live measurement against omp
// 17.1.8: the bare body is written but no session ever lists the rule, which
// defeats the purpose of emitting it.
func TestOMPAcceptance_E1_UnrecognizedKeysOnly(t *testing.T) {
	src := `---
category: workflow
---

# Body Heading

Body text.
`
	out, err := pkgcontent.TransformRuleForOMP(src)
	require.NoError(t, err)
	fm, body := splitEmittedFrontmatter(t, out)
	assert.Equal(t, "description: Body Heading", strings.TrimSpace(fm),
		"the unrecognized key is dropped and replaced by a synthesized description")
	assert.NotContains(t, out, "category:")
	assert.Equal(t, "# Body Heading\n\nBody text.", strings.TrimSpace(body),
		"the body stays byte-identical")
}

// TestOMPAcceptance_S1_EmptyBodyRejected covers REQ-004 fail-closed behavior.
func TestOMPAcceptance_S1_EmptyBodyRejected(t *testing.T) {
	_, err := pkgcontent.TransformRuleForOMP("---\ndescription: Empty\n---\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestOMPAcceptance_LegacyRuleNamespaceUpgraded pins the upgrade path out of the
// pre-relocation layout. An older ADK wrote rules to `.agents/rules/autopus/`,
// where omp — which discovers rules non-recursively — never saw them. Update
// must re-emit the surface into the flat `.omp/rules/autopus-*.md` namespace and
// take the stale legacy file with it, while a user file sharing the legacy tree
// survives because no manifest ever recorded it.
func TestOMPAcceptance_LegacyRuleNamespaceUpgraded(t *testing.T) {
	dir := generateOMPOnly(t)

	// Rewind the workspace to what the previous release left behind: the rule
	// bodies live under `.agents/rules/autopus/`, `.omp/rules` does not exist,
	// and the manifest records the legacy paths.
	manifest, err := adapter.LoadManifest(dir, adapterName)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	const legacyDir = ".agents/rules/autopus"
	legacyRules := make(map[string]string)
	for path := range manifest.Files {
		slash := filepath.ToSlash(path)
		if !strings.HasPrefix(slash, ompRuleDir+"/") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(slash)))
		require.NoError(t, readErr)
		name := strings.TrimPrefix(strings.TrimPrefix(slash, ompRuleDir+"/"), ompRuleFilePrefix)
		legacyRules[legacyDir+"/"+name] = string(body)
		delete(manifest.Files, path)
	}
	require.Len(t, legacyRules, 4, "the fixture must rewind all 4 rules")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, filepath.FromSlash(ompRuleDir))))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.FromSlash(legacyDir)), 0o755))
	for path, body := range legacyRules {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, filepath.FromSlash(path)), []byte(body), 0o644))
		manifest.Files[path] = adapter.ManifestFile{
			Checksum: adapter.Checksum(body),
			Policy:   adapter.OverwriteAlways,
		}
	}
	require.NoError(t, manifest.Save(dir))

	// A user rule the ADK never recorded, sharing the legacy directory tree.
	userRule := filepath.Join(dir, ".agents", "rules", "my-own-rule.md")
	require.NoError(t, os.WriteFile(userRule, []byte("user rule\n"), 0o644))

	cfg := config.DefaultFullConfig("omp-acceptance")
	cfg.Platforms = []string{"omp"}
	_, err = NewWithRoot(dir).Update(context.Background(), cfg)
	require.NoError(t, err)

	legacyRule := filepath.Join(dir, filepath.FromSlash(legacyDir), "file-size-limit.md")
	assert.NoFileExists(t, legacyRule,
		"a legacy rule the previous manifest recorded must be pruned")
	assert.FileExists(t, filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"file-size-limit.md"),
		"the rule must be re-emitted into the flat .omp/rules namespace")

	survivingUser, err := os.ReadFile(userRule)
	require.NoError(t, err, "a user file in the legacy tree must survive the upgrade")
	assert.Equal(t, "user rule\n", string(survivingUser))

	for _, p := range manifestPaths(t, dir) {
		assert.False(t, strings.HasPrefix(p, ".agents/rules/"),
			"the rewritten manifest must record no legacy rule path, found %q", p)
	}
}
