package cli

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeSessionWriter says where a confirmed composition goes.
type composeSessionWriter int

const (
	// composeSessionFile persists through the quality writers into dir's
	// autopus.yaml and syncs the loaded config: update and quality.
	composeSessionFile composeSessionWriter = iota
	// composeSessionMemory applies the plan to the in-memory config only:
	// init, which saves that config whole before it generates.
	composeSessionMemory
)

// newComposeSession wires the composition step of one command run
// (SPEC-MODELUX-001 REQ-MUX-17) to the real table, role group editor, and
// binding planner. cfg is the loaded config: the table and the planner read
// it, a file write syncs its quality section, and a memory write sets it.
// sources are the embedded agent sources the generators read, and lang the
// locale of every screen. tools is the `auto quality` editor router and nil
// for update and init.
//
// No snapshot is passed in: the driver probes the Codex catalog once per run
// and hands that snapshot to every table it draws, and the planner describes
// each write against the snapshot the table last drew with, so the table,
// the preview, the change lines, and the caller's generation share one
// catalog.
func newComposeSession(
	dir string,
	cfg *config.HarnessConfig,
	sources map[string]composeAgentSource,
	lang string,
	writer composeSessionWriter,
	tools composeDriverTools,
) composeDriverSession {
	run := &composeSessionRun{dir: dir, cfg: cfg, sources: sources, lang: lang, writer: writer}
	return composeDriverSession{
		Config:  cfg,
		Table:   composeSessionTable{run: run},
		Groups:  composeSessionGroups{lang: lang},
		Planner: &composeSessionPlanner{run: run},
		Tools:   tools,
		Text:    composeTextFor(lang),
	}
}

// composeSessionRun is the state the table and the planner of one run share.
type composeSessionRun struct {
	dir     string
	cfg     *config.HarnessConfig
	sources map[string]composeAgentSource
	lang    string
	writer  composeSessionWriter
	// snapshot is the catalog snapshot of the last table drawn. The driver
	// draws the table before it plans anything.
	snapshot driftGenerationSnapshot
}

// composeSessionTable draws the Table Cell Contract table through
// buildComposeTable, the resolvers the generators call.
type composeSessionTable struct{ run *composeSessionRun }

func (t composeSessionTable) Table(out io.Writer, cfg *config.HarnessConfig, snapshot driftGenerationSnapshot) error {
	t.run.snapshot = snapshot
	return buildComposeTable(cfg, snapshot, t.run.sources).render(out, t.run.lang, nil)
}

// Preview marks every cell whose raw value differs from start's table; both
// tables come from the same binary and the same snapshot (REQ-MUX-19).
func (t composeSessionTable) Preview(out io.Writer, start, candidate *config.HarnessConfig, snapshot driftGenerationSnapshot) error {
	t.run.snapshot = snapshot
	before := buildComposeTable(start, snapshot, t.run.sources)
	return buildComposeTable(candidate, snapshot, t.run.sources).render(out, t.run.lang, &before)
}

// composeSessionGroups presents composeRoleGroups over the driver's
// composition map.
type composeSessionGroups struct{ lang string }

func (g composeSessionGroups) Groups(composition map[string]string) []composeDriverGroup {
	state := qualityComposition{tiers: composition}
	groups := make([]composeDriverGroup, len(composeRoleGroups))
	for index, group := range composeRoleGroups {
		groups[index] = composeDriverGroup{
			Label:   group.Label(g.lang),
			Members: slices.Clone(group.Agents),
			Display: state.GroupDisplay(group, g.lang),
		}
	}
	return groups
}

// SetGroupTier sets the group's members in place. The driver hands over only
// tiers its answer parser accepted, which SetGroupTier never refuses.
func (composeSessionGroups) SetGroupTier(composition map[string]string, group int, tier string) {
	state := qualityComposition{tiers: composition}
	_ = state.SetGroupTier(composeRoleGroups[group], tier)
}

// composeSessionPlanner turns the driver's edits into Binding Contract writes.
type composeSessionPlanner struct {
	run *composeSessionRun
	// refresh is the `u` plan once PlanRefresh ran. The driver asks the
	// composition question once, so every later edit is an `e` from that
	// refresh's preview and binds on top of the refreshed candidate.
	refresh *composeBindingPlan
}

func (p *composeSessionPlanner) start() qualityComposition {
	return newQualityComposition(p.run.cfg.Quality, p.run.sources)
}

func (p *composeSessionPlanner) Start() map[string]string {
	return p.start().Agents()
}

func (p *composeSessionPlanner) PlanEdit(edited map[string]string) (composeDriverPlan, error) {
	return p.driverPlan(p.bind(edited))
}

// bind plans the binding of edited: from the loaded start, or on top of the
// `u` refresh once PlanRefresh ran.
func (p *composeSessionPlanner) bind(edited map[string]string) (composeBindingPlan, qualityComposition) {
	composition := qualityComposition{tiers: maps.Clone(edited)}
	if p.refresh == nil {
		return planComposeBinding(p.run.cfg, p.start(), composition), composition
	}
	start := newQualityComposition(p.refresh.Candidate.Quality, p.run.sources)
	return planComposeBindingAfter(p.run.cfg, *p.refresh, start, composition), composition
}

func (p *composeSessionPlanner) PlanRefresh() (composeDriverPlan, error) {
	plan, ok := planComposeRefresh(p.run.cfg)
	if !ok {
		return composeDriverPlan{}, errors.New("no built-in quality preset is a previous-version default")
	}
	p.refresh = &plan
	return p.driverPlan(plan, newQualityComposition(plan.Candidate.Quality, p.run.sources))
}

// driverPlan hands plan to the driver with its change lines and its write. A
// plan that writes nothing is the starting composition (REQ-MUX-07).
func (p *composeSessionPlanner) driverPlan(plan composeBindingPlan, composition qualityComposition) (composeDriverPlan, error) {
	if plan.Writer == composeWriterNone {
		return composeDriverPlan{NoChange: true}, nil
	}
	source, err := p.run.source()
	if err != nil {
		return composeDriverPlan{}, err
	}
	return composeDriverPlan{
		Candidate:   plan.Candidate,
		Composition: composition.Agents(),
		Changes:     describeComposeBinding(p.run.cfg, plan, p.run.snapshot, source, p.run.lang),
		Apply:       func() error { return p.run.persist(plan) },
	}, nil
}

// source returns the autopus.yaml bytes a block write would rewrite, for the
// rewrite notice. A memory write and a project without the file rewrite
// nothing, so they have no source.
func (r *composeSessionRun) source() ([]byte, error) {
	if r.writer == composeSessionMemory {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(r.dir, "autopus.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return data, nil
}

// persist performs a confirmed plan. The file writers leave the file and the
// config as they were on refusal or failure; the memory write checks every
// planned built-in before it sets anything.
func (r *composeSessionRun) persist(plan composeBindingPlan) error {
	if r.writer == composeSessionMemory {
		return plan.apply(&r.cfg.Quality)
	}
	return persistComposeBinding(r.dir, r.cfg, plan)
}
