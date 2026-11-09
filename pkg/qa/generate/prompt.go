// Package generate asks a headless agent to draft scenario candidates from a
// SPEC's acceptance criteria (SPEC-QALOOP-001 REQ-6).
//
// The agent proposes; the harness decides. Every document the agent prints is
// parsed with the same strictness as a file a person wrote, and is then held to
// the parsed criteria: a step or case that cites an acceptance id the SPEC does
// not declare is rejected before anything touches disk. Accepted documents land
// only in the candidate directories, which nothing compiles or runs until a
// person promotes them.
package generate

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// Prompt bounds keep a very long acceptance.md from producing a prompt the
// agent silently truncates. Anything left out is named as omitted, so the gap
// shows up as uncovered criteria rather than as a quietly shorter prompt.
const (
	MaxPromptCriteria    = 60
	MaxCriterionChars    = 2000
	MaxPromptScenarioIDs = 200
	MaxPromptJourneys    = 20
)

// JourneyHint names a Journey Pack a generated scenario may run under and the
// first origin it allows.
type JourneyHint struct {
	ID     string `json:"id"`
	Origin string `json:"origin,omitempty"`
	// ReadOnly marks a gui-explore pack. Its guard blocks click and fill, so
	// only scenarios without action steps may name it.
	ReadOnly bool `json:"read_only,omitempty"`
}

// PromptInput is everything the generation prompt is built from.
type PromptInput struct {
	SpecID              string
	Criteria            []acceptance.Criterion
	ExistingScenarioIDs []string
	Journeys            []JourneyHint
	// Readings are answered intent questions; Undecided are open ones.
	Readings  []string
	Undecided []string
}

const scenarioSchema = `schema_version: qamesh.scenario.v2
id: lowercase-kebab-id            # must not reuse an existing scenario id
title: short description
journey: <journey id>             # Journey Pack that runs the scenario
origin: http://host:port          # optional; one the journey allows
intent_source: acceptance
spec: %[1]s
acceptance_refs: [<criterion id>] # every ac a step cites
screens:
  - id: lowercase-kebab-id
    path: /route                  # origin-relative, starts with one '/'
    steps:                        # each step sets exactly one action or expect key
      - fill: {label: Email, value: user@example.com}
      - fill: {label: Password, value_env: E2E_PASSWORD}
      - click: {role: button, name: Sign in}
      - press: {key: Enter}
      - check: {label: Remember me}
      - select: {label: Country, option: Korea}
      - wait_url: /dashboard
      - expect_url: /dashboard
        ac: <criterion id>
      - expect_title: Dashboard
        ac: <criterion id>
      - expect_text: Welcome back
        ac: <criterion id>
      - expect_role: {role: heading, name: Dashboard, exact: true}
        ac: <criterion id>
      - expect_count: {role: listitem, name: Product, count: 3}
        ac: <criterion id>
`

const testScenariosSchema = `schema_version: qamesh.test-scenarios.v1
spec: %[1]s
cases:
  - id: lowercase-kebab-id
    ac: <criterion id>
    kind: happy | negative | edge
    title: short description
    given: precondition
    when: action
    then: expected outcome, as stated by the criterion
    automation:
      type: gui | exec | command | manual
      scenario: <user scenario id>   # gui only
      exec:                          # exec only: run the product, check what a user sees
        build: [go, build, -o, $BIN/tool, .]   # runs in the project root; name a package directory that exists
        argv: [$BIN/tool, --flag, input.txt]            # runs in a fresh dir holding only files
        stdin: ""
        files: {input.txt: "line one\n"}
        timeout: 1m
        expect:                      # at least one; each value must come from the criterion
          exit_code: 0               # or exit_nonzero: true when only "fails" is stated
          stdout: "exact output\n"
          stdout_contains: [substring]
          stdout_not_contains: [substring]   # must not appear anywhere in stdout
          stdout_lines: [first, second]      # exactly this many non-empty lines; line i contains entry i
          stderr_contains: [substring]
          files: {out.txt: "exact content"}
        # A server instead: argv starts it on $PORT (also the PORT env var),
        # requests run in order against it, then it is stopped. Use requests
        # with no top-level expect and no stdin.
        # ready: /healthz            # polled until it answers; default waits for the port
        # requests:
        #   - method: POST           # a setup request may omit expect; the last request must have one
        #     path: /items           # a path on this server, never a URL
        #     headers: {Content-Type: application/json}
        #     body: '{"name": "apple"}'
        #     expect: {status: 201}
        #   - restart: true          # stop and start the server again on the same port
        #   - method: GET
        #     path: /items
        #     expect: {status: 200, body_contains: [apple], body_not_contains: [banana], headers: {Content-Type: json}}
        #   - method: GET
        #     path: /empty
        #     expect: {status: 200, body: ""}   # body is an exact match; "" is an empty body
        #   - method: GET
        #     path: /items
        #     expect: {status: 200, body: "apple,banana"}   # quote it: inside {...} a bare comma ends the value
      check:                         # command only: a test runner's exit code
        argv: [go, test, ./...]
        cwd: .
        timeout: 2m
      reason: what a person must judge  # manual only
`

