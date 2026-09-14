package adapter_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// componentPathRe pulls the path out of an "Installed Components" bullet. A
// bullet may describe an invocation route instead of a path, which carries no
// path to check.
var componentPathRe = regexp.MustCompile(
	`^- [^:]+: (\.?[A-Za-z0-9_][A-Za-z0-9_./<>*-]*)`)

// installedComponentBullets returns the path in every Installed Components
// bullet of a rendered marker section.
func installedComponentBullets(t *testing.T, section string) []string {
	t.Helper()
	_, after, found := strings.Cut(section, "## Installed Components\n")
	require.True(t, found, "marker section must declare Installed Components")
	body, _, _ := strings.Cut(after, "\n## ")

	var paths []string
	for _, line := range strings.Split(body, "\n") {
		m := componentPathRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		paths = append(paths, strings.TrimSuffix(m[1], "/"))
	}
	require.NotEmpty(t, paths, "Installed Components must list at least one path")
	return paths
}

// rootDocOwners enumerates the adapters that can author the AGENTS.md marker
// and the platform set under which each actually owns it. Opencode wins the
// arbitration whenever it is installed (codexOwnsRootDoc returns false and the
// codex adapter drops its AGENTS.md mapping), so the codex marker is only
// reachable without opencode. Each marker is checked against the surface its
// own platform set installs, not a union it never sees.
var rootDocOwners = []struct {
	owner     string
	platforms []string
}{
	{owner: "opencode", platforms: allPlatforms},
	{owner: "codex", platforms: []string{"claude-code", "codex", "antigravity-cli", "omp"}},
}

// Every advertised native root must be installed. Detailed file inventory is
// discoverable from manifests, not duplicated into the initial prompt.
func TestRootDoc_DiscoveryListsInstalledRoots(t *testing.T) {
	t.Parallel()

	for _, owner := range rootDocOwners {
		surface := generateSurface(t, owner.platforms)
		section, ok := surface.files["AGENTS.md"]
		require.True(t, ok, "%s must own an AGENTS.md mapping for %v", owner.owner, owner.platforms)

		for _, p := range installedComponentBullets(t, section) {
			if !surface.resolve(p, "") && !surface.resolve(p+"/", "") {
				t.Errorf("%s marker lists %q, which no install manifest writes", owner.owner, p)
			}
		}

	}

}
