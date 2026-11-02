package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskTrimData is the JSON data of every trim run, dry run, apply, inactive,
// or refused (REQ-DISK-TRIM-09), and the trim_preview of `auto disk status`.
type diskTrimData struct {
	Run        string            `json:"run"`
	Mode       string            `json:"mode"`
	Active     bool              `json:"active"`
	Refused    *diskRefusal      `json:"refused"`
	FreeBytes  int64             `json:"free_bytes"`
	FloorBytes int64             `json:"floor_bytes"`
	Mount      string            `json:"mount"`
	CI         diskCIData        `json:"ci"`
	Roots      diskRoots         `json:"roots"`
	Rules      []diskRuleData    `json:"rules"`
	Skipped    diskspace.Skipped `json:"skipped"`
	TotalBytes int64             `json:"total_bytes"`
}

type diskRefusal struct {
	Root    string `json:"root"`
	Kind    string `json:"kind"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type diskCIData struct {
	State  string `json:"state"`
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
}

type diskRoots struct {
	Cache diskRootData `json:"cache"`
	Work  diskRootData `json:"work"`
}

type diskRootData struct {
	Path      string `json:"path"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type diskRuleData struct {
	ID          string `json:"id"`
	MinBytes    int64  `json:"min_bytes"`
	IdleMinutes int    `json:"idle_minutes"`
	Candidates  int    `json:"candidates"`
	Bytes       int64  `json:"bytes"`
	diskspace.Counters
	Files *[]string `json:"files,omitempty"`
}

func (r diskRuleData) label() string {
	return diskspace.RulePlan{ID: r.ID, MinBytes: r.MinBytes, IdleMinutes: r.IdleMinutes}.Label()
}

type diskTrimOptions struct {
	flags diskFlags
	apply bool
	list  bool
	ci    string
}

const diskTrimLong = "Reclaims idle Go build cache entries and abandoned go build work directories, as a dry run unless --apply is given.\n\n" +
	"Rules, in attribution order: go_build_large_files, go_build_files at the idle threshold of the mode (emergency below " +
	"disk.min_free_gb, ci while a CI job runs or detection fails, idle otherwise), and go_build_work_dirs by the newest " +
	"mtime in each tree.\n\n" +
	"An idle threshold of N minutes guarantees only N minus 60 minutes without use, because Go refreshes a reused cache " +
	"entry's mtime at most hourly: trimming by mtime bounds, but does not eliminate, the chance of removing an entry a " +
	"running build has already looked up.\n\n" +
	"Deletion never leaves the resolved cache and work dir roots, never follows a symlink, and skips any entry that " +
	"changed since the scan. Exit status: 0 done, 1 a refused root, a failed removal, or a usage, configuration, or " +
	"measurement failure."

func newDiskTrimCmd() *cobra.Command {
	var opts diskTrimOptions
	cmd := &cobra.Command{
		Use:           "trim [--apply] [--list] [--ci running|idle]",
		Short:         "Reclaim idle go-build cache entries and abandoned work dirs (dry run by default)",
		Long:          diskTrimLong,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return runDiskTrim(cmd, opts) },
	}
	opts.flags.register(cmd)
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "delete the candidates instead of listing what would go")
	cmd.Flags().BoolVar(&opts.list, "list", false, "print every candidate path under its rule")
	cmd.Flags().StringVar(&opts.ci, "ci", "", "override CI detection: running or idle")
	return cmd
}

func runDiskTrim(cmd *cobra.Command, opts diskTrimOptions) error {
	jsonMode, err := resolveJSONMode(opts.flags.jsonOut, opts.flags.format)
	if err != nil {
		return err
	}
	if opts.ci != "" && opts.ci != diskspace.CIRunning && opts.ci != diskspace.CIIdle {
		return diskFail(cmd, jsonMode, 1, "disk_trim_invalid", "disk trim: --ci must be running or idle", nil)
	}
	dc, err := loadDiskContext(cmd, opts.flags, jsonMode, "trim")
	if err != nil {
		return err
	}
	s := surveyDisk(cmd.Context(), dc, opts.ci)
	run := "dry-run"
	if opts.apply {
		run = "apply"
	}
	data := s.trimData(run, opts.list)
	if opts.apply && data.Refused == nil && data.Active {
		result := diskspace.Apply(s.plan, diskspace.ApplyOptions{Cache: s.cacheRoot, WorkDir: s.workRoot, UID: s.uid, BeforeRemove: diskBeforeRemove})
		data.withApply(result)
		recordDiskEvent(diskspace.Event{Kind: diskspace.EventTrimApply, FreeBytes: dc.space.AvailBytes,
			ReclaimedBytes: result.Total.DeletedBytes, Count: result.Total.Deleted})
	}
	warnings := s.warnings()
	writeDiskWarnings(cmd, jsonMode, warnings)
	switch {
	case !data.Active:
		if jsonMode {
			return writeJSONResult(cmd, jsonStatusOK, data, warnings, nil)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "disk trim: inactive (free above start_below_gb)")
		return nil
	case data.Refused != nil:
		return diskFail(cmd, jsonMode, 1, "disk_trim_refused", "disk trim: refused: "+data.Refused.Message, data, warnings...)
	}
	totals, _ := data.totals()
	if jsonMode && totals.Failed == 0 {
		return writeJSONResult(cmd, jsonStatusOK, data, warnings, nil)
	}
	if !jsonMode {
		writeTrimText(cmd.OutOrStdout(), data)
	}
	if totals.Failed > 0 {
		noun := "candidates"
		if totals.Failed == 1 {
			noun = "candidate"
		}
		return diskFail(cmd, jsonMode, 1, "disk_trim_failed", fmt.Sprintf("disk trim: %d %s could not be removed", totals.Failed, noun), data, warnings...)
	}
	return nil
}

