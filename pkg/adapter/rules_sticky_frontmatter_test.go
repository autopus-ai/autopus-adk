package adapter_test

// SPEC-STICKYRULE-001 REQ-STICKYRULE-SCHEMA-02 / REQ-STICKYRULE-VERIFY-02.
// Scenario S8 asserts that `alwaysApply: true` survives to every platform that
// already preserves rule frontmatter, and scenario S7 asserts the claude-code
// rule surface is unchanged by the flag. S15 (parity neutrality) is covered by
// pkg/adapter/parity_test.go, which this SPEC must leave untouched.

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

// stickyKeyLine is the exact frontmatter line the sticky contract adds.
const stickyKeyLine = "alwaysApply: true"

// stickyRules are the two rules this SPEC marks sticky at the source of truth.
var stickyRules = []string{"language-policy.md", "objective-reasoning.md"}

// stickyControlRule travels the same per-platform rule pipeline without the
// sticky key, so it proves each adapter propagates `alwaysApply` from its
// source rather than synthesizing it for every rule.
const stickyControlRule = "project-identity.md"

// stickyPlatformRuleRoots lists active markdown rule roots. Gemini has two:
// the CLI rule directory and the .agents plugin mirror imported by GEMINI.md.
var stickyPlatformRuleRoots = map[string][]string{
	"claude":   {".claude/rules/autopus/"},
	"gemini":   {".gemini/rules/autopus/", ".agents/plugins/autopus/rules/"},
	"opencode": {".opencode/rules/autopus/"},
}

func stickyGenerate(t *testing.T, platform string) []adapter.FileMapping {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("sticky-frontmatter-test")
	ctx := context.Background()

	var pf *adapter.PlatformFiles
	var err error
	switch platform {
	case "claude":
		pf, err = claude.NewWithRoot(dir).Generate(ctx, cfg)
	case "gemini":
		pf, err = antigravity.NewWithRoot(dir).Generate(ctx, cfg)
	case "opencode":
		pf, err = opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7")).Generate(ctx, cfg)
	default:
		t.Fatalf("unknown platform: %s", platform)
	}
	require.NoError(t, err)
	require.NotNil(t, pf)
	return pf.Files
}

// stickyRuleEmission returns the content a platform emits for a rule source
// file and requires mirrored copies to be byte-identical.
func stickyRuleEmission(t *testing.T, files []adapter.FileMapping, platform, ruleFile string) string {
	t.Helper()
	var found []string
	for _, f := range files {
		p := filepath.ToSlash(f.TargetPath)
		if !strings.Contains(p, "rules") || extractRuleName(p) != ruleFile {
			continue
		}
		for _, root := range stickyPlatformRuleRoots[platform] {
			if strings.HasPrefix(p, root) {
				found = append(found, string(f.Content))
				break
			}
		}
	}
	require.NotEmpty(t, found, "%s must emit a rule mapping for %s", platform, ruleFile)
	for i := 1; i < len(found); i++ {
		require.Equal(t, found[0], found[i], "%s copies of %s must be byte-identical", platform, ruleFile)
	}
	return found[0]
}

// stickyFrontmatterOf mirrors the frontmatter contract the adapters use: the
// block opens with "---\n" and closes with "\n---\n". Several content rules
// ship without any frontmatter, so a scan over every source must tolerate that.
func stickyFrontmatterOf(content string) (string, bool) {
	if !strings.HasPrefix(content, "---\n") {
		return "", false
	}
	rest := strings.TrimPrefix(content, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", false
	}
	return rest[:idx], true
}

// stickySplitFrontmatter requires a frontmatter block and returns it with the
// body that follows.
func stickySplitFrontmatter(t *testing.T, content string) (string, string) {
	t.Helper()
	frontmatter, ok := stickyFrontmatterOf(content)
	require.True(t, ok, "rule must declare a closed frontmatter block")
	return frontmatter, content[len("---\n")+len(frontmatter)+len("\n---\n"):]
}

func stickyContentSource(t *testing.T, ruleFile string) string {
	t.Helper()
	data, err := fs.ReadFile(contentfs.FS, pkgcontent.EmbeddedPath("rules", ruleFile))
	require.NoError(t, err)
	return string(data)
}

func stickyGeminiTemplate(t *testing.T, ruleFile string) string {
	t.Helper()
	data, err := templates.FS.ReadFile("gemini/rules/autopus/" + ruleFile + ".tmpl")
	require.NoError(t, err)
	return string(data)
}

