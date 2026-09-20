package agentprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Decode accepts one bounded strict JSON object and validates its event chain.
func Decode(reader io.Reader) (Evidence, error) {
	var evidence Evidence
	data, err := io.ReadAll(io.LimitReader(reader, MaxEvidenceBytes+1))
	if err != nil {
		return evidence, err
	}
	if len(data) > MaxEvidenceBytes {
		return evidence, fmt.Errorf("lifecycle evidence exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueKeys(decoder, 0); err != nil {
		return evidence, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return evidence, fmt.Errorf("trailing lifecycle evidence")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, fmt.Errorf("invalid lifecycle evidence schema")
	}
	_, err = Evaluate(evidence)
	return evidence, err
}

func uniqueKeys(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return fmt.Errorf("lifecycle evidence nesting exceeds limit")
	}
	value, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid lifecycle JSON")
	}
	delimiter, ok := value.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("invalid lifecycle key")
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate lifecycle key")
			}
			seen[name] = true
			if err := uniqueKeys(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueKeys(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected lifecycle delimiter")
	}
	_, err = decoder.Token()
	return err
}
