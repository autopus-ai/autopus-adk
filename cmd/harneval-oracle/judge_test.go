//go:build unix

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// canonicalDir is a fresh temporary directory at its canonical path, as the
// runner passes every root (t.TempDir on macOS lies below the /var symlink).
func canonicalDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileBundle(path string, expected []byte) input {
	return input{SchemaVersion: InputSchema, TaskID: "GT-AGENT-X01", ArtifactExit: intPtr(0), Stdout: []byte("3\n"),
		Assertions: []assertion{
			{ID: "exit", Kind: KindExitCode, ExitCode: intPtr(0)},
			{ID: "stdout", Kind: KindStdout, Expected: bytesPtr([]byte("3\n"))},
			{ID: "result", Kind: KindFile, Path: path, Expected: bytesPtr(expected)},
		}}
}

func TestJudge_RegularOutputsMatchingEveryExpectation_AllPass(t *testing.T) {
	t.Parallel()
	// Given
	out := canonicalDir(t)
	write(t, filepath.Join(out, "nested", "result.txt"), []byte("3\n"))
	// When
	got := judge(fileBundle("nested/result.txt", []byte("3\n")), out)
	// Then
	want := []assertionResult{{"exit", true}, {"stdout", true}, {"result", true}}
	if got.OutputCheck != CheckOK || !equalResults(got.Assertions, want) || got.SchemaVersion != ResultSchema {
		t.Fatalf("judge = %+v, want ok with %v", got, want)
	}
}

// TestJudge_OutputStructureThreats_AreRejectedOrTooLarge is the REQ-HR-10
// output self-check: every threat type is link_rejected, an oversized file is
// too_large, and only a regular single-link file is ok.
func TestJudge_OutputStructureThreats_AreRejectedOrTooLarge(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		path  string
		setup func(t *testing.T, out, outside string)
		want  string
	}{
		{"regular file", "result.txt", func(t *testing.T, out, _ string) {
			write(t, filepath.Join(out, "result.txt"), []byte("3\n"))
		}, CheckOK},
		{"symlink inside the root", "result.txt", func(t *testing.T, out, _ string) {
			write(t, filepath.Join(out, "real.txt"), []byte("3\n"))
			symlink(t, "real.txt", filepath.Join(out, "result.txt"))
		}, CheckLinkRejected},
		{"symlink out of the root", "result.txt", func(t *testing.T, out, outside string) {
			write(t, filepath.Join(outside, "expected.txt"), []byte("3\n"))
			symlink(t, filepath.Join(outside, "expected.txt"), filepath.Join(out, "result.txt"))
		}, CheckLinkRejected},
		{"directory symlink", "sub/result.txt", func(t *testing.T, out, _ string) {
			write(t, filepath.Join(out, "real", "result.txt"), []byte("3\n"))
			symlink(t, "real", filepath.Join(out, "sub"))
		}, CheckLinkRejected},
		{"hard link", "result.txt", func(t *testing.T, out, outside string) {
			write(t, filepath.Join(outside, "expected.txt"), []byte("3\n"))
			if err := os.Link(filepath.Join(outside, "expected.txt"), filepath.Join(out, "result.txt")); err != nil {
				t.Fatal(err)
			}
		}, CheckLinkRejected},
		{"fifo", "result.txt", func(t *testing.T, out, _ string) {
			if err := syscall.Mkfifo(filepath.Join(out, "result.txt"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, CheckLinkRejected},
		{"directory in place of the file", "result.txt", func(t *testing.T, out, _ string) {
			write(t, filepath.Join(out, "result.txt", "inner"), []byte("3\n"))
		}, CheckLinkRejected},
		{"file in place of a directory", "sub/result.txt", func(t *testing.T, out, _ string) {
			write(t, filepath.Join(out, "sub"), []byte("3\n"))
		}, CheckLinkRejected},
		{"file over the limit", "result.txt", func(t *testing.T, out, _ string) {
			write(t, filepath.Join(out, "result.txt"), bytes.Repeat([]byte("x"), OutputLimit+1))
		}, CheckTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given
			out, outside := canonicalDir(t), canonicalDir(t)
			tc.setup(t, out, outside)
			// When
			started := time.Now()
			got := judge(fileBundle(tc.path, []byte("3\n")), out)
			// Then
			if got.OutputCheck != tc.want {
				t.Fatalf("output_check = %q, want %q", got.OutputCheck, tc.want)
			}
			if tc.want != CheckOK && len(got.Assertions) != 0 {
				t.Fatalf("a rejected run holds assertions: %v", got.Assertions)
			}
			if time.Since(started) > 5*time.Second {
				t.Fatal("the check blocked")
			}
		})
	}
}

func TestJudge_OutputRootSwappedForALink_IsRejected(t *testing.T) {
	t.Parallel()
	// Given: the root path is a symlink to a real output directory, and another
	// root path runs through a symlinked ancestor
	base := canonicalDir(t)
	write(t, filepath.Join(base, "real", "out", "result.txt"), []byte("3\n"))
	symlink(t, filepath.Join(base, "real", "out"), filepath.Join(base, "out"))
	symlink(t, filepath.Join(base, "real"), filepath.Join(base, "run"))
	// When
	viaLink := judge(fileBundle("result.txt", []byte("3\n")), filepath.Join(base, "out"))
	viaAncestor := judge(fileBundle("result.txt", []byte("3\n")), filepath.Join(base, "run", "out"))
	// Then
	if viaLink.OutputCheck != CheckLinkRejected || viaAncestor.OutputCheck != CheckLinkRejected {
		t.Fatalf("checks = %q and %q, want link_rejected", viaLink.OutputCheck, viaAncestor.OutputCheck)
	}
}

func TestJudge_MissingOutputs_FailOnlyTheirAssertion(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"result.txt", "missing/result.txt"} {
		// Given: an early exit that wrote nothing below the root
		out := canonicalDir(t)
		// When
		got := judge(fileBundle(name, []byte("3\n")), out)
		// Then
		want := []assertionResult{{"exit", true}, {"stdout", true}, {"result", false}}
		if got.OutputCheck != CheckOK || !equalResults(got.Assertions, want) {
			t.Fatalf("%s: judge = %+v, want ok with %v", name, got, want)
		}
	}
	gone := judge(fileBundle("result.txt", []byte("3\n")), filepath.Join(canonicalDir(t), "absent"))
	if gone.OutputCheck != CheckOK || gone.Assertions[2].Passed {
		t.Fatalf("a missing root = %+v, want ok with a failed file assertion", gone)
	}
}

