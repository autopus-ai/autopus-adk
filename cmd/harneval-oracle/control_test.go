package main

import (
	"reflect"
	"testing"
)

// refusalBundle is a task whose own assertions expect a refusal (exit 1,
// empty stdout) with a positive control over a valid input that must exit 0
// and print "ok\n". The output root is never read: no file assertion.
func refusalBundle(controlExit *int, controlStdout string) input {
	return input{SchemaVersion: InputSchema, TaskID: "GT-AGENT-X01", ArtifactExit: intPtr(1), Stdout: []byte{},
		Assertions: []assertion{
			{ID: "exit", Kind: KindExitCode, ExitCode: intPtr(1)},
			{ID: "stdout", Kind: KindStdout, Expected: bytesPtr([]byte{})},
		},
		PositiveControl: &control{ArtifactExit: controlExit, Stdout: []byte(controlStdout),
			ExpectedStdout: bytesPtr([]byte("ok\n"))}}
}

func TestJudge_PositiveControl_ReportsItsTwoAssertionsAfterTheTasks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		bundle input
		want   []assertionResult
	}{
		{"a fix that refuses only the bad input passes both", refusalBundle(intPtr(0), "ok\n"),
			[]assertionResult{{"exit", true}, {"stdout", true}, {ControlExitID, true}, {ControlStdoutID, true}}},
		{"a fix that refuses every input fails the control", refusalBundle(intPtr(1), ""),
			[]assertionResult{{"exit", true}, {"stdout", true}, {ControlExitID, false}, {ControlStdoutID, false}}},
		{"a control killed by a signal has no exit status", refusalBundle(nil, "ok\n"),
			[]assertionResult{{"exit", true}, {"stdout", true}, {ControlExitID, false}, {ControlStdoutID, true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// When
			got := judge(tc.bundle, t.TempDir()+"/absent")
			// Then
			if got.OutputCheck != CheckOK || got.TimedOut || !reflect.DeepEqual(got.Assertions, tc.want) ||
				got.ArtifactExit == nil || *got.ArtifactExit != 1 {
				t.Fatalf("judge = %+v, want ok, the task's exit status and %v", got, tc.want)
			}
		})
	}
}

// TestJudge_PositiveControl_TimeoutOrOverflowComparesNothing: a control that
// timed out makes the run not_checked, one whose stdout overflowed too_large,
// exactly as the task's own invocation would.
func TestJudge_PositiveControl_TimeoutOrOverflowComparesNothing(t *testing.T) {
	t.Parallel()
	timedOut := refusalBundle(nil, "")
	timedOut.PositiveControl.TimedOut = true
	overflow := refusalBundle(intPtr(0), "ok\n")
	overflow.PositiveControl.StdoutOverflow = true
	for name, tc := range map[string]struct {
		bundle   input
		check    string
		timedOut bool
	}{"timeout": {timedOut, CheckNotChecked, true}, "overflow": {overflow, CheckTooLarge, false}} {
		got := judge(tc.bundle, t.TempDir())
		if got.OutputCheck != tc.check || got.TimedOut != tc.timedOut || len(got.Assertions) != 0 {
			t.Fatalf("%s: judge = %+v, want %s with timed_out %v and no assertion", name, got, tc.check, tc.timedOut)
		}
	}
}

func TestDecodeInput_PositiveControl_IsStrict(t *testing.T) {
	t.Parallel()
	valid := func(document map[string]any) {
		document["positive_control"] = map[string]any{"artifact_exit": 0, "timed_out": false, "stdout": []byte("ok\n"),
			"stdout_overflow": false, "expected_stdout": []byte("ok\n")}
	}
	bundle, err := decodeInput(bundleJSON(t, valid), "GT-AGENT-X01")
	if err != nil || bundle.PositiveControl == nil || string(*bundle.PositiveControl.ExpectedStdout) != "ok\n" {
		t.Fatalf("decodeInput = %+v, %v; want the control decoded", bundle, err)
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown control field": func(document map[string]any) {
			valid(document)
			document["positive_control"].(map[string]any)["path"] = "x"
		},
		"no expected stdout": func(document map[string]any) {
			valid(document)
			delete(document["positive_control"].(map[string]any), "expected_stdout")
		},
		"a timed-out control with an exit status": func(document map[string]any) {
			valid(document)
			document["positive_control"].(map[string]any)["timed_out"] = true
		},
		"a task assertion in the reserved namespace": func(document map[string]any) {
			valid(document)
			document["assertions"].([]map[string]any)[0]["id"] = ControlExitID
		},
	} {
		if _, err := decodeInput(bundleJSON(t, edit), "GT-AGENT-X01"); err == nil {
			t.Errorf("%s: decodeInput accepted the bundle", name)
		}
	}
}
