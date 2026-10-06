package harneval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
)

// InvalidError reports why a set, task, or baseline is rejected with reason
// invalid. Detail is one closed detail code; Path names the repository-relative
// file the defect was found in when the loader knows it.
type InvalidError struct {
	Detail string
	Path   string
	Err    error
}

func (e *InvalidError) Error() string {
	msg := ReasonInvalid + ": " + e.Detail
	if e.Path != "" {
		msg += " (" + e.Path + ")"
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *InvalidError) Unwrap() error { return e.Err }

func invalidf(detail, format string, args ...any) *InvalidError {
	return &InvalidError{Detail: detail, Err: fmt.Errorf(format, args...)}
}

// withPath attaches the file a defect was found in, keeping an inner path.
func withPath(err error, rel string) error {
	var invalid *InvalidError
	if errors.As(err, &invalid) && invalid.Path == "" {
		invalid.Path = rel
	}
	return err
}

var (
	taskIDPattern   = regexp.MustCompile(`^GT-[A-Z][A-Z0-9-]{2,40}$`)
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	headingPattern  = regexp.MustCompile(`^#{1,6} \S(.*\S)?$`)
)

// strictDecode decodes exactly one JSON document, rejecting unknown fields and
// any data after the document.
func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return classifyDecodeError(err)
	}
	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		return invalidf(DetailTrailingData, "data follows the JSON document")
	}
	return nil
}

func classifyDecodeError(err error) error {
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return &InvalidError{Detail: DetailUnknownField, Err: err}
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field == "assertions" || strings.HasPrefix(typeErr.Field, "assertions.") {
			return &InvalidError{Detail: DetailAssertionFieldInvalid, Err: err}
		}
		return &InvalidError{Detail: DetailFieldInvalid, Err: err}
	}
	return &InvalidError{Detail: DetailMalformedJSON, Err: err}
}

// isCleanRelPath reports whether p is a non-empty slash-separated relative path
// that path.Clean leaves unchanged and that never leaves its root.
func isCleanRelPath(p string) bool {
	if p == "" || p == "." || strings.ContainsAny(p, "\\\x00") || path.IsAbs(p) {
		return false
	}
	return path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }

func isPlatform(name string) bool { return slices.Contains(Platforms, name) }

// DecodeManifest strictly decodes and validates a harness_golden_set.v1 document.
func DecodeManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := strictDecode(data, &manifest); err != nil {
		return manifest, err
	}
	return manifest, validateManifest(manifest)
}

func validateManifest(m Manifest) error {
	if m.SchemaVersion != SetSchemaV1 || blank(m.SetVersion) || len(m.ActivePaths) == 0 {
		return invalidf(DetailFieldInvalid, "manifest needs %s, a set_version, and active_paths", SetSchemaV1)
	}
	for _, active := range m.ActivePaths {
		if err := validateActivePath(active); err != nil {
			return err
		}
	}
	if err := validatePolicy(m.Floors, m.Live); err != nil {
		return err
	}
	return validatePins(m.Pins)
}

func validateActivePath(p string) error {
	if !isCleanRelPath(p) || !strings.HasPrefix(p, EvalRoot+"/") {
		return invalidf(DetailUncleanPath, "active path %q must be a clean path under %s/", p, EvalRoot)
	}
	if p == QuarantinePath || strings.HasPrefix(p, QuarantinePath+"/") {
		return invalidf(DetailReservedActivePath, "active path %q is inside %s", p, QuarantinePath)
	}
	return nil
}

func validatePolicy(floors Floors, live LivePolicy) error {
	if floors.SurfaceTasks < 1 || floors.AgentTasks < 1 || live.K < 1 ||
		live.ThresholdBP < -10000 || live.ThresholdBP > 0 ||
		live.CompletenessFloor <= 0 || live.CompletenessFloor > 1 ||
		live.MaxAgentRuns < 1 || live.TrialTimeoutSeconds < 1 {
		return invalidf(DetailPolicyOutOfRange, "floors or live policy value out of range")
	}
	if !revisionPattern.MatchString(live.WorkspaceRevision) || blank(live.BaselineRef) || blank(live.Model) {
		return invalidf(DetailFieldInvalid, "live needs a 40-hex workspace_revision, a baseline_ref, and a model")
	}
	return nil
}

