package workerreceipt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

const maxMarkedReceiptTokens = 2_000
const maxReceiptTextBytes = 512

var canonicalReceiptHash = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var receiptRawContentMarkers = []string{
	"raw prompt", "raw body", "raw query", "provider payload", "raw tool result", "system prompt",
}

// ParseMarkedOutput returns nil for markerless legacy output. Once any marker
// is present, the terminal marked envelope is strict and fail-closed.
// @AX:ANCHOR [AUTO]: Preserve this canonical worker handoff trust boundary and markerless legacy compatibility.
// @AX:REASON: Pipeline and direct consumers rely on terminal marked output being parsed exactly once or rejected.
// @AX:SPEC: SPEC-CONTEXT-ENGINEERING-EVOLUTION-001
// @AX:WARN [AUTO]: This parser has more than eight fail-closed marker, size, JSON, schema, and path validation branches.
// @AX:REASON: Accepting ambiguous structure here would turn untrusted worker prose into persisted orchestration evidence.
func ParseMarkedOutput(output string) (*Envelope, error) {
	beginCount := strings.Count(output, BeginMarker)
	endCount := strings.Count(output, EndMarker)
	if beginCount == 0 && endCount == 0 {
		return nil, nil
	}
	if beginCount != 1 || endCount != 1 {
		return nil, fmt.Errorf("worker receipt requires exactly one marker pair")
	}
	begin := strings.Index(output, BeginMarker)
	end := strings.Index(output, EndMarker)
	if begin < 0 || end < begin+len(BeginMarker) {
		return nil, fmt.Errorf("worker receipt markers are out of order")
	}
	terminal := end + len(EndMarker)
	if strings.TrimSpace(output[terminal:]) != "" {
		return nil, fmt.Errorf("worker receipt marker must be terminal")
	}
	marked := output[begin:terminal]
	if promptlayer.EstimateTokens(marked) > maxMarkedReceiptTokens {
		return nil, fmt.Errorf("worker receipt exceeds %d estimated tokens", maxMarkedReceiptTokens)
	}
	body := strings.TrimSpace(output[begin+len(BeginMarker) : end])
	if body == "" {
		return nil, fmt.Errorf("worker receipt body is empty")
	}
	if err := validateUniqueJSONKeys([]byte(body)); err != nil {
		return nil, fmt.Errorf("worker receipt JSON: %w", err)
	}

	var envelope Envelope
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("worker receipt decode: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("worker receipt decode: %w", err)
	}
	if err := validateEnvelope(body, &envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}

func validateEnvelope(raw string, envelope *Envelope) error {
	if envelope.SchemaVersion != SchemaVersion {
		return fmt.Errorf("worker receipt schema must be %s", SchemaVersion)
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &outer); err != nil {
		return fmt.Errorf("worker receipt envelope: %w", err)
	}
	if _, ok := outer["schema_version"]; !ok {
		return fmt.Errorf("worker receipt schema_version is required")
	}
	receiptRaw, ok := outer["receipt"]
	if !ok {
		return fmt.Errorf("worker receipt body is required")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(receiptRaw, &fields); err != nil {
		return fmt.Errorf("worker receipt body: %w", err)
	}
	for _, field := range []string{
		"owned_paths", "changed_files", "verification", "blockers", "next_required_step",
	} {
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("worker receipt field %s is required", field)
		}
	}
	if envelope.Receipt.OwnedPaths == nil || envelope.Receipt.ChangedFiles == nil ||
		envelope.Receipt.Verification == nil || envelope.Receipt.Blockers == nil {
		return fmt.Errorf("worker receipt array fields must not be null")
	}
	if err := validateReceiptText(envelope.Receipt.NextRequiredStep, "next_required_step"); err != nil {
		return err
	}
	for _, paths := range [][]string{envelope.Receipt.OwnedPaths, envelope.Receipt.ChangedFiles} {
		for _, path := range paths {
			if err := validateReceiptReference(path); err != nil {
				return err
			}
		}
	}
	for _, fieldValues := range []struct {
		name   string
		values []string
	}{
		{name: "verification", values: envelope.Receipt.Verification},
		{name: "blockers", values: envelope.Receipt.Blockers},
	} {
		for _, value := range fieldValues.values {
			if err := validateReceiptText(value, fieldValues.name); err != nil {
				return err
			}
		}
	}
	if err := ValidateOwnership(envelope.Receipt, nil); err != nil {
		return err
	}
	if rawEvidence, present := outer["evidence"]; present {
		if bytes.Equal(bytes.TrimSpace(rawEvidence), []byte("null")) {
			return fmt.Errorf("worker receipt evidence must not be null")
		}
		for _, evidence := range envelope.Evidence {
			if err := validateReceiptReference(evidence.Ref); err != nil {
				return err
			}
			if !canonicalReceiptHash.MatchString(evidence.Hash) {
				return fmt.Errorf("worker receipt evidence hash is not canonical SHA-256")
			}
		}
	}
	return nil
}

func validateReceiptText(value, field string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") ||
		len(value) > maxReceiptTextBytes {
		return fmt.Errorf("worker receipt %s must be one bounded non-empty line", field)
	}
	sanitized := promptlayer.SanitizeContent(value, promptlayer.ContextOptions{
		MaxBytes: maxReceiptTextBytes,
		Required: true,
	})
	if sanitized.RedactionStatus != promptlayer.RedactionPassed ||
		sanitized.Content != value || receiptTextContainsAbsolutePath(value) {
		return fmt.Errorf("worker receipt %s contains unsafe content", field)
	}
	lower := strings.ToLower(value)
	for _, marker := range receiptRawContentMarkers {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("worker receipt %s contains unsafe content", field)
		}
	}
	return nil
}

func receiptTextContainsAbsolutePath(value string) bool {
	for _, token := range strings.Fields(value) {
		if strings.Contains(token, "://") {
			continue
		}
		cleanToken := strings.Trim(token, `"'()[]{}<>,;`)
		if hasWindowsDrivePrefix(cleanToken) || strings.HasPrefix(cleanToken, `\\`) {
			return true
		}
		for _, candidate := range strings.FieldsFunc(token, func(r rune) bool {
			return r == '=' || r == ':'
		}) {
			candidate = strings.Trim(candidate, `"'()[]{}<>,;`)
			if filepath.IsAbs(candidate) || hasWindowsDrivePrefix(candidate) ||
				strings.HasPrefix(candidate, `\\`) {
				return true
			}
		}
	}
	return false
}

func validateReceiptReference(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') ||
		strings.Contains(value, `\`) || filepath.IsAbs(value) || filepath.VolumeName(value) != "" ||
		hasWindowsDrivePrefix(value) {
		return fmt.Errorf("worker receipt path must be a clean project-relative reference")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("worker receipt path must be a clean project-relative reference")
	}
	return nil
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) &&
		value[1] == ':'
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON data")
		}
		return err
	}
	return nil
}

func validateUniqueJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walkValue func() error
	walkValue = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate field %q", key)
				}
				seen[key] = true
				if err := walkValue(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walkValue(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
	}
	if err := walkValue(); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}
