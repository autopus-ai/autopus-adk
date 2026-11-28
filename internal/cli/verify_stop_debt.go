package cli

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/debt"
	"github.com/insajin/autopus-adk/pkg/verifygate"
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
	debtBlock, debtWarn := debtGate(ctx, in.SessionID, root, changed)
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

// debtGate is the ratchet: a change may not add a debt item the committed
// baseline lacks. Like the credential gate, a new set of items blocks once
// and the same set again only warns; a file near the size limit only warns.
// Without a baseline it suggests one, once a session, and never blocks.
func debtGate(ctx context.Context, session, root string, changed []string) (block *stopHookOutput, warning string) {
	if !debtStopEnabled(root) {
		return nil, ""
	}
	base, err := debt.LoadBaseline(root)
	if errors.Is(err, debt.ErrNoBaseline) {
		if readStopState(session, "debt-nobase") == "1" {
			return nil, ""
		}
		writeStopState(session, "debt-nobase", "1")
		return nil, "auto verify stop: no technical-debt baseline here, so new debt is not checked; run `auto debt baseline` and commit " + debt.BaselinePath
	}
	if err != nil {
		return nil, "auto verify stop: debt baseline unreadable: " + err.Error()
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
	blocking, warn := debt.Blocking(debt.New(items, base))
	if len(warn) > 0 {
		warning = "auto verify stop: near the file-size limit: " + describeDebt(warn) + "; split before the next change pushes it over"
	}
	if len(blocking) == 0 {
		return nil, warning
	}
	keys := make([]string, len(blocking))
	for i, it := range blocking {
		keys[i] = it.Key
	}
	sort.Strings(keys)
	fp := verifygate.Fingerprint(root, keys)
	where := describeDebt(blocking)
	if readStopState(session, "debt") == fp || countStopBlock(session) > verifyStopMaxBlocks {
		return nil, joinWarnings(warning, "auto verify stop: new technical debt is still in the change: "+where)
	}
	writeStopState(session, "debt", fp)
	return &stopHookOutput{Decision: "block", Reason: "This change adds technical debt the baseline (" + debt.BaselinePath + ") does not have: " + where + ".\n\n" +
		"Remove each one before finishing: call one shared function instead of repeating a block, and do what a TODO/FIXME/HACK/XXX defers " +
		"(or reference its issue, e.g. TODO(#123), when it must wait). If one is deliberate, say why in your reply and record it with " +
		"`auto debt baseline --force`; the baseline is committed, so a person sees it in review."}, warning
}

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
