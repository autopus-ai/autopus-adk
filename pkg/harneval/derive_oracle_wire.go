package harneval

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
)

// The key sets of harness_oracle_result.v1 as the trusted runner's
// golden_blackbox.decode_result reads them: every key present, none repeated,
// and null only for artifact_exit. encoding/json alone would keep the last of
// two repeated keys and read an absent or null boolean as false.
var (
	oracleResultKeys    = []string{"schema_version", "task_id", "output_check", "assertions", "artifact_exit", "timed_out"}
	oracleAssertionKeys = []string{"id", "passed"}
)

// checkOracleResultShape refuses a result whose objects repeat a key, lack or
// add one, or hold null anywhere but artifact_exit.
func checkOracleResultShape(data []byte) error {
	if err := rejectRepeatedKeys(data); err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		return invalidf(DetailMalformedJSON, "oracle result is not a JSON object")
	}
	if err := exactKeys(top, oracleResultKeys, "artifact_exit"); err != nil {
		return err
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(top["assertions"], &items); err != nil {
		return invalidf(DetailFieldInvalid, "oracle result assertions is not a list of objects")
	}
	for _, item := range items {
		if err := exactKeys(item, oracleAssertionKeys); err != nil {
			return err
		}
	}
	return nil
}

// exactKeys requires object to hold exactly keys, each non-null unless it is
// one of nullable.
func exactKeys(object map[string]json.RawMessage, keys []string, nullable ...string) error {
	for _, key := range keys {
		raw, present := object[key]
		if !present || (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !slices.Contains(nullable, key)) {
			return invalidf(DetailFieldInvalid, "oracle result %s is absent or null", key)
		}
	}
	if len(object) != len(keys) {
		return invalidf(DetailFieldInvalid, "oracle result object needs exactly the keys %v", keys)
	}
	return nil
}

// jsonFrame is one open container of rejectRepeatedKeys: an array, or an
// object with the keys it named and whether a key comes next.
type jsonFrame struct {
	object  bool
	keys    map[string]bool
	wantKey bool
}

// rejectRepeatedKeys refuses a JSON document in which any object, at any
// depth, names a key twice.
func rejectRepeatedKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var stack []*jsonFrame
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return invalidf(DetailMalformedJSON, "oracle result: %v", err)
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
			if key, isKey := token.(string); isKey {
				if stack[n-1].keys[key] {
					return invalidf(DetailFieldInvalid, "oracle result repeats the key %q", key)
				}
				stack[n-1].keys[key], stack[n-1].wantKey = true, false
				continue
			}
		}
		switch token {
		case json.Delim('{'):
			stack = append(stack, &jsonFrame{object: true, keys: map[string]bool{}, wantKey: true})
		case json.Delim('['):
			stack = append(stack, &jsonFrame{})
		case json.Delim('}'), json.Delim(']'):
			stack = stack[:len(stack)-1]
			valueDone()
		default:
			valueDone()
		}
	}
}
