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

// diskFree measures free space; tests substitute it to fix the reading.
var diskFree = diskspace.Free

// newDiskCmd builds the `auto disk` namespace (SPEC-DISK-001).
func newDiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "disk",
		Short:         "Read the disk budget of a shared machine and gate heavy steps on it",
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
// measures free space on the file system that holds the project.
func loadDiskContext(cmd *cobra.Command, flags diskFlags, jsonMode bool) (diskContext, error) {
	dir := flags.dir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return diskContext{}, diskFail(cmd, jsonMode, 1, "disk_project_invalid", fmt.Sprintf("disk: current directory is unavailable: %v", err), nil)
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
		return diskContext{}, diskFail(cmd, jsonMode, 1, "disk_project_invalid", fmt.Sprintf("disk: project directory %s: %v", dir, err), nil)
	}
	harness, err := config.LoadPreview(abs)
	if err != nil {
		return diskContext{}, diskFail(cmd, jsonMode, 1, "disk_config_invalid", "disk: "+err.Error(), nil)
	}
	space, err := diskFree(abs)
	if errors.Is(err, diskspace.ErrUnsupported) {
		return diskContext{}, diskFail(cmd, jsonMode, 1, "disk_unsupported", "disk: "+err.Error(), nil)
	}
	if err != nil {
		return diskContext{}, diskFail(cmd, jsonMode, 1, "disk_measure_failed", fmt.Sprintf("disk: free space of %s cannot be measured: %v", abs, err), nil)
	}
	return diskContext{projectDir: abs, disk: harness.Disk.Effective(), space: space}, nil
}

// diskExit is a disk command failure whose line is already on stderr.
type diskExit struct {
	code int
	line string
}

func (e *diskExit) Error() string { return e.line }

// ExitCode satisfies exitCoder.
func (e *diskExit) ExitCode() int { return e.code }

// diskFail ends a disk command with code after writing line, alone, to
// stderr, and in JSON mode the error envelope to stdout as well. The exact
// line is the contract (REQ-DISK-CHECK-02, REQ-DISK-FREE-02), so it must not
// reach the "Error: " printer of runRootCommand: like harnessFailure, the
// error travels as a jsonFatalError, the already-reported channel that
// printer skips, and exitCodeForError still finds the code by unwrapping it.
func diskFail(cmd *cobra.Command, jsonMode bool, code int, errCode, line string, data any) error {
	line = terminalSafe(line)
	fmt.Fprintln(cmd.ErrOrStderr(), line)
	exit := &diskExit{code: code, line: line}
	if !jsonMode {
		return &jsonFatalError{cause: exit}
	}
	if data == nil {
		data = map[string]any{}
	}
	return writeJSONResultAndExit(cmd, jsonStatusError, exit, errCode, data, nil, nil)
}