func validatePins(p Pins) error {
	if blank(p.GeneratorVersion) || blank(p.ProjectName) || blank(p.CodexCLIVersion) || blank(p.OpencodeCLIVersion) {
		return invalidf(DetailFieldInvalid, "pins need generator, project, codex, and opencode values")
	}
	if p.CodexModelCatalog != "" && !isCleanRelPath(p.CodexModelCatalog) {
		return invalidf(DetailUncleanPath, "codex_model_catalog %q is not a clean relative path", p.CodexModelCatalog)
	}
	return nil
}

// DecodeTask strictly decodes and validates one harness_golden_task.v1 document
// without touching the file system; corpus digests are checked by LoadSet.
func DecodeTask(data []byte) (Task, error) {
	var task Task
	if err := strictDecode(data, &task); err != nil {
		return task, err
	}
	return task, validateTask(task)
}

func validateTask(t Task) error {
	if t.SchemaVersion != TaskSchemaV1 || !taskIDPattern.MatchString(t.ID) ||
		(t.Kind != KindSurface && t.Kind != KindAgent) ||
		blank(t.Category) || blank(t.Intent) || blank(t.Outcome) {
		return invalidf(DetailFieldInvalid, "task %q needs %s, a GT id, a kind, a category, an intent, and an outcome", t.ID, TaskSchemaV1)
	}
	if !slices.Contains([]string{"manual", "benchmark", "incident"}, t.Provenance.Kind) || blank(t.Provenance.Ref) {
		return invalidf(DetailFieldInvalid, "task %q provenance needs a known kind and a ref", t.ID)
	}
	if (t.Status.State != StateActive && t.Status.State != StateRetired) ||
		(t.Status.State == StateRetired && blank(t.Status.Reason)) {
		return invalidf(DetailFieldInvalid, "task %q status must be active or a retired tombstone with a reason", t.ID)
	}
	if err := validateVariants(t); err != nil {
		return err
	}
	if len(t.Assertions) == 0 {
		return invalidf(DetailNoAssertions, "task %q has no assertions", t.ID)
	}
	for index, assertion := range t.Assertions {
		if err := validateAssertion(assertion); err != nil {
			return fmt.Errorf("task %q assertion %d: %w", t.ID, index, err)
		}
	}
	return validateAgentFields(t)
}

func validateVariants(t Task) error {
	seen := make(map[string]bool, len(t.Variants))
	for _, variant := range t.Variants {
		if blank(variant.Name) || seen[variant.Name] {
			return invalidf(DetailFieldInvalid, "task %q variant names must be non-empty and unique", t.ID)
		}
		seen[variant.Name] = true
		for key := range variant.Overrides {
			if key != OverridePreCommitArch {
				return invalidf(DetailFieldInvalid, "task %q variant %q overrides unknown key %q", t.ID, variant.Name, key)
			}
		}
	}
	return nil
}

func validateAgentFields(t Task) error {
	if t.Kind == KindSurface {
		if t.CorpusRef != nil || len(t.ExpectedTests) > 0 {
			return invalidf(DetailFieldInvalid, "surface task %q cannot carry corpus_ref or expected_tests", t.ID)
		}
		return nil
	}
	ref := t.CorpusRef
	if ref == nil || blank(ref.TaskID) || !sha256Pattern.MatchString(ref.FileSHA256) {
		return invalidf(DetailFieldInvalid, "agent task %q needs corpus_ref with task_id and a 64-hex file_sha256", t.ID)
	}
	if !isCleanRelPath(ref.File) {
		return invalidf(DetailUncleanPath, "agent task %q corpus file %q is not a clean relative path", t.ID, ref.File)
	}
	if len(t.ExpectedTests) == 0 {
		return invalidf(DetailExpectedTestsMissing, "agent task %q names no expected_tests", t.ID)
	}
	seen := make(map[string]bool, len(t.ExpectedTests))
	for _, name := range t.ExpectedTests {
		if blank(name) || strings.ContainsAny(name, "/ \t\r\n") || seen[name] {
			return invalidf(DetailFieldInvalid, "agent task %q expected test %q must be a unique top-level test name", t.ID, name)
		}
		seen[name] = true
	}
	return nil
}
