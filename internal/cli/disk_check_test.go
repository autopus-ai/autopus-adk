package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/diskspace"
)

const diskS3Section = "disk:\n  min_free_gb: 45\n  reserves: {release: 30, nightly-full: 60}\n"

// diskS3Rows is the ten-row table of acceptance S3.
var diskS3Rows = []struct {
	flags  []string
	free   float64
	code   int
	stdout bool
	line   string
}{
	{nil, 9.0, 3, false, "disk check: insufficient free space: 9.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 36.0 GiB or lower the requirement"},
	{nil, 45.0, 0, true, "disk check: ok: 45.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data"},
	{[]string{"--need", "30"}, 36.0, 3, false, "disk check: insufficient free space: 36.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 9.0 GiB or lower the requirement"},
	{[]string{"--need", "50"}, 52.5, 0, true, "disk check: ok: 52.5 GiB free, 50.0 GiB required (--need) on /System/Volumes/Data"},
	{[]string{"--for", "release"}, 31.0, 3, false, "disk check: insufficient free space: 31.0 GiB free, 45.0 GiB required (disk.min_free_gb) on /System/Volumes/Data; free 14.0 GiB or lower the requirement"},
	{[]string{"--for", "nightly-full"}, 61.0, 0, true, "disk check: ok: 61.0 GiB free, 60.0 GiB required (disk.reserves.nightly-full) on /System/Volumes/Data"},
	{[]string{"--for", "nightly-full"}, 59.9, 3, false, "disk check: insufficient free space: 59.9 GiB free, 60.0 GiB required (disk.reserves.nightly-full) on /System/Volumes/Data; free 0.1 GiB or lower the requirement"},
	{[]string{"--for", "weekly"}, 200.0, 1, false, `disk check: reserve "weekly" is not configured under disk.reserves in autopus.yaml`},
	{[]string{"--need", "0"}, 200.0, 1, false, "disk check: --need must be a positive number of GiB"},
	{[]string{"--need", "30", "--for", "release"}, 200.0, 1, false, "disk check: --need and --for are mutually exclusive"},
}

// S3: every row exits with its code and prints exactly its line on its
// stream, with nothing else on either stream: no "Error: " prefix.
func TestDiskCheck_S3ExitCodesAndLines(t *testing.T) {
	dir := writeDiskProject(t, diskS3Section)
	for _, row := range diskS3Rows {
		withDiskFree(t, gib(row.free), diskDataMount)
		run := runDiskCLI(t, append([]string{"disk", "check", "--dir", dir}, row.flags...)...)

		name := strings.Join(row.flags, " ")
		assert.Equal(t, row.code, run.code, name)
		if row.stdout {
			assert.Equal(t, row.line+"\n", run.stdout, name)
			assert.Empty(t, run.stderr, name)
		} else {
			assert.Equal(t, row.line+"\n", run.stderr, name)
			assert.Empty(t, run.stdout, name)
		}
	}
}

// S3 JSON: the exit codes are identical, ok maps to status ok and exit 3 to
// status error with error.code disk_insufficient, and data carries the
// reading. The stderr line stays, in JSON mode as in text mode.
func TestDiskCheck_S3JSONMode(t *testing.T) {
	dir := writeDiskProject(t, diskS3Section)
	for _, row := range diskS3Rows {
		withDiskFree(t, gib(row.free), diskDataMount)
		run := runDiskCLI(t, append([]string{"disk", "check", "--dir", dir, "--json"}, row.flags...)...)

		name := strings.Join(row.flags, " ")
		require.Equal(t, row.code, run.code, name)
		envelope := decodeDiskEnvelope(t, run.stdout)
		if row.code == 0 {
			assert.Equal(t, "ok", envelope.Status, name)
			assert.Nil(t, envelope.Error, name)
			assert.Empty(t, run.stderr, name)
		} else {
			assert.Equal(t, "error", envelope.Status, name)
			require.NotNil(t, envelope.Error, name)
			assert.Equal(t, row.line, envelope.Error.Message, name)
			assert.Equal(t, row.line+"\n", run.stderr, name)
		}
		if row.code == 1 {
			assert.Equal(t, "disk_invalid_args", envelope.Error.Code, name)
			continue
		}
		if row.code == 3 {
			assert.Equal(t, "disk_insufficient", envelope.Error.Code, name)
		}
		var data map[string]any
		require.NoError(t, json.Unmarshal(envelope.Data, &data), name)
		assert.Equal(t, float64(gib(row.free)), data["free_bytes"], name)
		assert.Equal(t, diskDataMount, data["mount"], name)
		assert.Equal(t, row.code == 0, data["ok"], name)
		assert.Contains(t, row.line, "("+data["source"].(string)+")", name)
		assert.Contains(t, row.line, diskspace.GiBString(int64(data["required_bytes"].(float64)))+" GiB required", name)
	}
}

