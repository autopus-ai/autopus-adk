package cli

import (
	"slices"
	"strings"
)

// The args model of the review tab's CLI edits (SPEC-REVMODEL-001 CLI Model
// Contract, editCLIArgs): an edit changes only the values of the flags its
// row names, appends a missing flag as a pair after the last element, and
// keeps every other element, its order, and its duplicates. An edit is a set
// of element operations, so the same edit turns the raw list, the expanded
// list, and the raw text into their edited forms.

// reviewArgsRow names the flags one CLI Model Contract row edits.
type reviewArgsRow struct {
	kind   string   // reviewRowClaude, reviewRowCodex, reviewRowAgy, or reviewRowOther
	model  []string // model flag spellings; the first is appended when absent
	effort []string // claude's effort flag spellings
}

const (
	reviewRowClaude = "claude"
	reviewRowCodex  = "codex"
	reviewRowAgy    = "agy"
	reviewRowOther  = ""
)

var (
	reviewClaudeRow = reviewArgsRow{kind: reviewRowClaude, model: []string{"--model"}, effort: []string{"--effort"}}
	reviewCodexRow  = reviewArgsRow{kind: reviewRowCodex, model: []string{"-m", "--model"}}
	reviewAgyRow    = reviewArgsRow{kind: reviewRowAgy, model: []string{"--model"}}
)

// reviewCodexEffortKey is the codex config key the codex row's effort sets.
const reviewCodexEffortKey = "model_reasoning_effort"

// reviewCLIValue is a CLI provider's model and effort. An agy id carries its
// effort in its name, so its Effort is empty.
type reviewCLIValue struct {
	Model, Effort string
}

// reviewArgValue is the last occurrence of a flag value in an args list: the
// element that holds it and, for `--flag=value`, the prefix that stays.
type reviewArgValue struct {
	found    bool
	index    int // the element holding the value
	flag     int // the flag element of a pair, or -1 for an inline value
	value    string
	prefix   string
	spelling string
}

// findReviewArg finds the last `<flag> <value>` pair or `--flag=<value>`
// element of any spelling before a `--` separator.
func findReviewArg(args, spellings []string) reviewArgValue {
	found := reviewArgValue{flag: -1}
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		for _, spelling := range spellings {
			switch {
			case args[i] == spelling && i+1 < len(args):
				found = reviewArgValue{found: true, index: i + 1, flag: i, value: args[i+1], spelling: spelling}
			case strings.HasPrefix(spelling, "--") && strings.HasPrefix(args[i], spelling+"="):
				found = reviewArgValue{found: true, index: i, flag: -1, value: strings.TrimPrefix(args[i], spelling+"="),
					prefix: spelling + "=", spelling: spelling}
			default:
				continue
			}
			if found.flag >= 0 {
				i++
			}
			break
		}
	}
	return found
}

// findReviewCodexEffort finds the last model_reasoning_effort assignment: the
// value of a `-c`/`--config` pair, a `--config=` element, or a bare one, as
// codexProfileFromArgs reads them. Its value keeps the TOML quotes.
func findReviewCodexEffort(args []string) reviewArgValue {
	found := reviewArgValue{flag: -1}
	assignment := reviewCodexEffortKey + "="
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		switch {
		case (args[i] == "-c" || args[i] == "--config") && i+1 < len(args) && strings.HasPrefix(strings.TrimSpace(args[i+1]), assignment):
			found = reviewArgValue{found: true, index: i + 1, flag: i, spelling: reviewCodexEffortKey}
			found.prefix, found.value, _ = strings.Cut(args[i+1], "=")
			found.prefix += "="
			i++
		case strings.HasPrefix(args[i], "--config="+assignment):
			found = reviewArgValue{found: true, index: i, flag: -1, prefix: "--config=" + assignment,
				value: strings.TrimPrefix(args[i], "--config="+assignment), spelling: reviewCodexEffortKey}
		case strings.HasPrefix(strings.TrimSpace(args[i]), assignment):
			found = reviewArgValue{found: true, index: i, flag: -1, spelling: reviewCodexEffortKey}
			found.prefix, found.value, _ = strings.Cut(args[i], "=")
			found.prefix += "="
		}
	}
	return found
}

// reviewCodexEffortText is an assignment value without its TOML quotes.
func reviewCodexEffortText(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

// reviewCLIValueOf reads row's model and effort from args.
func reviewCLIValueOf(row reviewArgsRow, args []string) reviewCLIValue {
	value := reviewCLIValue{Model: findReviewArg(args, row.model).value}
	switch row.kind {
	case reviewRowClaude:
		value.Effort = findReviewArg(args, row.effort).value
	case reviewRowCodex:
		value.Effort = reviewCodexEffortText(findReviewCodexEffort(args).value)
	}
	return value
}

// reviewArgsEdit is an args edit as element operations: values set at
// element indexes, elements removed, and elements appended after the last.
type reviewArgsEdit struct {
	set    map[int]string
	remove []int
	add    []string
}

func (e reviewArgsEdit) empty() bool { return len(e.set) == 0 && len(e.remove) == 0 && len(e.add) == 0 }

func (e reviewArgsEdit) apply(args []string) []string {
	out := make([]string, 0, len(args)+len(e.add))
	for i, arg := range args {
		if slices.Contains(e.remove, i) {
			continue
		}
		if value, ok := e.set[i]; ok {
			arg = value
		}
		out = append(out, arg)
	}
	return append(out, e.add...)
}

// written marks the elements of apply(args) the edit writes: the values it
// sets and the elements it appends. Every other element is one args had.
func (e reviewArgsEdit) written(args []string) []bool {
	out := make([]bool, 0, len(args)+len(e.add))
	for i := range args {
		if slices.Contains(e.remove, i) {
			continue
		}
		_, set := e.set[i]
		out = append(out, set)
	}
	for range e.add {
		out = append(out, true)
	}
	return out
}

// planReviewArgsEdit plans setting args to want under row: a named flag whose
// value differs gets the new value in place, a missing one is appended as a
// pair, and claude's empty effort removes its effort flag. Flags that
// already hold the wanted value produce no operation. A wanted value starting
// with `-` would read as a flag of its own, so it refuses the edit (false).
func planReviewArgsEdit(args []string, row reviewArgsRow, want reviewCLIValue) (reviewArgsEdit, bool) {
	if strings.HasPrefix(want.Model, "-") || strings.HasPrefix(want.Effort, "-") {
		return reviewArgsEdit{}, false
	}
	edit := reviewArgsEdit{set: map[int]string{}}
	setValue := func(found reviewArgValue, flag, value string) {
		switch {
		case !found.found:
			edit.add = append(edit.add, flag, value)
		case found.value != value:
			edit.set[found.index] = found.prefix + value
		}
	}
	if want.Model != "" {
		setValue(findReviewArg(args, row.model), row.model[0], want.Model)
	}
	switch row.kind {
	case reviewRowClaude:
		effort := findReviewArg(args, row.effort)
		if want.Effort != "" {
			setValue(effort, row.effort[0], want.Effort)
		} else if effort.found {
			edit.remove = append(edit.remove, effort.index)
			if effort.flag >= 0 {
				edit.remove = append(edit.remove, effort.flag)
			}
		}
	case reviewRowCodex:
		effort := findReviewCodexEffort(args)
		quoted := `"` + want.Effort + `"`
		switch {
		case want.Effort == "":
		case !effort.found:
			edit.add = append(edit.add, "-c", reviewCodexEffortKey+"="+quoted)
		case reviewCodexEffortText(effort.value) != want.Effort:
			edit.set[effort.index] = effort.prefix + quoted
		}
	}
	return edit, true
}
