package content

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOMPDoesNotInventAdditionalDispatch(t *testing.T) {
	source := "```python\nAgent(subagent_type=\"reviewer\", prompt=\"Review this change\")\n```"
	body := NormalizeOMPSemanticReferences(source)
	blocks := regexp.MustCompile("(?s)```json\\n(.*?)\\n```").FindAllStringSubmatch(body, -1)
	dispatches := 0
	for _, block := range blocks {
		var invocation struct {
			Tasks []json.RawMessage `json:"tasks"`
		}
		require.NoError(t, json.Unmarshal([]byte(block[1]), &invocation))
		dispatches += len(invocation.Tasks)
	}
	require.Equal(t, 1, dispatches, "normalizing one invocation must not add a second illustrative worker task")
	require.Equal(t, body, NormalizeOMPSemanticReferences(body), "re-rendering must not accumulate instructions")
}

func TestOMPTranslationPreservesOtherProductFacts(t *testing.T) {
	source := "Claude Code 2.1.263, Codex 0.153.4, OpenCode 1.18.7, Gemini CLI 0.52.0, and OMP 18.1.19 have different native capabilities."
	require.Equal(t, source, ReplacePlatformReferences(source, "omp"))
}
