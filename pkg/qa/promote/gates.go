package promote

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

type criteriaSet struct {
	ids map[string]bool
	err error
}

type promoter struct {
	projectDir string
	opts       Options
	criteria   map[string]criteriaSet
	activeIDs  map[string]string
}

var errConflict = errors.New("active file differs")

func (p *promoter) promote(c candidate, r *Report) {
	skip := func(code, format string, args ...any) {
		r.Skipped = append(r.Skipped, Skip{ID: c.id, Kind: c.kind, Code: code, Message: fmt.Sprintf(format, args...)})
	}
	if c.err != nil {
		skip(CodeInvalid, "%s", c.err.Error())
		return
	}
	body, accepted := c.body, 0
	if c.kind == KindScenario {
		if msg := p.missingRefs(c.scenario.Spec, citedRefs(c.scenario), c.scenario.IntentSource == scenario.IntentAcceptance); msg != "" {
			skip(CodeAcceptanceRefMissing, "%s", msg)
			return
		}
		if n := unconfirmed(c.scenario); n > 0 {
			if !p.opts.AcceptAgentAssertions {
				skip(CodeUnconfirmedAgentAssertion, "%d agent-authored assertion(s) await confirmation: give each step an ac, or review them and re-run with --accept-agent-assertions", n)
				return
			}
			rewritten, err := acceptAssertions(c.rel, c.body)
			if err != nil {
				skip(CodeInvalid, "%s", err.Error())
				return
			}
			body, accepted = rewritten, n
		}
		if other, clash := p.activeIDs[c.scenario.ID]; clash && other != c.name {
			skip(CodeConflict, "active scenario %s already declares id %q", other, c.scenario.ID)
			return
		}
	} else {
		var refs []string
		for _, tc := range c.doc.Cases {
			refs = append(refs, strings.TrimSpace(tc.Ac))
		}
		if msg := p.missingRefs(c.doc.Spec, refs, true); msg != "" {
			skip(CodeAcceptanceRefMissing, "%s", msg)
			return
		}
	}
	item := Item{ID: c.id, Kind: c.kind, From: c.rel, To: c.activeRel, AcceptedAssertions: accepted}
	switch err := p.move(c, body, &item); {
	case errors.Is(err, errConflict):
		skip(CodeConflict, "%s already exists with different content and is never overwritten", c.activeRel)
	case err != nil:
		skip(CodeFailed, "%s", err.Error())
	default:
		if c.kind == KindScenario && !p.opts.DryRun {
			p.activeIDs[c.scenario.ID] = c.name
		}
		r.Promoted = append(r.Promoted, item)
	}
}

// missingRefs explains the first cited acceptance id that no longer resolves.
// An ac that names no spec cannot be shown to exist, so it does not pass either.
func (p *promoter) missingRefs(spec string, refs []string, needSpec bool) string {
	if len(refs) == 0 && !needSpec {
		return ""
	}
	if spec = strings.TrimSpace(spec); spec == "" {
		if len(refs) == 0 {
			return "names no spec to resolve its acceptance criteria against"
		}
		return fmt.Sprintf("cites %s but names no spec to resolve it against", refs[0])
	}
	set, ok := p.criteria[spec]
	if !ok {
		criteria, _, err := acceptance.ParseSpec(p.projectDir, spec)
		set = criteriaSet{ids: map[string]bool{}, err: err}
		for _, c := range criteria {
			set.ids[c.ID] = true
		}
		p.criteria[spec] = set
	}
	if set.err != nil {
		return fmt.Sprintf("acceptance criteria of %s are unavailable: %v", spec, set.err)
	}
	for _, ref := range refs {
		if !set.ids[ref] {
			return fmt.Sprintf("%s is not a criterion of %s", ref, spec)
		}
	}
	return ""
}

// move writes the active file, then removes the candidate. An identical active
// file counts as promoted; a different one is a conflict and is left alone.
func (p *promoter) move(c candidate, body []byte, item *Item) error {
	target := filepath.Join(p.projectDir, filepath.FromSlash(c.activeRel))
	current, err := os.ReadFile(target)
	switch {
	case err == nil && bytes.Equal(current, body):
		item.AlreadyActive = true
	case err == nil:
		return errConflict
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if p.opts.DryRun {
		return nil
	}
	if !item.AlreadyActive {
		if err := createExclusive(target, body); err != nil {
			return err
		}
	}
	return os.Remove(c.path)
}

func createExclusive(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return errConflict
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

// citedRefs lists the acceptance ids a scenario cites: refs, then step acs.
func citedRefs(s scenario.Scenario) []string {
	var refs []string
	for _, ref := range s.AcceptanceRef {
		if ref = strings.TrimSpace(ref); ref != "" {
			refs = append(refs, ref)
		}
	}
	for _, screen := range s.Screens {
		for _, step := range screen.Steps {
			if ac := strings.TrimSpace(step.Ac); ac != "" {
				refs = append(refs, ac)
			}
		}
	}
	return refs
}

// unconfirmed counts steps still awaiting a person. A step that gained an ac is
// tied to a criterion, which is the confirmation REQ-16 asks for.
func unconfirmed(s scenario.Scenario) int {
	n := 0
	for _, screen := range s.Screens {
		for _, step := range screen.Steps {
			if blocking(step.Confirm, step.Ac) {
				n++
			}
		}
	}
	return n
}

func blocking(confirm, ac string) bool {
	return strings.TrimSpace(confirm) == scenario.ConfirmRequired && strings.TrimSpace(ac) == ""
}

// acceptAssertions drops `confirm: required` from every blocking step. It edits
// the YAML node tree rather than re-encoding the struct, so key order, styles,
// and comments survive, and re-validates the result before it can be written.
func acceptAssertions(name string, body []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	doc := &root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		doc = doc.Content[0]
	}
	for _, screen := range sequence(mapValue(doc, "screens")) {
		for _, step := range sequence(mapValue(screen, "steps")) {
			confirm, ac := mapValue(step, "confirm"), mapValue(step, "ac")
			if confirm == nil || !blocking(confirm.Value, scalar(ac)) {
				continue
			}
			for i := 0; i+1 < len(step.Content); i += 2 {
				if step.Content[i].Value == "confirm" {
					step.Content = append(step.Content[:i], step.Content[i+2:]...)
					break
				}
			}
		}
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	rewritten, err := scenario.ParseBytes(name, buf.Bytes())
	if err != nil {
		return nil, err
	}
	if unconfirmed(rewritten) > 0 {
		return nil, fmt.Errorf("%s: could not clear every unconfirmed assertion", name)
	}
	return buf.Bytes(), nil
}

func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func sequence(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

func scalar(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return node.Value
}
