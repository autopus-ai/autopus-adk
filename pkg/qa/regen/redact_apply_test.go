package regen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qaevidence "github.com/insajin/autopus-adk/pkg/qa/evidence"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
)

// AC-QAMESH11-010 (regen half): extracted flow text carrying a token-like secret
// and an absolute local path must not survive into the serialized diff; the
// redaction placeholder appears instead, and AssertDiffSafe passes.
func TestRedactDiff_StripsSecretAndPath(t *testing.T) {
	// Joined from fragments so no provider-format literal is committed; the
	// bytes are unchanged.
	secret := "sk-" + "proj-ABCDEFGHIJKLMNOP1234567890"
	localPath := "/Users/victim/secrets/app"

	diff := Diff{
		ChangedCount: 1,
		Changed: []DiffEntry{{
			JourneyID: "flow-1",
			Category:  "changed",
			ChangedFields: []FieldChange{
				{Field: "command.argv", Before: "[run " + localPath + "]", After: "[run --token=" + secret + "]"},
			},
		}},
	}

	redacted := RedactDiff(diff)
	body, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(body)
	if strings.Contains(serialized, secret) {
		t.Fatalf("raw secret leaked into diff: %s", serialized)
	}
	if strings.Contains(serialized, "/Users/victim") {
		t.Fatalf("raw local user path leaked into diff: %s", serialized)
	}
	if !strings.Contains(serialized, qaevidence.RedactedSecret) && !strings.Contains(serialized, qaevidence.RedactedUser) {
		t.Fatalf("expected a redaction placeholder, got: %s", serialized)
	}
	if err := AssertDiffSafe(redacted); err != nil {
		t.Fatalf("AssertDiffSafe after redaction: %v", err)
	}
}

// AC-QAMESH11-010 (pack half): every synthesized starter pack's text passes
// AssertSafeText (no secrets/paths baked into the templates).
func TestSynthesizedPacks_AssertSafeText(t *testing.T) {
	for _, surface := range []string{SurfaceWeb, SurfaceDesktop, SurfaceMobile} {
		pack, _ := synthesizeSurface(surface)
		body, err := json.Marshal(pack)
		if err != nil {
			t.Fatal(err)
		}
		if err := qaevidence.AssertSafeText(string(body), "synth."+surface); err != nil {
			t.Fatalf("%s pack text unsafe: %v", surface, err)
		}
	}
}

// AC-QAMESH11-012: ApplyPacks writes only the valid pack; the invalid mobile
// pack is excluded with no file written, and the result reports the exclusion.
func TestApplyPacks_ExcludesInvalidMobile(t *testing.T) {
	dir := t.TempDir()

	goodWeb := validWebPack("good-web")
	badMobile := mobileStarterPack()
	badMobile.ID = "bad-mobile"
	badMobile.PassFailAuthority = "ai" // fails journey.Validate (mobile policy)

	result, err := ApplyPacks(dir, []journey.Pack{goodWeb, badMobile})
	if err != nil {
		t.Fatalf("ApplyPacks: %v", err)
	}

	if len(result.Written) != 1 || result.Written[0].JourneyID != "good-web" {
		t.Fatalf("written = %+v, want only good-web", result.Written)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].JourneyID != "bad-mobile" {
		t.Fatalf("excluded = %+v, want only bad-mobile", result.Excluded)
	}
	if result.Excluded[0].Reason != "qa_journey_mobile_policy_invalid" {
		t.Fatalf("exclude reason = %q", result.Excluded[0].Reason)
	}

	goodPath := filepath.Join(dir, ".autopus", "qa", "journeys", "good-web.yaml")
	if _, err := os.Stat(goodPath); err != nil {
		t.Fatalf("good-web.yaml not written: %v", err)
	}
	badPath := filepath.Join(dir, ".autopus", "qa", "journeys", "bad-mobile.yaml")
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatalf("bad-mobile.yaml must not exist, stat err = %v", err)
	}

	// Round-trip: the written pack reloads and re-validates clean.
	reloaded, err := journey.LoadFile(goodPath)
	if err != nil {
		t.Fatalf("reload good-web: %v", err)
	}
	if err := journey.Validate(reloaded, dir); err != nil {
		t.Fatalf("reloaded good-web fails validate: %v", err)
	}
}

// Defense in depth: a pack whose id encodes a path traversal must be excluded
// (reason "unsafe_journey_id") with no file written anywhere outside the
// journeys directory. journey.Validate permits any non-empty id, so this guard
// is ApplyPacks' own fail-closed check for its exported-API contract.
func TestApplyPacks_RejectsTraversalID(t *testing.T) {
	dir := t.TempDir()

	evil := validWebPack("../../../tmp/escape")

	result, err := ApplyPacks(dir, []journey.Pack{evil})
	if err != nil {
		t.Fatalf("ApplyPacks: %v", err)
	}
	if len(result.Written) != 0 {
		t.Fatalf("written = %+v, want none for traversal id", result.Written)
	}
	if len(result.Excluded) != 1 || result.Excluded[0].Reason != "unsafe_journey_id" {
		t.Fatalf("excluded = %+v, want one unsafe_journey_id", result.Excluded)
	}
	// No file escaped the journeys directory.
	if _, statErr := os.Stat(filepath.Join(dir, "tmp", "escape.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("traversal write escaped journeys dir, stat err = %v", statErr)
	}
	journeysDir := filepath.Join(dir, ".autopus", "qa", "journeys")
	entries, _ := os.ReadDir(journeysDir)
	if len(entries) != 0 {
		t.Fatalf("journeys dir must be empty, got %d entries", len(entries))
	}
}

// AC-QAMESH11-013 (load half): LoadExistingPacks tolerates an unparseable pack
// file and still returns the readable ones.
func TestLoadExistingPacks_SkipsUnparseable(t *testing.T) {
	dir := t.TempDir()
	journeysDir := filepath.Join(dir, ".autopus", "qa", "journeys")
	if err := os.MkdirAll(journeysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journeysDir, "broken.yaml"), []byte("::: not yaml :::"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := webStarterPack()
	body, _ := json.Marshal(good)
	// yaml accepts JSON; reuse as a valid parseable pack file.
	if err := os.WriteFile(filepath.Join(journeysDir, "good.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	packs, err := LoadExistingPacks(dir)
	if err != nil {
		t.Fatalf("LoadExistingPacks: %v", err)
	}
	if _, ok := packs[good.ID]; !ok {
		t.Fatalf("expected %q in loaded packs, got %v", good.ID, keys(packs))
	}
}

func keys(m map[string]journey.Pack) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
