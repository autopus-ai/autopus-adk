package record

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// maxJoinedLines bounds how far a wrapped statement is followed, so one
// unbalanced line cannot swallow the rest of the file.
const maxJoinedLines = 20

// codegenWrappers are the structural lines of both codegen layouts: the test
// callback, the library IIFE, and closing braces. They hold no step.
var codegenWrappers = []*regexp.Regexp{
	regexp.MustCompile(`^test\(\s*(?:'(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*"|` + "`[^`]*`" +
		`)\s*,\s*async\s*\(\s*\{[\w\s,]*\}\s*\)\s*=>\s*\{$`),
	regexp.MustCompile(`^\(\s*async\s*\(\s*\)\s*=>\s*\{$`),
	regexp.MustCompile(`^\}\s*\)\s*\(\s*\)\s*;?$`),
	regexp.MustCompile(`^[{}]\s*\)?\s*;?$`),
}

// codegenBoilerplate is the setup codegen emits around the steps: imports,
// require, browser and context creation, test.use, and teardown.
var codegenBoilerplate = []*regexp.Regexp{
	regexp.MustCompile(`^import\s`),
	regexp.MustCompile(`^(?:const|let|var)\s*\{[\w\s,:]*\}\s*=\s*require\(\s*['"][^'"]+['"]\s*\)\s*;?$`),
	regexp.MustCompile(`^(?:const|let|var)\s+\w+\s*=\s*await\s+[\w.]+\.(?:launch|launchPersistentContext|newContext|newPage)\(.*\)\s*;?$`),
	regexp.MustCompile(`^test\.use\(.*\)\s*;?$`),
	regexp.MustCompile(`^await\s+(?:page|context|browser)\.close\(\s*\)\s*;?$`),
}

// statementStart marks lines a formatter may have wrapped over several lines.
var statementStart = regexp.MustCompile(`^(?:await\s|(?:const|let|var)\s|test\.use\()`)

// ParseCodegen reads Playwright codegen JavaScript or TypeScript in either the
// test or the library layout. Imports, wrappers, browser setup, and comments
// are skipped; every other line converts to an event or is reported with its
// 1-based line number. A person drives codegen, so every event is by human.
func ParseCodegen(src []byte) (Recording, []Unsupported) {
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	var rec Recording
	var bad []Unsupported
	inComment := false
	for i := 0; i < len(lines); i++ {
		text := strings.TrimSpace(lines[i])
		if inComment {
			inComment = !strings.Contains(text, "*/")
			continue
		}
		switch {
		case text == "" || strings.HasPrefix(text, "//") || matchesAny(codegenWrappers, text):
			continue
		case strings.HasPrefix(text, "/*"):
			inComment = !strings.Contains(text, "*/")
			continue
		}
		start, stmt := i, text
		if statementStart.MatchString(text) {
			stmt, i = joinStatement(lines, i)
		}
		if matchesAny(codegenBoilerplate, stmt) {
			continue
		}
		event, err := codegenEvent(stmt)
		if err != nil {
			bad = append(bad, Unsupported{Line: start + 1, Text: stmt, Reason: err.Error()})
			continue
		}
		event.Line, event.By = start+1, scenario.ByHuman
		rec.Events = append(rec.Events, event)
	}
	return rec, bad
}

func matchesAny(patterns []*regexp.Regexp, text string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// joinStatement folds a statement a formatter wrapped over several lines back
// into one and returns it with the index of its last line.
func joinStatement(lines []string, i int) (string, int) {
	stmt := strings.TrimSpace(lines[i])
	depth, end := bracketDepth(stmt), i
	for depth > 0 && end+1 < len(lines) && end-i < maxJoinedLines {
		end++
		next := strings.TrimSpace(lines[end])
		stmt += " " + next
		depth += bracketDepth(next)
	}
	return stmt, end
}

// bracketDepth counts unclosed brackets outside string literals and comments.
func bracketDepth(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '/' && strings.HasPrefix(s[i:], "//"):
			return depth
		case strings.IndexByte("({[", c) >= 0:
			depth++
		case strings.IndexByte(")}]", c) >= 0:
			depth--
		}
	}
	return depth
}

func codegenEvent(stmt string) (Event, error) {
	chain, err := parseAwait(stmt)
	if err != nil {
		return Event{}, err
	}
	head := chain[0]
	switch {
	case head.name == "page" && !head.call:
		return pageEvent(chain[1:])
	case head.name == "expect" && head.call && len(chain) == 2:
		return expectEvent(head.args, chain[1])
	}
	return Event{}, errors.New("only page actions and expect assertions are recorded")
}

