package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// driverHB2 and driverHU1 are the Historical Preset Registry rows HB2 and
// HU1, copied from the SPEC so a fixture classifies as historical.
var (
	driverHB2 = map[string]string{
		"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet",
		"devops": "sonnet", "validator": "sonnet", "explorer": "sonnet",
	}
	driverHU1 = map[string]string{
		"planner": "opus", "executor": "opus", "validator": "opus", "tester": "opus",
		"reviewer": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
		"debugger": "opus", "explorer": "opus", "devops": "opus",
	}
)

// driverGroups mirrors the Role Group Contract so the fakes edit realistic
// compositions.
var driverGroups = []composeDriverGroup{
	{Label: "설계·판단", Members: []string{"planner", "architect", "spec-writer", "security-auditor"}},
	{Label: "구현", Members: []string{"executor", "debugger", "deep-worker", "devops", "frontend-specialist", "perf-engineer"}},
	{Label: "검증·리뷰", Members: []string{"reviewer", "validator", "tester", "ux-validator"}},
	{Label: "탐색·보조", Members: []string{"explorer", "annotator"}},
}

// driverStart is fixture D's starting composition: the balanced default.
func driverStart() map[string]string {
	return map[string]string{
		"planner": "fable", "architect": "fable", "spec-writer": "fable", "security-auditor": "fable",
		"executor": "sonnet", "debugger": "fable", "deep-worker": "fable", "devops": "sonnet",
		"frontend-specialist": "sonnet", "perf-engineer": "sonnet",
		"reviewer": "fable", "validator": "sonnet", "tester": "sonnet", "ux-validator": "sonnet",
		"explorer": "sonnet", "annotator": "sonnet",
	}
}

func driverConfig(platforms ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("p")
	cfg.Platforms = platforms
	return cfg
}

// driverHistoricalConfig carries fixture R's quality: ultra HU1, balanced
// HB2, and quality.default ultra.
func driverHistoricalConfig(platforms ...string) *config.HarnessConfig {
	cfg := driverConfig(platforms...)
	cfg.Quality.Default = "ultra"
	cfg.Quality.Presets = maps.Clone(cfg.Quality.Presets)
	cfg.Quality.Presets["ultra"] = config.QualityPreset{Description: "ultra", Agents: maps.Clone(driverHU1)}
	cfg.Quality.Presets["balanced"] = config.QualityPreset{Description: "balanced", Agents: maps.Clone(driverHB2)}
	return cfg
}

// driverFakeGroups shows a group as its distinct member tiers joined by "+";
// the driver prints whatever display the real editor computes.
type driverFakeGroups struct{}

func (driverFakeGroups) Groups(composition map[string]string) []composeDriverGroup {
	views := make([]composeDriverGroup, len(driverGroups))
	for index, group := range driverGroups {
		var tiers []string
		seen := map[string]bool{}
		for _, member := range group.Members {
			if tier := composition[member]; !seen[tier] {
				seen[tier] = true
				tiers = append(tiers, tier)
			}
		}
		views[index] = composeDriverGroup{Label: group.Label, Members: group.Members, Display: strings.Join(tiers, "+")}
	}
	return views
}

func (driverFakeGroups) SetGroupTier(composition map[string]string, group int, tier string) {
	for _, member := range driverGroups[group].Members {
		composition[member] = tier
	}
}

// driverFakeTable records the snapshot and configs of every table it draws.
type driverFakeTable struct {
	tables     []driftGenerationSnapshot
	previews   []driverFakePreview
	err        error // fails Table
	previewErr error // fails Preview
}

type driverFakePreview struct {
	start, candidate *config.HarnessConfig
	snapshot         driftGenerationSnapshot
}

func (f *driverFakeTable) Table(out io.Writer, _ *config.HarnessConfig, snapshot driftGenerationSnapshot) error {
	f.tables = append(f.tables, snapshot)
	fmt.Fprintln(out, "[table]")
	return f.err
}

func (f *driverFakeTable) Preview(out io.Writer, start, candidate *config.HarnessConfig, snapshot driftGenerationSnapshot) error {
	f.previews = append(f.previews, driverFakePreview{start: start, candidate: candidate, snapshot: snapshot})
	fmt.Fprintln(out, "[preview]")
	return f.previewErr
}

