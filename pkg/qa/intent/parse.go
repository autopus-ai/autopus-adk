package intent

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
)

// Question is one ambiguity the user resolves. Recommended indexes Options
// and names the reading the criterion was written with.
type Question struct {
	ID          string   `yaml:"id" json:"id"`
	Criterion   string   `yaml:"criterion" json:"criterion"`
	Question    string   `yaml:"question" json:"question"`
	Options     []string `yaml:"options" json:"options"`
	Recommended int      `yaml:"recommended" json:"recommended"`
}

// Draft is a parsed and checked agent answer.
type Draft struct {
	Acceptance string
	Criteria   []acceptance.Criterion
	Questions  []Question
}

// ParseOutput reads the two fenced blocks from the agent's stdout and checks
// them. A draft that cannot serve as an oracle is an error, not a warning:
// every criterion needs an observable Then, and every question must point at
// a criterion and offer a real choice.
func ParseOutput(stdout string) (Draft, error) {
	body, ok := fencedBlock(stdout, "acceptance")
	if !ok {
		return Draft{}, &Error{Code: CodeOutputInvalid, Message: "the agent returned no ```acceptance block"}
	}
	criteria, problems := acceptance.ParseBytes([]byte(body))
	if len(criteria) == 0 {
		return Draft{}, &Error{Code: CodeNoCriteria, Message: "the acceptance block holds no AC-<n> criterion"}
	}
	if len(problems) > 0 {
		p := problems[0]
		return Draft{}, &Error{Code: CodeOutputInvalid, Message: fmt.Sprintf("criterion %s: %s (line %d)", p.ID, p.Code, p.Line)}
	}
	known := map[string]bool{}
	for _, c := range criteria {
		known[c.ID] = true
	}
	questions := []Question{}
	if raw, ok := fencedBlock(stdout, "intent-questions"); ok && strings.TrimSpace(raw) != "" {
		if err := yaml.Unmarshal([]byte(raw), &questions); err != nil {
			return Draft{}, &Error{Code: CodeOutputInvalid, Message: "intent-questions is not a YAML list: " + err.Error()}
		}
	}
	seen := map[string]bool{}
	for _, q := range questions {
		switch {
		case strings.TrimSpace(q.ID) == "" || seen[q.ID]:
			return Draft{}, &Error{Code: CodeOutputInvalid, Message: fmt.Sprintf("question id %q is empty or repeated", q.ID)}
		case !known[q.Criterion]:
			return Draft{}, &Error{Code: CodeOutputInvalid, Message: fmt.Sprintf("question %s names unknown criterion %q", q.ID, q.Criterion)}
		case len(q.Options) < 2 || len(q.Options) > 4:
			return Draft{}, &Error{Code: CodeOutputInvalid, Message: fmt.Sprintf("question %s has %d options; want 2 to 4", q.ID, len(q.Options))}
		case q.Recommended < 0 || q.Recommended >= len(q.Options):
			return Draft{}, &Error{Code: CodeOutputInvalid, Message: fmt.Sprintf("question %s recommends option %d of %d", q.ID, q.Recommended, len(q.Options))}
		}
		seen[q.ID] = true
	}
	return Draft{Acceptance: strings.TrimSpace(body) + "\n", Criteria: criteria, Questions: questions}, nil
}

// fencedBlock returns the body of the first ```lang fence in text.
func fencedBlock(text, lang string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "```"+lang {
			continue
		}
		var body []string
		for _, inner := range lines[i+1:] {
			if strings.TrimSpace(inner) == "```" {
				return strings.Join(body, "\n"), true
			}
			body = append(body, inner)
		}
		return "", false
	}
	return "", false
}
