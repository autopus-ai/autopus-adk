package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/debt"
)

// preBuildGates run before the build, cheapest first: credentials, version
// pins, then the debt ratchet. The first that blocks wins; the warnings of
// the others ride along.
func preBuildGates(ctx context.Context, in stopHookInput, root string, changed []string) (*stopHookOutput, string) {
	block, warning := secretGate(ctx, in.SessionID, root)
	if block != nil {
		return block, ""
	}
	versionBlock, versionWarn := versionGate(ctx, in, root)
	if versionBlock != nil {
		out := withSecretWarning(*versionBlock, warning)
		return &out, ""
	}
	warning = joinWarnings(warning, versionWarn)
	debtBlock, debtWarn := debtGate(ctx, in, root, changed)
	warning = joinWarnings(warning, debtWarn)
	if debtBlock != nil {
		out := withSecretWarning(*debtBlock, warning)
		return &out, ""
	}
	return nil, warning
}

// debtStopBudget bounds the debt scan in a stop; a first scan of a large
// repository builds its index over several stops and does not block until
// it has caught up.
var debtStopBudget = 800 * time.Millisecond

// debtStopEnabled reports whether the stop gate checks the change for new
// debt items: on unless AUTOPUS_DEBT_STOP=off or hooks.debt_stop: false.
func debtStopEnabled(root string) bool {
	if strings.EqualFold(os.Getenv("AUTOPUS_DEBT_STOP"), "off") {
		return false
	}
	cfg := nearestConfig(root)
	return cfg == nil || cfg.Hooks.IsDebtStopEnabled()
}

// debtMaxLines is the project's size limit for the near-limit measure.
func debtMaxLines(root string) int {
	if cfg := nearestConfig(root); cfg != nil && cfg.Architecture.MaxFileLines > 0 {
		return cfg.Architecture.MaxFileLines
	}
	return debt.DefaultMaxLines
}

// debtMaxBlocks caps the debt blocks of one session, apart from the build
// failures' budget: debt must not use up the blocks a failing test needs.
const debtMaxBlocks = 2

