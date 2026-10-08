package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func intPtr(value int) *int { return &value }

func bytesPtr(value []byte) *[]byte { return &value }

// bundleJSON is a stdin bundle with an exit and a stdout assertion only, so it
// needs no output file and runs on every platform.
func bundleJSON(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	document := map[string]any{
		"schema_version": InputSchema, "task_id": "GT-AGENT-X01", "artifact_exit": 0, "timed_out": false,
		"stdout": []byte("3\n"), "stdout_overflow": false,
		"assertions": []map[string]any{
			{"id": "exit", "kind": KindExitCode, "exit_code": 0},
			{"id": "stdout", "kind": KindStdout, "expected": []byte("3\n")},
		},
	}
	if edit != nil {
		edit(document)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func runOracle(t *testing.T, stdin []byte, args ...string) (int, string, string) {
	t.Helper()
	resultDir := t.TempDir()
	if args == nil {
		args = []string{"--task", "GT-AGENT-X01", "--outputs", filepath.Join(resultDir, "out"), "--result", resultDir}
	}
	var stderr bytes.Buffer
	code := run(args, bytes.NewReader(stdin), &stderr)
	return code, resultDir, stderr.String()
}

func TestRun_ValidBundle_WritesOneExclusiveResultDocument(t *testing.T) {
	t.Parallel()
	// Given
	stdin := bundleJSON(t, nil)
	// When
	code, dir, stderr := runOracle(t, stdin)
	// Then
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	path := filepath.Join(dir, ResultFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":"harness_oracle_result.v1","task_id":"GT-AGENT-X01","output_check":"ok",` +
		`"assertions":[{"id":"exit","passed":true},{"id":"stdout","passed":true}],"artifact_exit":0,"timed_out":false}` + "\n"
	if string(data) != want {
		t.Fatalf("result = %s, want %s", data, want)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("result mode = %v, want 0600", info.Mode().Perm())
	}
	again := run([]string{"--task", "GT-AGENT-X01", "--outputs", filepath.Join(dir, "out"), "--result", dir},
		bytes.NewReader(bundleJSON(t, func(m map[string]any) { m["artifact_exit"] = 1 })), &bytes.Buffer{})
	if after, _ := os.ReadFile(path); again != 1 || string(after) != want {
		t.Fatalf("a second run exited %d and left %s; want 1 and the first result", again, after)
	}
}

func TestRun_TimedOutBundle_IsNotCheckedWithEmptyAssertions(t *testing.T) {
	t.Parallel()
	stdin := bundleJSON(t, func(m map[string]any) { m["timed_out"], m["artifact_exit"] = true, nil })
	code, dir, stderr := runOracle(t, stdin)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ResultFile))
	if !strings.Contains(string(data), `"output_check":"not_checked","assertions":[],"artifact_exit":null,"timed_out":true`) {
		t.Fatalf("result = %s", data)
	}
}

func TestRun_InvalidArgumentsOrBundle_LeaveNoResult(t *testing.T) {
	t.Parallel()
	file := map[string]any{"id": "f", "kind": KindFile, "path": "out.txt", "expected": []byte("x")}
	assertionsWith := func(items ...map[string]any) func(map[string]any) {
		return func(m map[string]any) { m["assertions"] = items }
	}
	cases := map[string][]byte{
		"not json":            []byte("{"),
		"trailing data":       append(bundleJSON(t, nil), []byte(" {}")...),
		"unknown field":       bundleJSON(t, func(m map[string]any) { m["verdict"] = "pass" }),
		"wrong schema":        bundleJSON(t, func(m map[string]any) { m["schema_version"] = "v0" }),
		"other task":          bundleJSON(t, func(m map[string]any) { m["task_id"] = "GT-AGENT-X02" }),
		"timeout with status": bundleJSON(t, func(m map[string]any) { m["timed_out"] = true }),
		"stdout over limit":   bundleJSON(t, func(m map[string]any) { m["stdout"] = bytes.Repeat([]byte("x"), OutputLimit+1) }),
		"no assertion":        bundleJSON(t, assertionsWith()),
		"repeated id": bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": KindExitCode, "exit_code": 0},
			map[string]any{"id": "a", "kind": KindExitCode, "exit_code": 0})),
		"malformed id":       bundleJSON(t, assertionsWith(map[string]any{"id": "../x", "kind": KindExitCode, "exit_code": 0})),
		"unknown kind":       bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": "regex", "expected": []byte("x")})),
		"exit without code":  bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": KindExitCode})),
		"exit with expected": bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": KindExitCode, "exit_code": 0, "expected": []byte("x")})),
		"stdout with path":   bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": KindStdout, "path": "x", "expected": []byte("x")})),
		"stdout no expected": bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": KindStdout})),
		"file with code":     bundleJSON(t, assertionsWith(map[string]any{"id": "f", "kind": KindFile, "path": "x", "exit_code": 0, "expected": []byte("x")})),
		"expected too large": bundleJSON(t, assertionsWith(map[string]any{"id": "a", "kind": KindStdout, "expected": bytes.Repeat([]byte("x"), OutputLimit+1)})),
	}
	for _, name := range []string{"", "/abs", "../up", "a/../b", "a/./b", "a//b", "a/", ".", `a\b`} {
		bad := map[string]any{"id": "f", "kind": KindFile, "path": name, "expected": []byte("x")}
		cases["file path "+name] = bundleJSON(t, assertionsWith(bad))
	}
	cases["valid control"] = bundleJSON(t, assertionsWith(file))
	for name, stdin := range cases {
		code, dir, _ := runOracle(t, stdin)
		_, err := os.Stat(filepath.Join(dir, ResultFile))
		if name == "valid control" {
			if code != 0 || err != nil {
				t.Fatalf("%s: exit %d, result error %v", name, code, err)
			}
			continue
		}
		if code != 1 || !os.IsNotExist(err) {
			t.Errorf("%s: exit %d, result error %v; want 1 and no result", name, code, err)
		}
	}
	for _, args := range [][]string{{}, {"--task", "GT-AGENT-X01", "--outputs", "rel", "--result", "/tmp"},
		{"--task", "", "--outputs", "/a", "--result", "/b"}, {"--bogus"}, {"--task", "x", "--outputs", "/a", "--result", "/b", "extra"}} {
		if code, _, _ := runOracle(t, bundleJSON(t, nil), args...); code != 1 {
			t.Errorf("args %q: exit %d, want 1", args, code)
		}
	}
}

func TestRun_UnwritableResultDirectory_ExitsOne(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "absent")
	code, _, stderr := runOracle(t, bundleJSON(t, nil), "--task", "GT-AGENT-X01", "--outputs", missing, "--result", missing)
	if code != 1 || !strings.Contains(stderr, "result:") {
		t.Fatalf("exit %d (%s), want 1 with a result error", code, stderr)
	}
}

func TestRun_OversizedStdin_IsRejectedBeforeDecoding(t *testing.T) {
	t.Parallel()
	code, _, stderr := runOracle(t, bytes.Repeat([]byte(" "), maxInput+1))
	if code != 1 || !strings.Contains(stderr, "exceeds") {
		t.Fatalf("exit %d (%s), want 1 for an oversized bundle", code, stderr)
	}
}
