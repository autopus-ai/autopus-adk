package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// Check IDs of the surface SPEC-PANERM-001 retired (REQ-14).
const (
	legacyOrchestraConfigCheckID = "doctor.legacy_orchestra_config"
	staleCompletionHooksCheckID  = "doctor.stale_completion_hooks"
	userLevelStaleHooksCheckID   = "doctor.stale_completion_hooks.user_level"
	localStaleHooksCheckID       = "doctor.stale_completion_hooks.local"
)

// localStaleHookSource is Claude Code's local settings file: Claude runs its
// handlers like the shared file's, but update never writes it, so its stale
// handlers are reported apart and the scripts they run stay.
var localStaleHookSource = staleHookSource{file: ".claude/settings.local.json", platform: "claude-code", hookSet: "hooks"}

// staleHookSource is one settings file whose handlers can run a group S
// script of platform. hookSet is the key of the event map inside the file:
// the nested "hooks" map, or the "autopus" hook set that the antigravity-cli
// update regenerates in .agents/hooks.json.
type staleHookSource struct {
	file, platform, hookSet string
}

var staleHookSources = []staleHookSource{
	{file: ".claude/settings.json", platform: "claude-code", hookSet: "hooks"},
	{file: ".codex/hooks.json", platform: "codex", hookSet: "hooks"},
	{file: ".agents/hooks.json", platform: "antigravity-cli", hookSet: "autopus"},
	{file: ".gemini/settings.json", platform: "antigravity-cli", hookSet: "hooks"},
}

// retiredOrchestraReport is what doctor finds of the retired surface. The
// project lists come from the functions that load and update use, so doctor
// names what `auto update` removes: config.PruneRetiredKeys for group K, and
// for group S the configured platforms' adapter.IsStaleCompletionHookCommand
// handlers, adapter.PresentStaleCompletionScripts files, and OpenCode plugin
// entries. A platform that is not configured is not updated, so its leftovers
// are not listed. User-level and local settings are reported apart: update
// never edits them.
type retiredOrchestraReport struct {
	legacyKeys []string
	staleHooks []string // "<file> <event> <script>" members, then script paths
	// openCodeHeld lists the staleHooks scripts that opencode.json names while
	// opencode is not configured. Only the opencode update edits that file and
	// every update keeps a script a settings file names, so no update removes
	// them until the user does (W-mix).
	openCodeHeld   []string
	userLevelHooks []string
	localHooks     []string
	opencodeErr    error // effectivePluginConfig rejected opencode.json
}

func collectRetiredOrchestraReport(dir string, cfg *config.HarnessConfig) retiredOrchestraReport {
	report := retiredOrchestraReport{legacyKeys: retiredConfigKeysInFile(dir),
		localHooks: sortedUniqueStrings(staleHookHandlers(dir, localStaleHookSource, localStaleHookSource.file))}
	var members, scripts []string
	for _, platform := range cfg.Platforms {
		for _, source := range staleHookSources {
			if source.platform == platform {
				members = append(members, staleHookHandlers(dir, source, source.file)...)
			}
		}
		scripts = append(scripts, adapter.PresentStaleCompletionScripts(dir, adapter.StaleCompletionHookScripts(platform))...)
		if platform == "opencode" {
			entries, loaded, err := staleOpenCodeMembers(dir)
			members = append(members, entries...)
			scripts = append(scripts, loaded...)
			report.opencodeErr = err
		}
	}
	listed := sortedUniqueStrings(scripts)
	report.staleHooks = append(sortedUniqueStrings(members), listed...)
	if !slices.Contains(cfg.Platforms, "opencode") {
		report.openCodeHeld = scriptsOnlyOpenCodeConfigNames(dir, listed)
	}
	if home, err := os.UserHomeDir(); err == nil && !sameDirectory(home, dir) {
		var user []string
		for _, source := range staleHookSources {
			user = append(user, staleHookHandlers(home, source, "~/"+source.file)...)
		}
		report.userLevelHooks = sortedUniqueStrings(user)
	}
	return report
}

