package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskStatusData is the JSON data of `auto disk status` (REQ-DISK-STATUS-01).
type diskStatusData struct {
	FreeBytes   int64         `json:"free_bytes"`
	Mount       string        `json:"mount"`
	Floor       diskFloorData `json:"floor"`
	Consumers   diskConsumers `json:"consumers"`
	TrimPreview diskTrimData  `json:"trim_preview"`
	// SPEC-DISK-002 additions, filled by statusExtras.
	ReclaimPreview diskReclaimData       `json:"reclaim_preview"`
	Leases         []diskspace.LeaseInfo `json:"leases"`
	Trend          *diskspace.Trend      `json:"trend"`
}

type diskFloorData struct {
	Bytes int64 `json:"bytes"`
	OK    bool  `json:"ok"`
}

type diskConsumers struct {
	GoBuildCache struct {
		diskConsumer
		Entries int `json:"entries"`
	} `json:"go_build_cache"`
	GoBuildWorkDirs struct {
		diskConsumer
		Dirs     int `json:"dirs"`
		IdleDirs int `json:"idle_dirs"`
	} `json:"go_build_work_dirs"`
	Worktrees struct {
		diskConsumer
		Base             string             `json:"base,omitempty"`
		CargoTargetBytes int64              `json:"cargo_target_bytes"`
		Items            []diskWorktreeData `json:"items"`
	} `json:"worktrees"`
	Scratchpads struct {
		diskConsumer
		Dirs             int   `json:"dirs"`
		OldestAgeMinutes int64 `json:"oldest_age_minutes"`
		NewestAgeMinutes int64 `json:"newest_age_minutes"`
	} `json:"scratchpads"`
}

type diskConsumer struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	Root      string `json:"root,omitempty"`
	Bytes     int64  `json:"bytes"`
}

