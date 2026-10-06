package cli

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"
)

const (
	// readinessStreamKeepBytes is the largest probe stream that is classified.
	readinessStreamKeepBytes = 65536
	// readinessStreamReadLimit reads one byte past the bound to detect oversized output.
	readinessStreamReadLimit = readinessStreamKeepBytes + 1
)

// providerReadinessCommand is one status probe argv, executed without a shell.
type providerReadinessCommand struct {
	Argv []string
	Env  []string
	Dir  string
	// ompIdentity pins the canonical OMP executable; only the OMP probe sets it.
	ompIdentity *pipelineOMPExecutableIdentity
}

// providerReadinessProcess exposes the raw, unbounded streams of a started probe.
// Wait reports the exit code and must release both streams once it returns.
type providerReadinessProcess struct {
	Stdout io.Reader
	Stderr io.Reader
	Wait   func() (int, error)
}

// providerReadinessRunner is the process seam: it starts an allowlisted status
// command and returns raw streams. Bounding and classification stay outside it.
var providerReadinessRunner = startProviderReadinessProcess

var errReadinessProbeNotAllowlisted = errors.New("provider readiness probe is not allowlisted")

// readinessProbeWaitDelay bounds how long Wait lingers after cancellation for
// a probe that survived the kill, keeping a timed-out probe within 5.5 s.
const readinessProbeWaitDelay = 250 * time.Millisecond

// startProviderReadinessProcess execs one Readiness Contract argv directly,
// never through a shell; ctx cancellation kills the probe's process group.
func startProviderReadinessProcess(ctx context.Context, command providerReadinessCommand) (providerReadinessProcess, error) {
	if err := checkProviderReadinessCommand(command); err != nil {
		return providerReadinessProcess{}, err
	}
	cmd := exec.CommandContext(ctx, command.Argv[0], command.Argv[1:]...)
	cmd.Env = command.Env
	cmd.Dir = command.Dir
	killReadinessProcessGroupOnCancel(cmd)
	cmd.WaitDelay = readinessProbeWaitDelay
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return providerReadinessProcess{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return providerReadinessProcess{}, err
	}
	if err := cmd.Start(); err != nil {
		return providerReadinessProcess{}, err
	}
	return providerReadinessProcess{Stdout: stdout, Stderr: stderr, Wait: func() (int, error) {
		return readinessExitCode(cmd.Wait())
	}}, nil
}

// checkProviderReadinessCommand admits only `claude auth status --json`,
// `codex login status`, and the pinned canonical `omp usage --json --redact`.
func checkProviderReadinessCommand(command providerReadinessCommand) error {
	if len(command.Argv) == 0 {
		return errReadinessProbeNotAllowlisted
	}
	executable, args := command.Argv[0], command.Argv[1:]
	if command.ompIdentity != nil {
		if !filepath.IsAbs(executable) || !slices.Equal(args, readinessStatusArgs[readinessProbeOMP]) {
			return errReadinessProbeNotAllowlisted
		}
		return verifyPipelineOMPExecutable(executable, *command.ompIdentity)
	}
	switch kind := readinessProbeKind(executable); kind {
	case readinessProbeClaude, readinessProbeCodex:
		if slices.Equal(args, readinessStatusArgs[kind]) {
			return nil
		}
	}
	return errReadinessProbeNotAllowlisted
}

// readinessExitCode keeps a status code and reports abnormal termination as an error.
func readinessExitCode(err error) (int, error) {
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		return exitErr.ExitCode(), nil
	default:
		return -1, err
	}
}

const readinessRedacted = "[REDACTED]"

// readinessSecretPatterns match account identifiers and credentials that a
// status command may print; rendered readiness text never keeps them.
var readinessSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`),
	regexp.MustCompile(`(?i)\bbearer\s+[^\s"']+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`),
	regexp.MustCompile(`\borg[-_][A-Za-z0-9_-]+`),
	regexp.MustCompile(`\b[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\b`),
}

// redactReadinessText scrubs emails, account and organization ids, and tokens.
func redactReadinessText(text string) string {
	for _, pattern := range readinessSecretPatterns {
		text = pattern.ReplaceAllString(text, readinessRedacted)
	}
	return text
}

// readinessStream is at most readinessStreamKeepBytes of one probe stream.
type readinessStream struct {
	Data      []byte
	Oversized bool
	Err       error
}

func readReadinessStream(reader io.Reader) readinessStream {
	if reader == nil {
		return readinessStream{}
	}
	data, err := io.ReadAll(io.LimitReader(reader, readinessStreamReadLimit))
	if len(data) > readinessStreamKeepBytes {
		return readinessStream{Data: data[:readinessStreamKeepBytes], Oversized: true}
	}
	return readinessStream{Data: data, Err: err}
}

// readinessEvidence is the bounded result of one probe execution.
type readinessEvidence struct {
	ExitCode int
	Stdout   readinessStream
	Stderr   readinessStream
	Failure  string
}

// collectReadinessEvidence runs one probe under the probe timeout. Both streams
// are bounded concurrently; an oversized stream cancels the probe at once.
func collectReadinessEvidence(ctx context.Context, command providerReadinessCommand) readinessEvidence {
	probeCtx, cancel := context.WithTimeout(ctx, providerReadinessProbeTimeout)
	defer cancel()
	process, err := providerReadinessRunner(probeCtx, command)
	if err != nil {
		return readinessEvidence{Failure: readinessReasonProbeFailed}
	}
	var evidence readinessEvidence
	var readers sync.WaitGroup
	readers.Add(2)
	readBounded := func(reader io.Reader, stream *readinessStream) {
		defer readers.Done()
		*stream = readReadinessStream(reader)
		if stream.Oversized {
			cancel()
		}
	}
	go readBounded(process.Stdout, &evidence.Stdout)
	go readBounded(process.Stderr, &evidence.Stderr)
	streamsDone := make(chan struct{})
	go func() {
		readers.Wait()
		close(streamsDone)
	}()
	streamsFinished := true
	select {
	case <-streamsDone:
	case <-probeCtx.Done():
		// Wait releases streams that an exited process's children still hold.
		streamsFinished = false
	}
	exitCode, waitErr := process.Wait()
	<-streamsDone
	evidence.ExitCode = exitCode
	evidence.Failure = readinessFailure(evidence, streamsFinished, waitErr,
		errors.Is(probeCtx.Err(), context.DeadlineExceeded))
	return evidence
}

func readinessFailure(evidence readinessEvidence, streamsFinished bool, waitErr error, deadline bool) string {
	switch {
	case evidence.Stdout.Oversized || evidence.Stderr.Oversized:
		return "oversized_output"
	case deadline && (!streamsFinished || waitErr != nil):
		return "timeout"
	case waitErr != nil || evidence.Stdout.Err != nil || evidence.Stderr.Err != nil:
		return readinessReasonProbeFailed
	default:
		return ""
	}
}
