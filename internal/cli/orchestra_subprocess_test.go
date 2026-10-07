package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestResolveSubprocessRounds(t *testing.T) {
	t.Parallel()

	t.Run("default 1", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{}
		assert.Equal(t, 1, resolveSubprocessRounds(conf))
	})

	t.Run("configured value", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{
			Subprocess: config.SubprocessConf{Rounds: 3},
		}
		assert.Equal(t, 3, resolveSubprocessRounds(conf))
	})
}

func TestResolveMaxConcurrent(t *testing.T) {
	t.Parallel()

	t.Run("default 3", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{}
		assert.Equal(t, 3, resolveMaxConcurrent(conf))
	})

	t.Run("configured value", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{
			Subprocess: config.SubprocessConf{MaxConcurrent: 5},
		}
		assert.Equal(t, 5, resolveMaxConcurrent(conf))
	})
}

func TestResolveSubprocessTimeout(t *testing.T) {
	t.Parallel()

	t.Run("default 120s", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{}
		entry := config.ProviderEntry{}
		assert.Equal(t, 120*time.Second, resolveSubprocessTimeout(conf, entry))
	})

	t.Run("global timeout", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{TimeoutSeconds: 60}
		entry := config.ProviderEntry{}
		assert.Equal(t, 60*time.Second, resolveSubprocessTimeout(conf, entry))
	})

	t.Run("per-provider overrides global", func(t *testing.T) {
		t.Parallel()
		conf := &config.OrchestraConf{TimeoutSeconds: 60}
		entry := config.ProviderEntry{
			Subprocess: config.SubprocessProvConf{Timeout: 300},
		}
		assert.Equal(t, 300*time.Second, resolveSubprocessTimeout(conf, entry))
	})
}

func TestResolveProviders_SubprocessFields(t *testing.T) {
	t.Parallel()

	conf := &config.OrchestraConf{
		Providers: map[string]config.ProviderEntry{
			"claude": {
				Binary: "claude",
				Args:   []string{"--print"},
				Subprocess: config.SubprocessProvConf{
					SchemaFlag:   "--schema",
					StdinMode:    "pipe",
					OutputFormat: "json",
				},
			},
		},
	}

	providers := resolveProviders(conf, "", nil)
	require.Len(t, providers, 1)
	assert.Equal(t, "--schema", providers[0].SchemaFlag)
	assert.Equal(t, "pipe", providers[0].StdinMode)
	assert.Equal(t, "json", providers[0].OutputFormat)
}

func TestSubprocessConf_YAMLParsing(t *testing.T) {
	t.Parallel()

	content := `
mode: full
project_name: test
platforms:
  - claude-code
orchestra:
  enabled: true
  default_strategy: debate
  timeout_seconds: 120
  subprocess:
    enabled: true
    max_concurrent: 4
    work_dir: /tmp/orchestra
    rounds: 2
  providers:
    claude:
      binary: claude
      args: [--print]
      subprocess:
        schema_flag: "--response-schema"
        stdin_mode: pipe
        output_format: json
        timeout: 180
`
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	// subprocess.enabled is a retired key (SPEC-PANERM-001): it still loads and
	// is ignored, and its siblings keep their values.
	assert.Equal(t, 4, cfg.Orchestra.Subprocess.MaxConcurrent)
	assert.Equal(t, "/tmp/orchestra", cfg.Orchestra.Subprocess.WorkDir)
	assert.Equal(t, 2, cfg.Orchestra.Subprocess.Rounds)

	claude := cfg.Orchestra.Providers["claude"]
	assert.Equal(t, "--response-schema", claude.Subprocess.SchemaFlag)
	assert.Equal(t, "pipe", claude.Subprocess.StdinMode)
	assert.Equal(t, "json", claude.Subprocess.OutputFormat)
	assert.Equal(t, 180, claude.Subprocess.Timeout)
}
