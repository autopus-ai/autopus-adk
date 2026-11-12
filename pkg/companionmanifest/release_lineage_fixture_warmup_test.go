package companionmanifest

import (
	"sync"
	"testing"
)

// @AX:NOTE [AUTO]: Two concurrent GoReleaser fixtures saturate the four-core CI runner without unbounded process fan-out.
const lineageFixtureWarmupParallelism = 2

var goReleaserLineageWarmupOnce sync.Once

func ensureGoReleaserLineageFixtures(t *testing.T, tools executableLineageTools) {
	t.Helper()
	goReleaserLineageWarmupOnce.Do(func() {
		if !t.Run("bounded-fixture-warmup", func(t *testing.T) {
			slots := make(chan struct{}, lineageFixtureWarmupParallelism)
			shard := activeLineageShard(t)
			for _, coordinate := range goReleaserLineageCoordinates {
				coordinate := coordinate
				if !shard.holds(coordinate.tag) {
					continue
				}
				t.Run(coordinate.tag, func(t *testing.T) {
					t.Parallel()
					slots <- struct{}{}
					defer func() { <-slots }()
					populateGoReleaserLineageCache(t, tools, coordinate)
				})
			}
		}) {
			t.FailNow()
		}
	})
}

func populateGoReleaserLineageCache(
	t *testing.T,
	tools executableLineageTools,
	coordinate goReleaserLineageCoordinate,
) {
	t.Helper()
	cache := goReleaserLineageCaches[coordinate.tag]
	if cache == nil {
		t.Fatalf("missing GoReleaser lineage cache for %s", coordinate.tag)
		return
	}
	cache.once.Do(func() {
		cache.runs.Add(1)
		cache.evidence = produceUncachedGoReleaserFixtureEvidence(
			t, tools, coordinate.tag, coordinate.version, coordinate.annotated,
		)
	})
}
