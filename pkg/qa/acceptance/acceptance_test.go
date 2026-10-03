package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const qaLoopAcceptance = "../../../.autopus/specs/SPEC-QALOOP-001/acceptance.md"

func doc(lines ...string) []byte { return []byte(strings.Join(lines, "\n") + "\n") }

// AC-QALOOP-001: this SPEC's own acceptance.md yields every id in order.
func TestParse_QALoopAcceptance_YieldsEveryCriterionInOrder(t *testing.T) {
	t.Parallel()
	criteria, problems, err := Parse(qaLoopAcceptance)
	require.NoError(t, err)
	assert.Empty(t, problems)

	raw, err := os.ReadFile(qaLoopAcceptance)
	require.NoError(t, err)
	lines := strings.Split(string(raw), "\n")

	require.Len(t, criteria, 19)
	for i, c := range criteria {
		want := fmt.Sprintf("AC-QALOOP-%03d", i+1)
		assert.Equal(t, want, c.ID)
		require.True(t, c.Line >= 1 && c.Line <= len(lines), "%s line %d out of range", c.ID, c.Line)
		heading := fmt.Sprintf("### S%d: %s ", i+1, want)
		assert.True(t, strings.HasPrefix(lines[c.Line-1], heading), "%s: line %d is %q", c.ID, c.Line, lines[c.Line-1])
		assert.NotEmpty(t, c.Title, c.ID)
		assert.NotEmpty(t, c.When, c.ID)
		assert.NotEmpty(t, c.Then, c.ID)
		assert.NotEmpty(t, c.Given, c.ID)
	}

	first := criteria[0]
	assert.Equal(t, 7, first.Line)
	assert.Equal(t, "Acceptance criteria parse", first.Title)
	assert.Equal(t, "this SPEC's own `acceptance.md`", first.Given)
	assert.Equal(t, "the acceptance parser runs", first.When)
	assert.Equal(t, "it yields every `AC-QALOOP-*` id in document order, each with non-empty "+
		"given/when/then text and a correct line number.; a criterion with no THEN line is reported as `missing_then`.", first.Then)
	assert.Equal(t, 73, criteria[7].Line)
	assert.Equal(t, "Agent runner argv and overrides", criteria[7].Title)
}