// REQ-DISK-CHECK-03: --need takes only a positive finite number.
func TestDiskCheck_RejectsNonPositiveNeed(t *testing.T) {
	dir := writeDiskProject(t, "")
	withDiskFree(t, gib(200), diskDataMount)
	for _, need := range []string{"-5", "abc", "NaN", "+Inf", ""} {
		run := runDiskCLI(t, "disk", "check", "--dir", dir, "--need", need)
		assert.Equal(t, 1, run.code, need)
		assert.Equal(t, "disk check: --need must be a positive number of GiB\n", run.stderr, need)
	}
	run := runDiskCLI(t, "disk", "check", "--dir", dir, "--need", "0.5")
	assert.Equal(t, 0, run.code)
	assert.Equal(t, "disk check: ok: 200.0 GiB free, 10.0 GiB required (disk.min_free_gb) on /System/Volumes/Data\n", run.stdout,
		"without a disk section the floor is the 10 GiB default")
}

// S2/REQ-DISK-FREE-02: without a measurement the command exits 1 with the
// single unsupported line, in text and JSON mode.
func TestDiskCheck_UnsupportedOSIsASingleLine(t *testing.T) {
	dir := writeDiskProject(t, "")
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) { return diskspace.Space{}, diskspace.UnsupportedOn("windows") })

	run := runDiskCLI(t, "disk", "check", "--dir", dir)
	assert.Equal(t, 1, run.code)
	assert.Equal(t, "disk: free space measurement is not supported on windows\n", run.stderr)
	assert.Empty(t, run.stdout)

	run = runDiskCLI(t, "disk", "check", "--dir", dir, "--json")
	assert.Equal(t, 1, run.code)
	assert.Equal(t, "disk: free space measurement is not supported on windows\n", run.stderr)
	assert.Equal(t, "disk_unsupported", decodeDiskEnvelope(t, run.stdout).Error.Code)
}

// A failed measurement and an invalid disk section are exit 1, never 3.
func TestDiskCheck_MeasurementAndConfigFailuresExitOne(t *testing.T) {
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) {
		return diskspace.Space{}, errors.New("statfs: input/output error")
	})
	run := runDiskCLI(t, "disk", "check", "--dir", writeDiskProject(t, ""))
	assert.Equal(t, 1, run.code)
	assert.Contains(t, run.stderr, "cannot be measured: statfs: input/output error")

	withDiskFree(t, gib(200), diskDataMount)
	run = runDiskCLI(t, "disk", "check", "--dir", writeDiskProject(t, "disk:\n  min_free_gb: -1\n"))
	assert.Equal(t, 1, run.code)
	assert.Contains(t, run.stderr, "disk.min_free_gb must be >= 0")
	assert.Equal(t, 1, strings.Count(run.stderr, "\n"), "one stderr line")
}

func diskFileSHA(t *testing.T, path string) [32]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return sha256.Sum256(data)
}

// S10: config.Load rewrites the platform alias claude; the disk commands load
// through LoadPreview and leave autopus.yaml byte-identical.
func TestDiskCommands_NeverRewriteAutopusYAML(t *testing.T) {
	body := []byte("mode: full\nproject_name: disk\nplatforms:\n  - claude\n")
	dir, control := t.TempDir(), t.TempDir()
	for _, d := range []string{dir, control} {
		require.NoError(t, os.WriteFile(filepath.Join(d, "autopus.yaml"), body, 0o600))
	}
	path := filepath.Join(dir, "autopus.yaml")
	before := diskFileSHA(t, path)
	withDiskFree(t, gib(200), diskDataMount)

	run := runDiskCLI(t, "disk", "check", "--dir", dir)

	require.Equal(t, 0, run.code, run.stderr)
	assert.Equal(t, before, diskFileSHA(t, path))
	_, err := config.Load(control)
	require.NoError(t, err)
	assert.NotEqual(t, before, diskFileSHA(t, filepath.Join(control, "autopus.yaml")), "the control proves Load writes")
}
