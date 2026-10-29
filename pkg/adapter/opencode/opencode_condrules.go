package opencode

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// openCodeConditionalRule is a hook-fired rule the V2 plugin delivers itself:
// a shell call whose command matches one of Conditions adds Body to the
// system prompt of that session's later generations.
type openCodeConditionalRule struct {
	Name       string   `json:"name"`
	Conditions []string `json:"conditions"`
	Body       string   `json:"body"`
}

// jsUnportableRegexp names RE2 constructs that JavaScript reads differently
// or rejects: inline flags and groups, \A and \z anchors, \Q quoting, POSIX
// classes, and Unicode classes without the u flag. A condition using one stays
// always-loaded rather than firing on a different set of commands.
var jsUnportableRegexp = regexp.MustCompile(`\(\?|\\[AzQpP]|\[:`)

// openCodeConditionalRules returns the embedded rules the V2 plugin delivers on
// a shell match, sorted by name.
func openCodeConditionalRules() ([]openCodeConditionalRule, error) {
	entries, err := contentfs.FS.ReadDir("rules")
	if err != nil {
		return nil, err
	}
	var out []openCodeConditionalRule
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := contentfs.FS.ReadFile("rules/" + entry.Name())
		if err != nil {
			return nil, err
		}
		rule, err := rulecond.ParseRule(entry.Name(), raw)
		if err != nil {
			return nil, err
		}
		if !pluginDeliversRule(rule) {
			continue
		}
		body := pkgcontent.ReplacePlatformReferences(rule.Body, "opencode")
		if strings.TrimSpace(body) == "" {
			return nil, fmt.Errorf("conditional rule %s has an empty body", rule.Name)
		}
		out = append(out, openCodeConditionalRule{Name: rule.Name, Conditions: rule.Conditions, Body: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// pluginDeliversRule reports whether the V2 plugin can take over a rule: it
// must be hook-fired, scoped only to the shell tool (`tool:bash`, which
// OpenCode V2 names `shell`), and use conditions that mean the same thing to
// RE2 and to JavaScript.
func pluginDeliversRule(rule *rulecond.Rule) bool {
	if rulecond.Classify(rule) != rulecond.ClassHookFired {
		return false
	}
	for _, trigger := range rulecond.Triggers(rule) {
		if trigger.Matcher != rulecond.MatcherBash || trigger.Glob != "" {
			return false
		}
	}
	for _, condition := range rule.Conditions {
		if jsUnportableRegexp.MatchString(condition) {
			return false
		}
		if _, err := regexp.Compile(condition); err != nil {
			return false
		}
	}
	return true
}
