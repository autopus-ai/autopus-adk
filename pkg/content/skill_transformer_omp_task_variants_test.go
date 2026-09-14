package content_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/content"
)

// The renderer has two legitimate output forms for a legacy dispatch: a fenced
// or bare call becomes a native `task` batch payload, and an inline code span
// becomes a native prose reference. Both must produce a valid native
// invocation, and neither may invent the conditional `isolated`/`effort`
// fields that only a live schema can authorize.
func TestOMPStaticTaskRenderer_UsesIntentBatchCoreAcrossDynamicVariants(t *testing.T) {
	for _, test := range []struct {
		name, source string
		inlineProse  bool
	}{
		{name: "inline code span", source: "`Agent(subagent_type=\"executor\", prompt=\"Implement\")`", inlineProse: true},
		{name: "batch on", source: "```text\nAgent(subagent_type=\"executor\", prompt=\"Implement\")\nAgent(subagent_type=\"reviewer\", prompt=\"Review\")\n```"},
		{name: "isolation none", source: "Agent(subagent_type=\"executor\", prompt=\"Implement\")"},
		{name: "isolation on", source: "Agent(subagent_type=\"executor\", prompt=\"Implement\", isolated=true)"},
		{name: "effort off", source: "Agent(subagent_type=\"executor\", prompt=\"Implement\")"},
		{name: "effort on", source: "Agent(subagent_type=\"executor\", prompt=\"Implement\", effort=\"hi\")"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rendered := content.ReplacePlatformReferences(test.source, "omp")
			for _, token := range ompLegacyCoordinationTokens {
				assert.NotContains(t, rendered, token)
			}
			assert.NotContains(t, rendered, `"isolated"`)
			assert.NotContains(t, rendered, `"effort"`)

			if test.inlineProse {
				assert.Contains(t, rendered, "`task` batch",
					"an inline dispatch must still name the native task batch")
				assert.NotContains(t, rendered, "```json",
					"an inline code span must not expand into a full payload example")
				return
			}

			payload := firstOMPJSONExample(t, rendered)
			assert.Equal(t, []string{"context", "i", "tasks"}, sortedOMPJSONKeys(payload))
			assert.NotEmpty(t, payload["i"])
			encoded, err := json.Marshal(payload)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), `"isolated"`)
			assert.NotContains(t, string(encoded), `"effort"`)
		})
	}
}

func TestOMPCoordinationExamples_IncludeIntentForTaskHubAndTodo(t *testing.T) {
	rendered := content.ReplacePlatformReferences(
		`TeamCreate(name="delivery") TaskCreate(subject="Implement") SendMessage(recipient="executor", content="Review")`,
		"omp",
	)
	assert.Contains(t, rendered, `"i": "Dispatching bounded OMP work"`)
	assert.Contains(t, rendered, `{"i":"Updating parent-owned progress","op":"append"`)
	assert.Contains(t, rendered, `{"i":"Following up with an existing worker","op":"send"`)
}

func firstOMPJSONExample(t *testing.T, body string) map[string]any {
	t.Helper()
	const fence = "```json\n"
	start := strings.Index(body, fence)
	require.NotEqual(t, -1, start)
	remaining := body[start+len(fence):]
	end := strings.Index(remaining, "\n```")
	require.NotEqual(t, -1, end)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(remaining[:end]), &payload))
	return payload
}

func sortedOMPJSONKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
