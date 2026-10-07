package editguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// MaxPayloadBytes is the hard cap on one hook payload. The dialects decode
// only the target fields and skip every other value, so a large content body
// below the cap is decided like any other; a payload past it allows the call
// (M2, docs/edit-guard.md Limitations).
const MaxPayloadBytes = 64 << 20

func readPayload(stdin io.Reader, limit int) ([]byte, string) {
	if limit <= 0 {
		limit = MaxPayloadBytes
	}
	data, err := io.ReadAll(io.LimitReader(stdin, int64(limit)+1))
	switch {
	case err != nil:
		return nil, "payload unreadable"
	case len(data) == 0:
		return nil, "empty payload"
	case len(data) > limit:
		return nil, "payload over 64 MiB"
	}
	return data, ""
}

var (
	errNotAnObject = errors.New("editguard: payload value is not an object")
	errNotAString  = errors.New("editguard: payload field is not a string")
	errNotAnArray  = errors.New("editguard: payload targets is not an array")
	errTrailing    = errors.New("editguard: data after the payload")
)

// payloadDecoder walks one JSON payload token by token. Keys match exactly
// and a later occurrence of a key replaces an earlier one, as the hosts' JSON
// parsers do, so a second spelling cannot point the guard at another file
// than the one the host writes.
type payloadDecoder struct{ dec *json.Decoder }

func newPayloadDecoder(payload []byte) payloadDecoder {
	return payloadDecoder{json.NewDecoder(bytes.NewReader(payload))}
}

// object walks one object and calls field for each key in order; field must
// consume the value. A null is an object without fields.
func (d payloadDecoder) object(field func(key string) error) error {
	token, err := d.dec.Token()
	if err != nil || token == nil {
		return err
	}
	if token != json.Delim('{') {
		return errNotAnObject
	}
	for d.dec.More() {
		key, err := d.dec.Token()
		if err != nil {
			return err
		}
		if err := field(key.(string)); err != nil {
			return err
		}
	}
	_, err = d.dec.Token()
	return err
}

// text reads one value: a string reports ok, and any other value is consumed
// and reports not ok.
func (d payloadDecoder) text() (string, bool, error) {
	var raw discardValue
	if err := d.dec.Decode(&raw); err != nil {
		return "", false, err
	}
	return raw.text, raw.isText, nil
}

// requireText reads a string field into *dst; null keeps it, and any other
// value makes the payload malformed.
func (d payloadDecoder) requireText(dst *string) error {
	var raw discardValue
	if err := d.dec.Decode(&raw); err != nil {
		return err
	}
	switch {
	case raw.isText:
		*dst = raw.text
	case !raw.isNull:
		return errNotAString
	}
	return nil
}

// skip discards one value, a large content body included, unread.
func (d payloadDecoder) skip() error {
	var value skippedValue
	return d.dec.Decode(&value)
}

// skippedValue is scanned for validity and never decoded.
type skippedValue struct{}

func (*skippedValue) UnmarshalJSON([]byte) error { return nil }

// end rejects data after the top-level value.
func (d payloadDecoder) end() error {
	if _, err := d.dec.Token(); !errors.Is(err, io.EOF) {
		return errTrailing
	}
	return nil
}

// discardValue keeps a string value and drops every other one unread.
type discardValue struct {
	text           string
	isText, isNull bool
}

func (v *discardValue) UnmarshalJSON(data []byte) error {
	switch {
	case len(data) > 0 && data[0] == '"':
		v.isText = true
		return json.Unmarshal(data, &v.text)
	case string(data) == "null":
		v.isNull = true
	}
	return nil
}

// hookPayload is what the Claude Code, Codex, and Gemini CLI dialects read
// from a pre-tool payload: the cwd, the tool name, and one tool_input string.
type hookPayload struct {
	cwd, toolName string
	input         string
	hasInput      bool
}

func decodeHookPayload(payload []byte, inputKey string) (hookPayload, error) {
	d := newPayloadDecoder(payload)
	var doc hookPayload
	err := d.object(func(key string) error {
		switch key {
		case "cwd":
			return d.requireText(&doc.cwd)
		case "tool_name":
			return d.requireText(&doc.toolName)
		case "tool_input":
			doc.input, doc.hasInput = "", false
			return d.object(func(key string) error {
				if key != inputKey {
					return d.skip()
				}
				var err error
				doc.input, doc.hasInput, err = d.text()
				return err
			})
		}
		return d.skip()
	})
	if err != nil {
		return hookPayload{}, err
	}
	return doc, d.end()
}

// decodeOpenCodePayload reads the cwd, the targets array, and the displaced
// array of the payload the OpenCode plugin synthesizes; a non-string target
// is a dropped target, and a non-string displaced entry names nothing.
func decodeOpenCodePayload(payload []byte) (Call, error) {
	d := newPayloadDecoder(payload)
	var call Call
	err := d.object(func(key string) error {
		switch key {
		case "cwd":
			return d.requireText(&call.Cwd)
		case "targets":
			call.Targets, call.Dropped = nil, 0
			return d.strings(call.add)
		case "displaced":
			call.Displaced = nil
			return d.strings(func(p string) {
				if p != "" {
					call.Displaced = append(call.Displaced, p)
				}
			})
		}
		return d.skip()
	})
	if err != nil {
		return Call{}, err
	}
	return call, d.end()
}

// strings walks one array and passes each entry to add, a non-string entry
// as "".
func (d payloadDecoder) strings(add func(string)) error {
	token, err := d.dec.Token()
	if err != nil || token == nil {
		return err
	}
	if token != json.Delim('[') {
		return errNotAnArray
	}
	for d.dec.More() {
		entry, _, err := d.text()
		if err != nil {
			return err
		}
		add(entry)
	}
	_, err = d.dec.Token()
	return err
}
