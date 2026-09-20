package agentprobe

import (
	"encoding/json"
)

// Decode only the item discriminator before variant-specific fields. Native
// extension/control frames cannot be interpreted as collaboration payloads.
func codexItemEnvelope(m codexRPCMessage) (string, json.RawMessage, string, error) {
	if m.Method != "item/started" && m.Method != "item/completed" {
		return "", nil, "", nil
	}
	var envelope struct {
		ThreadID string          `json:"threadId"`
		Item     json.RawMessage `json:"item"`
	}
	if json.Unmarshal(m.Params, &envelope) != nil {
		return "", nil, "", codexDiagnostic("native_item_envelope_invalid", nil)
	}
	var tag struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(envelope.Item, &tag) != nil {
		return "", nil, "", codexDiagnostic("native_item_discriminator_invalid", nil)
	}
	return envelope.ThreadID, envelope.Item, tag.Type, nil
}

type codexReadItem struct {
	Type string
	Text string
}

func (item *codexReadItem) UnmarshalJSON(data []byte) error {
	var tag struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return err
	}
	item.Type = tag.Type
	if tag.Type != "agentMessage" {
		return nil
	}
	var message struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		return err
	}
	item.Text = message.Text
	return nil
}
