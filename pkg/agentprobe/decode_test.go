package agentprobe

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeEvidenceRoundTrip(t *testing.T) {
	data, err := json.Marshal(fixture())
	if err != nil {
		t.Fatal(err)
	}
	e, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Evaluate(e)
	if err != nil || r.Overall != "pass" {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestDecodeRejectsAmbiguousAndUnboundedJSON(t *testing.T) {
	for _, input := range []string{`{"version":1,"version":1}`, `{"version":1,"unknown":true}`, `{} {}`, strings.Repeat(" ", MaxEvidenceBytes+1), strings.Repeat("[", 40) + strings.Repeat("]", 40)} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
