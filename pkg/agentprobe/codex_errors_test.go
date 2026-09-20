package agentprobe

import (
	"context"
	"errors"
	"testing"
)

func TestCodexFailureCodeDoesNotExposeRawDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, "none"}, {errors.New("secret/path credential prompt"), "native_probe_failed"},
		{codexDiagnostic("native_spawn_unobserved", nil), "native_spawn_unobserved"},
		{codexDiagnostic("cleanup_unverified", context.DeadlineExceeded), "deadline_exceeded"},
	} {
		if got := CodexFailureCode(tc.err); got != tc.want {
			t.Fatalf("%s != %s", got, tc.want)
		}
	}
}

func TestCodexFailureCodePreservesInnermostClosedStage(t *testing.T) {
	err := codexDiagnostic("native_success_activity_failed", codexDiagnostic("native_child_read_failed", codexDiagnostic("rpc_thread_read_rejected", nil)))
	if code := CodexFailureCode(err); code != "rpc_thread_read_rejected" {
		t.Fatal(code)
	}
	if code := codexRPCFailureCode("user-supplied-secret-method"); code != "rpc_request_rejected" {
		t.Fatal(code)
	}
}
