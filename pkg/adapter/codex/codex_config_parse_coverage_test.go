package codex

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Guards against mis-reading a user TOML string value: an unterminated quote
// must fail rather than yield a truncated model name, and escapes must decode.
func TestParseCodexStringValue_HonoursQuotingRules(t *testing.T) {
	for _, test := range []struct {
		name, raw, want string
		ok              bool
	}{
		{name: "basic string", raw: `"gpt-5"`, want: "gpt-5", ok: true},
		{name: "surrounding whitespace", raw: "  \"gpt-5\"  ", want: "gpt-5", ok: true},
		{name: "escaped quote", raw: `"say \"hi\""`, want: `say "hi"`, ok: true},
		{name: "literal string", raw: `'gpt-5'`, want: "gpt-5", ok: true},
		{name: "literal keeps backslash", raw: `'a\b'`, want: `a\b`, ok: true},
		{name: "unterminated basic", raw: `"gpt-5`},
		{name: "escaped terminator only", raw: `"gpt-5\"`},
		{name: "unterminated literal", raw: `'gpt-5`},
		{name: "invalid escape", raw: `"bad\q"`},
		{name: "bare token", raw: `gpt-5`},
		{name: "number", raw: `42`},
		{name: "empty", raw: "   "},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, ok := parseCodexStringValue(test.raw)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.want, value)
			assert.Equal(t, test.ok && test.want == "gpt-5", codexStringEquals(test.raw, "gpt-5"))
		})
	}
}

// Guards against stripping a '#' that belongs inside a user string value,
// which would silently corrupt the preserved setting.
func TestCodexTOMLValueWithoutComment_OnlyStripsRealComments(t *testing.T) {
	for _, test := range []struct {
		name, value, want string
	}{
		{name: "trailing comment", value: `"gpt-5" # pick`, want: `"gpt-5" `},
		{name: "hash inside basic string", value: `"tag#1"`, want: `"tag#1"`},
		{name: "hash inside literal string", value: `'tag#1'`, want: `'tag#1'`},
		{name: "escaped quote then comment", value: `"a\"b" # c`, want: `"a\"b" `},
		{name: "hash after closed string", value: `"a" # "b"`, want: `"a" `},
		{name: "multiline basic keeps hash", value: `"""a#b"""`, want: `"""a#b"""`},
		{name: "multiline basic then comment", value: `"""a""" # c`, want: `"""a""" `},
		{name: "multiline literal keeps hash", value: `'''a#b'''`, want: `'''a#b'''`},
		{name: "multiline literal then comment", value: `'''a''' # c`, want: `'''a''' `},
		{name: "unterminated string swallows hash", value: `"a#b`, want: `"a#b`},
		{name: "no comment", value: `true`, want: `true`},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, codexTOMLValueWithoutComment(test.value))
		})
	}
}

// Guards against interpreting the body of a multiline user string as TOML:
// assignments and section headers inside it must never be collected.
func TestCollectCodexConfigOverrides_IgnoresMultilineStringBodies(t *testing.T) {
	content := strings.Join([]string{
		`model = "user-model"`,
		`notes = """`,
		`model = "decoy"`,
		`[decoy_section]`,
		`model_verbosity = "decoy"`,
		`"""`,
		`model_reasoning_effort = 'ultra' # user choice`,
		`[profiles.other]`,
		`model = "scoped"`,
	}, "\n")

	overrides := collectCodexConfigOverrides(content)

	assert.Equal(t, map[string]string{
		".model":                  `"user-model"`,
		".model_reasoning_effort": `'ultra' # user choice`,
	}, overrides, "only root-section user-owned keys outside strings are overrides")
}

// Guards against a key parser that accepts dotted or whitespace-bearing keys,
// which would let a scoped setting masquerade as a root-owned one.
func TestParseCodexConfigKey_RejectsAmbiguousKeys(t *testing.T) {
	for _, test := range []struct {
		raw, want string
		ok        bool
	}{
		{raw: `model`, want: "model", ok: true},
		{raw: `"model"`, want: "model", ok: true},
		{raw: `'model'`, want: "model", ok: true},
		{raw: `"profiles.model"`},
		{raw: `'profiles.model'`},
		{raw: `profiles.model`},
		{raw: `""`},
		{raw: `''`},
		{raw: `'model`},
		{raw: `"model`},
		{raw: `my model`},
		{raw: ``},
	} {
		value, ok := parseCodexConfigKey(test.raw)
		assert.Equal(t, test.ok, ok, "key %q", test.raw)
		if ok {
			assert.Equal(t, test.want, value, "key %q", test.raw)
		}
	}
}

