package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// turnTranscriptTail bounds how much of the transcript the language check
// reads: a turn's replies sit at the end of a file that can grow to
// hundreds of megabytes over a long session.
const turnTranscriptTail = 8 << 20

type transcriptEntry struct {
	Type        string `json:"type"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type transcriptPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// turnReplies returns the text the assistant wrote to the user in the
// current turn, oldest first: every text block after the last prompt, so
// progress notes between tool calls are checked as well as the final reply.
// A missing or unreadable transcript yields nil.
func turnReplies(path string) []string {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	offset := max(info.Size()-turnTranscriptTail, 0)
	data, err := io.ReadAll(io.NewSectionReader(f, offset, info.Size()-offset))
	if err != nil {
		return nil
	}
	lines := bytes.Split(data, []byte("\n"))
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:] // the first line was cut by the offset
	}
	var replies []string
	for i := len(lines) - 1; i >= 0; i-- {
		var entry transcriptEntry
		if json.Unmarshal(lines[i], &entry) != nil || entry.IsSidechain {
			continue
		}
		texts, prompt := entryTexts(entry)
		if prompt {
			break
		}
		if entry.Type == "assistant" {
			replies = append(texts, replies...)
		}
	}
	return replies
}

// entryTexts returns an assistant entry's text blocks, and reports whether a
// user entry is a prompt (typed text or a notification) rather than a tool
// result or an injected meta message.
func entryTexts(entry transcriptEntry) (texts []string, prompt bool) {
	var plain string
	if json.Unmarshal(entry.Message.Content, &plain) == nil {
		return nil, entry.Type == "user" && !entry.IsMeta
	}
	var parts []transcriptPart
	if json.Unmarshal(entry.Message.Content, &parts) != nil {
		return nil, false
	}
	for _, part := range parts {
		if part.Type != "text" {
			continue
		}
		if entry.Type == "user" {
			return nil, !entry.IsMeta
		}
		texts = append(texts, part.Text)
	}
	return texts, false
}
