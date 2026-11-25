package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// reviewFindingsPath is where the verification round's reviewer records
// what it reproduced, relative to the repository root. Pilot C measured the
// reviewer reproducing defects the agent then never applied because it ended
// its turn; the file makes those findings outlive the reviewer's reply.
const reviewFindingsPath = ".autopus/review/findings.json"

// reviewFindingsCap is how many findings the gate enforces: the most severe
// five. A longer list is the reviewer's to trim; the rest are not enforced.
const reviewFindingsCap = 5

// reviewFinding is one finding in the findings file. The hook never runs
// Repro: the file is written by an agent, and a repro may be slow or
// unsafe. A finding counts as resolved when its status is fixed (its repro
// now shows Expected) or rebutted, and Evidence holds the execution that
// shows it.
type reviewFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity,omitempty"`
	Summary  string `json:"summary"`
	Repro    string `json:"repro"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

const (
	findingOpen     = "open"
	findingFixed    = "fixed"
	findingRebutted = "rebutted"
)

// unresolvedWhy says why a finding still holds the stop, or "" when it is
// resolved.
func (f reviewFinding) unresolvedWhy() string {
	switch strings.ToLower(strings.TrimSpace(f.Status)) {
	case findingFixed, findingRebutted:
		if strings.TrimSpace(f.Evidence) == "" {
			return "status " + f.Status + " without evidence"
		}
		return ""
	case findingOpen, "":
		return "open"
	}
	return "unknown status " + strconv.Quote(f.Status)
}

var severityRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

func rankOf(severity string) int {
	if r, ok := severityRank[strings.ToLower(strings.TrimSpace(severity))]; ok {
		return r
	}
	return len(severityRank)
}

// readReviewFindings reads the findings file: a JSON list of findings, or an
// object holding it under "findings". A missing file is no findings.
func readReviewFindings(root string) ([]reviewFinding, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, reviewFindingsPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	var list []reviewFinding
	if err := json.Unmarshal(data, &list); err != nil {
		var wrapped struct {
			Findings []reviewFinding `json:"findings"`
		}
		if err2 := json.Unmarshal(data, &wrapped); err2 != nil {
			return nil, true, err
		}
		list = wrapped.Findings
	}
	slices.SortStableFunc(list, func(a, b reviewFinding) int { return rankOf(a.Severity) - rankOf(b.Severity) })
	if len(list) > reviewFindingsCap {
		list = list[:reviewFindingsCap]
	}
	return list, true, nil
}

// unresolvedFindings lists the findings that still hold the stop, one line
// each with id, summary and repro.
func unresolvedFindings(root string) []string {
	list, exists, err := readReviewFindings(root)
	if !exists {
		return nil
	}
	if err != nil {
		return []string{fmt.Sprintf("%s cannot be read as a findings list (%s); rewrite it as a JSON list", reviewFindingsPath, clip(err.Error(), 200))}
	}
	var lines []string
	for i, f := range list {
		why := f.unresolvedWhy()
		if why == "" {
			continue
		}
		id := f.ID
		if id == "" {
			id = "#" + strconv.Itoa(i+1)
		}
		line := fmt.Sprintf("- %s [%s]: %s\n  repro: %s", id, why, clip(firstLine(f.Summary), 200), clip(firstLine(f.Repro), 300))
		lines = append(lines, line)
	}
	return lines
}

// findingsResolve is what a block asks the agent to do.
const findingsResolve = "For each one, either fix it so its repro shows the expected result and set \"status\": \"fixed\", " +
	"or show by execution that it is not a defect and set \"status\": \"rebutted\"; in both cases put the command you ran " +
	"and its output in \"evidence\". A finding about code no longer in the tree is rebutted with that evidence. " +
	"Do not finish while a finding is open."

// withFindings holds a stop that would otherwise go through while the
// reviewer's findings file has unresolved findings. A block from the build,
// the tests or the verification round comes first; the findings are checked
// at the stop after it. Unlike a build block it repeats on the same tree
// (open findings are not fixed by stopping again), but it has its own count
// under the same verifyStopMaxBlocks cap, after which the findings are
// reported as a note and the session may end.
func withFindings(in stopHookInput, root string, out stopHookOutput) stopHookOutput {
	if out.Decision == "block" || backgroundWriters(in) > 0 {
		return out
	}
	open := unresolvedFindings(root)
	if len(open) == 0 {
		_ = os.Remove(stopStateFile(in.SessionID, "findings"))
		return out
	}
	list := strings.Join(open, "\n")
	if countFindingsBlock(in.SessionID) > verifyStopMaxBlocks {
		return withSecretWarning(out, fmt.Sprintf("auto verify stop: %d review finding(s) in %s are still unresolved after %d blocks; "+
			"ending anyway. Unresolved:\n%s", len(open), reviewFindingsPath, verifyStopMaxBlocks, list))
	}
	out.Decision = "block"
	out.Reason = fmt.Sprintf("%d review finding(s) in %s are not resolved:\n%s\n\n%s",
		len(open), reviewFindingsPath, list, findingsResolve)
	return out
}

// countFindingsBlock records one more findings block for the session and
// returns the total. It is not reset by a passing build, so fixing other
// things cannot keep the findings block going forever.
func countFindingsBlock(session string) int {
	n, _ := strconv.Atoi(readStopState(session, "findings"))
	n++
	writeStopState(session, "findings", strconv.Itoa(n))
	return n
}