func TestParseBytes_Recognition(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		body     []byte
		criteria []Criterion
		problems []Problem
	}{
		{
			name: "bold keywords with colons",
			body: doc(
				"## AC-LOGIN-1: User signs in",
				"",
				"**Given** a registered user",
				"**When:** they submit valid credentials",
				"__THEN__: the dashboard is shown",
				"- **And** a welcome toast appears",
				"**But a retry banner is hidden**",
			),
			criteria: []Criterion{{ID: "AC-LOGIN-1", Title: "User signs in", Given: "a registered user",
				When: "they submit valid credentials",
				Then: "the dashboard is shown; a welcome toast appears; a retry banner is hidden", Line: 1}},
		},
		{
			name: "list item criteria with checkboxes",
			body: doc(
				"Intro text.",
				"",
				"- [ ] AC-CART-2 — Cart keeps items",
				"  - given a cart with two items",
				"  - when the page reloads",
				"  - then both items remain",
				"1. [x] **AC-CART-3**: Cart empties",
				"   * Given a cart",
				"   * When it is cleared",
				"   * Then it is empty",
			),
			criteria: []Criterion{
				{ID: "AC-CART-2", Title: "Cart keeps items", Given: "a cart with two items", When: "the page reloads", Then: "both items remain", Line: 3},
				{ID: "AC-CART-3", Title: "Cart empties", Given: "a cart", When: "it is cleared", Then: "it is empty", Line: 7},
			},
		},
		{
			name: "scenario heading without an AC token is S<n>",
			body: doc(
				"# Acceptance",
				"",
				"### S3: Logout clears the session",
				"",
				"GIVEN a signed-in user",
				"WHEN they log out",
				"THEN the session cookie is gone",
				"",
				"#### S4: AC-OUT-9 — Logout redirects",
				"GIVEN a signed-in user",
				"WHEN they log out",
				"THEN they land on /login",
			),
			criteria: []Criterion{
				{ID: "S3", Title: "Logout clears the session", Given: "a signed-in user", When: "they log out", Then: "the session cookie is gone", Line: 3},
				{ID: "AC-OUT-9", Title: "Logout redirects", Given: "a signed-in user", When: "they log out", Then: "they land on /login", Line: 9},
			},
		},
		{
			name: "missing THEN is reported, not dropped",
			body: doc(
				"### S1: AC-A-1 — Has no outcome",
				"GIVEN a precondition",
				"WHEN something happens",
				"",
				"### S2: AC-A-2 — Complete",
				"GIVEN a",
				"WHEN b",
				"THEN c",
			),
			criteria: []Criterion{
				{ID: "AC-A-1", Title: "Has no outcome", Given: "a precondition", When: "something happens", Line: 1},
				{ID: "AC-A-2", Title: "Complete", Given: "a", When: "b", Then: "c", Line: 5},
			},
			problems: []Problem{{ID: "AC-A-1", Code: CodeMissingThen, Line: 1}},
		},
		{
			name: "wrapped lines continue a clause until a blank line",
			body: doc(
				"### AC-M-1: Multi-line clauses",
				"GIVEN a user with",
				"two saved carts",
				"WHEN they open the second cart",
				"and switch tabs",
				"THEN the cart total",
				"matches the server total",
				"AND no request",
				"is retried",
				"",
				"Prose after a blank line belongs to no clause.",
				"",
				"Then a later paragraph adds an outcome",
			),
			criteria: []Criterion{{ID: "AC-M-1", Title: "Multi-line clauses",
				Given: "a user with two saved carts",
				When:  "they open the second cart and switch tabs",
				Then:  "the cart total matches the server total; no request is retried; a later paragraph adds an outcome",
				Line:  1}},
		},
		{
			name: "fenced code is skipped",
			body: doc(
				"### AC-F-1: Fences",
				"GIVEN a fixture",
				"```markdown",
				"### AC-F-2: Not a criterion",
				"THEN not a clause",
				"```",
				"WHEN parsed",
				"THEN only the first criterion exists",
			),
			criteria: []Criterion{{ID: "AC-F-1", Title: "Fences", Given: "a fixture", When: "parsed", Then: "only the first criterion exists", Line: 1}},
		},
		{
			name: "duplicate id keeps the first",
			body: doc(
				"### AC-D-1: First",
				"GIVEN first given",
				"WHEN first when",
				"THEN first then",
				"",
				"### AC-D-1: Second",
				"GIVEN second given",
				"THEN second then",
			),
			criteria: []Criterion{{ID: "AC-D-1", Title: "First", Given: "first given", When: "first when", Then: "first then", Line: 1}},
			problems: []Problem{{ID: "AC-D-1", Code: CodeDuplicateID, Line: 6}},
		},
		{
			name: "unrelated section heading closes the criterion",
			body: doc(
				"## AC-H-1: Closed by a section ##",
				"GIVEN a",
				"WHEN b",
				"THEN c",
				"## Notes",
				"THEN stray outcome",
				"# AC-H-2 level one is not a criterion",
				"- TRAC-7 is not an id",
				"- Andrew is not a keyword",
			),
			criteria: []Criterion{{ID: "AC-H-1", Title: "Closed by a section", Given: "a", When: "b", Then: "c", Line: 1}},
		},
		{
			name:     "CRLF line endings and a BOM",
			body:     []byte("\ufeff### AC-W-1: Windows file\r\nGIVEN a\r\nWHEN b\r\nTHEN c\r\n"),
			criteria: []Criterion{{ID: "AC-W-1", Title: "Windows file", Given: "a", When: "b", Then: "c", Line: 1}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			criteria, problems := ParseBytes(tc.body)
			assert.Equal(t, tc.criteria, criteria)
			assert.Equal(t, tc.problems, problems)
		})
	}
}

func TestParseSpec_ReadsSpecAcceptance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := SpecPath(dir, "SPEC-T-1")
	assert.Equal(t, filepath.Join(dir, ".autopus", "specs", "SPEC-T-1", "acceptance.md"), path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, doc("### S1: AC-T-1 — Works", "GIVEN a", "WHEN b", "THEN c"), 0o644))

	criteria, problems, err := ParseSpec(dir, "SPEC-T-1")
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Equal(t, []Criterion{{ID: "AC-T-1", Title: "Works", Given: "a", When: "b", Then: "c", Line: 1}}, criteria)
}

func TestParseSpec_MissingFile_WrapsNotExist(t *testing.T) {
	t.Parallel()
	_, _, err := ParseSpec(t.TempDir(), "SPEC-NONE-1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "got %v", err)
}

func TestParseSpec_RejectsPathLikeIDs(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"", ".", "..", "../SPEC-X", "a/b", `a\b`} {
		_, _, err := ParseSpec(t.TempDir(), id)
		assert.Error(t, err, "spec id %q", id)
	}
}

func TestCriterion_JSONUsesSnakeCaseKeys(t *testing.T) {
	t.Parallel()
	got, err := json.Marshal(Criterion{ID: "AC-1", Title: "t", Given: "g", When: "w", Then: "th", Line: 3})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"AC-1","title":"t","given":"g","when":"w","then":"th","line":3}`, string(got))
	got, err = json.Marshal(Problem{ID: "AC-1", Code: CodeMissingThen, Line: 3})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"AC-1","code":"missing_then","line":3}`, string(got))
}
