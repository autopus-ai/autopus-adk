package adapter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// generator is the minimal surface the regression test needs from each adapter.
type generator interface {
	Generate(ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error)
}

// TestNonClaudeAdaptersNeverEmitTeamWorkflow proves S10 / T-regression: the
// claude-only team workflow surface (route_team JS + AUTOPUS_WORKFLOW_QUALITY)
// never leaks into codex, gemini, or opencode generated output.
func TestNonClaudeAdaptersNeverEmitTeamWorkflow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		make func(root string) generator
	}{
		{"codex", func(root string) generator { return codex.NewWithRoot(root) }},
		{"gemini", func(root string) generator { return antigravity.NewWithRoot(root) }},
		{"opencode", func(root string) generator { return opencode.NewWithRoot(root, opencode.WithCLIVersion("1.18.7")) }},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			g := tc.make(dir)
			if _, err := g.Generate(context.Background(), config.DefaultFullConfig("p")); err != nil {
				t.Fatalf("%s Generate failed: %v", tc.name, err)
			}

			err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				base := d.Name()
				// No generated *.js file may reference the team route or any
				// workflow JS surface (those are claude-only).
				if strings.HasSuffix(base, ".js") &&
					(strings.Contains(base, "route_team") || strings.Contains(base, "workflow")) {
					t.Errorf("%s leaked workflow JS file: %s", tc.name, path)
				}
				// No generated file may carry the team quality env token.
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}
				if strings.Contains(string(data), "AUTOPUS_WORKFLOW_QUALITY") {
					t.Errorf("%s leaked AUTOPUS_WORKFLOW_QUALITY into %s", tc.name, path)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s output: %v", tc.name, err)
			}
		})
	}
}