func (s diskSurvey) trimData(run string, list bool) diskTrimData {
	d := diskTrimData{Run: run, Mode: s.plan.Mode, Active: !s.plan.Inactive, FreeBytes: s.dc.space.AvailBytes,
		FloorBytes: s.dc.disk.FloorBytes(), Mount: s.dc.space.Mount, CI: diskCIData{State: s.ci.State, Source: s.ci.Source, Reason: s.ci.Reason},
		Roots: diskRoots{Cache: rootData(s.cacheRoot), Work: rootData(s.workRoot)}, Rules: []diskRuleData{},
		Skipped: s.plan.Skipped, TotalBytes: s.plan.TotalBytes}
	if root := s.refused(); root != nil {
		d.Refused = refusalData(*root)
	}
	for _, rule := range s.plan.Rules {
		r := diskRuleData{ID: rule.ID, MinBytes: rule.MinBytes, IdleMinutes: rule.IdleMinutes, Candidates: len(rule.Candidates), Bytes: rule.Bytes}
		if list {
			files := make([]string, 0, len(rule.Candidates))
			for _, candidate := range rule.Candidates {
				files = append(files, candidate.Rel)
			}
			r.Files = &files
		}
		d.Rules = append(d.Rules, r)
	}
	return d
}

func rootData(root diskspace.Root) diskRootData {
	return diskRootData{Path: root.Path, Available: root.Available(), Reason: root.Reason}
}

func refusalData(root diskspace.Root) *diskRefusal {
	return &diskRefusal{Root: root.Path, Kind: root.Kind, Reason: root.Reason, Message: root.Message}
}

func (d *diskTrimData) withApply(result diskspace.ApplyResult) {
	if result.RootChanged != nil {
		d.Refused = refusalData(*result.RootChanged)
		return
	}
	for i := range d.Rules {
		d.Rules[i].Counters = result.Rules[d.Rules[i].ID]
	}
	d.Skipped = result.Skipped
}

// totals sums the per-rule removal counters, and reports whether the run
// removed anything at all.
func (d diskTrimData) totals() (diskspace.Counters, bool) {
	var total diskspace.Counters
	for _, rule := range d.Rules {
		total.Deleted += rule.Deleted
		total.DeletedBytes += rule.DeletedBytes
		total.SkippedChanged += rule.SkippedChanged
		total.Vanished += rule.Vanished
		total.Failed += rule.Failed
	}
	return total, d.Run == "apply"
}

// writeTrimText prints the dry-run or apply block of REQ-DISK-TRIM-01.
func writeTrimText(w io.Writer, d diskTrimData) {
	labels := make([]string, 0, len(d.Rules))
	threshold := 0
	for _, rule := range d.Rules {
		labels = append(labels, rule.label())
		if rule.ID == diskspace.RuleFiles {
			threshold = rule.IdleMinutes
		}
	}
	fmt.Fprintf(w, "disk trim: run=%s mode=%s free=%s GiB ci=%s rules=%s\n",
		d.Run, d.Mode, diskspace.GiBString(d.FreeBytes), d.CI.State, strings.Join(labels, " "))
	fmt.Fprintf(w, "  roots: cache=%s work=%s\n", rootWord(d.Roots.Cache), rootWord(d.Roots.Work))
	for _, root := range []struct {
		name string
		data diskRootData
	}{{"go_build_cache", d.Roots.Cache}, {"go_build_work_dirs", d.Roots.Work}} {
		if !root.data.Available {
			fmt.Fprintf(w, "  %s: unavailable (%s)\n", root.name, root.data.Reason)
		}
	}
	fmt.Fprintf(w, "  note: idle>%dm guarantees %dm without use; mtime trimming bounds the risk to a running build, it does not remove it\n",
		threshold, max(threshold-60, 0))
	for _, rule := range d.Rules {
		fmt.Fprintf(w, "  %s: candidates=%d size=%s\n", rule.ID, rule.Candidates, diskspace.SizeString(rule.Bytes))
		if rule.Files != nil {
			for _, file := range *rule.Files {
				fmt.Fprintf(w, "    %s\n", diskspace.DisplayPath(file))
			}
		}
	}
	sk := d.Skipped
	fmt.Fprintf(w, "  skipped_symlink=%d (go_build_cache=%d, go_build_work_dirs=%d)\n", sk.Symlink.Total(), sk.Symlink.Cache, sk.Symlink.WorkDir)
	for _, extra := range []struct {
		name  string
		count int
	}{{"skipped_device", sk.Device.Total()}, {"skipped_owner", sk.Owner}, {"skipped_shape", sk.Shape}, {"skipped_unbounded", sk.Unbounded}} {
		if extra.count > 0 {
			fmt.Fprintf(w, "  %s=%d\n", extra.name, extra.count)
		}
	}
	total, applied := d.totals()
	if !applied {
		fmt.Fprintf(w, "disk trim: would reclaim %s (dry-run; pass --apply to delete)\n", diskspace.SizeString(d.TotalBytes))
		return
	}
	fmt.Fprintf(w, "disk trim: reclaimed %s (deleted=%d skipped_changed=%d skipped_symlink=%d skipped_device=%d skipped_owner=%d vanished=%d failed=%d)\n",
		diskspace.SizeString(total.DeletedBytes), total.Deleted, total.SkippedChanged, sk.Symlink.Total(), sk.Device.Total(), sk.Owner, total.Vanished, total.Failed)
}

// rootWord is a root's path, or why it cannot be read.
func rootWord(root diskRootData) string {
	if root.Available {
		return diskspace.DisplayPath(root.Path)
	}
	return root.Reason
}
