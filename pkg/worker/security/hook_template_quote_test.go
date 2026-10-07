package security

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L4: the policy path is one POSIX single-quoted word whatever it holds, so a
// quote in it cannot end the quoting and splice shell syntax into the hook.
func TestWorkerHookEntry_PolicyPathIsOnePOSIXWord(t *testing.T) {
	t.Parallel()

	policy := "/tmp/it's here/$(touch pwned)'; echo x '/policy.json"
	handlers, _ := workerHookEntry(policy)["hooks"].([]any)
	require.Len(t, handlers, 1)
	command, _ := handlers[0].(map[string]any)["command"].(string)
	assert.Equal(t, `auto worker validate --policy '/tmp/it'\''s here/$(touch pwned)'\''; echo x '\''/policy.json'`+
		` --command "$TOOL_INPUT"`, command)
	assert.True(t, strings.HasPrefix(command, workerHookCommandPrefix), "the handler keeps its ownership prefix")

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on PATH")
	}
	args := strings.TrimPrefix(strings.TrimSuffix(command, ` --command "$TOOL_INPUT"`), workerHookCommandPrefix)
	cmd := exec.Command(sh, "-c", `set -- `+args+`; printf '%s\n' "$#" "$2"`)
	cmd.Dir = t.TempDir() // a broken quoting must not run anything in the package
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "2\n"+policy+"\n", string(out), "the shell reads --policy and exactly the original path")
}
