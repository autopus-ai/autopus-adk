package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

const diskDataMount = "/System/Volumes/Data"

// diskResult is one `auto ...` run mapped to its process outcome the way
// Execute maps it: the exit code, and stderr as runRootCommand leaves it.
type diskResult struct {
	code           int
	stdout, stderr string
}

func runDiskCLI(t *testing.T, args ...string) diskResult {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	code, _ := runRootCommand(context.Background(), root, &stderr)
	return diskResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// withDiskFree pins the free-space reading of every disk command and of the
// doctor disk row for the rest of the test.
func withDiskFree(t *testing.T, availBytes int64, mount string) {
	t.Helper()
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) {
		return diskspace.Space{AvailBytes: availBytes, BlockUnit: 4096, Mount: mount}, nil
	})
}

func withDiskFreeFunc(t *testing.T, free func(string) (diskspace.Space, error)) {
	t.Helper()
	previous := diskFree
	diskFree = free
	t.Cleanup(func() { diskFree = previous })
}

// gib converts a fractional GiB count to bytes at run time, as the tables of
// acceptance.md state free space.
func gib(v float64) int64 { return int64(v * float64(diskspace.GiB)) }

// writeDiskProject creates a project whose autopus.yaml holds the given disk
// section below a minimal valid harness config.
func writeDiskProject(t *testing.T, diskSection string) string {
	t.Helper()
	dir := t.TempDir()
	body := "mode: full\nproject_name: disk\nplatforms:\n  - claude-code\n" + diskSection
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(body), 0o600))
	return dir
}

// diskEnvelope is the shared JSON envelope with data left raw.
type diskEnvelope struct {
	Status string `json:"status"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Warnings []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"warnings"`
	Data json.RawMessage `json:"data"`
}

func decodeDiskEnvelope(t *testing.T, stdout string) diskEnvelope {
	t.Helper()
	var envelope diskEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope), stdout)
	return envelope
}
