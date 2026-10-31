package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// diskFree measures free space; tests substitute it to fix the reading.
var diskFree = diskspace.Free

// newDiskCmd builds the `auto disk` namespace (SPEC-DISK-001).
func newDiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "disk",
		Short:         "Read the disk budget of a shared machine, gate heavy steps on it, and trim the Go build cache",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newDiskCheckCmd())
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
func diskFail(cmd *cobra.Command, jsonMode bool, code int, errCode, line string, data any) error {
	line = terminalSafe(line)
	exit := &diskExit{code: code, line: line}
	if !jsonMode {
		fmt.Fprintln(cmd.ErrOrStderr(), line)
		return exit
	}
	if data == nil {
		data = map[string]any{}
	}
	return writeJSONResultAndExit(cmd, jsonStatusError, exit, errCode, data, nil, nil)
}
