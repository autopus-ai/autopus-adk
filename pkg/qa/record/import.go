package record

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// ImportOptions selects the recording file and the scenario it becomes.
type ImportOptions struct {
	From string
	// Format is auto (by extension), codegen, or jsonl.
	Format       string
	ID           string
	Title        string
	Journey      string
	Origin       string
	AllowPartial bool
	// RecordingRef overrides the default "<file>@sha256:<prefix>" reference.
	RecordingRef string
}

// Result describes the candidate an import wrote.
type Result struct {
	Path            string        `json:"path"`
	Created         bool          `json:"created"`
	ID              string        `json:"id"`
	Journey         string        `json:"journey"`
	Origin          string        `json:"origin"`
	Format          string        `json:"format"`
	RecordingRef    string        `json:"recording_ref"`
	Screens         int           `json:"screens"`
	Steps           int           `json:"steps"`
	ConfirmRequired int           `json:"confirm_required"`
	Unsupported     []Unsupported `json:"unsupported,omitempty"`
	// Recording is the raw codegen file a failed live recording kept.
	Recording string `json:"recording,omitempty"`
}

// Import converts a recording file into a v2 recording candidate under the
// candidates directory (REQ-14, REQ-16). Unsupported lines fail the import
// unless AllowPartial is set; then they are dropped and still listed. The
// origin defaults to the journey's first allowed origin.
func Import(projectDir string, opts ImportOptions) (Result, error) {
	format, err := detectFormat(opts.From, opts.Format)
	if err != nil {
		return Result{}, err
	}
	body, err := os.ReadFile(opts.From)
	if err != nil {
		return Result{}, failf(CodeSourceUnreadable, "cannot read recording: %v", err)
	}
	rec, unsupported := ParseCodegen(body)
	if format == FormatJSONL {
		rec, unsupported = ParseJSONL(body)
	}
	result := Result{ID: strings.TrimSpace(opts.ID), Format: format, Unsupported: unsupported}
	if len(unsupported) > 0 && !opts.AllowPartial {
		return result, unsupportedError(unsupported)
	}
	journeyID, origin, err := importTarget(projectDir, opts.Journey, opts.Origin)
	if err != nil {
		return result, err
	}
	result.Journey, result.RecordingRef = journeyID, strings.TrimSpace(opts.RecordingRef)
	if result.RecordingRef == "" {
		result.RecordingRef = Ref(filepath.Base(opts.From), body)
	}
	s, err := ToScenario(rec, Options{ID: result.ID, Title: opts.Title, Journey: journeyID,
		Origin: origin, RecordingRef: result.RecordingRef})
	if err != nil {
		return result, err
	}
	result.Origin, result.Screens = s.Origin, len(s.Screens)
	for _, screen := range s.Screens {
		result.Steps += len(screen.Steps)
		for _, step := range screen.Steps {
			if step.Confirm == scenario.ConfirmRequired {
				result.ConfirmRequired++
			}
		}
	}
	result.Path, result.Created, err = WriteCandidate(projectDir, s, recordingHeader(result))
	return result, err
}

// importTarget resolves the journey and origin a recording runs under: the
// explicit ones, or else the journey's first allowed origin.
func importTarget(projectDir, journeyID, origin string) (string, string, error) {
	packs, err := journey.LoadDir(projectDir)
	if err != nil && (strings.TrimSpace(journeyID) == "" || strings.TrimSpace(origin) == "") {
		return "", "", fmt.Errorf("load Journey Packs: %w", err)
	}
	id, resolved, _ := ResolveJourney(packs, journeyID, origin)
	if resolved == "" {
		return "", "", failf(CodeOriginMissing, "no origin: pass --origin or declare gui.allowed_origins in a Journey Pack")
	}
	if id == "" {
		return "", "", failf(CodeJourneyMissing, "no Journey Pack declares gui.allowed_origins; pass --journey")
	}
	return id, resolved, nil
}

func detectFormat(from, format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "auto":
	case FormatCodegen:
		return FormatCodegen, nil
	case FormatJSONL:
		return FormatJSONL, nil
	default:
		return "", failf(CodeFormatUnknown, "unknown recording format %q: use codegen or jsonl", format)
	}
	switch strings.ToLower(filepath.Ext(from)) {
	case ".jsonl", ".ndjson":
		return FormatJSONL, nil
	case ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts":
		return FormatCodegen, nil
	}
	return "", failf(CodeFormatUnknown,
		"cannot tell the format of %s from its extension; name it codegen or jsonl explicitly", filepath.Base(from))
}

// Ref names a recording by file name and content hash, so a candidate can be
// traced to the exact bytes it came from.
func Ref(name string, body []byte) string {
	sum := sha256.Sum256(body)
	return name + "@sha256:" + hex.EncodeToString(sum[:])[:12]
}

func recordingHeader(result Result) []string {
	header := []string{
		"Recorded journey imported by `auto qa record` from " + result.RecordingRef + ".",
		"Steps replay what was recorded; review them before promoting this candidate.",
	}
	if n := len(result.Unsupported); n > 0 {
		header = append(header, fmt.Sprintf("%d unsupported line(s) were dropped by --allow-partial.", n))
	}
	if result.ConfirmRequired > 0 {
		header = append(header, fmt.Sprintf(
			"%d agent assertion(s) stay confirm: required until a person confirms them or adds an ac.", result.ConfirmRequired))
	}
	return header
}

// Marshal renders a scenario as YAML under a comment header. Field order
// follows the struct and empty optional fields are omitted, so one scenario
// always yields the same bytes.
func Marshal(s scenario.Scenario, header []string) ([]byte, error) {
	var buf bytes.Buffer
	for _, line := range header {
		buf.WriteString(strings.TrimRight("# "+line, " ") + "\n")
	}
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(s); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteCandidate writes s as <id>.yaml in the candidates directory. The bytes
// are first re-parsed with the loader's strictness, which also vets the id
// before it names a file, so a candidate that would not load is never
// written. An identical existing file is left alone (created is false); a
// different one is never overwritten.
func WriteCandidate(projectDir string, s scenario.Scenario, header []string) (string, bool, error) {
	body, err := Marshal(s, header)
	if err != nil {
		return "", false, err
	}
	name := s.ID + ".yaml"
	if _, err := scenario.ParseBytes(name, body); err != nil {
		return "", false, err
	}
	dir := scenario.CandidatesDir(projectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, body) {
			return path, false, nil
		}
		return path, false, failf(CodeCandidateExists,
			"%s already exists with different content; pass another --id or remove it first", filepath.ToSlash(path))
	}
	if err != nil {
		return "", false, err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", false, err
	}
	if err := file.Close(); err != nil {
		return "", false, err
	}
	return path, true, nil
}
