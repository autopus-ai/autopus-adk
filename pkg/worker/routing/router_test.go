package routing

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoute(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Enabled = true

	tests := []struct {
		name     string
		provider string
		message  string
		want     string
	}{
		{
			name:     "S5: claude simple message returns sonnet",
			provider: "claude",
			message:  "현재 상태 확인",
			want:     "claude-sonnet-5-5",
		},
		{
			name:     "claude complex message returns fable",
			provider: "claude",
			message:  strings.Repeat("x", 1200) + " 리팩토링 아키텍처",
			want:     "claude-fable-5-1",
		},
		{
			name:     "claude medium message returns opus",
			provider: "claude",
			message:  strings.Repeat("a", 500) + " 수정 변경",
			want:     "claude-opus-5-5",
		},
		{
			name:     "unknown provider returns empty",
			provider: "unknown",
			message:  "hello",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRouter(cfg)
			got := r.Route(tt.provider, tt.message)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRouteDisabled(t *testing.T) {
	t.Parallel()

	// S7: Enabled=false → returns "" (passthrough).
	cfg := DefaultConfig()
	cfg.Enabled = false

	r := NewRouter(cfg)
	got := r.Route("claude", "리팩토링 아키텍처 분석 설계")
	assert.Equal(t, "", got)
}

func TestRouteCustomMapping(t *testing.T) {
	t.Parallel()

	// S6: custom mapping override.
	cfg := RoutingConfig{
		Enabled: true,
		Thresholds: ClassifierThresholds{
			SimpleMaxChars:  200,
			ComplexMinChars: 1000,
		},
		Models: map[string]ProviderModels{
			"custom": {Simple: "my-small", Medium: "my-medium", Complex: "my-large"},
		},
	}

	r := NewRouter(cfg)
	got := r.Route("custom", "확인")
	assert.Equal(t, "my-small", got)
}

func TestRouteLogging(t *testing.T) {
	// S8: Enabled=true, complex message → logs contain complexity and model info.
	// Capture log output.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	cfg := DefaultConfig()
	cfg.Enabled = true

	r := NewRouter(cfg)
	r.Route("claude", strings.Repeat("x", 1200)+" 리팩토링 아키텍처")

	logOutput := buf.String()
	assert.Contains(t, logOutput, "[routing]")
	assert.Contains(t, logOutput, "provider=claude")
	assert.Contains(t, logOutput, "complexity=complex")
}
