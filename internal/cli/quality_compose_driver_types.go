package cli

import (
	"io"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeDriverMaxInvalid is how many consecutive unusable answers a prompt
// takes before it keeps what it had (SPEC-MODELUX-001 REQ-MUX-13).
const composeDriverMaxInvalid = 3

// composeDriverTable draws the composition table (Table Cell Contract). Both
// methods resolve the Claude Code and Codex cells with the run's snapshot,
// so the table cannot show a catalog the same run's generation does not see.
// When a cell refuses Codex generation, the drawn table carries the
// `Codex 생성이 실패합니다` line itself.
type composeDriverTable interface {
	// Table writes the table of cfg.
	Table(out io.Writer, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) error
	// Preview writes the after-table of candidate, marking with `*` every cell
	// whose raw value differs from the table of start (REQ-MUX-06).
	Preview(out io.Writer, start, candidate *config.HarnessConfig, snapshot driftGenerationSnapshot) error
}

// composeDriverGroup is one role group as the operator sees it.
type composeDriverGroup struct {
	Label   string   // localized group label
	Members []string // agents in display order
	Display string   // one tier, or 혼합 with per-tier counts (REQ-MUX-03)
}

// composeDriverGroups owns the role groups (Role Group Contract). It works on
// a composition, agent name to tier, that the driver holds, so a candidate
// can be edited again after its preview.
type composeDriverGroups interface {
	// Groups returns the four groups in contract order, each with its
	// display computed from composition.
	Groups(composition map[string]string) []composeDriverGroup
	// SetGroupTier sets every member of the group at index group to tier.
	SetGroupTier(composition map[string]string, group int, tier string)
}

// composeDriverPlan is one pending write the binding planner computed.
type composeDriverPlan struct {
	// NoChange reports an edit equal to the starting composition; it writes
	// nothing (REQ-MUX-07).
	NoChange bool
	// Candidate is the detached config the write leaves; the preview draws it.
	Candidate *config.HarnessConfig
	// Composition is the composition Candidate resolves to; `e` edits it.
	Composition map[string]string
	// Changes lists every autopus.yaml key the write changes and every effect
	// outside the table, one line each.
	Changes []string
	// Apply performs the write once the operator confirms. A refused or
	// failed write leaves the original file as it was.
	Apply func() error
}

// composeDriverPlanner turns edits into writes (Binding Contract).
type composeDriverPlanner interface {
	// Start returns the starting composition (REQ-MUX-02).
	Start() map[string]string
	// PlanEdit plans persisting edited, a group or advanced edit.
	PlanEdit(edited map[string]string) (composeDriverPlan, error)
	// PlanRefresh plans the `u` refresh of every historical built-in preset
	// (REQ-MUX-11).
	PlanRefresh() (composeDriverPlan, error)
}

// composeDriverTools hands `auto quality` input to its existing editors
// (REQ-MUX-16).
type composeDriverTools interface {
	// Targets lists the target names chooseQualityTarget offers.
	Targets() []string
	// Route runs the editor of target as chooseQualityTarget does once that
	// target is chosen. The empty target is the `t` key: the existing target
	// menu asks first.
	Route(target string) error
}

// composeDriverOutcome is how a composition session ended.
type composeDriverOutcome int

const (
	composeDriverKept    composeDriverOutcome = iota // nothing written
	composeDriverApplied                             // a confirmed plan was applied
	composeDriverRouted                              // an existing editor took over the input
)

// composeDriverResult reports a session. Snapshot is the catalog snapshot the
// table read; the same run generates with it whatever the outcome, so the
// generated files equal the table the operator last approved or kept
// (REQ-MUX-19). It is the zero snapshot when no table was drawn.
type composeDriverResult struct {
	Outcome  composeDriverOutcome
	Snapshot driftGenerationSnapshot
	Plan     composeDriverPlan // the applied plan
}

// composeDriverSession is the interactive composition step init, update, and
// quality share (REQ-MUX-17).
type composeDriverSession struct {
	// Config is the loaded config: it feeds the table and the GAP 1 labels
	// and is never a write source.
	Config  *config.HarnessConfig
	Table   composeDriverTable
	Groups  composeDriverGroups
	Planner composeDriverPlanner
	// Tools is set only by `auto quality`; it adds `t` and the target names.
	Tools composeDriverTools
	// Text is the operator's locale text when left zero.
	Text composeText
}
