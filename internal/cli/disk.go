package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// exitCodeDiskInsufficient is the `auto disk check` status when free space is
// below the requirement (REQ-DISK-CHECK-02). A usage, configuration, or
// measurement failure stays exit 1, so a caller can tell the two apart.
const exitCodeDiskInsufficient = 3

// JSON error codes shared by the disk commands.
const (
	diskCodeProjectInvalid    = "disk_project_invalid"
	diskCodeConfigInvalid     = "disk_config_invalid"
	diskCodeMeasurementFailed = "disk_measurement_failed"
)

// Seams of the disk commands (REQ-DISK-STATUS-04): tests fix free space, the
// clock, the uid, the scratchpad root, every external program, the process
// lister, and the moment between a trim's scan and each removal.
var (
	diskFree         = diskspace.Free
	diskNow          = time.Now
	diskUID          = os.Getuid
	diskScratchpad   = diskspace.ProductionScratchpadRoot
	diskRunner       = diskspace.Runner(diskspace.ExecRunner)
	diskLister       = diskspace.SystemProcessLister
	diskBeforeRemove func(rule, rel string)
)

// newDiskCmd builds the `auto disk` namespace (SPEC-DISK-001).
func newDiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "disk",
		Short:         "Read the disk budget of a shared machine, gate heavy steps on it, and trim the Go build cache",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newDiskStatusCmd(), newDiskCheckCmd(), newDiskTrimCmd())
	return cmd
}

// diskFlags are the flags every disk command shares.
type diskFlags struct {
	dir     string
	jsonOut bool
	format  string
}

func (f *diskFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.dir, "dir", "", "Project directory (default: current directory)")
	addJSONFlags(cmd, &f.jsonOut, &f.format)
}

// diskContext is what every disk command reads before its own work.
type diskContext struct {
	projectDir string
	disk       config.DiskEffective
	space      diskspace.Space
}

// loadDiskContext resolves the project directory, loads its disk budget
// through LoadPreview, which never writes autopus.yaml (REQ-DISK-CFG-03), and
// measures free space on the file system that holds the project. sub names
// the command in its failure lines.
func loadDiskContext(cmd *cobra.Command, flags diskFlags, jsonMode bool, sub string) (diskContext, error) {
	dir := flags.dir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return diskContext{}, diskFail(cmd, jsonMode, 1, diskCodeProjectInvalid, fmt.Sprintf("disk %s: current directory is unavailable: %v", sub, err), nil)
		}
		dir = cwd
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		var info os.FileInfo
		if info, err = os.Stat(abs); err == nil && !info.IsDir() {
			err = errors.New("not a directory")
		}
	}
	if err != nil {
		return diskContext{}, diskFail(cmd, jsonMode, 1, diskCodeProjectInvalid, fmt.Sprintf("disk %s: project directory %s: %v", sub, dir, err), nil)
	}
	harness, err := config.LoadPreview(abs)
	if err != nil {
		return diskContext{}, diskFail(cmd, jsonMode, 1, diskCodeConfigInvalid, fmt.Sprintf("disk %s: %v", sub, err), nil)
	}
	space, err := diskFree(abs)
	if errors.Is(err, diskspace.ErrUnsupported) {
		return diskContext{}, diskFail(cmd, jsonMode, 1, diskCodeMeasurementFailed, "disk: "+err.Error(), nil)
	}
	if err != nil {
		return diskContext{}, diskFail(cmd, jsonMode, 1, diskCodeMeasurementFailed, fmt.Sprintf("disk %s: free space measurement failed: %v", sub, err), nil)
	}
	return diskContext{projectDir: abs, disk: harness.Disk.Effective(), space: space}, nil
}

// diskExit is a disk command failure whose diagnostic is already written.
type diskExit struct {
	code int
	line string
}

func (e *diskExit) Error() string { return e.line }

// ExitCode satisfies exitCoder.
func (e *diskExit) ExitCode() int { return e.code }

func (e *diskExit) alreadyReported() {}

