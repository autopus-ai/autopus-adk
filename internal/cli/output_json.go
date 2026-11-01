package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const cliJSONSchemaVersion = "1.0.0"

type jsonEnvelopeStatus string

const (
	jsonStatusOK    jsonEnvelopeStatus = "ok"
	jsonStatusWarn  jsonEnvelopeStatus = "warn"
	jsonStatusError jsonEnvelopeStatus = "error"
)

type jsonMessage struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type jsonCheck struct {
	ID       string `json:"id,omitempty"`
	Severity string `json:"severity,omitempty"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	// Fields carries the check's structured readings when a single detail
	// sentence cannot be consumed programmatically.
	Fields map[string]string `json:"fields,omitempty"`
}

type jsonErrorPayload struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type jsonEnvelope struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Status        jsonEnvelopeStatus `json:"status"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Warnings      []jsonMessage      `json:"warnings,omitempty"`
	Checks        []jsonCheck        `json:"checks,omitempty"`
	Error         *jsonErrorPayload  `json:"error,omitempty"`
	Data          any                `json:"data"`
}

type jsonEnvelopeOptions struct {
	Command  string
	Status   jsonEnvelopeStatus
	Data     any
	Warnings []jsonMessage
	Checks   []jsonCheck
	Error    *jsonErrorPayload
}

type jsonFatalError struct {
	cause error
}

func (e *jsonFatalError) Error() string {
	return e.cause.Error()
}

func (e *jsonFatalError) Unwrap() error {
	return e.cause
}

// alreadyReported satisfies the root.go marker: the envelope is the report.
func (e *jsonFatalError) alreadyReported() {}

func addJSONFlags(cmd *cobra.Command, jsonOutput *bool, format *string) {
	cmd.Flags().BoolVar(jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().StringVar(format, "format", "text", "Output format (text|json)")
}

func resolveJSONMode(jsonOutput bool, format string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	switch normalized {
	case "", "text", "plain":
		return jsonOutput, nil
	case "json":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported format %q: must be text or json", format)
	}
}

func writeJSONResult(
	cmd *cobra.Command,
	status jsonEnvelopeStatus,
	data any,
	warnings []jsonMessage,
	checks []jsonCheck,
) error {
	return writeJSONEnvelope(cmd.OutOrStdout(), jsonEnvelopeOptions{
		Command:  cmd.CommandPath(),
		Status:   status,
		Data:     data,
		Warnings: warnings,
		Checks:   checks,
	})
}

func writeJSONResultAndExit(
	cmd *cobra.Command,
	status jsonEnvelopeStatus,
	cause error,
	code string,
	data any,
	warnings []jsonMessage,
	checks []jsonCheck,
) error {
	if err := writeJSONEnvelope(cmd.OutOrStdout(), jsonEnvelopeOptions{
		Command:  cmd.CommandPath(),
		Status:   status,
		Data:     data,
		Warnings: warnings,
		Checks:   checks,
		Error: &jsonErrorPayload{
			Code:    code,
			Message: cause.Error(),
		},
	}); err != nil {
		return err
	}
	return &jsonFatalError{cause: cause}
}

func isJSONFatalError(err error) bool {
	var target *jsonFatalError
	return errors.As(err, &target)
}

func writeJSONEnvelope(w io.Writer, opts jsonEnvelopeOptions) error {
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = "auto"
	}

	envelope := jsonEnvelope{
		SchemaVersion: cliJSONSchemaVersion,
		Command:       command,
		Status:        opts.Status,
		GeneratedAt:   time.Now().UTC(),
		Warnings:      sanitizeJSONWarnings(opts.Warnings),
		Checks:        sanitizeJSONChecks(opts.Checks),
		Error:         sanitizeJSONError(opts.Error),
		Data:          sanitizeJSONData(opts.Data),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}

func sanitizeJSONWarnings(warnings []jsonMessage) []jsonMessage {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]jsonMessage, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, jsonMessage{
			Code:    warning.Code,
			Message: sanitizeJSONString("", warning.Message),
		})
	}
	return out
}

func sanitizeJSONChecks(checks []jsonCheck) []jsonCheck {
	if len(checks) == 0 {
		return nil
	}
	out := make([]jsonCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, jsonCheck{
			ID:       check.ID,
			Severity: check.Severity,
			Status:   check.Status,
			Detail:   sanitizeJSONString("", check.Detail),
			Fields:   sanitizeJSONCheckFields(check.Fields),
		})
	}
	return out
}

func sanitizeJSONCheckFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		out[key] = sanitizeJSONString(key, value)
	}
	return out
}

func sanitizeJSONError(payload *jsonErrorPayload) *jsonErrorPayload {
	if payload == nil {
		return nil
	}
	return &jsonErrorPayload{
		Code:    payload.Code,
		Message: sanitizeJSONString("", payload.Message),
	}
}

func sanitizeJSONData(v any) any {
	if v == nil {
		return map[string]any{}
	}

	data, err := json.Marshal(v)
	if err != nil {
		return sanitizeJSONString("", fmt.Sprintf("%v", v))
	}

	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return sanitizeJSONString("", string(data))
	}

	return sanitizeJSONValue("", decoded)
}

func sanitizeJSONValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			out[childKey] = sanitizeJSONValue(childKey, childValue)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = sanitizeJSONValue(key, typed[i])
		}
		return out
	case string:
		return sanitizeJSONString(key, typed)
	default:
		return value
	}
}

func sanitizeJSONString(key, value string) string {
	if isSensitiveJSONKey(key) && value != "" {
		return "[REDACTED]"
	}
	return maskHomePath(value)
}

func isSensitiveJSONKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}

	fragments := []string{
		"token",
		"secret",
		"password",
		"cookie",
		"authorization",
		"api_key",
		"apikey",
		"access_key",
		"refresh_key",
	}

	for _, fragment := range fragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func maskHomePath(value string) string {
	if value == "" {
		return value
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return value
	}

	masked := strings.ReplaceAll(value, home, "~")
	masked = strings.ReplaceAll(masked, strings.ReplaceAll(home, "\\", "/"), "~")
	return masked
}
