package brainstorm

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// block is one fenced piece of untrusted text in the provider results.
type block struct {
	heading string // empty for the diagnosis
	text    string
	tail    bool // a size cut keeps the end (CI logs) instead of the start
}

// blocks returns the diagnosis first, then each log and report. Control
// sequences that slipped past the caller are stripped as defense in depth.
func (r Request) blocks() []block {
	var blocks []block
	if text := healthband.StripControls(r.Diagnosis.Text); text != "" {
		blocks = append(blocks, block{text: text})
	}
	for _, log := range r.Logs {
		heading := fmt.Sprintf("### Evidence: CI run %d attempt %d%s", log.RunID, log.Attempt, reasonSuffix(log.Evidence.Reasons))
		blocks = append(blocks, block{heading: heading, text: healthband.StripControls(log.Evidence.Text), tail: true})
	}
	for _, report := range r.Reports {
		heading := fmt.Sprintf("### Evidence: react report for CI run %d%s", report.RunID, reasonSuffix(report.Evidence.Reasons))
		blocks = append(blocks, block{heading: heading, text: healthband.StripControls(report.Evidence.Text)})
	}
	return blocks
}

func reasonSuffix(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return " (" + strings.Join(reasons, ", ") + ")"
}

func (b block) render(text string) string {
	fenced := healthband.Fence(text)
	if b.heading != "" {
		fenced = b.heading + "\n" + fenced
	}
	return "\n" + fenced + "\n"
}

// fit renders b within budget bytes, cutting its text at a line boundary.
// cut reports a shortened text; ok is false when not even one line fits. A
// cut text never holds a longer backtick run, so its fence is never longer.
func (b block) fit(budget int) (rendered string, cut, ok bool) {
	text := strings.TrimRight(b.text, "\n")
	if full := b.render(text); len(full) <= budget {
		return full, false, true
	} else if b.tail {
		text = cutTail(text, budget-(len(full)-len(text)))
	} else {
		text = cutHead(text, budget-(len(full)-len(text)))
	}
	return b.render(text), true, text != ""
}

// sizeCapLine marks a body whose untrusted text was cut or dropped.
const sizeCapLine = "\nevidence_status: " + healthband.ReasonSizeCap + "\n"

// fitBlocks keeps the blocks within budget bytes (Untrusted Input Contract
// item 8). The diagnosis comes first and the evidence after it, so evidence
// is cut or dropped before the diagnosis is cut; everything after the first
// cut block is dropped.
func fitBlocks(blocks []block, budget int) string {
	var out strings.Builder
	for _, b := range blocks {
		out.WriteString(b.render(strings.TrimRight(b.text, "\n")))
	}
	if out.Len() <= budget {
		return out.String()
	}
	out.Reset()
	for _, b := range blocks {
		rendered, cut, ok := b.fit(budget - len(sizeCapLine) - out.Len())
		if ok {
			out.WriteString(rendered)
		}
		if cut || !ok {
			break
		}
	}
	return out.String() + sizeCapLine
}

// cutTail keeps at most n bytes of the end of text, starting at a line.
func cutTail(text string, n int) string {
	if n >= len(text) {
		return text
	}
	if n <= 0 {
		return ""
	}
	start := len(text) - n
	if text[start-1] != '\n' {
		i := strings.IndexByte(text[start:], '\n')
		if i < 0 {
			return ""
		}
		start += i + 1
	}
	return text[start:]
}

// cutHead keeps at most n bytes of the start of text, ending at a line.
func cutHead(text string, n int) string {
	if n >= len(text) {
		return text
	}
	if n <= 0 {
		return ""
	}
	if text[n] == '\n' {
		return text[:n]
	}
	end := strings.LastIndexByte(text[:n], '\n')
	if end < 0 {
		return ""
	}
	return text[:end]
}
