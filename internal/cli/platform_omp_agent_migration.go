package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompAgentSurfaceText is the locale text of the OMP agent lines
// (SPEC-MODELUX-003 Form Text). The paths and values inside stay the same in
// both languages.
type ompAgentSurfaceText struct {
	userFile, userOwned, unclaimable, kept, migration, restored, sonic string
}

var ompAgentSurfaceTextKO = ompAgentSurfaceText{
	userFile:  "%s는 사용자 파일이라 그대로 둡니다",
	userOwned: ".omp/config.yml의 task.disabledAgents를 사용자가 정해 두어 scout, security-reviewer, sonic, designer, librarian을 끄지 않았습니다",
	unclaimable: ".omp/config.yml을 안전하게 읽지 못해(symlink, 잘못된 YAML, 여러 문서, alias, flow 형식, " +
		"단독 CR·NEL·LS·PS 줄바꿈) task.disabledAgents를 쓰지 않았습니다. scout, security-reviewer, sonic, designer, librarian이 켜져 있습니다",
	kept: ".omp/config.yml에 ADK가 고치지 않는 줄바꿈(단독 CR, NEL, LS, PS)이 있어 task.disabledAgents를 " +
		"마지막으로 쓴 그대로 둡니다. 파일을 다시 읽을 수 있으면 이어서 관리합니다",
	migration: "OMP는 이제 Autopus 에이전트 16개를 씁니다. v0.50.126 동안 role_model_policy.agent_source: native로 되돌릴 수 있습니다.",
	restored:  "task.disabledAgents 복원",
	sonic:     "경고: role_model_policy.agents.sonic에 model을 지정했지만 autopus 모드는 sonic을 끄므로 그 model을 받는 에이전트가 없습니다",
}

var ompAgentSurfaceTextEN = ompAgentSurfaceText{
	userFile:  "%s is a user file; left unchanged",
	userOwned: "task.disabledAgents in .omp/config.yml is user-owned; scout, security-reviewer, sonic, designer, and librarian stay enabled",
	unclaimable: "task.disabledAgents was not written: .omp/config.yml cannot be read safely " +
		"(a symlink, invalid YAML, several documents, an alias, a flow-style mapping, " +
		"or a lone CR, NEL, LS, or PS line break); scout, security-reviewer, sonic, designer, and librarian stay enabled",
	kept: "task.disabledAgents is left as last written: .omp/config.yml holds a lone CR, NEL, LS, or PS " +
		"line break, which ADK does not edit; it is managed again once the file reads cleanly",
	migration: "OMP now uses the 16 Autopus agents; set role_model_policy.agent_source: native to go back during v0.50.126.",
	restored:  "task.disabledAgents restored",
	sonic:     "warning: role_model_policy.agents.sonic pins a model, but autopus mode disables sonic, so no agent receives it",
}

func ompAgentSurfaceTextFor(lang string) ompAgentSurfaceText {
	if lang == "ko" {
		return ompAgentSurfaceTextKO
	}
	return ompAgentSurfaceTextEN
}

// reportOMPAgentSurface prints, before the OMP write of one platform step,
// what that write does to the agent surface (REQ-MUX3-09, REQ-MUX3-14,
// REQ-MUX3-15, Migration Contract). It only reads: the OMP adapter's own
// transaction makes every change, and autopus.yaml is never written here. A
// surface it cannot read prints nothing, since the write reports that
// failure itself.
func reportOMPAgentSurface(out io.Writer, dir, platform string, cfg *config.HarnessConfig, update bool) {
	if platform != "omp" || cfg == nil || !containsPlatform(cfg.Platforms, "omp") {
		return
	}
	report, err := omp.InspectOMPAgentSurface(dir, cfg)
	if err != nil {
		return
	}
	for _, line := range ompAgentSurfaceLines(report, cfg, update, ompAgentSurfaceTextFor(composeLanguage())) {
		fmt.Fprintln(out, "    "+terminalSafe(line))
	}
}

// ompAgentSurfaceLines lists the agent lines for report. In autopus mode a
// user file is named wherever generation skips it, and an update whose
// manifest records no generated agent is the migration: every agent file it
// adds, the claim of task.disabledAgents, and the way back. In native mode
// an update names each recorded file it prunes and a released claim. A
// sonic pin, which no agent receives in autopus mode, is warned about.
func ompAgentSurfaceLines(report omp.OMPAgentSurfaceReport, cfg *config.HarnessConfig, update bool, text ompAgentSurfaceText) []string {
	var lines []string
	if report.Source == config.OMPAgentSourceNative {
		if !update {
			return nil
		}
		for _, agent := range report.RecordedAgents {
			lines = append(lines, "- "+omp.OMPAgentDefinitionPath(agent))
		}
		switch {
		case report.DisabledAgentsClaimed && report.ProjectConfigUnclaimable:
			lines = append(lines, text.kept)
		case report.DisabledAgentsClaimed:
			lines = append(lines, text.restored)
		}
		return lines
	}
	migrating := update && len(report.RecordedAgents) == 0
	for _, agent := range config.CanonicalAgentNames() {
		path := omp.OMPAgentDefinitionPath(agent)
		switch {
		case slices.Contains(report.UserAgentFiles, agent):
			lines = append(lines, fmt.Sprintf(text.userFile, path))
		case migrating:
			lines = append(lines, "+ "+path)
		}
	}
	switch {
	case report.ProjectConfigUnclaimable && report.DisabledAgentsClaimed:
		lines = append(lines, text.kept)
	case report.ProjectConfigUnclaimable:
		lines = append(lines, text.unclaimable)
	case report.DisabledAgentsUserOwned:
		lines = append(lines, text.userOwned)
	case migrating && !report.DisabledAgentsClaimed:
		lines = append(lines, "task.disabledAgents: "+strings.Join(config.OMPAutopusDisabledAgents(), ", "))
	}
	if migrating {
		lines = append(lines, text.migration)
	}
	if len(cfg.RoleModelPolicy.Agents["sonic"].Candidates) > 0 {
		lines = append(lines, text.sonic)
	}
	return lines
}
