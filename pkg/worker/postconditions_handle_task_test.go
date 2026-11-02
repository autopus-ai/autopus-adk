package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/worker/a2a"
	"github.com/insajin/autopus-adk/pkg/worker/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const resultLine = `echo '{"type":"result","output":"done","cost_usd":0.01,"duration_ms":5}'`

func handleTaskFixture(t *testing.T, script string) (*gitFixture, *mockAdapter, *WorkerLoop) {
	t.Helper()
	f := newGitFixture(t)
	mock := &mockAdapter{name: "mock", script: "head -c0; " + script}
	wl := &WorkerLoop{config: LoopConfig{Provider: mock, WorkDir: f.repo}, postconditionGit: f.git}
	return f, mock, wl
}

// commitScript makes the mock provider commit inside the fixture repository;
// the mock itself runs in the test's working directory.
func commitScript(f *gitFixture) string {
	return fmt.Sprintf(`cd %q && GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_AUTHOR_NAME=T GIT_AUTHOR_EMAIL=t@t.invalid `+
		`GIT_COMMITTER_NAME=T GIT_COMMITTER_EMAIL=t@t.invalid git commit -q --allow-empty -m work && %s`, f.repo, resultLine)
}

func postconditionArtifact(t *testing.T, result *a2a.TaskResult) postconditionReport {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Name == "postconditions.json" {
			return decodeReport(t, artifact.Data)
		}
	}
	require.Fail(t, "no postconditions.json artifact")
	return postconditionReport{}
}

// S1 (SPEC-WORKERPOST-001) through the task handler: on 109c7ea4 the legacy
// payload ended failed with "postcondition failed: commit: HEAD did not
// advance after execution".
func TestHandleTask_ReadOnlyTaskCompletesWithoutGitChanges(t *testing.T) {
	payloads := map[string]string{
		"legacy":   `{"prompt":"Review only; do not commit or push"}`,
		"contract": `{"prompt":"Review only","completion_contract":{"forbidden_actions":["commit","push"]}}`,
	}
	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			f, mock, wl := handleTaskFixture(t, resultLine)
			repoRefs, remoteRefs := f.refs(f.repo), f.refs(f.bare)

			result, err := wl.handleTask(context.Background(), "task-readonly-"+name, []byte(payload))
			require.NoError(t, err)
			assert.Equal(t, a2a.StatusCompleted, result.Status)
			require.Len(t, mock.calls, 1)
			report := postconditionArtifact(t, result)
			assert.Contains(t, []string{statusNotRequired, statusSatisfied}, report.Outcome)
			assert.Equal(t, repoRefs, f.refs(f.repo))
			assert.Equal(t, remoteRefs, f.refs(f.bare))
		})
	}
}

// S9 through the task handler: an invalid contract, null included, fails
// before any provider call, and no credential reaches the result or an event.
func TestHandleTask_InvalidContractFailsBeforeProvider(t *testing.T) {
	contracts := []string{
		`{"version":2}`, `{"required_actions":["deploy"]}`,
		`{"required_actions":["push"],"forbidden_actions":["push"]}`, `{"ref":"refs/tags/v1"}`,
		`{"ref":"feature/../x"}`, `{"remote":"-oProxyCommand=x"}`, `{"expected_commit":"abc123"}`, `{"push_match":"prefix"}`,
		`null`, `[]`, `{"version":"1"}`, `{"remote":"https://user:secret@host/repo.git"}`,
	}
	for i, contract := range contracts {
		_, mock, wl := handleTaskFixture(t, resultLine)
		var events []HostEvent
		wl.AddHostObserver(HostObserverFunc(func(event HostEvent) { events = append(events, event) }))
		payload := `{"prompt":"do the work","completion_contract":` + contract + `}`

		result, err := wl.handleTask(context.Background(), fmt.Sprintf("task-invalid-contract-%d", i), []byte(payload))
		require.Error(t, err, contract)
		assert.True(t, strings.HasPrefix(err.Error(), "invalid completion_contract: "), err.Error())
		assert.Equal(t, a2a.StatusFailed, result.Status)
		assert.Empty(t, mock.calls, "the provider must not run for %s", contract)
		assert.NotContains(t, result.Error, "secret")
		for _, event := range events {
			assert.NotContains(t, event.Message, "secret")
		}
	}
}

func TestHandleTask_ContractDecidesCompletion(t *testing.T) {
	contract := `{"prompt":"update the docs","completion_contract":{"required_actions":["commit"]}}`

	_, _, idle := handleTaskFixture(t, resultLine)
	result, err := idle.handleTask(context.Background(), "task-contract-no-commit", []byte(contract))
	require.ErrorIs(t, err, errPostconditionFailed)
	assert.Contains(t, err.Error(), "commit: HEAD did not advance after execution")
	assert.Equal(t, a2a.StatusFailed, result.Status)

	f, mock, wl := handleTaskFixture(t, "")
	mock.script = "head -c0; " + commitScript(f)
	result, err = wl.handleTask(context.Background(), "task-contract-commit", []byte(contract))
	require.NoError(t, err)
	assert.Equal(t, a2a.StatusCompleted, result.Status)
	assert.Equal(t, statusSatisfied, postconditionArtifact(t, result).Outcome)
}

// The pipeline path judges the same plan: the prompt alone would require
// nothing, so only the contract can fail this task.
func TestHandleTask_PipelinePathUsesTheContract(t *testing.T) {
	_, mock, wl := handleTaskFixture(t, resultLine)
	payload, err := json.Marshal(taskPayloadMessage{
		Prompt:             "backend-built prompt",
		PipelinePhases:     []string{"planner"},
		CompletionContract: json.RawMessage(`{"required_actions":["commit"]}`),
	})
	require.NoError(t, err)

	result, err := wl.handleTask(context.Background(), "task-pipeline-contract", payload)
	require.ErrorIs(t, err, errPostconditionFailed)
	assert.Equal(t, a2a.StatusFailed, result.Status)
	require.Len(t, mock.calls, 1)
}

// The baseline is read after the queue slot and the worktree. On 109c7ea4 it
// was read first, so a commit made in the main checkout while the task queued
// passed as the task's own commit.
func TestExecuteWithParallel_BaselineFollowsTheQueuedWorktree(t *testing.T) {
	f := newGitFixture(t)
	mock := &mockAdapter{name: "mock", script: "head -c0; " + resultLine}
	wl := NewWorkerLoop(LoopConfig{Provider: mock, WorkDir: f.repo, MaxConcurrency: 1, WorktreeIsolation: true})
	wl.configureExecutionConcurrency()
	wl.postconditionGit = f.git
	require.NoError(t, wl.semaphore.Acquire(context.Background())) // hold the only slot

	done := make(chan error, 1)
	go func() {
		_, err := wl.executeWithParallel(context.Background(), adapter.TaskConfig{
			TaskID: "queued-baseline", Prompt: "commit your work", WorkDir: f.repo,
		}, nil, newTaskRunMeta("queued-baseline", ""))
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	f.commit("A1") // the main checkout moves while the task waits
	wl.semaphore.Release()

	err := <-done
	require.ErrorIs(t, err, errPostconditionFailed)
	assert.Contains(t, err.Error(), "commit: HEAD did not advance after execution")
}
