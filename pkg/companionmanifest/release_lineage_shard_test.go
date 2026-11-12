package companionmanifest

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// lineageShardEnv splits the executable GoReleaser coordinates across CI jobs
// as "<index>/<count>" (1-based). Each GoReleaser build costs about a minute
// and two of them saturate a four-core runner, so the 22 builds only shrink by
// running on separate runners. Coordinate i belongs to shard i%count+1, which
// puts every coordinate in exactly one shard; unset means one shard holding
// all of them, which is how local and release runs behave.
const lineageShardEnv = "AUTOPUS_LINEAGE_SHARD"

type lineageShard struct {
	index, count int
}

var (
	activeLineageShardOnce sync.Once
	activeLineageShardVal  lineageShard
	activeLineageShardErr  error
)

func parseLineageShard(value string) (lineageShard, error) {
	if value == "" {
		return lineageShard{index: 1, count: 1}, nil
	}
	left, right, ok := strings.Cut(value, "/")
	index, indexErr := strconv.Atoi(left)
	count, countErr := strconv.Atoi(right)
	if !ok || indexErr != nil || countErr != nil || count < 1 || index < 1 || index > count {
		return lineageShard{}, fmt.Errorf("%s=%q, want <index>/<count> with 1 <= index <= count", lineageShardEnv, value)
	}
	return lineageShard{index: index, count: count}, nil
}

// activeLineageShard fails the test on a malformed value: a typo must never
// quietly select no coordinates and let the lane pass on nothing.
func activeLineageShard(t *testing.T) lineageShard {
	t.Helper()
	activeLineageShardOnce.Do(func() {
		activeLineageShardVal, activeLineageShardErr = parseLineageShard(os.Getenv(lineageShardEnv))
	})
	if activeLineageShardErr != nil {
		t.Fatal(activeLineageShardErr)
	}
	return activeLineageShardVal
}

func (s lineageShard) holds(tag string) bool {
	for i, coordinate := range goReleaserLineageCoordinates {
		if coordinate.tag == tag {
			return i%s.count+1 == s.index
		}
	}
	return false
}

func TestLineageShard_ParseAndPartition(t *testing.T) {
	for _, bad := range []string{"0/3", "4/3", "1/0", "a/3", "2", "1/3/2", "-1/3"} {
		if _, err := parseLineageShard(bad); err == nil {
			t.Fatalf("parseLineageShard(%q) accepted a malformed shard", bad)
		}
	}
	all, err := parseLineageShard("")
	if err != nil || all != (lineageShard{index: 1, count: 1}) {
		t.Fatalf("unset shard = %+v, %v; want 1/1", all, err)
	}
	// Every coordinate lands in exactly one of three shards, and each shard
	// gets a share, so no split can leave a coordinate unbuilt.
	const count = 3
	owners := make(map[string]int, len(goReleaserLineageCoordinates))
	sizes := make([]int, count)
	for index := 1; index <= count; index++ {
		shard := lineageShard{index: index, count: count}
		for _, coordinate := range goReleaserLineageCoordinates {
			if shard.holds(coordinate.tag) {
				owners[coordinate.tag]++
				sizes[index-1]++
			}
		}
	}
	for _, coordinate := range goReleaserLineageCoordinates {
		if owners[coordinate.tag] != 1 {
			t.Fatalf("coordinate %s is held by %d shards, want 1", coordinate.tag, owners[coordinate.tag])
		}
		if !all.holds(coordinate.tag) {
			t.Fatalf("the unsharded run does not hold %s", coordinate.tag)
		}
	}
	for index, size := range sizes {
		if size == 0 {
			t.Fatalf("shard %d/%d holds no coordinate", index+1, count)
		}
	}
	if (lineageShard{index: 1, count: 1}).holds("v0.0.0-not-a-coordinate") {
		t.Fatal("an unknown tag must not be held by any shard")
	}
}