// Guards against a section header parser that accepts nested or empty
// brackets and then attributes user settings to the wrong section.
func TestParseCodexConfigSection_RequiresWellFormedHeader(t *testing.T) {
	section, ok := parseCodexConfigSection(`[profiles.alpha] # comment`)
	assert.True(t, ok)
	assert.Equal(t, "profiles.alpha", section)

	for _, header := range []string{`[]`, `[a[b]`, `[a]b]`, `model = 1`, `[unterminated`} {
		_, ok := parseCodexConfigSection(header)
		assert.False(t, ok, "header %q", header)
	}
}

// Guards against an assignment parser that treats comments or valueless lines
// as settings.
func TestParseCodexConfigAssignment_RequiresKeyAndValue(t *testing.T) {
	key, value, ok := parseCodexConfigAssignment(`model = "gpt-5"`)
	assert.True(t, ok)
	assert.Equal(t, "model", key)
	assert.Equal(t, `"gpt-5"`, value)

	for _, line := range []string{"", "# model = 1", "model", "model =", `profiles.model = 1`} {
		_, _, ok := parseCodexConfigAssignment(line)
		assert.False(t, ok, "line %q", line)
	}
}

// Guards against rewriting a line that carries no assignment at all.
func TestReplaceCodexConfigAssignmentValue_LeavesNonAssignmentsAlone(t *testing.T) {
	assert.Equal(t, `model = "new"`, replaceCodexConfigAssignmentValue(`model = "old"`, `"new"`))
	assert.Equal(t, `# comment`, replaceCodexConfigAssignmentValue(`# comment`, `"new"`))
}

// Guards against honouring a marker comment that lives inside a user multiline
// string, and against a marker key list widening past user-owned keys.
func TestMarkedCodexModelOverrides_ScopesMarkerToRealComments(t *testing.T) {
	overrides := map[string]string{
		".model":                  `"user"`,
		".model_reasoning_effort": `"ultra"`,
	}

	inString := strings.Join([]string{
		`notes = """`,
		codexUserModelMarker,
		`"""`,
	}, "\n")
	_, ok := markedCodexModelOverrides(inString, overrides)
	assert.False(t, ok, "a marker inside a user string is data, not ownership")

	all, ok := markedCodexModelOverrides(codexUserModelMarker+"\nmodel = \"user\"", overrides)
	require.True(t, ok)
	assert.Equal(t, overrides, all)

	subset, ok := markedCodexModelOverrides(
		codexUserModelMarker+": model, sandbox_mode, unknown_key", overrides)
	require.True(t, ok)
	assert.Equal(t, map[string]string{".model": `"user"`}, subset,
		"the marker must not claim ownership of keys outside the user-owned set")
}

// Guards against an unescaped-token scan that miscounts escapes and then
// leaves the parser stuck inside or outside a multiline string.
func TestCodexTOMLScanState_TracksEscapedDelimiters(t *testing.T) {
	var scan codexTOMLScanState
	assert.False(t, scan.skipSyntaxLine(`model = "a"`), "no multiline is open yet")

	scan.observeValue(`"""opening`)
	assert.True(t, scan.skipSyntaxLine(`model = "decoy"`))
	assert.True(t, scan.skipSyntaxLine(`trailing backslash \\"""`),
		"an even backslash run leaves the delimiter escaped-free but this line closes it")
	assert.False(t, scan.skipSyntaxLine(`model = "real"`), "the multiline string is closed")

	var balanced codexTOMLScanState
	balanced.observeValue(`"""inline"""`)
	assert.False(t, balanced.skipSyntaxLine(`model = "real"`),
		"a single-line multiline string must not open a scan state")

	var literal codexTOMLScanState
	literal.observeValue(`'''opening`)
	assert.True(t, literal.skipSyntaxLine(`model = "decoy"`))
	assert.True(t, literal.skipSyntaxLine(`'''`), "the closing line itself is still syntax")
	assert.False(t, literal.skipSyntaxLine(`model = "real"`))
}

// Guards against isEscapedAt treating an escaped backslash as escaping the
// following delimiter.
func TestIsEscapedAt_CountsBackslashRuns(t *testing.T) {
	assert.False(t, isEscapedAt(`a"`, 1))
	assert.True(t, isEscapedAt(`a\"`, 2))
	assert.False(t, isEscapedAt(`a\\"`, 3))
	assert.True(t, isEscapedAt(`a\\\"`, 4))
	assert.False(t, hasUnescapedToken(`a\"`, `"`))
	assert.True(t, hasUnescapedToken(`a\""`, `"`))
	assert.False(t, hasUnescapedToken(`abc`, `"`))
}