func pageEvent(rest []segment) (Event, error) {
	switch {
	case len(rest) == 1 && rest[0].name == "goto" && rest[0].call:
		url, err := onlyString(rest[0])
		return Event{Action: ActionGoto, URL: url}, err
	case len(rest) == 2 && rest[0].name == "keyboard" && !rest[0].call && rest[1].name == "press" && rest[1].call:
		key, err := onlyString(rest[1])
		return Event{Action: ActionPress, Key: key}, err
	}
	target, used, err := locator(rest)
	if err != nil {
		return Event{}, err
	}
	if used != len(rest)-1 || !rest[used].call {
		return Event{}, errors.New("expected exactly one action after the locator")
	}
	act := rest[used]
	switch act.name {
	case "click", "check":
		if len(act.args) != 0 {
			return Event{}, fmt.Errorf("%s options are not supported", act.name)
		}
		if act.name == "check" {
			return Event{Action: ActionCheck, Target: target}, nil
		}
		return Event{Action: ActionClick, Target: target}, nil
	case "fill":
		value, err := onlyString(act)
		return Event{Action: ActionFill, Target: target, Value: value}, err
	case "press":
		key, err := onlyString(act)
		return Event{Action: ActionPress, Target: target, Key: key}, err
	case "selectOption":
		option, err := onlyString(act)
		return Event{Action: ActionSelect, Target: target, Value: option}, err
	}
	return Event{}, fmt.Errorf("action %s has no scenario step", act.name)
}

// expectEvent maps an assertion. Text matchers become page-level expect_text,
// which is what the scenario dialect can state; a visibility check maps only
// for getByText and getByRole, the two locators an expect step can name.
func expectEvent(args []jsValue, matcher segment) (Event, error) {
	if len(args) != 1 || args[0].kind != valChain || !matcher.call {
		return Event{}, errors.New("unsupported expect form")
	}
	subject := args[0].chain
	if subject[0].name != "page" || subject[0].call {
		return Event{}, errors.New("expect must wrap page or a page locator")
	}
	if len(subject) == 1 {
		switch matcher.name {
		case "toHaveURL":
			url, err := onlyString(matcher)
			return Event{Expect: ExpectURL, URL: url}, err
		case "toHaveTitle":
			title, err := onlyString(matcher)
			return Event{Expect: ExpectTitle, Value: title}, err
		}
		return Event{}, fmt.Errorf("page assertion %s has no scenario step", matcher.name)
	}
	target, used, err := locator(subject[1:])
	if err != nil {
		return Event{}, err
	}
	if used != len(subject)-1 {
		return Event{}, errors.New("unsupported locator chain")
	}
	switch matcher.name {
	case "toBeVisible":
		switch {
		case len(matcher.args) != 0:
			return Event{}, errors.New("toBeVisible options are not supported")
		case target.Text != "":
			return Event{Expect: ExpectText, Value: target.Text}, nil
		case target.Role != "":
			return Event{Expect: ExpectRole, Target: target}, nil
		}
		return Event{}, errors.New("only getByText and getByRole visibility has a scenario step")
	case "toHaveText", "toContainText":
		text, err := onlyString(matcher)
		return Event{Expect: ExpectText, Value: text}, err
	}
	return Event{}, fmt.Errorf("assertion %s has no scenario step", matcher.name)
}

// locator reads one getBy* call plus an optional .first() or .nth(0). Both
// pick the first match, which every compiled locator does already.
func locator(segs []segment) (scenario.Target, int, error) {
	if len(segs) == 0 || !segs[0].call {
		return scenario.Target{}, 0, errors.New("expected a getBy* locator")
	}
	head := segs[0]
	var target scenario.Target
	var opts []jsField
	var err error
	switch head.name {
	case "getByRole":
		target.Role, opts, err = locatorArgs(head)
	case "getByLabel":
		target.Label, opts, err = locatorArgs(head)
	case "getByPlaceholder":
		target.Placeholder, opts, err = locatorArgs(head)
	case "getByText":
		target.Text, opts, err = locatorArgs(head)
	case "getByTestId":
		target.TestID, opts, err = locatorArgs(head)
	default:
		return scenario.Target{}, 0, fmt.Errorf("locator %s has no scenario equivalent", head.name)
	}
	if err != nil {
		return scenario.Target{}, 0, err
	}
	for _, opt := range opts {
		switch {
		case opt.key == "exact" && opt.value.kind == valBool && head.name != "getByTestId":
			target.Exact = opt.value.truth
		case opt.key == "name" && opt.value.kind == valString && head.name == "getByRole":
			target.Name = opt.value.text
		default:
			return scenario.Target{}, 0, fmt.Errorf("%s option %s is not supported", head.name, opt.key)
		}
	}
	if len(segs) > 1 && isFirstPick(segs[1]) {
		return target, 2, nil
	}
	return target, 1, nil
}

func locatorArgs(s segment) (string, []jsField, error) {
	if len(s.args) == 0 || len(s.args) > 2 || s.args[0].kind != valString || strings.TrimSpace(s.args[0].text) == "" {
		return "", nil, fmt.Errorf("%s needs one non-empty string", s.name)
	}
	if len(s.args) == 1 {
		return s.args[0].text, nil, nil
	}
	if s.args[1].kind != valObject {
		return "", nil, fmt.Errorf("%s options must be an object literal", s.name)
	}
	return s.args[0].text, s.args[1].fields, nil
}

// onlyString reads the single non-empty string argument of a call. An empty
// one (a cleared field, a blank key) has no scenario step.
func onlyString(s segment) (string, error) {
	if len(s.args) != 1 || s.args[0].kind != valString {
		return "", fmt.Errorf("%s takes exactly one string here", s.name)
	}
	if strings.TrimSpace(s.args[0].text) == "" {
		return "", fmt.Errorf("%s with an empty string has no scenario step", s.name)
	}
	return s.args[0].text, nil
}