func TestJudge_ForgedSuccessAndWrongStatus_FailTheirAssertions(t *testing.T) {
	t.Parallel()
	out := canonicalDir(t)
	write(t, filepath.Join(out, "result.txt"), []byte("4\n"))
	cases := []struct {
		name   string
		mutate func(*input)
		want   []assertionResult
	}{
		{"forged success text", func(in *input) { in.Stdout = []byte("PASS ok\n") },
			[]assertionResult{{"exit", true}, {"stdout", false}, {"result", false}}},
		{"nonzero exit", func(in *input) { in.ArtifactExit = intPtr(2) },
			[]assertionResult{{"exit", false}, {"stdout", true}, {"result", false}}},
		{"killed by a signal", func(in *input) { in.ArtifactExit = nil },
			[]assertionResult{{"exit", false}, {"stdout", true}, {"result", false}}},
		{"empty stdout", func(in *input) { in.Stdout = nil },
			[]assertionResult{{"exit", true}, {"stdout", false}, {"result", false}}},
	}
	for _, tc := range cases {
		// Given
		bundle := fileBundle("result.txt", []byte("3\n"))
		tc.mutate(&bundle)
		// When
		got := judge(bundle, out)
		// Then
		if got.OutputCheck != CheckOK || !equalResults(got.Assertions, tc.want) {
			t.Fatalf("%s: judge = %+v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestJudge_TimeoutAndStdoutOverflow_CompareNothing(t *testing.T) {
	t.Parallel()
	out := canonicalDir(t)
	write(t, filepath.Join(out, "result.txt"), []byte("3\n"))
	timedOut := fileBundle("result.txt", []byte("3\n"))
	timedOut.TimedOut, timedOut.ArtifactExit = true, nil
	overflow := fileBundle("result.txt", []byte("3\n"))
	overflow.StdoutOverflow = true
	for bundle, want := range map[*input]string{&timedOut: CheckNotChecked, &overflow: CheckTooLarge} {
		got := judge(*bundle, out)
		if got.OutputCheck != want || len(got.Assertions) != 0 || got.TimedOut != bundle.TimedOut {
			t.Fatalf("judge = %+v, want %s without assertions", got, want)
		}
	}
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func equalResults(got, want []assertionResult) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