// S8: the sticky key reaches active frontmatter-preserving rule surfaces, stays
// inside frontmatter, and is never synthesized for a non-sticky rule.
func TestStickyFrontmatter_S8_ReachesEveryFrontmatterPreservingPlatform(t *testing.T) {
	wantPlatformLine := map[string]string{
		"gemini": "platform: antigravity-cli",
	}

	for _, platform := range []string{"opencode", "gemini"} {
		t.Run(platform, func(t *testing.T) {
			files := stickyGenerate(t, platform)

			for _, rule := range stickyRules {
				emitted := stickyRuleEmission(t, files, platform, rule)
				frontmatter, body := stickySplitFrontmatter(t, emitted)
				lines := strings.Split(frontmatter, "\n")

				assert.Contains(t, lines, stickyKeyLine,
					"%s must preserve the sticky key in %s frontmatter", platform, rule)
				if want, ok := wantPlatformLine[platform]; ok {
					assert.Contains(t, lines, want,
						"%s must retain its platform key alongside the sticky key in %s", platform, rule)
				}
				assert.Equal(t, 1, strings.Count(emitted, "alwaysApply"),
					"%s must emit the sticky key exactly once for %s", platform, rule)
				assert.NotContains(t, body, "alwaysApply",
					"%s must confine the sticky key to frontmatter for %s", platform, rule)
			}

			control := stickyRuleEmission(t, files, platform, stickyControlRule)
			assert.NotContains(t, control, "alwaysApply",
				"%s must not synthesize the sticky key for the non-sticky control rule", platform)
		})
	}
}

// S8: the sticky key is confined to frontmatter in both sources of truth, which
// is what makes the emitted-body assertions below proof that no body byte moved.
func TestStickyFrontmatter_S8_SourcesDeclareTheKeyInFrontmatterOnly(t *testing.T) {
	for _, rule := range stickyRules {
		source := stickyContentSource(t, rule)
		frontmatter, body := stickySplitFrontmatter(t, source)
		assert.Contains(t, strings.Split(frontmatter, "\n"), stickyKeyLine,
			"content/rules/%s must declare the sticky key", rule)
		assert.NotContains(t, body, "alwaysApply", "content/rules/%s body must stay clean", rule)

		tmpl := stickyGeminiTemplate(t, rule)
		tmplFrontmatter, tmplBody := stickySplitFrontmatter(t, tmpl)
		tmplLines := strings.Split(tmplFrontmatter, "\n")
		assert.Contains(t, tmplLines, stickyKeyLine,
			"gemini template for %s must declare the sticky key", rule)
		assert.Contains(t, tmplLines, "platform: antigravity-cli",
			"gemini template for %s must keep its platform key", rule)
		assert.NotContains(t, tmplBody, "alwaysApply", "gemini template body for %s must stay clean", rule)
	}
}

// S8: OpenCode reads content/rules directly, so emission must equal the source
// bytes after only the platform-reference rewrite.
func TestStickyFrontmatter_S8_OpencodeEmitsSourceBytes(t *testing.T) {
	opencodeFiles := stickyGenerate(t, "opencode")

	for _, rule := range stickyRules {
		source := stickyContentSource(t, rule)
		wantOpencode := pkgcontent.ReplacePlatformReferences(source, "opencode")
		assert.Equal(t, wantOpencode, stickyRuleEmission(t, opencodeFiles, "opencode", rule),
			"opencode emission for %s must be the source bytes, sticky key included", rule)
	}
}

// S8: gemini renders from templates/gemini/rules/autopus, so its frontmatter is
// pass-through and its imported body must remain the untouched content body.
func TestStickyFrontmatter_S8_GeminiPreservesTemplateFrontmatterAndImportedBody(t *testing.T) {
	files := stickyGenerate(t, "gemini")

	for _, rule := range stickyRules {
		emitted := stickyRuleEmission(t, files, "gemini", rule)
		emittedFrontmatter, emittedBody := stickySplitFrontmatter(t, emitted)
		tmplFrontmatter, _ := stickySplitFrontmatter(t, stickyGeminiTemplate(t, rule))

		assert.Equal(t, tmplFrontmatter, emittedFrontmatter,
			"gemini frontmatter for %s is pass-through, so every value stays byte-identical", rule)
		assert.NotContains(t, emittedBody, "alwaysApply",
			"gemini body for %s must not carry the sticky key", rule)
	}

	// objective-reasoning imports the content body verbatim; the emission must
	// still end with the frontmatter-stripped source body.
	emitted := stickyRuleEmission(t, files, "gemini", "objective-reasoning.md")
	_, sourceBody := stickySplitFrontmatter(t, stickyContentSource(t, "objective-reasoning.md"))
	assert.True(t, strings.HasSuffix(emitted, sourceBody),
		"gemini must import the objective-reasoning body unchanged and strip its frontmatter")
}