type diskWorktreeData struct {
	Path             string              `json:"path"`
	Branch           string              `json:"branch,omitempty"`
	Main             bool                `json:"main"`
	Locked           bool                `json:"locked"`
	Prunable         bool                `json:"prunable"`
	Harness          bool                `json:"harness"`
	Owned            bool                `json:"owned"`
	Clean            *diskspace.Tristate `json:"clean,omitempty"`
	Merged           *diskspace.Tristate `json:"merged,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	Base             string              `json:"base,omitempty"`
	Bytes            int64               `json:"bytes"`
	CargoTargetBytes int64               `json:"cargo_target_bytes"`
	Suggestion       string              `json:"suggestion,omitempty"`
}

func newDiskStatusCmd() *cobra.Command {
	var flags diskFlags
	var ci string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show free space, the floor verdict, the disk consumers, and the trim preview",
		Long: "Reads free space on the file system that holds the project, the Go build cache, go build work dirs, " +
			"git worktrees, and agent scratchpads, then prints what `auto disk trim` would reclaim now. It deletes nothing.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDiskStatus(cmd, flags, ci)
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&ci, "ci", "", "override CI detection for the trim preview: running or idle")
	return cmd
}

func runDiskStatus(cmd *cobra.Command, flags diskFlags, ci string) error {
	jsonMode, err := resolveJSONMode(flags.jsonOut, flags.format)
	if err != nil {
		return err
	}
	if ci != "" && ci != diskspace.CIRunning && ci != diskspace.CIIdle {
		return diskFail(cmd, jsonMode, 1, "disk_status_invalid", "disk status: --ci must be running or idle", nil)
	}
	dc, err := loadDiskContext(cmd, flags, jsonMode, "status")
	if err != nil {
		return err
	}
	s := surveyDisk(cmd.Context(), dc, ci)
	worktrees := diskspace.ScanWorktrees(cmd.Context(), diskRunner, dc.projectDir)
	scratch := diskspace.ScanScratchpads(diskScratchpad(s.uid), s.now)
	data := statusData(s, worktrees, scratch)
	statusExtras(cmd.Context(), s, worktrees, &data)
	warnings := s.warnings()
	writeDiskWarnings(cmd, jsonMode, warnings)
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, data, warnings, nil)
	}
	writeStatusText(cmd.OutOrStdout(), data, worktrees)
	return nil
}

func statusData(s diskSurvey, worktrees diskspace.WorktreeScan, scratch diskspace.ScratchpadScan) diskStatusData {
	d := diskStatusData{FreeBytes: s.dc.space.AvailBytes, Mount: s.dc.space.Mount,
		Floor: diskFloorData{Bytes: s.dc.disk.FloorBytes(), OK: s.dc.space.AvailBytes >= s.dc.disk.FloorBytes()}, TrimPreview: s.trimData("dry-run", false)}
	c := &d.Consumers
	c.GoBuildCache.diskConsumer = consumer(s.cacheRoot, s.cache.Bytes)
	c.GoBuildCache.Entries = len(s.cache.Entries)
	c.GoBuildWorkDirs.diskConsumer = consumer(s.workRoot, s.work.Bytes())
	c.GoBuildWorkDirs.Dirs = len(s.work.Dirs)
	c.GoBuildWorkDirs.IdleDirs = s.work.IdleDirs(s.now, s.dc.disk.WorkDirIdleMinutes)
	total, cargo := worktrees.Bytes()
	c.Worktrees.diskConsumer = consumer(worktrees.Root, total)
	c.Worktrees.Base, c.Worktrees.CargoTargetBytes, c.Worktrees.Items = worktrees.Base, cargo, []diskWorktreeData{}
	for _, wt := range worktrees.Items {
		item := diskWorktreeData{Path: wt.Path, Branch: wt.Branch, Main: wt.Main, Locked: wt.Locked, Prunable: wt.Prunable,
			Harness: wt.Harness, Reason: wt.Reason, Base: wt.Base, Bytes: wt.Bytes, CargoTargetBytes: wt.CargoTargetBytes, Suggestion: wt.Suggestion}
		if !wt.Main {
			clean, merged := wt.Clean, wt.Merged
			item.Clean, item.Merged = &clean, &merged
		}
		c.Worktrees.Items = append(c.Worktrees.Items, item)
	}
	c.Scratchpads.diskConsumer = consumer(scratch.Root, scratch.Bytes)
	c.Scratchpads.Dirs, c.Scratchpads.OldestAgeMinutes, c.Scratchpads.NewestAgeMinutes = scratch.Dirs, scratch.OldestAgeMinutes, scratch.NewestAgeMinutes
	return d
}

// consumer is the shared part of a consumer row. A root refused for trim
// can still be read when only its owner differs, but status reports what
// the trim would see: unavailable, with the refusal reason.
func consumer(root diskspace.Root, bytes int64) diskConsumer {
	if !root.Available() {
		return diskConsumer{Reason: root.Reason, Message: root.Message, Root: root.Path}
	}
	return diskConsumer{Available: true, Root: root.Path, Bytes: bytes}
}

func writeStatusText(w io.Writer, d diskStatusData, worktrees diskspace.WorktreeScan) {
	verdict := "ok"
	if !d.Floor.OK {
		verdict = "below floor"
	}
	fmt.Fprintf(w, "disk status: %s GiB free on %s; floor %s GiB (disk.min_free_gb): %s\n",
		diskspace.GiBString(d.FreeBytes), d.Mount, diskspace.GiBString(d.Floor.Bytes), verdict)
	c := d.Consumers
	row := func(name string, consumer diskConsumer, detail string) {
		if !consumer.Available {
			fmt.Fprintf(w, "  %s: unavailable (%s)\n", name, consumer.Reason)
			return
		}
		fmt.Fprintf(w, "  %s: %s, %s\n", name, diskspace.SizeString(consumer.Bytes), terminalSafe(detail))
	}
	row("go_build_cache", c.GoBuildCache.diskConsumer, fmt.Sprintf("%d entries (%s)", c.GoBuildCache.Entries, diskspace.DisplayPath(c.GoBuildCache.Root)))
	row("go_build_work_dirs", c.GoBuildWorkDirs.diskConsumer, fmt.Sprintf("%d dirs, %d idle (%s)", c.GoBuildWorkDirs.Dirs, c.GoBuildWorkDirs.IdleDirs,
		diskspace.DisplayPath(c.GoBuildWorkDirs.Root)))
	row("worktrees", c.Worktrees.diskConsumer, fmt.Sprintf("%d linked, cargo target %s", len(worktrees.Linked()), diskspace.SizeString(c.Worktrees.CargoTargetBytes)))
	for _, wt := range worktrees.Items {
		switch wt.Suggestion {
		case "":
		case diskspace.SuggestionWithheld:
			fmt.Fprintf(w, "    %s (%s)\n", diskspace.SuggestionWithheld, diskspace.DisplayPath(wt.Path))
		default:
			fmt.Fprintf(w, "    %s\n", terminalSafe(wt.Suggestion))
		}
	}
	row("scratchpads", c.Scratchpads.diskConsumer, fmt.Sprintf("%d dirs, oldest %dm, newest %dm (%s)", c.Scratchpads.Dirs,
		c.Scratchpads.OldestAgeMinutes, c.Scratchpads.NewestAgeMinutes, diskspace.DisplayPath(c.Scratchpads.Root)))
	switch preview := d.TrimPreview; {
	case !preview.Active:
		fmt.Fprintln(w, "disk trim: inactive (free above start_below_gb)")
	case preview.Refused != nil:
		fmt.Fprintln(w, terminalSafe("disk trim: refused: "+preview.Refused.Message))
	default:
		writeTrimText(w, preview)
	}
	writeStatusExtrasText(w, d)
}