func (f *driverFakeTable) draws() int { return len(f.tables) + len(f.previews) }

// driverFakePlanner plans every changed edit as one custom binding and
// records each write it performs.
type driverFakePlanner struct {
	start     map[string]string
	refreshed map[string]string // composition a refresh resolves to
	edits     []map[string]string
	refreshes int
	applied   []map[string]string
	applyErr  error
	planErr   error
	noWrite   bool // plans carry no Apply
}

// Start hands out its own map, so a driver that edits in place shows up as a
// changed start.
func (f *driverFakePlanner) Start() map[string]string { return f.start }

func (f *driverFakePlanner) PlanEdit(edited map[string]string) (composeDriverPlan, error) {
	f.edits = append(f.edits, maps.Clone(edited))
	if f.planErr != nil {
		return composeDriverPlan{}, f.planErr
	}
	if maps.Equal(edited, f.start) {
		return composeDriverPlan{NoChange: true}, nil
	}
	return f.plan(edited, "quality.default balanced → custom"), nil
}

func (f *driverFakePlanner) PlanRefresh() (composeDriverPlan, error) {
	f.refreshes++
	if f.planErr != nil {
		return composeDriverPlan{}, f.planErr
	}
	return f.plan(f.refreshed, "quality.presets.ultra ← 최신 기본값"), nil
}

func (f *driverFakePlanner) plan(composition map[string]string, change string) composeDriverPlan {
	composition = maps.Clone(composition)
	plan := composeDriverPlan{
		Candidate:   &config.HarnessConfig{ProjectName: "candidate"},
		Composition: composition,
		Changes:     []string{change},
	}
	if !f.noWrite {
		plan.Apply = func() error {
			if f.applyErr != nil {
				return f.applyErr
			}
			f.applied = append(f.applied, composition)
			return nil
		}
	}
	return plan
}

// driverFakeTools stands in for the existing `auto quality` editors. Each
// route reads the next lines from the command input the way those editors
// do, so a test can see whether the answers queued behind the menu survive.
type driverFakeTools struct {
	cmd     *cobra.Command
	targets []string
	routes  []string
	// snapshots holds the snapshot each route received.
	snapshots []driftGenerationSnapshot
	read      []string
	reads     int // lines each route reads
	err       error
}

func (f *driverFakeTools) Targets() []string { return f.targets }

func (f *driverFakeTools) Route(target string, snapshot driftGenerationSnapshot) error {
	f.routes = append(f.routes, target)
	f.snapshots = append(f.snapshots, snapshot)
	reader := bufio.NewReader(f.cmd.InOrStdin())
	for range f.reads {
		line, _ := reader.ReadString('\n')
		f.read = append(f.read, strings.TrimSpace(line))
	}
	return f.err
}

// driverFailingReader fails the test on any Read: the step must not touch
// stdin.
type driverFailingReader struct{ t *testing.T }

func (r driverFailingReader) Read([]byte) (int, error) {
	r.t.Error("composition step read stdin")
	return 0, errors.New("stdin must not be read")
}

func newDriverSession(cfg *config.HarnessConfig) (composeDriverSession, *driverFakeTable, *driverFakePlanner) {
	table := &driverFakeTable{}
	planner := &driverFakePlanner{start: driverStart(), refreshed: driverStart()}
	planner.refreshed["planner"] = "opus"
	session := composeDriverSession{
		Config:  cfg,
		Table:   table,
		Groups:  driverFakeGroups{},
		Planner: planner,
		Text:    composeTextFor("ko"),
	}
	return session, table, planner
}

func newDriverCommand(in io.Reader) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	out := &bytes.Buffer{}
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(out)
	return cmd, out
}

// runDriver runs one session over input with the catalog probe stubbed, so
// no test can reach a real codex binary.
func runDriver(t *testing.T, session composeDriverSession, input string) (composeDriverResult, string, error) {
	t.Helper()
	stubComposeCatalogProbe(t, composeCatalogA, nil)
	cmd, out := newDriverCommand(strings.NewReader(input))
	result, err := session.run(cmd)
	return result, out.String(), err
}
