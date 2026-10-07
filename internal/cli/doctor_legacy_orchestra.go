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
)

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
// are not listed. User-level settings are reported apart: update never edits
// them.
type retiredOrchestraReport struct {
	legacyKeys     []string
	staleHooks     []string // "<file> <event> <script>" members, then script paths
	userLevelHooks []string
	opencodeErr    error // effectivePluginConfig rejected opencode.json
}

func collectRetiredOrchestraReport(dir string, cfg *config.HarnessConfig) retiredOrchestraReport {
	report := retiredOrchestraReport{legacyKeys: retiredConfigKeysInFile(dir)}
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
	report.staleHooks = append(sortedUniqueStrings(members), sortedUniqueStrings(scripts)...)
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
					found = append(found, fmt.Sprintf("%s %s %s", label, event, staleHookScript(source.platform, command)))
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
}

func (f retiredOrchestraFinding) detail() string {
	return f.label + ": " + strings.Join(f.members, ", ")
}

func (report retiredOrchestraReport) findings() []retiredOrchestraFinding {
	return []retiredOrchestraFinding{
		{id: legacyOrchestraConfigCheckID, label: "legacy orchestra keys", pass: "no legacy orchestra keys in autopus.yaml",
			remedy: retiredOrchestraRemedy, members: report.legacyKeys},
		{id: staleCompletionHooksCheckID, label: "stale completion hooks", pass: "no stale completion hooks",
			remedy: retiredOrchestraRemedy, members: report.staleHooks},
		{id: userLevelStaleHooksCheckID, label: "user-level stale completion hooks",
			remedy: userLevelStaleHooksRemedy, members: report.userLevelHooks, advisory: true},
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
			check.Fields["opencode_config_error"] = report.opencodeErr.Error()
		}
		if !finding.advisory {
			r.status = jsonStatusWarn
		}
		r.checks = append(r.checks, check)
	}
}

// checkRetiredOrchestraText prints the report and returns false while a
// project member remains; user-level findings never fail the run.
func checkRetiredOrchestraText(out io.Writer, dir string, cfg *config.HarnessConfig) bool {
	tui.SectionHeader(out, "Retired Orchestra Surface")
	report := collectRetiredOrchestraReport(dir, cfg)
	healthy := true
	for _, finding := range report.findings() {
		if len(finding.members) == 0 {
			if finding.pass != "" {
				tui.OK(out, finding.pass)
			}
			continue
		}
		tui.SKIP(out, finding.detail())
		if finding.id == staleCompletionHooksCheckID && report.opencodeErr != nil {
			tui.Bullet(out, "opencode.json: "+report.opencodeErr.Error())
		}
		tui.Bullet(out, "remedy: "+finding.remedy)
		healthy = healthy && finding.advisory
	}
	return healthy
}
