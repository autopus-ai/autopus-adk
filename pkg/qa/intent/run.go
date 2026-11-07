package intent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
)

// Failure codes.
const (
	CodeRequestMissing = "qa_intent_request_missing"
	CodeRequestTooBig  = "qa_intent_request_too_large"
	CodeIDInvalid      = "qa_intent_id_invalid"
	CodeExists         = "qa_intent_exists"
	CodeOutputInvalid  = "qa_intent_output_invalid"
	CodeNoCriteria     = "qa_intent_no_criteria"
	CodeFailed         = "qa_intent_failed"
)

// Error is an intent failure with a stable reason code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// ErrorCode returns the stable code for err.
func ErrorCode(err error) string {
	var intentErr *Error
	if errors.As(err, &intentErr) {
		return intentErr.Code
	}
	if code := agentexec.ErrorCode(err); code != "" {
		return code
	}
	return CodeFailed
}

// AgentRunner is the part of agentexec.Runner intent uses.
type AgentRunner interface {
	Run(ctx context.Context, req agentexec.Request) (agentexec.Response, error)
}

// Options configures one run. An empty ID derives one from the request.
type Options struct {
	ProjectDir string
	ID         string
	Request    string
	Target     agentexec.Target
	Timeout    time.Duration
	Force      bool
	Runner     AgentRunner
}

// Report is what one run wrote.
type Report struct {
	ID         string     `json:"id"`
	Agent      string     `json:"agent"`
	Acceptance string     `json:"acceptance"`
	Criteria   []string   `json:"criteria"`
	Questions  []Question `json:"questions"`
}

var idRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*(-[A-Za-z0-9]+)+$`)

// DefaultID is INTENT- and the first eight hex digits of the request's
// sha256, so the same request maps to the same directory.
func DefaultID(request string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(request)))
	return "INTENT-" + hex.EncodeToString(sum[:])[:8]
}

// Dir is where an intent lives. It is a SPEC directory with no spec.md, so
// the QA pipeline reads its acceptance.md like any SPEC's while status, which
// lists only directories with a spec.md, does not count it as a SPEC.
func Dir(projectDir, id string) string {
	return filepath.Dir(acceptance.SpecPath(projectDir, id))
}

// Run asks the agent for criteria in an empty directory and writes
// acceptance.md, the verbatim request, and the open questions.
func Run(ctx context.Context, opts Options) (Report, error) {
	request := strings.TrimSpace(opts.Request)
	report := Report{Agent: string(opts.Target), Criteria: []string{}, Questions: []Question{}}
	switch {
	case request == "":
		return report, &Error{Code: CodeRequestMissing, Message: "the request text is empty"}
	case len(request) > MaxRequestBytes:
		return report, &Error{Code: CodeRequestTooBig, Message: fmt.Sprintf("the request is %d bytes; the limit is %d", len(request), MaxRequestBytes)}
	}
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = DefaultID(request)
	}
	if !idRe.MatchString(id) {
		return report, &Error{Code: CodeIDInvalid, Message: fmt.Sprintf("id %q must look like INTENT-login or QA-42", id)}
	}
	report.ID = id
	dir := Dir(opts.ProjectDir, id)
	path := filepath.Join(dir, "acceptance.md")
	if _, err := os.Stat(path); err == nil && !opts.Force {
		return report, &Error{Code: CodeExists, Message: path + " already exists; pass --force to replace it"}
	}
	empty, err := os.MkdirTemp("", "autopus-qa-intent-")
	if err != nil {
		return report, err
	}
	defer func() { _ = os.RemoveAll(empty) }()
	runner := opts.Runner
	if runner == nil {
		runner = agentexec.NewRunner()
	}
	resp, err := runner.Run(ctx, agentexec.Request{
		Target: opts.Target, Mode: agentexec.ModeGenerate, Prompt: BuildPrompt(request), WorkDir: empty, Timeout: opts.Timeout,
	})
	if err != nil {
		return report, err
	}
	draft, err := ParseOutput(resp.Stdout)
	if err != nil {
		return report, err
	}
	if err := write(dir, request, draft); err != nil {
		return report, err
	}
	report.Acceptance = path
	for _, c := range draft.Criteria {
		report.Criteria = append(report.Criteria, c.ID)
	}
	report.Questions = draft.Questions
	return report, nil
}

func write(dir, request string, draft Draft) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	questions, err := json.MarshalIndent(draft.Questions, "", "  ")
	if err != nil {
		return err
	}
	header := "<!-- intent_source: request. Written by `auto qa intent` from request.md alone, without the code.\n" +
		"     Edit a criterion only to apply the user's answer to a question in questions.json. -->\n\n"
	for name, body := range map[string][]byte{
		"request.md":     []byte(request + "\n"),
		"questions.json": append(questions, '\n'),
		"acceptance.md":  []byte(header + draft.Acceptance),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
