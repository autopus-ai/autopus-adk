package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// caseRejection is one case dropped from an otherwise usable document.
type caseRejection struct {
	ID   string
	Code string
	Err  string
}

// salvageTestScenarios keeps the valid cases of a test-scenarios document
// that failed as a whole. Agents put every case in one document, so a single
// malformed field would otherwise discard all of them. Each case is checked
// alone with the document's own header, and the survivors are checked again
// together, so the result passes exactly the validation a whole file gets.
func salvageTestScenarios(name, body string, check func(testscenario.Case) error) (testscenario.Document, string, []caseRejection, bool) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(body), &root); err != nil || len(root.Content) != 1 {
		return testscenario.Document{}, "", nil, false
	}
	mapping := root.Content[0]
	casesAt := -1
	if mapping.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == "cases" && mapping.Content[i+1].Kind == yaml.SequenceNode {
				casesAt = i + 1
			}
		}
	}
	if casesAt < 0 {
		return testscenario.Document{}, "", nil, false
	}
	var kept []*yaml.Node
	var dropped []caseRejection
	for _, c := range mapping.Content[casesAt].Content {
		single, err := withCases(mapping, casesAt, []*yaml.Node{c})
		var one testscenario.Document
		if err == nil {
			one, err = testscenario.ParseBytes(name, single)
		}
		if err == nil && check != nil && len(one.Cases) == 1 {
			err = check(one.Cases[0])
		}
		if err != nil {
			dropped = append(dropped, caseRejection{ID: caseID(c), Code: validationCode(err), Err: err.Error()})
			continue
		}
		kept = append(kept, c)
	}
	if len(kept) == 0 || len(dropped) == 0 {
		return testscenario.Document{}, "", nil, false
	}
	whole, err := withCases(mapping, casesAt, kept)
	if err != nil {
		return testscenario.Document{}, "", nil, false
	}
	doc, err := testscenario.ParseBytes(name, whole)
	if err != nil {
		return testscenario.Document{}, "", nil, false
	}
	return doc, string(whole), dropped, true
}

// withCases renders the document with its cases replaced, leaving the
// original node tree untouched.
func withCases(mapping *yaml.Node, casesAt int, cases []*yaml.Node) ([]byte, error) {
	copyMap := *mapping
	copyMap.Content = append([]*yaml.Node{}, mapping.Content...)
	seq := *mapping.Content[casesAt]
	seq.Content = cases
	copyMap.Content[casesAt] = &seq
	return yaml.Marshal(&copyMap)
}

func caseID(c *yaml.Node) string {
	if c.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(c.Content); i += 2 {
			if c.Content[i].Value == "id" {
				return c.Content[i+1].Value
			}
		}
	}
	return ""
}

// firstUnrunnable returns why the first case that cannot run cannot.
func (e *evaluation) firstUnrunnable(doc testscenario.Document) error {
	for _, c := range doc.Cases {
		if err := e.caseRunnable(c); err != nil {
			return err
		}
	}
	return nil
}

// caseRunnable rejects an exec case that can only fail before reaching the
// product: a build step naming a project path that does not exist (an agent
// copying an example such as ./cmd/tool), or, with no build step, a program
// that is not in the project. Every run of such a case would stop as
// "cannot run", and the loop could only stop on it.
func (e *evaluation) caseRunnable(c testscenario.Case) error {
	x := c.Automation.Exec
	if e.projectDir == "" || c.Automation.Type != testscenario.AutomationExec || x == nil || len(x.Argv) == 0 {
		return nil
	}
	if len(x.Build) > 0 {
		for _, arg := range x.Build[1:] {
			if rel, ok := projectPathArg(arg); ok && !e.exists(rel) {
				return &testscenario.ValidationError{Path: c.ID, Code: CodeExecProgramMissing, Message: fmt.Sprintf(
					"exec.build names %q, which is not in the project; build the package that holds the program, such as .", arg)}
			}
		}
		return nil
	}
	program := x.Argv[0]
	rel, ok := strings.CutPrefix(program, testscenario.PlaceholderProject+"/")
	if !ok {
		if !strings.Contains(program, "/") {
			return nil
		}
		rel = strings.TrimPrefix(program, "./")
	}
	if !e.exists(rel) {
		return &testscenario.ValidationError{Path: c.ID, Code: CodeExecProgramMissing, Message: fmt.Sprintf(
			"exec.argv[0] %q is not in the project and no build step makes it; add build, such as [go, build, -o, $BIN/<name>, .]", program)}
	}
	return nil
}

// projectPathArg reads a build argument that names a project path: ./x or
// $PROJECT/x. Output paths under $BIN and bare words are not paths to check.
func projectPathArg(arg string) (string, bool) {
	if rel, ok := strings.CutPrefix(arg, testscenario.PlaceholderProject+"/"); ok {
		return rel, true
	}
	if rel, ok := strings.CutPrefix(arg, "./"); ok && rel != "" && !strings.Contains(rel, "...") {
		return rel, true
	}
	return "", false
}

func (e *evaluation) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(e.projectDir, filepath.FromSlash(rel)))
	return err == nil
}
