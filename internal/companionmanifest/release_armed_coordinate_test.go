package companionmanifest

import (
	"regexp"
	"testing"
)

// The armed coordinate has four sites, three of which cannot be derived.
//
//   - The release.yaml push tag trigger: GitHub Actions cannot construct triggers
//     dynamically, and release_contract_test forbids a `v*` wildcard so an
//     arbitrary tag cannot enter the protected release job.
//   - The Homebrew recovery workflow guard: workflow_dispatch has no tag filter,
//     so this comparison prevents recovery from rerunning against an old release.
//   - The release_tag in prepare-release.sh: this is the operator's armed site.
//
// The fourth site is data in the producer coordinate table. This test fails as
// soon as any of the four diverge, rather than after an immutable tag is pushed.
func TestArmedReleaseCoordinateAgreesEverywhere(t *testing.T) {
	expected := releasePhases[len(releasePhases)-1].tag
	if expected == "" {
		t.Fatal("declared release phases carry no armed tag")
	}

	for _, site := range []struct {
		name    string
		file    string
		pattern string
	}{
		{
			name:    "release workflow push trigger",
			file:    ".github/workflows/release.yaml",
			pattern: `(?m)^\s*-\s*'(v[0-9][^']*)'\s*$`,
		},
		{
			name:    "homebrew recovery ref guard",
			file:    ".github/workflows/homebrew-formula-bridge-recovery.yaml",
			pattern: `github\.ref == 'refs/tags/(v[0-9][^']*)'`,
		},
		{
			name:    "prepare-release armed tag",
			file:    "scripts/companion-release/prepare-release.sh",
			pattern: `readonly release_tag='(v[0-9][^']*)'`,
		},
	} {
		t.Run(site.name, func(t *testing.T) {
			body := readReleaseFile(t, site.file)
			match := regexp.MustCompile(site.pattern).FindStringSubmatch(body)
			if match == nil {
				t.Fatalf("%s no longer states an armed coordinate matching %s",
					site.file, site.pattern)
			}
			if match[1] != expected {
				t.Fatalf("%s arms %s while the declared phase table arms %s — "+
					"every armed site must move together",
					site.file, match[1], expected)
			}
		})
	}
}

// Keep the failure diagnostic aligned with the exact site set above. The test,
// rather than a maintenance note, remains the source of truth.
func TestArmedReleaseCoordinateSiteCountIsBounded(t *testing.T) {
	const bounded = 4
	sites := []string{
		".github/workflows/release.yaml",
		".github/workflows/homebrew-formula-bridge-recovery.yaml",
		"scripts/companion-release/prepare-release.sh",
		releaseCoordinateTableScript,
	}
	if len(sites) != bounded {
		t.Fatalf("armed coordinate now lives in %d places, not %d: %v",
			len(sites), bounded, sites)
	}
	armed := regexp.MustCompile(`v?0\.\d+\.\d+`)
	for _, site := range sites {
		if !armed.MatchString(readReleaseFile(t, site)) {
			t.Fatalf("%s stopped naming a coordinate; "+
				"if it became derived, drop it from this list", site)
		}
	}
}
