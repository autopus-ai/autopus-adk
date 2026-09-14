package content

import (
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompNativeDefaultAgent is the bundled OMP agent a task runs on when the
// dispatch omits the agent field.
const ompNativeDefaultAgent = "task"

func parseOMPLegacyCalls(body string) []ompLegacyDispatch {
	calls := ompLegacyCallRe.FindAllStringSubmatch(body, -1)
	dispatches := make([]ompLegacyDispatch, 0, len(calls)+1)
	for _, call := range calls {
		if len(call) < 2 {
			continue
		}
		dispatches = append(dispatches, ompLegacyDispatch{
			agent:    firstOMPFieldValue(ompLegacyRoleRe.FindStringSubmatch(call[1])),
			task:     firstOMPFieldValue(ompLegacyTaskRe.FindStringSubmatch(call[1])),
			isolated: ompIsolationRe.MatchString(call[1]),
		})
	}

	spawns := ompSpawnStartRe.FindAllStringSubmatchIndex(body, -1)
	for i, match := range spawns {
		end := len(body)
		if i+1 < len(spawns) {
			end = spawns[i+1][0]
		}
		segment := body[match[0]:end]
		agent := ""
		if match[2] >= 0 && match[3] >= 0 {
			agent = body[match[2]:match[3]]
		}
		dispatches = append(dispatches, ompLegacyDispatch{
			agent:    agent,
			task:     firstOMPFieldValue(ompSpawnTaskRe.FindStringSubmatch(segment)),
			isolated: ompIsolationRe.MatchString(segment),
		})
	}

	if len(dispatches) == 0 {
		dispatches = append(dispatches, ompLegacyDispatch{})
	}
	return dispatches
}

func firstOMPFieldValue(match []string) string {
	if len(match) < 2 {
		return ""
	}
	for _, value := range match[1:] {
		if value != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func renderOMPTaskBatch(dispatches []ompLegacyDispatch) string {
	var b strings.Builder
	b.WriteString("```json\n{\n")
	b.WriteString("  \"i\": \"Dispatching bounded OMP work\",\n")
	b.WriteString("  \"context\": \"Shared goal, constraints, owned-path boundaries, and cross-task contracts.\",\n")
	b.WriteString("  \"tasks\": [\n")
	for i, dispatch := range dispatches {
		role := strings.TrimSpace(dispatch.agent)
		agent := ompNativeDispatchAgent(role)
		name := role
		if name == "" {
			name = "Worker"
		}
		taskText := strings.TrimSpace(dispatch.task)
		if taskText == "" {
			taskText = "Complete the assigned work and return the required receipt."
		}
		if len(dispatches) > 1 {
			name += "-" + strconv.Itoa(i+1)
		}
		b.WriteString("    {\n")
		b.WriteString("      \"name\": " + strconv.Quote(name) + ",\n")
		if agent != "" {
			b.WriteString("      \"agent\": " + strconv.Quote(agent) + ",\n")
		}
		b.WriteString("      \"task\": " + strconv.Quote(taskText) + ",\n")
		writeOMPReceiptSchema(&b, "      ")
		b.WriteString(",\n      \"schemaMode\": \"strict\"")
		b.WriteString("\n")
		b.WriteString("    }")
		if i+1 < len(dispatches) {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("  ]\n}\n```")
	return b.String()
}

// ompNativeDispatchAgent resolves a legacy dispatch role to the OMP agent that
// actually exists. OMP registers five agents, so the retired per-role names
// collapse onto them; `task` is the bundled default and stays implicit. An
// author-declared name outside the role catalog is left alone because it may
// be a genuine project agent, and an unusable token is dropped rather than
// emitted as a call to an agent nothing registers.
func ompNativeDispatchAgent(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return ""
	}
	if native, err := config.OMPNativeAgentForRole(role); err == nil {
		if native == ompNativeDefaultAgent {
			return ""
		}
		return native
	}
	if isOMPSafeIdentifier(role) {
		return role
	}
	return ""
}

func writeOMPReceiptSchema(b *strings.Builder, indent string) {
	b.WriteString(indent + "\"outputSchema\": {\n")
	b.WriteString(indent + "  \"type\": \"object\",\n")
	b.WriteString(indent + "  \"additionalProperties\": false,\n")
	b.WriteString(indent + "  \"required\": [\"owned_paths\", \"changed_files\", \"verification\", \"blockers\", \"next_required_step\"],\n")
	b.WriteString(indent + "  \"properties\": {\n")
	b.WriteString(indent + "    \"owned_paths\": {\"type\": \"array\", \"items\": {\"type\": \"string\"}},\n")
	b.WriteString(indent + "    \"changed_files\": {\"type\": \"array\", \"items\": {\"type\": \"string\"}},\n")
	b.WriteString(indent + "    \"verification\": {\"type\": \"array\", \"items\": {\"type\": \"string\"}},\n")
	b.WriteString(indent + "    \"blockers\": {\"type\": \"array\", \"items\": {\"type\": \"string\"}},\n")
	b.WriteString(indent + "    \"next_required_step\": {\"type\": \"string\"}\n")
	b.WriteString(indent + "  }\n")
	b.WriteString(indent + "}")
}
