package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Reject duplicate object keys at every nesting depth before typed decoding.
func rejectHarnessDuplicateKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := scanHarnessJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing harness evidence")
	}
	return nil
}
func scanHarnessJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("harness evidence nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return fmt.Errorf("invalid harness JSON")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return fmt.Errorf("invalid harness JSON")
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid harness JSON key")
			}
			seen[key] = true
			if err := scanHarnessJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := scanHarnessJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid harness JSON delimiter")
	}
	_, err = d.Token()
	return err
}
