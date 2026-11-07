package content

import (
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"gopkg.in/yaml.v3"
)

// ompAgentToolMap maps portable ADK tools onto OMP tool names. TodoWrite has
// no OMP counterpart, and WebFetch folds onto the same web_search tool.
var ompAgentToolMap = map[string]string{
	"Read": "read", "Write": "write", "Edit": "edit", "Grep": "grep",
	"Glob": "glob", "Bash": "bash", "WebSearch": "web_search", "WebFetch": "web_search",
}

// ompAgentNativeTools are the OMP-native capabilities a role needs on top of
// its portable tools. The browser roles name both OMP generations: omp/17.2.7
// ships browser and inspect_image as tools, while 18.x drives the browser from
// the eval prelude and reads images with read. OMP drops a frontmatter tool
// the running release lacks when it spawns the agent, so each release keeps
// only its own names.
var ompAgentNativeTools = map[string][]string{
	"debugger":            {"lsp"},
	"executor":            {"lsp"},
	"frontend-specialist": {"browser", "eval", "inspect_image", "lsp"},
	"qa-explorer":         {"browser", "eval", "inspect_image"},
	"reviewer":            {"lsp"},
	"ux-validator":        {"browser", "eval", "inspect_image"},
	"validator":           {"lsp"},
}

// ompReviewerAgent is the one generated agent that replaces a bundled agent
// of the same name, so it keeps that agent's output schema.
const ompReviewerAgent = "reviewer"

// ompReviewerOutputSchema is the output of the reviewer omp 18.6.1 bundles.
// OMP's /review sends its one task to the agent named reviewer and reads
// overall_correctness and confidence from this schema; an Autopus dispatch
// passes its own schema per call, which takes precedence over this one.
const ompReviewerOutputSchema = `properties:
  overall_correctness:
    metadata:
      description: Whether change correct (no bugs/blockers)
    enum:
    - correct
    - incorrect
  explanation:
    metadata:
      description: Plain-text verdict summary, 1-3 sentences
    type: string
  confidence:
    metadata:
      description: Verdict confidence (0.0-1.0)
    type: number
optionalProperties:
  findings:
    metadata:
      description: 'Populate via incremental yield sections under type: ["findings"]; don''t repeat it in a final payload.'
    elements:
      properties:
        title:
          metadata:
            description: Imperative, ≤80 chars
          type: string
        body:
          metadata:
            description: 'One paragraph: bug, trigger, impact'
          type: string
        priority:
          metadata:
            description: 'P0-P3: 0 blocks release, 1 fix next cycle, 2 fix eventually, 3 nice to have'
          type: number
        confidence:
          metadata:
            description: Confidence it's real bug (0.0-1.0)
          type: number
        file_path:
          metadata:
            description: Path to affected file
          type: string
        line_start:
          metadata:
            description: First line (1-indexed)
          type: number
        line_end:
          metadata:
            description: Last line (1-indexed, ≤10 lines)
          type: number
`

// ompReviewerVerdictContract asks the reviewer for the schema's verdict
// fields in its body. On omp 18.12.3 a subagent gets no yield tool in print
// mode and the task result falls back to the agent's text, so /review sees a
// verdict only if the prompt asks for one, as the bundled reviewer's does. The
// ask is unconditional because /review's parent writes its own acceptance
// format into the assignment, which an "unless told otherwise" ask yields to.
const ompReviewerVerdictContract = `
## OMP /review Verdict

Always end the review with this agent's verdict fields, after any result format the assignment asks for:

- ` + "`overall_correctness`" + `: ` + "`correct`" + ` (no bugs or blockers) or ` + "`incorrect`" + `.
- ` + "`explanation`" + `: a plain-text verdict summary of 1-3 sentences.
- ` + "`confidence`" + `: verdict confidence from 0.0 to 1.0.

When the ` + "`yield`" + ` tool is available, yield each finding under ` + "`type: [\"findings\"]`" + ` and each verdict field as its own section, then stop. Without it, write the three fields as the last lines of the report.
`

// OMPAgentDefinitionOptions selects what one generated .omp/agents definition
// declares beyond the agent source itself.
type OMPAgentDefinitionOptions struct {
	// ThinkingLevel is written as thinkingLevel. It stays empty for an agent
	// that task.agentModelOverrides routes, so the file never declares a
	// thinking level that competes with the override's.
	ThinkingLevel string
}

// RenderOMPAgentDefinition renders one canonical agent as an OMP agent
// definition. It never writes a model: an agent without a route inherits the
// parent session model, and a routed agent gets its model from the override.
func RenderOMPAgentDefinition(src AgentSource, opts OMPAgentDefinitionOptions) (string, error) {
	if opts.ThinkingLevel != "" && !config.IsOMPNativeThinkingLevel(opts.ThinkingLevel) {
		return "", fmt.Errorf("agent %s: unsupported OMP thinking level %q", src.Meta.Name, opts.ThinkingLevel)
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", OMPYAMLScalar(src.Meta.Name))
	if src.Meta.Description != "" {
		fmt.Fprintf(&sb, "description: %s\n", OMPYAMLScalar(src.Meta.Description))
	}
	if tools := ompAgentTools(src); len(tools) > 0 {
		sb.WriteString("tools:\n")
		for _, tool := range tools {
			fmt.Fprintf(&sb, "  - %s\n", OMPYAMLScalar(tool))
		}
	}
	if opts.ThinkingLevel != "" {
		fmt.Fprintf(&sb, "thinkingLevel: %s\n", OMPYAMLScalar(opts.ThinkingLevel))
	}
	if src.Meta.Name == ompReviewerAgent {
		output, err := ompIndentedYAMLBlock(ompReviewerOutputSchema)
		if err != nil {
			return "", err
		}
		sb.WriteString("output:\n")
		sb.WriteString(output)
	}
	sb.WriteString("---\n\n")
	sb.WriteString(NormalizeOMPResourcePaths(NormalizeAgentReferences(src.Body, "omp")))
	sb.WriteString("\n")
	if src.Meta.Name == ompReviewerAgent {
		sb.WriteString(ompReviewerVerdictContract)
	}
	return sb.String(), nil
}

// ompAgentTools starts from the canonical portable tools and adds only the
// native capabilities the role needs. MCP tools keep their exact names, except
// the REA server, which Autopus does not register for OMP.
func ompAgentTools(src AgentSource) []string {
	seen := make(map[string]bool)
	var tools []string
	add := func(tool string) {
		if tool != "" && !seen[tool] {
			seen[tool] = true
			tools = append(tools, tool)
		}
	}
	for _, tool := range strings.Split(src.Meta.Tools, ",") {
		tool = strings.TrimSpace(tool)
		if isReaMCPReference(tool) {
			continue
		}
		if strings.HasPrefix(tool, "mcp__") {
			add(tool)
			continue
		}
		add(ompAgentToolMap[tool])
	}
	for _, tool := range ompAgentNativeTools[src.Meta.Name] {
		add(tool)
	}
	sort.Strings(tools)
	return tools
}

// ompIndentedYAMLBlock checks that a schema is one YAML mapping and indents it
// under its frontmatter key.
func ompIndentedYAMLBlock(block string) (string, error) {
	var mapping map[string]any
	if err := yaml.Unmarshal([]byte(block), &mapping); err != nil || len(mapping) == 0 {
		return "", fmt.Errorf("OMP agent output schema must be a YAML mapping")
	}
	var sb strings.Builder
	for _, line := range strings.SplitAfter(block, "\n") {
		if strings.TrimSpace(line) != "" {
			sb.WriteString("  " + line)
		}
	}
	return sb.String(), nil
}