// debtGate is the ratchet: a change may not add a debt item the committed
// baseline lacks. Only items in the files this change touched block (a
// duplicate when any copy is touched); debt elsewhere means the baseline is
// stale and only warns. A block names the new items; it repeats only for an
// item not blocked before in this session, at most debtMaxBlocks times. A
// file near the size limit only warns. Without a baseline it suggests one,
// once a session, and never blocks. While a background agent may still be
// editing, it waits, as the build does.
func debtGate(ctx context.Context, in stopHookInput, root string, changed []string) (block *stopHookOutput, warning string) {
	session := in.SessionID
	if !debtStopEnabled(root) || writersPresent(in) {
		return nil, ""
	}
	base, err := debt.LoadBaseline(root)
	noBase := errors.Is(err, debt.ErrNoBaseline)
	if err != nil && !noBase {
		return nil, "auto verify stop: debt baseline unreadable: " + err.Error()
	}
	if noBase && readStopState(session, "debt-nobase") == "1" {
		return nil, ""
	}
	touched := map[string]bool{}
	for _, f := range changed {
		touched[f] = true
	}
	items, err := debt.ScanWith(ctx, root, debt.Options{MaxLines: debtMaxLines(root),
		SizeOf: func(f string) bool { return touched[f] }, Deadline: time.Now().Add(debtStopBudget)})
	if err != nil {
		return nil, "" // still indexing, or git could not list the tree: no verdict this stop
	}
	if noBase {
		// A project that never recorded its debt hears about the ratchet only
		// when a change of its own adds an item, once a session.
		for _, it := range items {
			if _, mine := inChange(it, touched); mine && it.Kind != debt.KindNearLimit {
				writeStopState(session, "debt-nobase", "1")
				return nil, "auto verify stop: this change touches technical debt (" + it.String() + "), and no debt baseline is " +
					"committed here, so new debt is not checked; run `auto debt baseline` and commit " + debt.BaselinePath
			}
		}
		return nil, ""
	}
	blocking, warn := debt.Blocking(debt.Unknown(items, base, touched))
	var mine []debt.Item
	elsewhere := 0
	for _, it := range blocking {
		if at, ok := inChange(it, touched); ok {
			mine = append(mine, at)
		} else {
			elsewhere++
		}
	}
	if len(warn) > 0 {
		warning = "auto verify stop: near the file-size limit: " + describeDebt(warn) + "; split before the next change pushes it over"
	}
	if elsewhere > 0 {
		warning = joinWarnings(warning, fmt.Sprintf("auto verify stop: %d technical-debt item(s) outside this change are not in the baseline; "+
			"it is stale: run `auto debt baseline` (and commit it) when they are meant to stay", elsewhere))
	}
	if len(mine) == 0 {
		return nil, warning
	}
	seen := map[string]bool{}
	for _, id := range strings.Split(readStopState(session, "debt"), "\n") {
		seen[id] = id != ""
	}
	fresh := false
	for _, it := range mine {
		fresh = fresh || !seen[debtID(it)]
	}
	where := describeDebt(mine)
	n, _ := strconv.Atoi(readStopState(session, "debt-blocks"))
	if !fresh || n >= debtMaxBlocks {
		return nil, joinWarnings(warning, "auto verify stop: new technical debt is still in the change: "+where)
	}
	for _, it := range mine {
		seen[debtID(it)] = true
	}
	ids := make([]string, 0, len(seen))
	for id, ok := range seen {
		if ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	writeStopState(session, "debt", strings.Join(ids, "\n"))
	writeStopState(session, "debt-blocks", strconv.Itoa(n+1))
	return &stopHookOutput{Decision: "block", Reason: "This change adds technical debt the baseline (" + debt.BaselinePath + ") does not have: " + where + ".\n\n" +
		"Remove each one before finishing: call one shared function instead of repeating a block, and do what a TODO/FIXME/HACK/XXX defers " +
		"(or reference its issue, e.g. TODO(#123), when it must wait). If one is deliberate, say why in your reply and record it with " +
		"`auto debt baseline --force`; the baseline is committed, so a person sees it in review."}, warning
}

// inChange reports whether an item is in a file the change touched, and
// names it there: a duplicate is reported at its touched copy.
func inChange(it debt.Item, touched map[string]bool) (debt.Item, bool) {
	if touched[it.File] {
		return it, true
	}
	for i, a := range it.Also {
		f, line, _ := strings.Cut(a, ":")
		if touched[f] {
			moved := it
			moved.Also = append(append([]string{fmt.Sprintf("%s:%d", it.File, it.Line)}, it.Also[:i]...), it.Also[i+1:]...)
			moved.File = f
			moved.Line, _ = strconv.Atoi(line)
			return moved, true
		}
	}
	return it, false
}

// debtID names an item for the session's record of what it blocked.
func debtID(it debt.Item) string { return fmt.Sprintf("%s|%s|%s", it.Kind, it.File, it.Key) }

func describeDebt(items []debt.Item) string {
	parts := make([]string, 0, min(len(items), maxSecretHitsShown)+1)
	for i, it := range items {
		if i == maxSecretHitsShown {
			parts = append(parts, "and more")
			break
		}
		parts = append(parts, it.String())
	}
	return strings.Join(parts, "; ")
}

// writersPresent reports a background agent that may still be editing,
// without counting a deferral: backgroundWriters records one per call for
// Codex sub-agents, and the build stage must keep its own.
func writersPresent(in stopHookInput) bool {
	for _, task := range in.BackgroundTasks {
		if slices.Contains(treeWriters, task.Type) {
			return true
		}
	}
	if in.SessionID == "" {
		return false
	}
	entries, err := os.ReadDir(stopStateDir)
	if err != nil {
		return false
	}
	prefix := stopStateName(in.SessionID, "agent-")
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		// A marker that already used its deferrals is a lost sub-agent the
		// build stage no longer waits for; neither does this gate.
		data, _ := os.ReadFile(filepath.Join(stopStateDir, e.Name()))
		if strings.Count(string(data), "\n") < subagentMaxDefers {
			return true
		}
	}
	return false
}
