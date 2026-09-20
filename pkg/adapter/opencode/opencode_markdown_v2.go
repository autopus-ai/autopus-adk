package opencode

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const openCodeV2NativeContract = `## OpenCode V2 native contract

Use the advertised native tool catalog. Delegation uses subagent, not a shell
command. The required input fields are agent, description, and prompt:

` + "```json\n" + `{"agent":"executor","description":"Implement scoped change","prompt":"Read the assigned context, change only owned files, and return verification."}` + "\n```\n" + `
Only configured subagent-mode agents are eligible. New children need complete
instructions; they do not inherit the parent's conversation. Foreground calls
wait for completion. Use background: true only for independent work; completion
is delivered to the parent. Do not poll or treat the initial running status as
completion. To continue that same child, pass its returned sessionID; it must
belong to the current parent. Do not invent task IDs or cancellation tools.

Omit model unless the user explicitly requested an override. Native model
references use provider/model#variant; translate an Autopus --variant flag to
that suffix rather than forwarding --variant to the V2 CLI. Use shell with its
workdir field for commands. Plugin wiring uses the effective plugins setting;
registration alone does not prove hook execution. If the advertised schema
differs, stop that dispatch and report the mismatch rather than guessing.
`

var openCodeV2AgentArgument = regexp.MustCompile(`(?m)(subagent\(\s*\n\s*)subagent_type(\s*=\s*"([^"]+)",)`)

func adaptOpenCodeV2Markdown(body string) string {
	// Keep the original role value and supply the description required by V2.
	body = strings.ReplaceAll(body, "task(", "subagent(")
	body = openCodeV2AgentArgument.ReplaceAllString(body, "${1}agent${2}\n  description = \"Run ${3} assignment\",")
	return strings.NewReplacer(
		"task(", "subagent(",
		"subagent_type", "agent",
		"`task`", "`subagent`",
		"task-based", "subagent-based",
		"OpenCode task pipeline", "OpenCode subagent pipeline",
		"task tool", "subagent tool",
	).Replace(body)
}

func (a *Adapter) adaptRuntimeMappings(files []adapter.FileMapping) []adapter.FileMapping {
	if !a.isV2() {
		return files
	}
	for i := range files {
		path := filepath.ToSlash(files[i].TargetPath)
		if !strings.HasSuffix(path, ".md") {
			continue
		}
		body := string(files[i].Content)
		if path == "AGENTS.md" {
			// Marker updates must never transform the user's surrounding document.
			body = markerRe.ReplaceAllStringFunc(body, func(section string) string {
				section = adaptOpenCodeV2Markdown(section)
				return strings.Replace(section, markerEnd, openCodeV2NativeContract+"\n"+markerEnd, 1)
			})
		} else {
			body = adaptOpenCodeV2Markdown(body)
			if path == ".agents/skills/auto/SKILL.md" || path == ".agents/skills/auto-go/SKILL.md" || path == ".agents/skills/agent-pipeline/SKILL.md" {
				body = injectAfterFirstHeading(body, openCodeV2NativeContract)
			}
		}
		files[i].Content = []byte(body)
		files[i].Checksum = adapter.Checksum(body)
	}
	return files
}
