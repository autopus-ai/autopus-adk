package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The surface a new project gets (machine_ops: false) adds no reference to
// a path it does not install, such as a dropped skill.
func TestGeneratedSurfaces_MachineOpsOffReferencesOnlyInstalledPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := config.DefaultFullConfig("reference-integrity-core")
	cfg.Platforms = allPlatforms
	off := false
	cfg.MachineOps = &off
	surface := newInstalledSurface()
	for _, name := range allPlatforms {
		pf, err := platformGenerators[name](t.TempDir(), ctx, cfg)
		require.NoError(t, err)
		surface.add(pf.Files)
	}
	observed := map[string]map[string]int{}
	for file, body := range surface.files {
		surface.collectDangling(file, body, observed)
	}
	for _, ref := range sortedRefs(observed) {
		if !danglingReferenceBaseline[ref] {
			count, examples := refSummary(observed[ref])
			t.Errorf("machine_ops: false surface references %q, which it does not install (%d, e.g. %s)", ref, count, examples)
		}
	}
}