// BuildPrompt renders the bounded generation prompt. The output contract is
// strict on purpose: YAML only, one fenced block per document, criteria cited by
// id, and no expected value the criteria do not state.
func BuildPrompt(in PromptInput) string {
	spec := strings.TrimSpace(in.SpecID)
	var b strings.Builder
	fmt.Fprintf(&b, "You are drafting QA test designs for %s from its acceptance criteria.\n", spec)
	b.WriteString("Do not create, edit, or delete any file. Reply only with YAML documents; the harness validates them and keeps the valid ones as candidates for a person to review.\n\n")
	b.WriteString("Rules:\n")
	fmt.Fprintf(&b, "- Produce exactly ONE %s document for spec %s. Give every criterion a happy case, plus negative and edge cases where the criterion implies them.\n", testscenario.SchemaVersion, spec)
	fmt.Fprintf(&b, "- For each criterion a user can observe in the GUI, also produce a %s user scenario with intent_source: %s and spec: %s, and point the matching case at it with automation type gui.\n", scenario.SchemaVersionV2, scenario.IntentAcceptance, spec)
	b.WriteString("- Put each document in its own ```yaml fenced block. Write no prose outside the blocks.\n")
	b.WriteString("- Cite criteria only by the ids listed below. Every expect step and every case needs an ac; every step ac must also appear in acceptance_refs.\n")
	b.WriteString("- Never invent expected values: assert only text, counts, titles, and URLs a criterion states. When a criterion is too vague to assert, write a manual case whose reason names what is missing.\n")
	b.WriteString("- Address elements by role and accessible name, label, placeholder, text, or test_id; never CSS or XPath. An action target uses exactly one locator kind.\n")
	b.WriteString("- Never put secrets in YAML: fill credentials with value_env naming an environment variable the criteria or their test data name.\n")
	b.WriteString("- Assert visible messages with expect_text. Never give expect_role a name for alert, status, log, marquee, or timer: those roles do not take their name from their text.\n")
	b.WriteString("- Use literal values for inputs that are not secrets, such as a deliberately wrong password or a search term; do not invent environment variables for them.\n")
	b.WriteString("- For each criterion a user observes by running the product itself (a CLI's output, exit code, or the files it writes), write an exec case: it carries the expected value, so prefer it to a command case, which only learns whether a test runner exited 0.\n")
	b.WriteString("- When a criterion fixes the order or number of output entries but not the line format, assert it with stdout_lines rather than writing a manual case; assert what must not appear with stdout_not_contains. stdout_contains alone cannot fail on wrong order, extra entries, or a duplicate.\n")
	b.WriteString("- For each criterion a client observes from a server the product runs (an HTTP status, a response body, data a later request reads back), write an exec case with requests: argv starts the server on $PORT and each request carries its own expect. Never call curl or another client from argv. When a criterion is about what survives a restart, put a `restart: true` entry between the requests that write and the ones that read. Double-quote every body, body_contains, and body_not_contains value: in a {...} mapping an unquoted comma ends the value and the rest is read as a new key.\n")
	b.WriteString("- Never assume the product is already built: an exec case that runs a compiled program needs a build step that writes it, usually into $BIN.\n")
	b.WriteString("- When a criterion says the command fails or exits with an error but names no code, use exit_nonzero: true; never guess a specific exit code.\n")
	fmt.Fprintf(&b, "- An exec argv[0] must be $BIN/<name> from its build step, a path inside the project such as ./bin/tool, or one of: %s. An exec build[0] and a command check's argv[0] must be one of: %s.\n", strings.Join(testscenario.CommandAllowlist, ", "), strings.Join(testscenario.CommandAllowlist, ", "))
	b.WriteString("- Unknown keys are rejected, so use only the keys shown in the schemas.\n\n")

	fmt.Fprintf(&b, "Schema %s:\n%s\n", scenario.SchemaVersionV2, fmt.Sprintf(scenarioSchema, spec))
	fmt.Fprintf(&b, "Schema %s:\n%s\n", testscenario.SchemaVersion, fmt.Sprintf(testScenariosSchema, spec))

	writeJourneys(&b, in.Journeys)
	writeExisting(&b, in.ExistingScenarioIDs)
	writeCriteria(&b, in.Criteria)
	writeReadings(&b, in.Readings, in.Undecided)
	return b.String()
}

