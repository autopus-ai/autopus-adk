package cli_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The learn store is written only through `auto learn record` and pruned only
// through `auto learn prune`: those paths redact secrets and keep entries that
// golden-task evals reference. These static tests keep generated guidance from
// telling an agent to edit .autopus/learnings/pipeline.jsonl by hand.
var (
	learnStoreRefRe     = regexp.MustCompile(`pipeline\.jsonl|\.autopus/learnings`)
	learnStoreVerbRe    = regexp.MustCompile(`(?i)\b(append|write|remove|delete|edit|modify|rewrite|truncate)\b`)
	learnStoreByHandRe  = regexp.MustCompile(`(?i)\b(directly|manually|by hand)\b`)
	learnStoreNegatedRe = regexp.MustCompile(`(?i)\b(do not|don't|never|must not|shall not)\b`)
	markdownHeadingRe   = regexp.MustCompile(`^#{1,6}\s`)
	learnInvocationRe   = regexp.MustCompile("auto learn (record|prune)([^`\n]*)")
	learnFlagRe         = regexp.MustCompile(`--([a-z][a-z0-9-]*)`)
	pruneDaysRe         = regexp.MustCompile(`--days[ =]([^\s]+)`)
)

// learnGuidanceRoot resolves the module root from this file's location, so the
// scan does not depend on the working directory other learn tests change.
func learnGuidanceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// guidanceFiles returns the text of every file under templates/ and content/.
func guidanceFiles(t *testing.T) map[string]string {
	t.Helper()
	root := learnGuidanceRoot(t)
	files := map[string]string{}
	for _, dir := range []string{"templates", "content"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			files[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		require.NoError(t, err)
	}
	require.NotEmpty(t, files)
	return files
}

// directStoreMutations returns every line that tells the reader to change the
// learn store by hand: a mutation verb on a line that names the store, or a
// mutation verb with "directly"/"manually" inside a section that names it.
// Negated lines ("Do NOT ...") are allowed.
func directStoreMutations(text string) []string {
	lines := strings.Split(text, "\n")
	var found []string
	check := func(start, end int) {
		section := strings.Join(lines[start:end], "\n")
		mentionsStore := learnStoreRefRe.MatchString(section)
		for i := start; i < end; i++ {
			line := lines[i]
			if !learnStoreVerbRe.MatchString(line) || learnStoreNegatedRe.MatchString(line) {
				continue
			}
			if learnStoreRefRe.MatchString(line) || (mentionsStore && learnStoreByHandRe.MatchString(line)) {
				found = append(found, fmt.Sprintf("line %d: %s", i+1, strings.TrimSpace(line)))
			}
		}
	}
	start, inFence := 0, false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence && markdownHeadingRe.MatchString(line) && i > start {
			check(start, i)
			start = i
		}
	}
	check(start, len(lines))
	return found
}

func TestLearnStoreGuidance_DetectorFlagsLegacyFallbacks(t *testing.T) {
	t.Parallel()
	legacy := "### Step 5\n\n" +
		"If the command fails, append directly to `.autopus/learnings/pipeline.jsonl`:\n\n" +
		"### Sync Target 4.5\n\nWHEN `.autopus/learnings/pipeline.jsonl` exists, THE SYSTEM SHALL:\n\n" +
		"If the command fails, read the file directly and remove entries older than 90 days.\n"
	assert.Len(t, directStoreMutations(legacy), 2)

	allowed := "### Step\n\nWHEN `.autopus/learnings/pipeline.jsonl` exists, read it directly and match entries.\n" +
		"Do NOT append to or edit `.autopus/learnings/pipeline.jsonl` directly.\n"
	assert.Empty(t, directStoreMutations(allowed))
}

func TestLearnStoreGuidance_NoDirectStoreMutation(t *testing.T) {
	t.Parallel()
	for path, text := range guidanceFiles(t) {
		assert.Empty(t, directStoreMutations(text), "%s tells the reader to change the learn store by hand", path)
	}
}

// TestLearnStoreGuidance_InvocationsUseRealFlags checks every documented
// `auto learn record|prune` against the real command, and every prune against
// an integer --days (pflag cannot parse "90d", so a bad flag made the
// documented direct-edit fallback the real path).
func TestLearnStoreGuidance_InvocationsUseRealFlags(t *testing.T) {
	t.Parallel()
	root := newTestRootCmd()
	prunes := 0
	for path, text := range guidanceFiles(t) {
		for _, m := range learnInvocationRe.FindAllStringSubmatch(text, -1) {
			sub, args := m[1], m[2]
			cmd, _, err := root.Find([]string{"learn", sub})
			require.NoError(t, err)
			flags := learnFlagRe.FindAllStringSubmatch(args, -1)
			for _, flag := range flags {
				assert.NotNil(t, cmd.Flags().Lookup(flag[1]), "%s: `auto learn %s` has no --%s flag", path, sub, flag[1])
			}
			// A bare command name in prose is a mention, not an invocation.
			if sub != "prune" || len(flags) == 0 {
				continue
			}
			prunes++
			assert.NotContains(t, args, "--max-age", "%s: prune must use --days", path)
			days := pruneDaysRe.FindStringSubmatch(args)
			if assert.NotNil(t, days, "%s: prune without --days: %q", path, m[0]) {
				assert.Regexp(t, `^[0-9]+$`, days[1], "%s: --days takes an integer", path)
			}
		}
	}
	assert.Positive(t, prunes, "the sync workflow documents at least one prune")
}

func TestLearnStoreGuidance_SyncTargetPrunesThroughCLIOnly(t *testing.T) {
	t.Parallel()
	text := guidanceFiles(t)["templates/claude/commands/auto-workflows.md.tmpl"]
	start := strings.Index(text, "### [REQUIRED] Sync Target 4.5")
	require.NotEqual(t, -1, start)
	section := text[start:]
	if end := strings.Index(section[1:], "\n### "); end >= 0 {
		section = section[:end+1]
	}
	assert.Contains(t, section, "auto learn prune --days 90")
	assert.Contains(t, section, "eval_links_unreadable")
	assert.Contains(t, section, "leave `.autopus/learnings/pipeline.jsonl` unchanged")
	assert.NotContains(t, section, "remove entries older than 90 days")
}
