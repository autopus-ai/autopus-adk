package harneval

import "encoding/json"

// expectationInput is the closed field set an expectation digest covers.
// intent, outcome, category, provenance, and status wording stay out.
type expectationInput struct {
	Kind           string          `json:"kind"`
	Variants       []Variant       `json:"variants"`
	Assertions     []Assertion     `json:"assertions"`
	CorpusRef      CorpusRef       `json:"corpus_ref"`
	ExpectedTests  []string        `json:"expected_tests"`
	OracleMode     string          `json:"oracle_mode,omitempty"`
	BlackBoxOracle *BlackBoxOracle `json:"black_box_oracle,omitempty"`
}

// ExpectationDigest is the SHA-256 hex of the Go json.Marshal of the task's
// {kind, variants, assertions, corpus_ref, expected_tests, oracle_mode,
// black_box_oracle}. Absent and empty collections encode alike as [] or {}; a
// surface task carries an empty corpus_ref and no expected tests, so only
// behavior moves the digest. A white-box task, whose oracle_mode is absent or
// white_box, encodes neither oracle field, so its digest is the
// SPEC-HARNEVAL-001 one; a black-box task adds its mode and its definition,
// whose fixture sha256 pins move the digest when an expectation changes.
func ExpectationDigest(task Task) string {
	input := expectationInput{
		Kind:           task.Kind,
		Variants:       make([]Variant, 0, len(task.Variants)),
		Assertions:     task.Assertions,
		ExpectedTests:  task.ExpectedTests,
		BlackBoxOracle: task.BlackBoxOracle,
	}
	if task.OracleMode == OracleModeBlackBox {
		input.OracleMode = OracleModeBlackBox
	}
	for _, variant := range task.Variants {
		if variant.Overrides == nil {
			variant.Overrides = map[string]bool{}
		}
		input.Variants = append(input.Variants, variant)
	}
	if input.Assertions == nil {
		input.Assertions = []Assertion{}
	}
	if input.ExpectedTests == nil {
		input.ExpectedTests = []string{}
	}
	if task.CorpusRef != nil {
		input.CorpusRef = *task.CorpusRef
	}
	return sha256Hex(mustMarshal(input))
}

type setEntry struct {
	ID                string `json:"id"`
	State             string `json:"state"`
	ExpectationDigest string `json:"expectation_digest"`
}

// SetDigest is the SHA-256 hex of set_version followed by the json.Marshal of
// the id-ascending {id, state, expectation_digest} entries of every loaded
// task, tombstones included.
func SetDigest(set *Set) string {
	return digestEntries(set, func(Task) bool { return true })
}

// AgentSetDigest is SetDigest restricted to agent tasks.
func AgentSetDigest(set *Set) string {
	return digestEntries(set, func(task Task) bool { return task.Kind == KindAgent })
}

// digestEntries relies on Set.Tasks being id-sorted, as LoadSet leaves it.
func digestEntries(set *Set, include func(Task) bool) string {
	entries := make([]setEntry, 0, len(set.Tasks))
	for _, task := range set.Tasks {
		if include(task) {
			entries = append(entries, setEntry{ID: task.ID, State: task.Status.State, ExpectationDigest: ExpectationDigest(task)})
		}
	}
	return sha256Hex(append([]byte(set.Manifest.SetVersion), mustMarshal(entries)...))
}

// mustMarshal encodes plain data types, which json.Marshal cannot reject.
func mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic("harneval: marshal plain data: " + err.Error())
	}
	return data
}
