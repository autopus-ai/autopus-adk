package healthband

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gpListing builds a git config --list --show-scope --show-origin -z
// listing; an item without "=" is a key with no value (an implicit true).
func gpListing(scope string, items ...string) []byte {
	var b strings.Builder
	for _, item := range items {
		key, value, hasValue := strings.Cut(item, "=")
		b.WriteString(scope + "\x00file:.git/config\x00" + key)
		if hasValue {
			b.WriteString("\n" + value)
		}
		b.WriteString("\x00")
	}
	return []byte(b.String())
}

func gpFinding(t *testing.T, listing []byte) string {
	t.Helper()
	entries, ok := parseGitConfigListing(listing)
	require.True(t, ok, "listing %q", listing)
	return gitConfigFinding(entries)
}

// SPEC-SIGMABAND-002 Git Execution Policy item 3 over listings: every rule,
// every scope, the byte-exact git-lfs allowlist, and the policy's own -c
// values, which the command scope lists and the check skips.
func TestGitConfigFinding_AppliesItemThreeToEveryScope(t *testing.T) {
	t.Parallel()

	lfs := "filter.lfs.process="
	cases := []struct {
		name, scope, item, want string
	}{
		{"local filter", "local", "filter.mark.clean=tools/clean.py", "filter.mark.clean"},
		{"global smudge", "global", "filter.mark.smudge=/usr/bin/python3 tools/smudge.py", "filter.mark.smudge"},
		{"system process", "system", "filter.fmt.process=fmt", "filter.fmt.process"},
		{"apple scope", "unknown", "filter.fmt.clean=fmt", "filter.fmt.clean"},
		{"implicit filter value", "local", "filter.x.smudge", "filter.x.smudge"},
		{"subsection with dots and case", "local", "filter.Mark.Dot.clean=x", "filter.Mark.Dot.clean"},
		{"lfs filter-process", "global", lfs + "git-lfs filter-process", ""},
		{"lfs absolute path", "global", lfs + "/usr/local/bin/git-lfs filter-process", ""},
		{"lfs clean", "global", "filter.lfs.clean=git-lfs clean -- %f", ""},
		{"lfs smudge", "global", "filter.lfs.smudge=git-lfs smudge -- %f", ""},
		{"lfs interpreter", "local", lfs + "/usr/bin/python3 tools/lfs.py", "filter.lfs.process"},
		{"lfs newline", "local", lfs + "git-lfs\nfilter-process", "filter.lfs.process"},
		{"lfs two spaces", "local", lfs + "git-lfs  filter-process", "filter.lfs.process"},
		{"lfs substitution", "local", lfs + "/usr/local/$(id)/git-lfs filter-process", "filter.lfs.process"},
		{"lfs relative path", "local", lfs + "tools/git-lfs filter-process", "filter.lfs.process"},
		{"lfs empty in a file", "local", lfs, "filter.lfs.process"},
		{"lfs implicit", "local", "filter.lfs.process", "filter.lfs.process"},
		{"driver LFS is not lfs", "local", "filter.LFS.process=git-lfs filter-process", "filter.LFS.process"},
		{"filter required", "local", "filter.lfs.required=true", ""},
		{"textconv", "local", "diff.tc.textconv=/bin/sh tools/tc.sh", "diff.tc.textconv"},
		{"diff command", "global", "diff.x.command=x", "diff.x.command"},
		{"diff external has --no-ext-diff", "global", "diff.external=x", ""},
		{"diff binary", "local", "diff.tc.binary=true", ""},
		{"merge driver", "local", "merge.m.driver=tools/m.sh", "merge.m.driver"},
		{"merge name", "local", "merge.m.name=m", ""},
		{"lfs extension", "local", "lfs.extension.x.clean=tools/x.sh", "lfs.extension.x.clean"},
		{"lfs extension case", "local", "lfs.Extension.x.smudge=x", "lfs.Extension.x.smudge"},
		{"lfs custom transfer", "local", "lfs.customtransfer.y.path=tools/y", "lfs.customtransfer.y.path"},
		{"lfs url", "local", "lfs.url=https://example.invalid", ""},
		{"alternate refs", "local", "core.alternaterefscommand=x", "core.alternateRefsCommand"},
		{"promisor", "local", "remote.origin.promisor=true", "remote.origin.promisor"},
		{"promisor implicit", "local", "remote.origin.promisor", "remote.origin.promisor"},
		{"promisor number", "local", "remote.origin.promisor=2", "remote.origin.promisor"},
		{"promisor false", "local", "remote.origin.promisor=false", ""},
		{"promisor off", "local", "remote.origin.promisor=Off", ""},
		{"partial clone filter", "local", "remote.origin.partialclonefilter=blob:none", "remote.origin.partialclonefilter"},
		{"partial clone extension", "local", "extensions.partialclone=origin", "extensions.partialclone"},
		{"config hook command", "global", "hook.lint.command=make lint", "hook.lint.command"},
		{"config hook event", "global", "hook.lint.event=pre-commit", ""},
		{"foreign command scope", "command", "filter.x.clean=x", "filter.x.clean"},
		{"own command scope", "command", "filter.lfs.process=", ""},
		{"credential helper", "unknown", "credential.helper=osxkeychain", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, gpFinding(t, gpListing(tc.scope, tc.item)), tc.name)
	}

	// git 2.50.1 lists the policy's flags with section and name lowercased.
	own := gpListing("command", "core.hookspath=/dev/null", "filter.lfs.process=", "filter.lfs.clean=",
		"filter.lfs.smudge=", "filter.lfs.required=false", "user.name=autopus-band")
	assert.Empty(t, gpFinding(t, own), "the policy's own flags as git lists them")
	assert.Equal(t, "filter.lfs.process", gpFinding(t, gpListing("command", "filter.lfs.process=x")),
		"a command-scope value that is not the policy's own is checked")
}

