package skillpolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decode rejects oversized, ambiguous, unknown-field, and trailing JSON input.
func Decode(reader io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(reader, MaxJSONBytes+1))
	if err != nil {
		return err
	}
	if len(data) > MaxJSONBytes {
		return fmt.Errorf("JSON exceeds %d bytes", MaxJSONBytes)
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueObjectKeys(tokens, 0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("JSON must contain one value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	switch value := target.(type) {
	case *Policy:
		return validatePolicy(*value)
	case *Task:
		return validateTask(*value)
	case *Cases:
		return validateCases(*value)
	default:
		return fmt.Errorf("unsupported policy document type")
	}
}

func uniqueObjectKeys(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("JSON nesting exceeds limit")
	}
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
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid JSON key")
			}
			seen[name] = true
			if err := uniqueObjectKeys(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueObjectKeys(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}
