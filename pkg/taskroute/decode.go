package taskroute

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const MaxFactsBytes = 1 << 20

// Decode rejects duplicate keys, excessive nesting and trailing JSON values.
func Decode(reader io.Reader) (Facts, error) {
	var f Facts
	data, err := io.ReadAll(io.LimitReader(reader, MaxFactsBytes+1))
	if err != nil {
		return f, fmt.Errorf("facts unreadable")
	}
	if len(data) > MaxFactsBytes {
		return f, fmt.Errorf("facts exceed size limit")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueValue(tokens, 0); err != nil {
		return f, err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return f, fmt.Errorf("trailing facts JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return f, fmt.Errorf("invalid facts JSON schema")
	}
	_, err = validate(f)
	return f, err
}
func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 16 {
		return fmt.Errorf("facts nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return fmt.Errorf("invalid facts JSON")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return fmt.Errorf("invalid facts key")
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate facts key")
			}
			seen[name] = true
			if err := uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid facts delimiter")
	}
	_, err = d.Token()
	return err
}
