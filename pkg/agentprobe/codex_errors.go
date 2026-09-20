package agentprobe

import (
	"context"
	"errors"
	"io"
)

type codexDiagnosticError struct {
	code  string
	cause error
}

func (e *codexDiagnosticError) Error() string { return e.code }
func (e *codexDiagnosticError) Unwrap() error { return e.cause }
func codexDiagnostic(code string, err error) error {
	return &codexDiagnosticError{code: code, cause: err}
}

// CodexFailureCode returns only a closed, body-free category. It never exposes
// native RPC text, filesystem paths, prompts, account data, or model responses.
func CodexFailureCode(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	code := ""
	for current := err; current != nil; current = errors.Unwrap(current) {
		if diagnostic, ok := current.(*codexDiagnosticError); ok {
			code = diagnostic.code
		}
	}
	if code != "" {
		return code
	}
	if errors.Is(err, io.EOF) {
		return "native_stream_closed"
	}
	return "native_probe_failed"
}

func codexRPCFailureCode(method string) string {
	switch method {
	case "initialize":
		return "rpc_initialize_rejected"
	case "thread/start":
		return "rpc_thread_start_rejected"
	case "turn/start":
		return "rpc_turn_start_rejected"
	case "thread/read":
		return "rpc_thread_read_rejected"
	case "turn/interrupt":
		return "rpc_turn_interrupt_rejected"
	case "thread/archive":
		return "rpc_thread_archive_rejected"
	case "thread/loaded/list":
		return "rpc_thread_inventory_rejected"
	default:
		return "rpc_request_rejected"
	}
}
