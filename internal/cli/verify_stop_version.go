package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/freshness"
)

// versionStopDeadline bounds every registry lookup of one stop together, so
// the Stop hook stays fast even when a registry hangs.
const versionStopDeadline = 10 * time.Second

// maxVersionItemsShown keeps the block reason short on a sweeping change.
const maxVersionItemsShown = 8

// newFreshnessClient builds the registry client; tests replace it.
var newFreshnessClient = freshness.NewClient

// versionStopSettings reports whether the stop gate checks newly added or
// bumped pins (on unless AUTOPUS_VERSION_STOP=off or hooks.version_stop:
// false) and how many lines behind may pass.
func versionStopSettings(root string) (enabled bool, maxBehind int) {
	if strings.EqualFold(os.Getenv("AUTOPUS_VERSION_STOP"), "off") {
		return false, 0
	}
	cfg := nearestConfig(root)
	if cfg == nil {
		return true, config.DefaultVersionStopMaxMajorBehind
	}
	return cfg.Hooks.IsVersionStopEnabled(), cfg.Hooks.VersionStopMaxMajorBehind()
}

// versionGate runs before the build. It looks only at pins this turn adds
// or changes against HEAD, never at old pins it did not touch. A pin more
// than maxBehind release lines behind the latest stable blocks once; the
// same set again, or a session past the block limit, ends with a warning.
// A lookup that fails or runs out of time is a warning, never a block.
func versionGate(ctx context.Context, in stopHookInput, dir string) (block *stopHookOutput, warning string) {
	enabled, maxBehind := versionStopSettings(dir)
	if !enabled {
		return nil, ""
	}
	_, changes, ok := freshness.Changed(ctx, dir)
	if !ok || len(changes) == 0 {
		return nil, ""
	}
	pins := freshness.ReplyPins(in.LastAssistantMessage)
	items := make([]freshness.Item, 0, len(changes))
	for _, c := range changes {
		if !freshness.PinnedBy(c.Item, pins) {
			items = append(items, c.Item)
		}
	}
	if len(items) == 0 {
		return nil, ""
	}
	lookupCtx, cancel := context.WithTimeout(ctx, versionStopDeadline)
	defer cancel()
	latests := newFreshnessClient().LookupAll(lookupCtx, items)
	var blockers, behind []freshness.Result
	var unknown []string
	for i, it := range items {
		r := freshness.Classify(it, latests[i])
		switch {
		case r.Status == freshness.StatusMajor && r.LinesBehind > maxBehind:
			blockers = append(blockers, r)
		case r.Status == freshness.StatusMajor || r.Status == freshness.StatusMinor:
			behind = append(behind, r)
		case r.Status == freshness.StatusUnknown:
			unknown = append(unknown, itemLabel(it)+" ("+r.Reason+")")
		}
	}
	warning = versionWarning(in.SessionID, behind, unknown)
	if len(blockers) == 0 {
		return nil, warning
	}
	where := describeVersionResults(blockers)
	fingerprint := versionFingerprint(blockers)
	if readStopState(in.SessionID, "versions") == fingerprint || countStopBlock(in.SessionID) > verifyStopMaxBlocks {
		return nil, joinWarnings("auto verify stop: pins still behind the latest stable line:"+where, warning)
	}
	writeStopState(in.SessionID, "versions", fingerprint)
	return &stopHookOutput{Decision: "block", Reason: "This change adds or bumps versions a full release line behind the latest stable:\n" +
		where + "\n\nUse the latest stable line (`auto deps latest <ecosystem>:<name>` looks it up; for a major jump read the " +
		"migration notes and check peer compatibility first). If the older version is deliberate, end your reply with one line " +
		"per item: `version-pin: <name> <reason>`, or put a `version-pin: <reason>` comment on the manifest line where the " +
		"format allows comments. The stack-freshness skill has the procedure."}, ""
}

// versionWarning reports pins behind by less than the block threshold and
// lookups that failed, once per distinct set within a session.
func versionWarning(session string, behind []freshness.Result, unknown []string) string {
	var parts []string
	if len(behind) > 0 {
		parts = append(parts, "auto verify stop: newly pinned versions behind the latest stable:"+describeVersionResults(behind))
	}
	if len(unknown) > 0 {
		parts = append(parts, "auto verify stop: could not check the latest version of "+strings.Join(firstN(unknown, maxVersionItemsShown), ", "))
	}
	text := strings.Join(parts, "\n")
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	key := hex.EncodeToString(sum[:])
	if readStopState(session, "versions-warn") == key {
		return ""
	}
	writeStopState(session, "versions-warn", key)
	return text
}

func describeVersionResults(results []freshness.Result) string {
	lines := make([]string, 0, min(len(results), maxVersionItemsShown)+1)
	for i, r := range results {
		if i == maxVersionItemsShown {
			lines = append(lines, fmt.Sprintf("- and %d more", len(results)-i))
			break
		}
		gap := string(r.Status)
		if r.LinesBehind > 0 {
			gap = fmt.Sprintf("%d release line(s) behind", r.LinesBehind)
		}
		lines = append(lines, fmt.Sprintf("- %s:%d %s: latest stable %s (%s; %s, checked %s)", r.Item.File, r.Item.Line,
			itemLabel(r.Item), r.Target, gap, r.Latest.Source, r.Latest.FetchedAt.UTC().Format(time.RFC3339)))
	}
	return "\n" + strings.Join(lines, "\n")
}

// itemLabel names an item the way a reply pin may name it back.
func itemLabel(it freshness.Item) string {
	switch it.Ecosystem {
	case freshness.EcoImage:
		return "image " + it.Name + ":" + it.Spec
	case freshness.EcoToolchain:
		return it.Name + " " + it.Spec
	}
	return string(it.Ecosystem) + ":" + it.Name + " " + it.Spec
}

func versionFingerprint(results []freshness.Result) string {
	keys := make([]string, 0, len(results))
	for _, r := range results {
		keys = append(keys, r.Item.File+"|"+r.Item.Key()+"|"+r.Item.Spec)
	}
	slices.Sort(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(sum[:])
}

func joinWarnings(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "\n" + b
}

func firstN(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(slices.Clone(s[:n]), fmt.Sprintf("and %d more", len(s)-n))
}
