// Package rulecond_test holds the SPEC-CONDRULE-001 oracle tests.
//
// These tests are the API contract for pkg/rulecond. The exported surface they
// exercise is:
//
//	consts   ManifestRelPath, BodyRootRelPath, MaxBodyBytes, MaxContextBytes,
//	         RuleHeaderPrefix, TruncationPrefix
//	types    Rule, Class, Trigger, Manifest, ManifestRule, Reason, ViolationError
//	funcs    ParseRule, Validate, Classify, ParseScope, ConditionSubject,
//	         ReadBody, CompileClaude, Fire
//
// Injection format contract: every injected rule body is preceded by a line
// beginning with RuleHeaderPrefix followed by the rule name. Dropped rules are
// named on one line beginning with TruncationPrefix. Fail-closed suppressions
// are reported on stderr as one "<rule> <reason_code>" line per offending rule.
package rulecond_test

import (
	"bytes"
	"errors"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// Condition regexes fixed by SPEC-CONDRULE-001 acceptance.md T7 assignment.
const (
	condLoreCommit      = `\bgit\s+commit\b`
	condShellPortable   = `(^|[;&|]\s)\s*g?timeout\s+[0-9]`
	condWorktreeSafety  = `\bgit\s+(gc|prune|repack)\b`
	condGoEditFixture   = `\.go$`
	editMatcher         = "Edit|Write|MultiEdit"
	bashMatcher         = "Bash"
	preToolUse          = "PreToolUse"
	loreCommitSignature = "🐙 Autopus <noreply@autopus.co>"
)

// fixture builds a throwaway project root holding the conditional manifest and
// the conditional body root, then runs the dispatcher against it.
type fixture struct {
	t    *testing.T
	root string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, rulecond.BodyRootRelPath), 0o755))
	return &fixture{t: t, root: root}
}

func (f *fixture) bodyRoot() string {
	return filepath.Join(f.root, rulecond.BodyRootRelPath)
}

func (f *fixture) writeBody(name, content string) {
	f.t.Helper()
	path := filepath.Join(f.bodyRoot(), name)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o644))
}

func (f *fixture) writeManifest(rules ...rulecond.ManifestRule) {
	f.t.Helper()
	blob, err := json.Marshal(rulecond.Manifest{Version: "1", Rules: rules})
	require.NoError(f.t, err)
	f.writeRawManifest(string(blob))
}

func (f *fixture) writeRawManifest(raw string) {
	f.t.Helper()
	path := filepath.Join(f.root, rulecond.ManifestRelPath)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(raw), 0o644))
}

func (f *fixture) removeManifest() {
	f.t.Helper()
	require.NoError(f.t, os.RemoveAll(filepath.Join(f.root, rulecond.ManifestRelPath)))
}

// fire runs one dispatcher invocation and returns stdout and stderr. The
// dispatcher is advisory, so it must never return an error.
func (f *fixture) fire(stdin string) (string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	err := rulecond.Fire(f.root, preToolUse, strings.NewReader(stdin), &out, &errOut)
	require.NoError(f.t, err, "dispatcher must stay advisory and never return an error")
	return out.String(), errOut.String()
}

// injectedContext parses dispatcher stdout and returns additionalContext.
// Empty stdout yields an empty string.
func injectedContext(t *testing.T, stdout string) string {
	t.Helper()
	if strings.TrimSpace(stdout) == "" {
		return ""
	}
	var payload struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload), "stdout must be a JSON hook payload")
	require.Equal(t, preToolUse, payload.HookSpecificOutput.HookEventName)
	return payload.HookSpecificOutput.AdditionalContext
}

var ruleHeaderRe = regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(rulecond.RuleHeaderPrefix) + `(.+)$`)

// firedRuleNames extracts the sorted rule-name set from dispatcher stdout.
func firedRuleNames(t *testing.T, stdout string) []string {
	t.Helper()
	names := []string{}
	for _, m := range ruleHeaderRe.FindAllStringSubmatch(injectedContext(t, stdout), -1) {
		names = append(names, strings.TrimSpace(m[1]))
	}
	sort.Strings(names)
	return names
}

func bashPayload(command string) string {
	return toolPayload("Bash", map[string]any{"command": command})
}

func filePayload(tool, path string) string {
	return toolPayload(tool, map[string]any{"file_path": path})
}

func toolPayload(tool string, input map[string]any) string {
	blob, err := json.Marshal(map[string]any{
		"hook_event_name": preToolUse,
		"tool_name":       tool,
		"tool_input":      input,
	})
	if err != nil {
		panic(err)
	}
	return string(blob)
}

func bashRule(name, condition, body string) rulecond.ManifestRule {
	return rulecond.ManifestRule{
		Name:       name,
		Event:      preToolUse,
		Matcher:    bashMatcher,
		Conditions: []string{condition},
		Body:       body,
	}
}

func editRule(name, condition, body string) rulecond.ManifestRule {
	return rulecond.ManifestRule{
		Name:       name,
		Event:      preToolUse,
		Matcher:    editMatcher,
		Conditions: []string{condition},
		Body:       body,
	}
}

// sourceRuleBody returns the rule body with its frontmatter removed, from the
// shipped rule or, for a retired one, its frozen copy (see ruleSourceText).
func sourceRuleBody(t *testing.T, name string) string {
	t.Helper()
	rule, err := rulecond.ParseRule(name+".md", []byte(ruleSourceText(t, name)))
	require.NoError(t, err)
	require.NotEmpty(t, rule.Body)
	return rule.Body
}

// ruleSourceText returns a rule file verbatim. A shipped rule comes from
// content/rules; a rule the verification harness retired comes from its
// frozen last-shipped copy under testdata/retired-rules, so the engine stays
// exercised with real multi-condition bodies after the rule left the product.
func ruleSourceText(t *testing.T, name string) string {
	t.Helper()
	raw, err := fs.ReadFile(contentfs.FS, "rules/"+name+".md")
	if errors.Is(err, fs.ErrNotExist) {
		raw, err = os.ReadFile(filepath.Join("testdata", "retired-rules", name+".md"))
	}
	require.NoError(t, err)
	return string(raw)
}
