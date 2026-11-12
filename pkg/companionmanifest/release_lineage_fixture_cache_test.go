package companionmanifest

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
)

type goReleaserLineageCache struct {
	version   string
	annotated bool
	once      sync.Once
	runs      atomic.Int32
	evidence  *goReleaserA0Evidence
}

type goReleaserLineageCoordinate struct {
	tag       string
	version   string
	annotated bool
}

var goReleaserLineageCoordinates = []goReleaserLineageCoordinate{
	{tag: publicKeyReceiptA0Tag, version: lineageA0Version},
	{tag: publicKeyReceiptA1Tag, version: lineageA1Version, annotated: true},
	{tag: publicKeyReceiptA2Tag, version: publicKeyReceiptA2Version, annotated: true},
	{tag: publicKeyReceiptA3Tag, version: publicKeyReceiptA3Version, annotated: true},
	{tag: publicKeyReceiptA4Tag, version: publicKeyReceiptA4Version, annotated: true},
	{tag: publicKeyReceiptA5Tag, version: publicKeyReceiptA5Version, annotated: true},
	{tag: publicKeyReceiptA6Tag, version: publicKeyReceiptA6Version, annotated: true},
	{tag: publicKeyReceiptA7Tag, version: publicKeyReceiptA7Version, annotated: true},
	{tag: publicKeyReceiptA8Tag, version: publicKeyReceiptA8Version, annotated: true},
	{tag: publicKeyReceiptA9Tag, version: publicKeyReceiptA9Version, annotated: true},
	{tag: publicKeyReceiptA10Tag, version: publicKeyReceiptA10Version, annotated: true},
	{tag: publicKeyReceiptA11Tag, version: publicKeyReceiptA11Version, annotated: true},
	{tag: publicKeyReceiptA12Tag, version: publicKeyReceiptA12Version, annotated: true},
	{tag: publicKeyReceiptA13Tag, version: publicKeyReceiptA13Version, annotated: true},
	{tag: publicKeyReceiptA14Tag, version: publicKeyReceiptA14Version, annotated: true},
	{tag: publicKeyReceiptA15Tag, version: publicKeyReceiptA15Version, annotated: true},
	{tag: publicKeyReceiptA16Tag, version: publicKeyReceiptA16Version, annotated: true},
	{tag: publicKeyReceiptA17Tag, version: publicKeyReceiptA17Version, annotated: true},
	{tag: publicKeyReceiptA18Tag, version: publicKeyReceiptA18Version, annotated: true},
	{tag: publicKeyReceiptA19Tag, version: publicKeyReceiptA19Version, annotated: true},
	{tag: publicKeyReceiptA20Tag, version: publicKeyReceiptA20Version, annotated: true},
	{tag: publicKeyReceiptA21Tag, version: publicKeyReceiptA21Version, annotated: true},
}

var goReleaserLineageCaches = newGoReleaserLineageCaches()

func newGoReleaserLineageCaches() map[string]*goReleaserLineageCache {
	caches := make(map[string]*goReleaserLineageCache, len(goReleaserLineageCoordinates))
	for _, coordinate := range goReleaserLineageCoordinates {
		if _, exists := caches[coordinate.tag]; exists {
			panic("duplicate GoReleaser lineage coordinate: " + coordinate.tag)
		}
		caches[coordinate.tag] = &goReleaserLineageCache{
			version: coordinate.version, annotated: coordinate.annotated,
		}
	}
	return caches
}

func produceGoReleaserFixtureEvidence(
	t *testing.T,
	tools executableLineageTools,
	releaseTag, releaseVersion string,
	annotated bool,
) *goReleaserA0Evidence {
	t.Helper()
	requireExecutableLineageIntegration(t)
	if shard := activeLineageShard(t); !shard.holds(releaseTag) {
		t.Skipf("GoReleaser coordinate %s runs in another lineage shard than %d/%d", releaseTag, shard.index, shard.count)
	}
	cache, ok := goReleaserLineageCaches[releaseTag]
	if !ok || cache.version != releaseVersion || cache.annotated != annotated {
		t.Fatalf("unsupported GoReleaser lineage cache coordinate %s/%s annotated=%t",
			releaseTag, releaseVersion, annotated)
	}
	ensureGoReleaserLineageFixtures(t, tools)
	if cache.evidence == nil {
		t.Fatal("GoReleaser lineage cache produced no evidence")
	}
	return cloneGoReleaserEvidence(cache.evidence)
}

func cloneGoReleaserEvidence(source *goReleaserA0Evidence) *goReleaserA0Evidence {
	clone := *source
	clone.archives = make(map[string]string, len(source.archives))
	for architecture, archive := range source.archives {
		clone.archives[architecture] = archive
	}
	clone.checksums = bytes.Clone(source.checksums)
	clone.receipt = bytes.Clone(source.receipt)
	clone.signature = bytes.Clone(source.signature)
	return &clone
}

func TestGoReleaserLineageFixtures_ProcessCacheBuildsEachCoordinateOnce(t *testing.T) {
	tools := newExecutableLineageTools(t)
	shard := activeLineageShard(t)
	for _, test := range goReleaserLineageCoordinates {
		if !shard.holds(test.tag) {
			continue
		}
		first := produceGoReleaserFixtureEvidence(t, tools, test.tag, test.version, test.annotated)
		second := produceGoReleaserFixtureEvidence(t, tools, test.tag, test.version, test.annotated)
		if first == second || !bytes.Equal(first.checksums, second.checksums) {
			t.Fatalf("GoReleaser lineage cache %s did not return independent equivalent evidence", test.tag)
		}
		if runs := goReleaserLineageCaches[test.tag].runs.Load(); runs != 1 {
			t.Fatalf("GoReleaser lineage cache %s runs = %d, want 1", test.tag, runs)
		}
	}
	if runs := executableLineageToolsBuildRuns.Load(); runs != 1 {
		t.Fatalf("executable lineage tool build runs = %d, want 1", runs)
	}
}

func TestGoReleaserLineageFixtures_RegistryMatchesCache(t *testing.T) {
	if len(goReleaserLineageCaches) != len(goReleaserLineageCoordinates) {
		t.Fatalf("GoReleaser lineage cache size = %d, coordinates = %d",
			len(goReleaserLineageCaches), len(goReleaserLineageCoordinates))
	}
	for _, coordinate := range goReleaserLineageCoordinates {
		cache := goReleaserLineageCaches[coordinate.tag]
		if cache == nil || cache.version != coordinate.version ||
			cache.annotated != coordinate.annotated {
			t.Fatalf("GoReleaser lineage cache drifted for %s", coordinate.tag)
		}
	}
}

func TestGoReleaserLineageFixtures_RequireIntegrationTag(t *testing.T) {
	if executableLineageIntegrationEnabled {
		t.Skip("non-integration contract")
	}
	if runs := executableLineageToolsBuildRuns.Load(); runs != 0 {
		t.Fatalf("non-integration executable lineage tool build runs = %d, want 0", runs)
	}
	for tag, cache := range goReleaserLineageCaches {
		if runs := cache.runs.Load(); runs != 0 {
			t.Fatalf("non-integration GoReleaser lineage cache %s runs = %d, want 0", tag, runs)
		}
	}
}
