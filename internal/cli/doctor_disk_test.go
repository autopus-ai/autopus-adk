package cli

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

// diskDoctorProject saves a harness config with a 45 GiB floor (S8).
func diskDoctorProject(t *testing.T) (string, *config.HarnessConfig) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("disk-doctor")
	floor := 45
	cfg.Disk.MinFreeGB = &floor
	require.NoError(t, config.Save(dir, cfg))
	return dir, cfg
}

// doctorResultBox is the verdict box that closes a text doctor run.
func doctorResultBox(t *testing.T, out string) string {
	t.Helper()
	i := strings.LastIndex(out, "╭")
	require.GreaterOrEqual(t, i, 0, "the doctor prints a result box")
	return out[i:]
}

// S8: the text rows are exact, sit under a Disk section header, and leave the
// verdict of the result box unchanged.
func TestDoctorDisk_TextRowIsExactAndAdvisory(t *testing.T) {
	dir, _ := diskDoctorProject(t)
	runDoctor := func(free float64) string {
		withDiskFree(t, gib(free), diskDataMount)
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		require.NoError(t, runDoctorText(cmd, doctorOptions{dir: dir}))
		return buf.String()
	}

	low, high := runDoctor(9.0), runDoctor(100.0)

	warn := "  [WARN] disk: 9.0 GiB free on /System/Volumes/Data is below the 45.0 GiB floor (disk.min_free_gb); run 'auto disk status'\n"
	ok := "  [OK] disk: 100.0 GiB free on /System/Volumes/Data (floor 45.0 GiB)\n"
	assert.Regexp(t, `\nDisk ─+\n`+regexp.QuoteMeta(warn), low)
	assert.Regexp(t, `\nDisk ─+\n`+regexp.QuoteMeta(ok), high)
	assert.NotContains(t, low, ok)
	assert.Equal(t, doctorResultBox(t, low), doctorResultBox(t, high), "the disk row must not change the verdict")
}

// S8 JSON: doctor.disk.free_space carries status, severity, and the three
// fields, and never flips the envelope status.
func TestDoctorDisk_JSONCheckIsAdvisory(t *testing.T) {
	_, cfg := diskDoctorProject(t)
	for _, tc := range []struct {
		free             float64
		status, severity string
	}{
		{9.0, "warn", "warning"},
		{100.0, "pass", "info"},
	} {
		withDiskFree(t, gib(tc.free), diskDataMount)
		report := doctorJSONReport{status: jsonStatusOK}

		report.collectDiskCheck(t.TempDir(), cfg)

		require.Len(t, report.checks, 1)
		check := report.checks[0]
		assert.Equal(t, "doctor.disk.free_space", check.ID)
		assert.Equal(t, tc.status, check.Status)
		assert.Equal(t, tc.severity, check.Severity)
		assert.Equal(t, map[string]string{
			"free_bytes":  strconv.FormatInt(gib(tc.free), 10),
			"floor_bytes": "48318382080",
			"mount":       diskDataMount,
		}, check.Fields)
		assert.Equal(t, jsonStatusOK, report.status)
	}
	assert.Equal(t, "9663676416", strconv.FormatInt(gib(9.0), 10))
}

// S8 JSON end to end: data.overall_ok and the envelope status are the same
// whether free space is under or over the floor.
func TestDoctorDisk_JSONOverallUnchanged(t *testing.T) {
	dir, _ := diskDoctorProject(t)
	runDoctor := func(free float64) (string, bool, string) {
		withDiskFree(t, gib(free), diskDataMount)
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		require.NoError(t, runDoctorJSON(cmd, doctorOptions{dir: dir}))
		var envelope struct {
			Status string `json:"status"`
			Checks []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"checks"`
			Data struct {
				OverallOK bool `json:"overall_ok"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(buf.Bytes(), &envelope))
		disk := ""
		for _, check := range envelope.Checks {
			if check.ID == "doctor.disk.free_space" {
				disk = check.Status
			}
		}
		return envelope.Status, envelope.Data.OverallOK, disk
	}

	lowStatus, lowOK, lowDisk := runDoctor(9.0)
	highStatus, highOK, highDisk := runDoctor(100.0)

	assert.Equal(t, "warn", lowDisk)
	assert.Equal(t, "pass", highDisk)
	assert.Equal(t, highStatus, lowStatus)
	assert.Equal(t, highOK, lowOK)
}

// REQ-DISK-FREE-02: where free space cannot be measured, the doctor row is a
// WARN with the same text as the disk commands, not a measurement.
func TestDoctorDisk_UnsupportedOSWarns(t *testing.T) {
	_, cfg := diskDoctorProject(t)
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) { return diskspace.Space{}, diskspace.UnsupportedOn("windows") })

	var buf bytes.Buffer
	checkDiskText(&buf, t.TempDir(), cfg)
	assert.Contains(t, buf.String(), "  [WARN] disk: free space measurement is not supported on windows\n")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectDiskCheck(t.TempDir(), cfg)
	require.Len(t, report.checks, 1)
	assert.Equal(t, "warn", report.checks[0].Status)
	assert.Equal(t, "disk: free space measurement is not supported on windows", report.checks[0].Detail)
	assert.Equal(t, map[string]string{"floor_bytes": "48318382080"}, report.checks[0].Fields)
	assert.Equal(t, jsonStatusOK, report.status)
}
