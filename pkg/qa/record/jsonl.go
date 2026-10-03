package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// jsonlTarget mirrors scenario.Target field for field so a decoded target
// converts directly; only the tags differ.
type jsonlTarget struct {
	Role        string `json:"role"`
	Name        string `json:"name"`
	Exact       bool   `json:"exact"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	Text        string `json:"text"`
	TestID      string `json:"test_id"`
}

type jsonlLine struct {
	SchemaVersion string       `json:"schema_version"`
	Action        string       `json:"action"`
	Expect        string       `json:"expect"`
	Target        *jsonlTarget `json:"target"`
	Value         string       `json:"value"`
	ValueEnv      string       `json:"value_env"`
	Key           string       `json:"key"`
	URL           string       `json:"url"`
	Option        string       `json:"option"`
	By            string       `json:"by"`
	Ac            string       `json:"ac"`
}

type fieldRule struct{ required, optional []string }

// payloadFields is every field that carries step data, in report order.
var payloadFields = []string{"target", "value", "value_env", "key", "url", "option"}

// jsonlRules lists the payload fields each line kind requires and may carry.
// A field the kind does not use is refused rather than dropped: the agent that
// logged it would otherwise believe it had been recorded.
var jsonlRules = map[string]fieldRule{
	ActionGoto:              {required: []string{"url"}},
	ActionClick:             {required: []string{"target"}},
	ActionCheck:             {required: []string{"target"}},
	ActionFill:              {required: []string{"target"}, optional: []string{"value", "value_env"}},
	ActionPress:             {required: []string{"key"}, optional: []string{"target"}},
	ActionSelect:            {required: []string{"target", "option"}},
	"expect " + ExpectText:  {required: []string{"value"}},
	"expect " + ExpectTitle: {required: []string{"value"}},
	"expect " + ExpectURL:   {required: []string{"url"}},
	"expect " + ExpectRole:  {required: []string{"target"}},
}

// ParseJSONL reads a qamesh.recording.v1 log, one JSON object per line. A
// line holding only schema_version is a header. A line that does not decode,
// names an unknown kind, omits by, or carries a field its kind does not use
// is reported instead of guessed at.
func ParseJSONL(src []byte) (Recording, []Unsupported) {
	var rec Recording
	var bad []Unsupported
	for index, raw := range strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n") {
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		event, header, err := jsonlEvent(text)
		switch {
		case err != nil:
			bad = append(bad, Unsupported{Line: index + 1, Text: text, Reason: err.Error()})
		case !header:
			event.Line = index + 1
			rec.Events = append(rec.Events, event)
		}
	}
	return rec, bad
}

func jsonlEvent(text string) (Event, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	var line jsonlLine
	if err := decoder.Decode(&line); err != nil {
		return Event{}, false, fmt.Errorf("not a recording line: %v", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Event{}, false, errors.New("a line holds exactly one JSON object")
	}
	if line.SchemaVersion != "" && line.SchemaVersion != SchemaVersion {
		return Event{}, false, fmt.Errorf("schema_version must be %s, got %q", SchemaVersion, line.SchemaVersion)
	}
	present := presentFields(line)
	if line.Action == "" && line.Expect == "" {
		if line.SchemaVersion != "" && len(present) == 0 && line.By == "" && line.Ac == "" {
			return Event{}, true, nil
		}
		return Event{}, false, errors.New("line sets neither action nor expect")
	}
	if line.Action != "" && line.Expect != "" {
		return Event{}, false, errors.New("line sets both action and expect")
	}
	if err := checkFields(line, present); err != nil {
		return Event{}, false, err
	}
	if line.By != scenario.ByHuman && line.By != scenario.ByAgent {
		return Event{}, false, fmt.Errorf("by must be human or agent, got %q", line.By)
	}
	event := Event{Action: line.Action, Expect: line.Expect, Value: line.Value, ValueEnv: line.ValueEnv,
		Key: line.Key, URL: line.URL, By: line.By, Ac: strings.TrimSpace(line.Ac)}
	if line.Action == ActionSelect {
		event.Value = line.Option
	}
	if line.Target != nil {
		event.Target = scenario.Target(*line.Target)
		if err := checkTarget(line, event.Target); err != nil {
			return Event{}, false, err
		}
	}
	return event, false, nil
}

func presentFields(line jsonlLine) map[string]bool {
	present := map[string]bool{}
	for name, set := range map[string]bool{
		"target": line.Target != nil, "value": line.Value != "", "value_env": line.ValueEnv != "",
		"key": line.Key != "", "url": line.URL != "", "option": line.Option != "",
	} {
		if set {
			present[name] = true
		}
	}
	return present
}

func lineKind(line jsonlLine) string {
	if line.Expect != "" {
		return "expect " + line.Expect
	}
	return line.Action
}

func checkFields(line jsonlLine, present map[string]bool) error {
	kind := lineKind(line)
	rule, ok := jsonlRules[kind]
	if !ok {
		if line.Expect != "" {
			return fmt.Errorf("unknown expect %q", line.Expect)
		}
		return fmt.Errorf("unknown action %q", line.Action)
	}
	allowed := map[string]bool{}
	for _, name := range rule.required {
		if !present[name] {
			return fmt.Errorf("%s requires %s", kind, name)
		}
		allowed[name] = true
	}
	for _, name := range rule.optional {
		allowed[name] = true
	}
	for _, name := range payloadFields {
		if present[name] && !allowed[name] {
			return fmt.Errorf("%s does not use %s", kind, name)
		}
	}
	if kind == ActionFill && present["value"] == present["value_env"] {
		return errors.New("fill needs exactly one of value or value_env")
	}
	return nil
}

// checkTarget refuses a target that addresses its element more than one way,
// a name with no role, and a role expectation whose target is not a role.
func checkTarget(line jsonlLine, t scenario.Target) error {
	kinds := 0
	for _, set := range []bool{t.Role != "" || t.Name != "", t.Label != "", t.Placeholder != "", t.Text != "", t.TestID != ""} {
		if set {
			kinds++
		}
	}
	switch {
	case kinds != 1:
		return errors.New("target needs exactly one of role, label, placeholder, text, or test_id")
	case t.Name != "" && strings.TrimSpace(t.Role) == "":
		return errors.New("target.name needs target.role")
	case line.Expect == ExpectRole && strings.TrimSpace(t.Role) == "":
		return errors.New("expect role needs target.role")
	}
	return nil
}
