package codex

import (
	"fmt"
	"strings"
)

func codexProjectContextLoadList() string {
	return "- `core`: `AGENTS.md`, `.autopus/project/workspace.md`\n" +
		"- `architecture`: `ARCHITECTURE.md`, `.autopus/project/product.md`, `.autopus/project/structure.md`, `.autopus/project/tech.md`\n" +
		"- `test`: `.autopus/project/scenarios.md`\n" +
		"- `signature`: `.autopus/context/signatures.md`\n" +
		"- `learning`: `.autopus/learnings/pipeline.jsonl`\n"
}

func injectCodexContextProfile(body string) string {
	if strings.Contains(body, "## Context Profile:") {
		return body
	}
	var profile string
	switch {
	case strings.Contains(body, "# auto-test"):
		profile = "## Context Profile: test\n\n- Required: core,test\n- Optional: signature,learning\n- Excluded: unrelated product documents\n\n### Test Input\n\nThe `test` profile includes `.autopus/project/scenarios.md`."
	}
	if profile == "" {
		return body
	}
	return injectAfterFirstHeading(body, profile)
}

func codexRouterExecutionContract() string {
	return fmt.Sprintf("## Router Execution Contract\n\n"+
		"- Treat this file as a thin entrypoint only.\n"+
		"- After resolving the subcommand, immediately load the matching detailed workflow surface (%s) before answering or acting.\n"+
		"- Do not stay at the router layer when a detailed workflow exists for the request.\n"+
		"- Load core workspace policy first, then only the selected command context profile.\n\n"+
		"## Context Load\n\n"+
		"Context profiles map to these documents:\n\n"+
		codexProjectContextLoadList()+"\n"+
		"- `test = core + test`; `signature` and `learning` are conditional.\n"+
		"- Do not load scenarios, signatures, learnings, or unrelated product documents outside the selected profile.\n"+
		"- If core documents are absent, explicitly note the gap and report which documents are missing instead of inventing their content.\n\n"+
		"## SPEC Path Resolution\n\n"+
		"When any workflow receives a SPEC-ID, resolve the actual file path before opening files, spawning workers, or running build/test commands:\n\n"+
		"1. Check `.autopus/specs/{SPEC-ID}/spec.md` (top-level, cross-module or legacy SPECs).\n"+
		"2. Recursively search `**/.autopus/specs/{SPEC-ID}/spec.md`, skipping `.git`, `node_modules`, `vendor`, `.cache`, and `dist`.\n\n"+
		"From the resolved path, extract:\n\n"+
		"- `SPEC_PATH`: full path to `spec.md`\n"+
		"- `SPEC_DIR`: parent SPEC directory\n"+
		"- `TARGET_MODULE`: submodule path, or `.` for top-level SPECs\n"+
		"- `WORKING_DIR`: the directory where build/test commands run (`TARGET_MODULE` or `.`)\n\n"+
		"Error handling:\n\n"+
		"- 0 matches: report the SPEC is missing and list available SPEC IDs.\n"+
		"- 2+ matches: report the duplicate paths and stop for clarification.\n"+
		"- All detailed workflows must use the resolved values instead of assuming `.autopus/specs/{SPEC-ID}` is rooted at the current directory.\n",
		routerDetailSkills(),
	)
}
