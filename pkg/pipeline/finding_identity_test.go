package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// S15: a fix moves line numbers, so a source location's line and column are
// masked; every other `:digits` is part of what the finding says.
func TestNormalizeFindingText_MasksOnlySourceLocations(t *testing.T) {
	t.Parallel()

	same := [][2]string{
		{"handler drops ctx (engine.go:42)", "Handler drops ctx (engine.go:57)"},
		{"see pkg/pipeline/engine_run.go:115:7", "see pkg/pipeline/engine_run.go:120:3"},
		{"target in src/Makefile:3", "target in src/Makefile:9"},
		{`path c:\repo\main.go:12 fails`, `path c:\repo\main.go:13 fails`},
		{"two places: a.ts:1 and b.py:2", "two places: a.ts:10 and b.py:20"},
	}
	for _, pair := range same {
		assert.Equal(t, normalizeFindingText(pair[0]), normalizeFindingText(pair[1]), "%q", pair[0])
	}
	distinct := [][2]string{
		{"client dials localhost:5432 without tls", "client dials localhost:6379 without tls"},
		{"exits with status:1", "exits with status:2"},
		{"cron fires at 10:30", "cron fires at 10:45"},
		{"calls api.example.com:443", "calls api.example.com:8443"},
		{"opens http://localhost:5432/db", "opens http://localhost:6379/db"},
		{"binds 127.0.0.1:8080", "binds 127.0.0.1:9090"},
		{"uses postgres://user@db:5432", "uses postgres://user@db:6543"},
	}
	for _, pair := range distinct {
		assert.NotEqual(t, normalizeFindingText(pair[0]), normalizeFindingText(pair[1]), "%q", pair[0])
	}
	assert.Equal(t, "handler drops ctx (engine.go:#)", normalizeFindingText("Handler  drops ctx (engine.go:42)"))
}
