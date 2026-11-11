package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// growthTextRows bounds the growth table of the status text.
const growthTextRows = 5

// writeExtraConsumersText prints one row per detected or configured
// consumer, marking a walk cut by its budget as a lower bound.
func writeExtraConsumersText(w io.Writer, sizes []diskspace.ConsumerSize) {
	for _, s := range sizes {
		if !s.Available {
			fmt.Fprintln(w, terminalSafe(fmt.Sprintf("  %s: unavailable (%s) (%s)", s.Name, s.Reason, diskspace.DisplayPath(s.Path))))
			continue
		}
		size := diskspace.SizeString(s.Bytes)
		if s.Partial {
			size = "at least " + size + ", walk cut at its time budget"
		}
		fmt.Fprintln(w, terminalSafe(fmt.Sprintf("  %s: %s (%s)", s.Name, size, diskspace.DisplayPath(s.Path))))
	}
}

// writeGrowthText prints the consumers that changed, largest growth first.
func writeGrowthText(w io.Writer, growth []diskspace.Growth) {
	var rows []diskspace.Growth
	for _, g := range growth {
		if g.DeltaBytes != 0 || g.LastDeltaBytes != 0 {
			rows = append(rows, g)
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "consumer growth: no change recorded yet (samples are kept at most every 5m)")
		return
	}
	now := diskNow()
	since := rows[0].Since
	for _, g := range rows {
		if g.Since.Before(since) {
			since = g.Since
		}
	}
	fmt.Fprintf(w, "consumer growth since %s (last hour, or the newest older sample):\n", since.Local().Format(time.DateTime))
	for i, g := range rows {
		if i == growthTextRows {
			fmt.Fprintf(w, "  ... %d more in --json\n", len(rows)-growthTextRows)
			break
		}
		fmt.Fprintln(w, terminalSafe(fmt.Sprintf("  %s; since the last sample %s", diskspace.GrowthPhrase(g, now),
			signedSize(g.LastDeltaBytes))))
	}
}

func signedSize(bytes int64) string {
	if bytes < 0 {
		return "-" + diskspace.SizeString(-bytes)
	}
	return "+" + diskspace.SizeString(bytes)
}
