package rulecond

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// Sticky hook shape. One entry total, because the event carries no matcher: a
// UserPromptSubmit hook fires once per user turn regardless of tooling.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: single source of truth for the sticky hook registration — pkg/content/hooks_conditional.go copies CompiledClaude.Hooks into settings.json under its existing claude-only guard, so this is the only place the command and timeout are stated.
// @AX:REASON: The 15 second timeout bounds a hook that runs before every user turn. It is sized for a cold start, the first run after the binary is replaced, because Claude Code drops an overrunning hook for that turn without a retry; the warm path answers in well under a second. Raising it here is the only edit point, and restating the command elsewhere would drift without failing a build.
const (
	stickyHookType    = "command"
	stickyHookCommand = "auto rules sticky --event " + EventUserPromptSubmit
	stickyHookTimeout = 15
)

// stickyHook is the single UserPromptSubmit registration. It is emitted only
// when the sticky set is non-empty (REQ-STICKYRULE-COMPILE-01), so a harness
// with no sticky rule registers no per-prompt process spawn at all, and the
// claude adapter's managed-event set drops a previously installed entry.
func stickyHook() adapter.HookConfig {
	return adapter.HookConfig{
		Event:   EventUserPromptSubmit,
		Matcher: "",
		Type:    stickyHookType,
		Command: stickyHookCommand,
		Timeout: stickyHookTimeout,
	}
}

// compileSticky collects the sticky rule set and refuses, at generation time,
// any entry the runtime would later drop or refuse (REQ-STICKYRULE-COMPILE-04).
//
// Failing generation is the point: an entry the runtime silently refuses is a
// rule that stops governing responses without anyone noticing, which is exactly
// the dilution this SPEC exists to fix.
func compileSticky(rules []*Rule) ([]StickyRule, error) {
	var out []StickyRule
	var entries []injected

	for _, rule := range rules {
		if !IsSticky(rule) {
			continue
		}
		if err := checkStickyBody(rule); err != nil {
			return nil, err
		}
		out = append(out, StickyRule{Name: rule.Name, Body: rule.Name + mdSuffix})
		entries = append(entries, injected{name: rule.Name, body: rule.Body})
	}
	if len(out) == 0 {
		return nil, nil
	}

	sorted := sortStickyRules(out)
	if err := checkStickyAggregate(sorted, entries); err != nil {
		return nil, err
	}
	return sorted, nil
}

// checkStickyBody applies both halves of the runtime's per-body cap.
//
// The injectable body is what REQ-STICKYRULE-FIRE-07 caps, and checkCompiledBody
// already measures exactly that while also refusing a name that could not be a
// contained body location. The source-file size is the second half: the sticky
// body root holds whole rule files, frontmatter included, and the runtime reads
// them through ReadBody, which refuses a file above MaxBodyBytes. Checking only
// the injectable body would let frontmatter push an emitted file past the size
// the runtime enforces, which is the silent-refusal case this function exists to
// prevent.
func checkStickyBody(rule *Rule) error {
	if !ruleNamePattern.MatchString(rule.Name) {
		return fmt.Errorf(
			"sticky rule %q: name must be a bare identifier of at most 64 characters, "+
				"because it is rendered into injected model context", rule.Name)
	}
	if err := checkCompiledBody(rule.Name, []byte(rule.Body)); err != nil {
		return fmt.Errorf("sticky %w", err)
	}
	if size := stickySourceSize(rule); size > MaxBodyBytes {
		return fmt.Errorf(
			"sticky rule %s: the emitted rule file measures %d bytes, above the %d byte cap "+
				"the runtime enforces when it reads the body", rule.Name, size, MaxBodyBytes)
	}
	return nil
}

// checkStickyAggregate refuses a sticky set the runtime would truncate. The
// measurement is the assembled context, headers and separators included, which
// is the same quantity assembleWithCap bounds at dispatch time; measuring bare
// bodies here would pass a set the runtime then drops a rule from.
func checkStickyAggregate(sorted []StickyRule, entries []injected) error {
	total := joinedLen(injectionBlocks(sortInjectedByName(entries)))
	if total <= MaxStickyContextBytes {
		return nil
	}

	names := make([]string, 0, len(sorted))
	for _, rule := range sorted {
		names = append(names, rule.Name)
	}
	return fmt.Errorf(
		"sticky rules %s: injectable bodies assemble to %d bytes, above the %d byte aggregate cap",
		strings.Join(names, ", "), total, MaxStickyContextBytes)
}

// stickySourceSize is the byte size of the rule file the claude surface installs
// into the sticky body root, which is what the runtime's per-body read measures.
//
// A paths-scoped rule is re-rendered by the compiler, so its emitted size is
// known exactly. An `always` rule flows through the verbatim copy path unchanged,
// so its emitted size is its source file: the delimiters splitFrontmatter parses
// plus the frontmatter and the body.
//
// @AX:WARN [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: the `always` branch reconstructs the emitted file size from delimiter literals instead of measuring the file.
// @AX:REASON: It is only correct while splitFrontmatter keeps exactly this delimiter shape; a parser change that accepts CRLF or a trailing-space fence makes this undercount, and the compile-time gate then passes a file the runtime's MaxBodyBytes read refuses.
func stickySourceSize(rule *Rule) int {
	if Classify(rule) == ClassPathsScoped {
		return len(renderPathsScoped(rule))
	}
	if rule.Frontmatter == "" {
		return len(rule.Body)
	}
	return len("---") + len(rule.Frontmatter) + len("\n---\n") + len(rule.Body)
}
