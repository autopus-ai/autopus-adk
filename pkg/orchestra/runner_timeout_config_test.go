package orchestra

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProviderExecutionTimeout_PrefersExecutionTimeout(t *testing.T) {
	t.Parallel()

	provider := ProviderConfig{
		Name:             "gemini",
		ExecutionTimeout: 180 * time.Second,
	}

	assert.Equal(t, 180*time.Second, providerExecutionTimeout(provider, 120))
}

func TestProviderExecutionTimeout_FallsBackToCommandTimeout(t *testing.T) {
	t.Parallel()

	provider := ProviderConfig{Name: "gemini"}

	assert.Equal(t, 120*time.Second, providerExecutionTimeout(provider, 120))
}
