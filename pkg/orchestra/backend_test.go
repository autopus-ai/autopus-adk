package orchestra

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubprocessBackend_Execute_WithEcho(t *testing.T) {
	t.Parallel()
	backend := NewSubprocessBackendImpl()
	req := ProviderRequest{
		Provider: "test",
		Prompt:   "hello subprocess",
		Config:   echoProvider("test"),
	}
	resp, err := backend.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "test", resp.Provider)
	assert.Contains(t, resp.Output, "hello subprocess")
}

func TestSubprocessBackend_Name(t *testing.T) {
	t.Parallel()
	backend := NewSubprocessBackendImpl()
	assert.Equal(t, "subprocess", backend.Name())
}

func TestSubprocessBackend_Execute_NoBinary(t *testing.T) {
	t.Parallel()
	backend := NewSubprocessBackendImpl()
	req := ProviderRequest{
		Provider: "bad",
		Config:   ProviderConfig{Name: "bad"},
	}
	_, err := backend.Execute(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no binary configured")
}

func TestSubprocessBackend_Execute_MissingBinary(t *testing.T) {
	t.Parallel()
	backend := NewSubprocessBackendImpl()
	req := ProviderRequest{
		Provider: "missing",
		Config:   ProviderConfig{Name: "missing", Binary: "binary_that_does_not_exist_xyz"},
	}
	_, err := backend.Execute(context.Background(), req)
	require.Error(t, err)
}

func TestNewSubprocessBackendImpl_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, NewSubprocessBackendImpl())
}

func TestValidateJSONOutput_Valid(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateJSONOutput(`{"key": "value"}`))
}

func TestValidateJSONOutput_ValidWithSurroundingText(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateJSONOutput(`some preamble {"key": "value"} trailing`))
}

func TestValidateJSONOutput_Empty(t *testing.T) {
	t.Parallel()
	assert.Error(t, validateJSONOutput(""))
}

func TestValidateJSONOutput_NoJSON(t *testing.T) {
	t.Parallel()
	assert.Error(t, validateJSONOutput("just plain text"))
}

func TestValidateJSONOutput_MalformedJSON(t *testing.T) {
	t.Parallel()
	assert.Error(t, validateJSONOutput(`{"broken`))
}

func TestSelectBackend_ReturnsSubprocess(t *testing.T) {
	t.Parallel()
	backend := SelectBackend(OrchestraConfig{})
	require.NotNil(t, backend)
	assert.Equal(t, "subprocess", backend.Name())
}
