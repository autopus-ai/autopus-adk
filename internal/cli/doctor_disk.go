package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskDoctorRow is the doctor reading of free space against the floor.
type diskDoctorRow struct {
	ok     bool
	line   string
	fields map[string]string
}

// diagnoseDisk measures free space on the file system holding dir against
// disk.min_free_gb. A missing measurement is a warning, never a failure.
func diagnoseDisk(dir string, cfg *config.HarnessConfig) diskDoctorRow {
	floor := cfg.Disk.Effective().FloorBytes()
	fields := map[string]string{"floor_bytes": strconv.FormatInt(floor, 10)}
	space, err := diskFree(dir)
	if errors.Is(err, diskspace.ErrUnsupported) {
		fields["reason"] = "unsupported_os"
		return diskDoctorRow{line: "disk: " + err.Error(), fields: fields}
	}
	if err != nil {
		fields["reason"] = "measurement_failed"
		return diskDoctorRow{line: fmt.Sprintf("disk: free space measurement failed: %v", err), fields: fields}
	}
	fields["free_bytes"] = strconv.FormatInt(space.AvailBytes, 10)
	fields["mount"] = space.Mount
	if space.AvailBytes >= floor {
		return diskDoctorRow{ok: true, fields: fields, line: fmt.Sprintf("disk: %s GiB free on %s (floor %s GiB)",
			diskspace.GiBString(space.AvailBytes), space.Mount, diskspace.GiBString(floor))}
	}
	return diskDoctorRow{fields: fields, line: fmt.Sprintf(
		"disk: %s GiB free on %s is below the %s GiB floor (disk.min_free_gb); run 'auto disk status'",
		diskspace.GiBString(space.AvailBytes), space.Mount, diskspace.GiBString(floor))}
}

// checkDiskText prints the Disk section of `auto doctor` (REQ-DISK-DOC-01).
// It is advisory like checkContextWeight: the doctor verdict is about the
// harness installation, and the gate for a heavy step is `auto disk check`,
// so callers MUST NOT flip allOK on it.
func checkDiskText(w io.Writer, dir string, cfg *config.HarnessConfig) {
	tui.SectionHeader(w, "Disk")
	row := diagnoseDisk(dir, cfg)
	if row.ok {
		tui.OK(w, terminalSafe(row.line))
		return
	}
	tui.SKIP(w, terminalSafe(row.line))
}

// collectDiskCheck mirrors checkDiskText into the JSON report as
// doctor.disk.free_space. A warn check never flips the envelope status.
func (r *doctorJSONReport) collectDiskCheck(dir string, cfg *config.HarnessConfig) {
	row := diagnoseDisk(dir, cfg)
	check := jsonCheck{ID: "doctor.disk.free_space", Severity: "info", Status: "pass", Detail: row.line, Fields: row.fields}
	if !row.ok {
		check.Severity, check.Status = "warning", "warn"
	}
	r.checks = append(r.checks, check)
}
