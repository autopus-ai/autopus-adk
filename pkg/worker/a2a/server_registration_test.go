package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/worker/security"
)

func TestPolicyDir_CreatesSecureDir(t *testing.T) {
	t.Parallel()

	dir, err := policyDir()
	require.NoError(t, err)

	assert.Equal(t, security.PolicyCacheDir(), dir)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o700), mode, "the policy dir is private to the user")
}

func TestCacheSecurityPolicy_AtomicWrite(t *testing.T) {
	t.Parallel()

	taskID := "test-atomic-write"
	policy := SecurityPolicy{AllowNetwork: false, AllowFS: true, TimeoutSec: 30}

	// Write initial policy.
	require.NoError(t, cacheSecurityPolicy(taskID, policy, ""))

	dir, _ := policyDir()
	target := filepath.Join(dir, fmt.Sprintf("autopus-policy-%s.json", taskID))
	defer func() { _ = os.Remove(target) }()

	// Overwrite with a different policy — should be atomic (no partial reads).
	policy2 := SecurityPolicy{AllowNetwork: true, AllowFS: false, TimeoutSec: 60}
	require.NoError(t, cacheSecurityPolicy(taskID, policy2, ""))

	data, err := os.ReadFile(target)
	require.NoError(t, err)

	var loaded SecurityPolicy
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, policy2, loaded)
}

func TestCacheSecurityPolicy_WritesFile(t *testing.T) {
	t.Parallel()

	policy := SecurityPolicy{
		AllowNetwork: true,
		AllowFS:      false,
		AllowedPaths: []string{"/tmp"},
		TimeoutSec:   120,
	}
	taskID := "test-policy-cache"

	require.NoError(t, cacheSecurityPolicy(taskID, policy, ""))

	path := filepath.Join(security.PolicyCacheDir(), fmt.Sprintf("autopus-policy-%s.json", taskID))
	defer func() { _ = os.Remove(path) }()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	var loaded SecurityPolicy
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, policy, loaded)
}

func TestRegisterAgentCard_CorrectJSONRPC(t *testing.T) {
	mb := newMockBackend()
	defer mb.close()

	srv := NewServer(ServerConfig{
		BackendURL: mb.wsURL(),
		WorkerName: "card-test",
		Skills:     []string{"skill-a", "skill-b"},
		Handler: func(_ context.Context, _ string, _ json.RawMessage) (*TaskResult, error) {
			return &TaskResult{}, nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		require.NoError(t, srv.Close())
	}()

	srv.config.BackendURL = mb.wsURL()
	require.NoError(t, srv.Start(ctx))

	msgs := mb.waitForMessages(t, 1, 3*time.Second)

	var req JSONRPCRequest
	require.NoError(t, json.Unmarshal(msgs[0], &req))
	assert.Equal(t, "2.0", req.JSONRPC)
	assert.Equal(t, MethodRegisterCard, req.Method)

	var card AgentCard
	require.NoError(t, json.Unmarshal(req.Params, &card))
	assert.Equal(t, "card-test", card.Name)
	assert.Equal(t, []string{"skill-a", "skill-b"}, card.Skills)
	assert.Equal(t, []string{"build_change"}, card.ExecutionLanes)
	assert.Equal(t, DefaultCapabilities(), card.Capabilities)
	assert.Equal(t, []string{"text"}, card.SupportedInputModes)
}
