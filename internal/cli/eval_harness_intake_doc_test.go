package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/insajin/autopus-adk/pkg/harneval/intake"
)

// readmeIntakeSection returns the "Incident intake" section of
// evals/harness/README.md, up to the next second-level heading.
func readmeIntakeSection(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRootForTest(t), "evals", "harness", "README.md"))
	require.NoError(t, err)
	_, section, found := strings.Cut(string(data), "\n## Incident intake")
	require.True(t, found, "README has no Incident intake section")
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	return section
}

// fencedBlock returns the body of the first ```lang block in text; the fence
// may be indented under a list item.
func fencedBlock(t *testing.T, text, lang string) string {
	t.Helper()
	_, rest, found := strings.Cut(text, "```"+lang+"\n")
	require.True(t, found, "no %s block", lang)
	body, _, found := strings.Cut(rest, "```")
	require.True(t, found, "unclosed %s block", lang)
	return body
}

// shellWords splits a documented command line on spaces and keeps a
// double-quoted run together, the only quoting the README uses.
func shellWords(t *testing.T, line string) []string {
	t.Helper()
	var words []string
	var word strings.Builder
	inWord, quoted := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted, inWord = !quoted, true
		case r == ' ' && !quoted:
			if inWord {
				words = append(words, word.String())
				word.Reset()
			}
			inWord = false
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	require.False(t, quoted, "unbalanced quote in %q", line)
	if inWord {
		words = append(words, word.String())
	}
	return words
}

// runLearnCLI runs `auto learn <args>` in the working directory.
func runLearnCLI(t *testing.T, args ...string) harnessOutcome {
	t.Helper()
	cmd := newLearnCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	code := 0
	if err := cmd.Execute(); err != nil {
		code = exitCodeForError(err)
		stderr.WriteString("Error: " + err.Error() + "\n")
	}
	return harnessOutcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// writeDocumentedDraft writes the README's draft fields into the task of the
// open candidate id below tree, as the person in the flow does.
func writeDocumentedDraft(t *testing.T, tree *harnessTree, section, id string) {
	t.Helper()
	var draft struct {
		Category   string               `json:"category"`
		Assertions []harneval.Assertion `json:"assertions"`
	}
	decoder := json.NewDecoder(strings.NewReader(fencedBlock(t, section, "json")))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&draft))
	rel := intake.IntakeDir + "/" + id + ".json"
	data, err := os.ReadFile(filepath.Join(tree.root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	var candidate intake.Candidate
	require.NoError(t, json.Unmarshal(data, &candidate))
	candidate.Task.Category, candidate.Task.Assertions = draft.Category, draft.Assertions
	tree.writeJSON(rel, candidate)
}

// TestEvalHarnessIntake_S9_ReadmeFlowRunsInOrder runs the README's intake
// commands verbatim from a project root, with the comment line standing for
// the documented draft edit. Not parallel: it changes the working directory,
// and promote and run swap PATH and HOME while they generate surfaces.
func TestEvalHarnessIntake_S9_ReadmeFlowRunsInOrder(t *testing.T) {
	// Given a golden set its baseline pins, with the project root as the
	// working directory the README runs from.
	section := readmeIntakeSection(t)
	tree := standardHarnessTree(t)
	deps := promoteHarnessDeps()
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)
	t.Chdir(tree.root)

	// When each documented line runs in order.
	var steps []string
	candidateID := ""
	for _, line := range strings.Split(fencedBlock(t, section, "sh"), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			require.NotEmpty(t, candidateID, "the draft is written once intake created the candidate")
			writeDocumentedDraft(t, tree, section, candidateID)
			steps = append(steps, "write the draft")
			continue
		}
		words := shellWords(t, line)
		require.GreaterOrEqual(t, len(words), 3, line)
		require.Equal(t, "auto", words[0], line)
		var got harnessOutcome
		switch {
		case words[1] == "learn":
			got = runLearnCLI(t, words[2:]...)
			steps = append(steps, "learn "+words[2])
		case words[1] == "eval" && words[2] == "harness":
			got = runHarness(t, deps, words[3:]...)
			steps = append(steps, words[3])
		default:
			t.Fatalf("the README documents an unknown command: %s", line)
		}
		require.Equal(t, 0, got.code, "%s\nstderr: %s", line, got.stderr)
		switch steps[len(steps)-1] {
		case "intake":
			rows := harnessDoc(t, got.stdout)["rows"].([]any)
			require.Len(t, rows, 1, got.stdout)
			row := rows[0].(map[string]any)
			require.Equal(t, intake.ResultCreated, row["result"], got.stdout)
			candidateID = row["candidate_id"].(string)
		case "promote":
			require.Equal(t, candidateID, words[4], "the README promotes the candidate intake created")
			doc := harnessDoc(t, got.stdout)
			assert.Equal(t, intake.PromoteResultPromoted, doc["result"])
			assert.Equal(t, intake.OutcomePass, doc["current_outcome"])
		}
	}

	// Then record, intake, the draft, promote, and the baseline update ran in
	// this order, and the golden set passes with the promoted task pinned.
	assert.Equal(t, []string{"learn record", "intake", "write the draft", "promote", "baseline"}, steps)
	run := runHarness(t, deps, "run", "--format", "json")
	require.Equal(t, 0, run.code, run.stderr)
	assert.Equal(t, harneval.StatusPass, harnessDoc(t, run.stdout)["status"])
	assert.FileExists(t, filepath.Join(tree.root, filepath.FromSlash(promoteTask)))
	assert.FileExists(t, filepath.Join(tree.root, filepath.FromSlash(promoteLink)))

	// And the README states the three caveats.
	for _, caveat := range []string{"**Mixed-version prune.**", "**Duplicate entries.**", "**Shell history.**"} {
		assert.Contains(t, section, caveat)
	}
}

func TestEvalHarnessIntake_S9_HelpTextsNameTheFlowAndCaveats(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"intake", "promote", "reject"} {
		got := runHarness(t, evalHarnessDeps{}, command, "--help")

		require.Equal(t, 0, got.code, got.stderr)
		for _, want := range []string{
			"auto learn record", "auto eval harness intake", "auto eval harness promote",
			"auto eval harness reject", "auto eval harness baseline --update", "evals/harness/README.md",
			"older than this flow", "does not join", "shell history",
		} {
			assert.Contains(t, got.stdout, want, command)
		}
	}
}
