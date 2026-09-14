package omp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	ompNativeSmokeToken    = "AUTOPUS_OMP_NATIVE_TASK_HUB_SMOKE"
	ompNativeCredential    = "AUTOPUS_NATIVE_CREDENTIAL_MUST_NOT_LEAK"
	ompNativeAlphaID       = "NativeAlpha"
	ompNativeBetaID        = "NativeBeta"
	ompNativeSharedContext = "Read-only native lifecycle smoke. Do not modify files or use process or network tools. Return only the required five-field receipt through yield."
	ompNativeAlphaTask     = "NATIVE_CHILD_ALPHA: inspect `.omp/skills/agent-pipeline/SKILL.md` read-only and return its strict receipt naming exactly that path without modifying anything."
	ompNativeBetaTask      = "NATIVE_CHILD_BETA: inspect `.omp/rules/autopus-branding.md` read-only and return its strict receipt naming exactly that path without modifying anything."
)

type ompNativeChildReceipt struct {
	OwnedPaths       []string `json:"owned_paths"`
	ChangedFiles     []string `json:"changed_files"`
	Verification     []string `json:"verification"`
	Blockers         []string `json:"blockers"`
	NextRequiredStep string   `json:"next_required_step"`
}

func ompNativeReceiptSchema() map[string]any {
	stringArray := func() map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"owned_paths":        stringArray(),
			"changed_files":      stringArray(),
			"verification":       stringArray(),
			"blockers":           stringArray(),
			"next_required_step": map[string]any{"type": "string"},
		},
		"required": []string{
			"owned_paths", "changed_files", "verification", "blockers", "next_required_step",
		},
	}
}

func ompNativeTaskArguments() map[string]any {
	return map[string]any{
		"i":       "Spawning read-only lifecycle probes",
		"context": ompNativeSharedContext,
		"tasks": []any{
			map[string]any{
				"name": ompNativeAlphaID, "agent": "scout", "task": ompNativeAlphaTask,
				"outputSchema": ompNativeReceiptSchema(), "schemaMode": "strict",
			},
			map[string]any{
				"name": ompNativeBetaID, "agent": "reviewer", "task": ompNativeBetaTask,
				"outputSchema": ompNativeReceiptSchema(), "schemaMode": "strict",
			},
		},
	}
}

func ompNativeExpectedReceipt(id string) ompNativeChildReceipt {
	switch id {
	case ompNativeAlphaID:
		return ompNativeChildReceipt{
			OwnedPaths: []string{".omp/skills/agent-pipeline/SKILL.md"}, ChangedFiles: []string{},
			Verification: []string{"native-alpha-read-only"}, Blockers: []string{}, NextRequiredStep: "none",
		}
	case ompNativeBetaID:
		return ompNativeChildReceipt{
			OwnedPaths: []string{".omp/rules/autopus-branding.md"}, ChangedFiles: []string{},
			Verification: []string{"native-beta-read-only"}, Blockers: []string{}, NextRequiredStep: "none",
		}
	default:
		panic("unknown native child receipt " + id)
	}
}

func validateOMPNativeTaskArguments(args any) error {
	actual, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode task arguments: %w", err)
	}
	expectedArgs := ompNativeTaskArguments()
	delete(expectedArgs, "i")
	expected, err := json.Marshal(expectedArgs)
	if err != nil {
		return fmt.Errorf("encode expected task arguments: %w", err)
	}
	if string(actual) != string(expected) {
		return fmt.Errorf("task arguments drifted: %s", actual)
	}
	return nil
}

func validateOMPNativeParentTools(tools []json.RawMessage) error {
	parameters, ok := ompNativeToolParameters(tools, "task")
	if !ok {
		return fmt.Errorf("task tool missing")
	}
	properties := ompNativeMap(parameters["properties"])
	if !ompNativeExactKeys(properties, "context", "i", "tasks") ||
		!ompNativeStringSet(parameters["required"], "context", "i", "tasks") {
		encoded, _ := json.Marshal(properties)
		return fmt.Errorf("task batch shape drifted: properties=%s", encoded)
	}
	tasks := ompNativeMap(properties["tasks"])
	items := ompNativeMap(tasks["items"])
	itemProperties := ompNativeMap(items["properties"])
	if !ompNativeExactKeys(itemProperties, "agent", "name", "outputSchema", "schemaMode", "task") {
		return fmt.Errorf("task item shape drifted")
	}
	taskDefinition := ompNativeToolDefinition(tools, "task")
	if !strings.Contains(taskDefinition, "scout") || !strings.Contains(taskDefinition, "reviewer") {
		return fmt.Errorf("bundled agents missing from task definition")
	}
	for _, forbidden := range []string{"effort", "model", "thinking", "isolated"} {
		if _, exists := itemProperties[forbidden]; exists {
			return fmt.Errorf("unexpected child override field %s", forbidden)
		}
	}
	hub, ok := ompNativeToolParameters(tools, "hub")
	if !ok {
		return fmt.Errorf("hub tool missing")
	}
	hubOp, _ := json.Marshal(ompNativeMap(hub["properties"])["op"])
	for _, op := range []string{"list", "jobs", "wait"} {
		if !strings.Contains(string(hubOp), `"`+op+`"`) {
			return fmt.Errorf("hub op %s missing", op)
		}
	}
	return nil
}