// writeReadings passes on how the user resolved the request's ambiguities.
// A resolved reading may be asserted; an undecided one must not be, in
// either direction, because nobody has said which one the user meant.
func writeReadings(b *strings.Builder, readings, undecided []string) {
	if len(readings) > 0 {
		b.WriteString("\nResolved readings (assert these where a criterion leaves the point open):\n")
		for _, r := range readings {
			fmt.Fprintf(b, "- %s\n", bound(r, MaxCriterionChars))
		}
	}
	if len(undecided) > 0 {
		b.WriteString("\nUndecided points (assert neither option; keep expectations that hold under every option):\n")
		for _, u := range undecided {
			fmt.Fprintf(b, "- %s\n", bound(u, MaxCriterionChars))
		}
	}
}

func writeJourneys(b *strings.Builder, journeys []JourneyHint) {
	if len(journeys) == 0 {
		b.WriteString("Journeys: none declares GUI origins, so use exec, command, or manual automation instead of gui cases and user scenarios.\n\n")
		return
	}
	b.WriteString("Journeys a user scenario may name (id and allowed origin):\n")
	for i, j := range journeys {
		if i == MaxPromptJourneys {
			fmt.Fprintf(b, "- (%d more omitted)\n", len(journeys)-i)
			break
		}
		mode := "actions allowed"
		if j.ReadOnly {
			mode = "read-only: expect steps only, never click/fill/press/check/select"
		}
		fmt.Fprintf(b, "- %s %s (%s)\n", oneLine(j.ID), oneLine(j.Origin), mode)
	}
	b.WriteString("\n")
}

func writeExisting(b *strings.Builder, ids []string) {
	if len(ids) == 0 {
		b.WriteString("Existing scenario ids: none.\n\n")
		return
	}
	shown := ids
	if len(shown) > MaxPromptScenarioIDs {
		shown = shown[:MaxPromptScenarioIDs]
	}
	fmt.Fprintf(b, "Existing scenario ids (taken: do not reuse them; a gui case may reference them): %s", strings.Join(shown, ", "))
	if len(ids) > len(shown) {
		fmt.Fprintf(b, " (%d more omitted)", len(ids)-len(shown))
	}
	b.WriteString("\n\n")
}

func writeCriteria(b *strings.Builder, criteria []acceptance.Criterion) {
	fmt.Fprintf(b, "Acceptance criteria (%d):\n", len(criteria))
	for i, c := range criteria {
		if i == MaxPromptCriteria {
			fmt.Fprintf(b, "(%d more criteria omitted from this prompt; leave them uncovered)\n", len(criteria)-i)
			break
		}
		b.WriteString(criterionBlock(c))
	}
}

func criterionBlock(c acceptance.Criterion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %s: %s\n", c.ID, oneLine(c.Title))
	fmt.Fprintf(&b, "  GIVEN %s\n", orUnstated(c.Given))
	fmt.Fprintf(&b, "  WHEN %s\n", orUnstated(c.When))
	if strings.TrimSpace(c.Then) == "" {
		b.WriteString("  THEN (not stated: write only a manual case for this id)\n")
	} else {
		fmt.Fprintf(&b, "  THEN %s\n", oneLine(c.Then))
	}
	return bound(b.String(), MaxCriterionChars)
}

func orUnstated(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(not stated)"
	}
	return oneLine(s)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// bound truncates on a rune boundary so a multibyte criterion never yields an
// invalid UTF-8 prompt.
func bound(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	const marker = " [truncated]\n"
	keep := limit - len([]rune(marker))
	if keep < 0 {
		keep = 0
	}
	return string(runes[:keep]) + marker
}