// staleHookHandlers lists, as "<label> <event> <script>", the handlers of one
// settings file under root whose command runs a group S script of the
// source's platform. A missing or undecodable file lists none.
func staleHookHandlers(root string, source staleHookSource, label string) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(source.file)))
	if err != nil {
		return nil
	}
	var doc map[string]any
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	events, _ := doc[source.hookSet].(map[string]any)
	var found []string
	for event, raw := range events {
		items, _ := raw.([]any)
		for _, item := range items {
			for _, command := range staleHookItemCommands(item) {
				if adapter.IsStaleCompletionHookCommand(source.platform, command) {
					found = append(found, fmt.Sprintf("%s %s %s", label, terminalSafe(event), staleHookScript(source.platform, command)))
				}
			}
		}
	}
	return found
}

// staleHookItemCommands returns the commands of one event item: the handlers
// of an entry ({matcher, hooks: [...]}), or the item itself when it is a
// handler, the shape .agents/hooks.json uses for Stop.
func staleHookItemCommands(item any) []string {
	object, _ := item.(map[string]any)
	if command, ok := object["command"].(string); ok {
		return []string{command}
	}
	handlers, _ := object["hooks"].([]any)
	var commands []string
	for _, handler := range handlers {
		fields, _ := handler.(map[string]any)
		if command, ok := fields["command"].(string); ok {
			commands = append(commands, command)
		}
	}
	return commands
}

func staleHookScript(platform, command string) string {
	for _, script := range adapter.StaleCompletionHookScripts(platform) {
		if strings.Contains(command, script) {
			return script
		}
	}
	return strings.TrimSpace(command)
}

// staleOpenCodeMembers lists the opencode.json plugin entries that load a
// group S script and the present scripts they load, read by the function the
// opencode update retracts them with. When effectivePluginConfig rejects the
// config, update fails and changes nothing; the group S scripts the raw file
// names are listed instead, together with the rejection.
func staleOpenCodeMembers(dir string) (entries, scripts []string, err error) {
	loaded, err := opencode.StaleCompletionPluginScripts(dir)
	if err != nil {
		data, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
		loaded = nil
		for _, script := range adapter.AllStaleCompletionHookScripts() {
			if bytes.Contains(data, []byte(script)) {
				loaded = append(loaded, script)
			}
		}
	}
	for _, script := range loaded {
		entries = append(entries, "opencode.json plugin "+script)
	}
	return entries, adapter.PresentStaleCompletionScripts(dir, loaded), err
}

// scriptsOnlyOpenCodeConfigNames returns the present scripts that an update
// keeps because opencode.json names them, read by the function every update
// plans its script removes with. The other settings files are planned empty,
// as the updates of the configured platforms leave their stale handlers, so a
// script that stays is named by opencode.json alone (a settings file that
// cannot be read keeps every script, as it does for update).
func scriptsOnlyOpenCodeConfigNames(dir string, scripts []string) []string {
	if len(scripts) == 0 {
		return nil
	}
	var others []adapter.TransactionWrite
	for _, source := range append(slices.Clone(staleHookSources), localStaleHookSource) {
		others = append(others, adapter.TransactionWrite{Path: source.file})
	}
	removed := map[string]bool{}
	for _, remove := range adapter.StaleCompletionScriptRemoves(dir, scripts, others) {
		removed[remove.Path] = true
	}
	var held []string
	for _, script := range scripts {
		if !removed[script] {
			held = append(held, script)
		}
	}
	return held
}

func sameDirectory(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}

func sortedUniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	return slices.Compact(slices.Sorted(slices.Values(items)))
}

// retiredOrchestraFinding is one check of the report in the shape both doctor
// modes print. An advisory finding warns without failing the run, and a
// finding without a pass detail is printed only while it has members.
type retiredOrchestraFinding struct {
	id, label, pass, remedy string
	members                 []string
	advisory                bool
	updateKeepsAll          bool // no update removes any member
}