// ompNativeChildToolContract is the toolset a spawned child must present, per
// bundled agent. Before the native cutover the project shipped its own
// `.omp/agents/<role>.md`, so every child carried one ADK-authored toolset
// (bash/glob/grep/read). Children are now OMP's bundled agents, whose
// capabilities differ by agent and are OMP's to declare: `scout` investigates
// without a shell, `reviewer` gets bash, lsp, and ast_grep. `hub` is injected
// by the runtime for background children and belongs to neither definition.
//
// Measured against omp/17.2.7 `omp agents unpack`.
var ompNativeChildToolContract = map[string]struct {
	required  []string
	forbidden []string
}{
	ompNativeAlphaID: {
		required:  []string{"glob", "grep", "hub", "read", "web_search", "yield"},
		forbidden: []string{"bash", "edit", "task", "write"},
	},
	ompNativeBetaID: {
		// `lsp` and `ast_grep` sit in the bundled definition but are runtime
		// gated (a language server, an installed ast-grep), so they are not
		// required here. A superset still passes; the write set never may.
		required:  []string{"bash", "glob", "grep", "hub", "read", "task", "web_search", "yield"},
		forbidden: []string{"edit", "write"},
	},
}

func validateOMPNativeChildTools(child string, tools []json.RawMessage) error {
	contract, known := ompNativeChildToolContract[child]
	if !known {
		return fmt.Errorf("no tool contract for child %s", child)
	}
	names := ompNativeToolNames(tools)
	for _, required := range contract.required {
		if !names[required] {
			return fmt.Errorf("%s child tool %s missing", child, required)
		}
	}
	for _, forbidden := range contract.forbidden {
		if names[forbidden] {
			return fmt.Errorf("%s child exposed %s", child, forbidden)
		}
	}
	yieldParameters, ok := ompNativeToolParameters(tools, "yield")
	if !ok {
		return fmt.Errorf("yield tool missing")
	}
	if !ompNativeContainsReceiptSchema(yieldParameters) {
		encoded, _ := json.Marshal(yieldParameters)
		if len(encoded) > 4000 {
			encoded = encoded[:4000]
		}
		return fmt.Errorf("strict five-field yield schema missing: %s", encoded)
	}
	return nil
}

func ompNativeToolNames(tools []json.RawMessage) map[string]bool {
	result := make(map[string]bool, len(tools))
	for _, raw := range tools {
		var tool struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if json.Unmarshal(raw, &tool) == nil && tool.Function.Name != "" {
			result[tool.Function.Name] = true
		}
	}
	return result
}
func ompNativeToolDefinition(tools []json.RawMessage, name string) string {
	for _, raw := range tools {
		var tool struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if json.Unmarshal(raw, &tool) == nil && tool.Function.Name == name {
			return string(raw)
		}
	}
	return ""
}

func ompNativeToolParameters(tools []json.RawMessage, name string) (map[string]any, bool) {
	for _, raw := range tools {
		var tool struct {
			Function struct {
				Name       string         `json:"name"`
				Parameters map[string]any `json:"parameters"`
			} `json:"function"`
		}
		if json.Unmarshal(raw, &tool) == nil && tool.Function.Name == name {
			return tool.Function.Parameters, true
		}
	}
	return nil, false
}

func ompNativeContainsReceiptSchema(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		properties := ompNativeMap(typed["properties"])
		if ompNativeExactKeys(properties,
			"blockers", "changed_files", "next_required_step", "owned_paths", "verification") &&
			ompNativeStringSet(typed["required"],
				"blockers", "changed_files", "next_required_step", "owned_paths", "verification") {
			return true
		}
		for _, child := range typed {
			if ompNativeContainsReceiptSchema(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if ompNativeContainsReceiptSchema(child) {
				return true
			}
		}
	}
	return false
}

func ompNativeMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func ompNativeExactKeys(values map[string]any, expected ...string) bool {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(expected)
	return strings.Join(keys, "\x00") == strings.Join(expected, "\x00")
}

func ompNativeStringSet(value any, expected ...string) bool {
	items, ok := value.([]any)
	if !ok || len(items) != len(expected) {
		return false
	}
	actual := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return false
		}
		actual = append(actual, text)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	return strings.Join(actual, "\x00") == strings.Join(expected, "\x00")
}
