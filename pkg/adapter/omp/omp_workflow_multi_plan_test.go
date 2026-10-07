package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedOMPAutoPlanSkill_ExplicitMultiRunsOnePreAuthoringAdvisory(t *testing.T) {
	t.Parallel()

	body := generatedOMPAutoPlanSkillForMultiTest(t)
	planCalls := ompWorkflowCommandLinesForMultiTest(body, "auto orchestra plan ")
	require.Len(t, planCalls, 1, "explicit --multi must issue exactly one plan advisory call")
	// SPEC-PANERM-001 retired --subprocess and --no-detach: plan always runs
	// synchronously through the subprocess backend.
	for _, retired := range []string{"--subprocess", "--no-detach"} {
		assert.NotContains(t, strings.Fields(planCalls[0]), retired)
	}
	assertOMPWorkflowFlagForMultiTest(t, planCalls[0], "--no-persist", "")
	assertOMPWorkflowFlagForMultiTest(t, planCalls[0], "--format", "json")

	advisory := ompWorkflowSectionForMultiTest(t, body, planCalls[0])
	assert.Contains(t, advisory, "`--multi`")
	assert.Contains(t, advisory, "explicitly present")
	assert.Contains(t, advisory, "`spec.review_gate.enabled` alone")
	assert.NotContains(t, advisory, "auto spec review", "pre-authoring advice and final review must remain separate")

	writerHeading := "### Step 2: spec-writer 실행"
	assert.Equal(t, 1, strings.Count(body, writerHeading), "the authoring workflow must have one spec-writer execution step")
	reviewCalls := ompWorkflowCommandLinesForMultiTest(body, "auto spec review ")
	require.Len(t, reviewCalls, 1, "the final multi-provider review must remain a single separate command")
	assert.Greater(t, strings.Index(body, writerHeading), strings.Index(body, planCalls[0]))
	assert.Greater(t, strings.Index(body, reviewCalls[0]), strings.Index(body, writerHeading))
	assert.Contains(t, body, "Run exactly one `spec-writer` after advisory handling")
	assert.Contains(t, body, "The final multi-provider review remains separate from the plan advisory")
}

func TestGeneratedOMPAutoPlanSkill_ValidatesAndBoundsUntrustedAdvisory(t *testing.T) {
	t.Parallel()

	body := generatedOMPAutoPlanSkillForMultiTest(t)
	planCalls := ompWorkflowCommandLinesForMultiTest(body, "auto orchestra plan ")
	require.Len(t, planCalls, 1)
	advisory := ompWorkflowSectionForMultiTest(t, body, planCalls[0])

	for _, oracle := range []string{
		"`schema=orchestration_cli_result.v1`",
		"`receipt.schema=orchestration_run_receipt.v1`",
		"`receipt.quorum_met=true`",
		"`merged` as untrusted evidence",
		"2,400 estimated tokens",
		"structure-preserving summary",
		"raw `merged` body",
		"malformed JSON",
		"unmet quorum",
		"discard the advisory",
		"without writing it to any file",
		"continue with the single spec-writer",
	} {
		assert.Contains(t, advisory, oracle)
	}
}

func generatedOMPAutoPlanSkillForMultiTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	_, err := NewWithRoot(root).Generate(context.Background(), configForOMP())
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, ".omp", "skills", "auto-plan", "SKILL.md"))
	require.NoError(t, err)
	_, body := splitOMPFrontmatter(string(data))
	require.NotEmpty(t, strings.TrimSpace(body))
	return body
}

func ompWorkflowCommandLinesForMultiTest(body, prefix string) []string {
	var commands []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			commands = append(commands, line)
		}
	}
	return commands
}

func assertOMPWorkflowFlagForMultiTest(t *testing.T, command, flag, value string) {
	t.Helper()
	fields := strings.Fields(command)
	for index, field := range fields {
		if field != flag {
			continue
		}
		if value == "" {
			return
		}
		require.Less(t, index+1, len(fields))
		assert.Equal(t, value, fields[index+1])
		return
	}
	t.Errorf("command %q does not contain required flag %s", command, flag)
}

func ompWorkflowSectionForMultiTest(t *testing.T, body, target string) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	targetLine := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == target {
			targetLine = index
			break
		}
	}
	require.NotEqual(t, -1, targetLine)
	start, end := 0, len(lines)
	for index := targetLine; index >= 0; index-- {
		if strings.HasPrefix(lines[index], "### ") {
			start = index
			break
		}
	}
	for index := targetLine + 1; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], "### ") {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