// detail is the finding's sentence for both doctor modes. Its members come
// from user files (provider keys, settings event names, commands), and
// encoding/json leaves a bidi override, DEL, or C1 control in the bytes it
// writes, so the sentence is escaped for the JSON report as for the terminal.
func (f retiredOrchestraFinding) detail() string {
	return terminalSafe(f.label + ": " + strings.Join(f.members, ", "))
}

// opencodeConfigError is the rejection of opencode.json as both doctor modes
// print it; the message can carry the project path.
func (report retiredOrchestraReport) opencodeConfigError() string {
	return terminalSafe(report.opencodeErr.Error())
}

func (report retiredOrchestraReport) findings() []retiredOrchestraFinding {
	staleRemedy := retiredOrchestraRemedy
	if len(report.openCodeHeld) > 0 {
		staleRemedy = openCodeHeldStaleHooksRemedy(report.openCodeHeld)
	}
	return []retiredOrchestraFinding{
		{id: legacyOrchestraConfigCheckID, label: "legacy orchestra keys", pass: "no legacy orchestra keys in autopus.yaml",
			remedy: retiredOrchestraRemedy, members: report.legacyKeys},
		{id: staleCompletionHooksCheckID, label: "stale completion hooks", pass: "no stale completion hooks",
			remedy: staleRemedy, members: report.staleHooks,
			updateKeepsAll: len(report.openCodeHeld) > 0 && len(report.openCodeHeld) == len(report.staleHooks)},
		{id: userLevelStaleHooksCheckID, label: "user-level stale completion hooks",
			remedy: userLevelStaleHooksRemedy, members: report.userLevelHooks, advisory: true},
		{id: localStaleHooksCheckID, label: "local stale completion hooks",
			remedy: localStaleHooksRemedy, members: report.localHooks, advisory: true},
	}
}

func (r *doctorJSONReport) collectRetiredOrchestraChecks(dir string, cfg *config.HarnessConfig) {
	report := collectRetiredOrchestraReport(dir, cfg)
	for _, finding := range report.findings() {
		if len(finding.members) == 0 {
			if finding.pass != "" {
				r.checks = append(r.checks, jsonCheck{ID: finding.id, Severity: "info", Status: "pass", Detail: finding.pass})
			}
			continue
		}
		check := jsonCheck{ID: finding.id, Severity: "warning", Status: "warn", Detail: finding.detail(),
			Fields: map[string]string{"remedy": finding.remedy}}
		if finding.id == staleCompletionHooksCheckID && report.opencodeErr != nil {
			check.Fields["opencode_config_error"] = report.opencodeConfigError()
		}
		if !finding.advisory {
			r.status = jsonStatusWarn
		}
		r.checks = append(r.checks, check)
	}
}

// checkRetiredOrchestraText prints the report. healthy is false while a
// project member remains; user-level findings never fail the run. updateFixes
// is true while a remaining member is one `auto update` removes, so the
// summary names update only when running it changes something.
func checkRetiredOrchestraText(out io.Writer, dir string, cfg *config.HarnessConfig) (healthy, updateFixes bool) {
	tui.SectionHeader(out, "Retired Orchestra Surface")
	report := collectRetiredOrchestraReport(dir, cfg)
	healthy = true
	for _, finding := range report.findings() {
		if len(finding.members) == 0 {
			if finding.pass != "" {
				tui.OK(out, finding.pass)
			}
			continue
		}
		tui.SKIP(out, finding.detail())
		if finding.id == staleCompletionHooksCheckID && report.opencodeErr != nil {
			tui.Bullet(out, "opencode.json: "+report.opencodeConfigError())
		}
		tui.Bullet(out, "remedy: "+finding.remedy)
		healthy = healthy && finding.advisory
		updateFixes = updateFixes || !finding.advisory && !finding.updateKeepsAll
	}
	return healthy, updateFixes
}