// The first finding follows the SPEC's rule order, not the listing order:
// a blob:none clone lists promisor before partialclonefilter, and either
// order gives remote.origin.promisor (S6).
func TestGitConfigFinding_RuleOrderDecidesTheCode(t *testing.T) {
	t.Parallel()

	listing := gpListing("local", "remote.origin.partialclonefilter=blob:none", "remote.origin.promisor=true")
	assert.Equal(t, "remote.origin.promisor", gpFinding(t, listing))
	listing = gpListing("local", "remote.origin.promisor=true", "filter.mark.clean=x")
	assert.Equal(t, "filter.mark.clean", gpFinding(t, listing))
	assert.Empty(t, gpFinding(t, nil), "an empty listing is safe")
}

func TestParseGitConfigListing_RefusesAnythingButWholeEntries(t *testing.T) {
	t.Parallel()

	for _, listing := range []string{
		"local\x00file:.git/config\x00core.bare\nfalse", // no final NUL
		"local\x00file:.git/config\x00",                 // two fields
		"local\x00file:.git/config\x00\nvalue\x00",      // empty key
	} {
		_, ok := parseGitConfigListing([]byte(listing))
		assert.False(t, ok, "%q", listing)
	}
}

// A hostile subsection cannot put control text into a code: bytes outside
// [A-Za-z0-9._-] become "_" and the key is cut at 128 bytes.
func TestGitCodeKey_SpellsASafeBoundedKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "filter.a_b__31m.clean", gitCodeKey("filter.a b\x1b[31m.clean"))
	assert.Equal(t, "core.alternateRefsCommand", gitCodeKey("core.alternaterefscommand"))
	long := gitCodeKey("filter." + strings.Repeat("x", 300) + ".clean")
	assert.Len(t, long, 128)
	assert.Equal(t, "filter.lfs.process", gitCodeKey("filter.lfs.process"))
}