// diskFail ends a disk command with code. In text mode it writes line, alone,
// to stderr; in JSON mode it writes only the error envelope, carrying line as
// its message, so stderr stays empty. Either way the command reports its own
// diagnostic, so runRootCommand adds no "Error: " line, and exitCodeForError
// reads code through the jsonFatalError the envelope path wraps it in
// (REQ-DISK-CHECK-02, REQ-DISK-FREE-02).
func diskFail(cmd *cobra.Command, jsonMode bool, code int, errCode, line string, data any, warnings ...jsonMessage) error {
	line = terminalSafe(line)
	exit := &diskExit{code: code, line: line}
	if !jsonMode {
		fmt.Fprintln(cmd.ErrOrStderr(), line)
		return exit
	}
	if data == nil {
		data = map[string]any{}
	}
	return writeJSONResultAndExit(cmd, jsonStatusError, exit, errCode, data, warnings, nil)
}

// diskSurvey is one scan of both roots and the plan built from it: what
// `auto disk trim` acts on and `auto disk status` previews.
type diskSurvey struct {
	dc        diskContext
	now       time.Time
	uid       int
	cacheRoot diskspace.Root
	workRoot  diskspace.Root
	cache     diskspace.CacheScan
	work      diskspace.WorkDirScan
	ci        diskspace.CIStatus
	plan      diskspace.Plan
}

func surveyDisk(ctx context.Context, dc diskContext, ciOverride string) diskSurvey {
	s := diskSurvey{dc: dc, now: diskNow(), uid: diskUID()}
	home, _ := os.UserHomeDir()
	locator := diskspace.Locator{Run: diskRunner, Home: home, ProjectDir: dc.projectDir, UID: s.uid}
	s.cacheRoot, s.workRoot = locator.Cache(ctx), locator.WorkDir()
	s.cache = diskspace.ScanCache(scannable(s.cacheRoot))
	s.work = diskspace.ScanWorkDirs(scannable(s.workRoot))
	s.ci = diskspace.DetectCI(ctx, diskLister(diskRunner), dc.disk.CIProcessNames, ciOverride)
	s.plan = diskspace.BuildPlan(diskspace.PlanInput{Now: s.now, FreeBytes: dc.space.AvailBytes, Config: dc.disk,
		CI: s.ci, UID: s.uid, Cache: s.cache, WorkDirs: s.work})
	return s
}

// scannable lets the scan read a root refused only because another uid owns
// it, so the report counts the candidates a trim would skip there. Any other
// refused root, which can be / or $HOME, is never walked.
func scannable(root diskspace.Root) diskspace.Root {
	if root.Refused && root.Reason == diskspace.ReasonNotOwned {
		root.Reason, root.Refused = "", false
	}
	return root
}

// refused is the first refused root, the cache before the work dir.
func (s diskSurvey) refused() *diskspace.Root {
	for _, root := range []diskspace.Root{s.cacheRoot, s.workRoot} {
		if root.Refused {
			return &root
		}
	}
	return nil
}

// warnings are the REQ-DISK-CFG-04 lines and the CI detection notice of
// REQ-DISK-TRIM-08, which an inactive trim has no use for.
func (s diskSurvey) warnings() []jsonMessage {
	var out []jsonMessage
	for _, idle := range s.dc.disk.BelowRecommendedIdle() {
		out = append(out, jsonMessage{Code: "idle_threshold_below_120", Message: fmt.Sprintf(
			"disk trim: warning: %s=%d is below 120 and may delete an entry a running build has already looked up", idle.Key, idle.Minutes)})
	}
	if s.ci.State == diskspace.CIUnknown && !s.plan.Inactive {
		out = append(out, jsonMessage{Code: "ci_detection_unavailable", Message: fmt.Sprintf(
			"disk trim: ci detection unavailable (%s); treating CI as running", s.ci.Reason)})
	}
	return out
}

// writeDiskWarnings prints warnings to stderr in text mode; JSON mode carries
// them in the envelope instead.
func writeDiskWarnings(cmd *cobra.Command, jsonMode bool, warnings []jsonMessage) {
	if jsonMode {
		return
	}
	for _, warning := range warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), terminalSafe(warning.Message))
	}
}
