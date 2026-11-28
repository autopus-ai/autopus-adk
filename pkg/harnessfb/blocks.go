package harnessfb

import (
	"encoding/json"
	"regexp"
	"strconv"
	"time"
)

// openBlock is a Stop block whose redo has not reached the next stop.
type openBlock struct {
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
	// Stop is the stop the block came from; every hook of one stop shares
	// its toolUseID.
	Stop string `json:"stop"`
}

// sampleLen caps the stored example of a block reason.
const sampleLen = 200

func (s *Session) foldAttachment(rec record) {
	var a struct {
		Type          string `json:"type"`
		HookEvent     string `json:"hookEvent"`
		ToolUseID     string `json:"toolUseID"`
		Prompt        string `json:"prompt"`
		BlockingError struct {
			BlockingError string `json:"blockingError"`
			Command       string `json:"command"`
		} `json:"blockingError"`
	}
	if json.Unmarshal(rec.Attachment, &a) != nil {
		return
	}
	switch {
	case a.HookEvent == "Stop" && (a.Type == "hook_blocking_error" || a.Type == "hook_success"):
		s.stopEvent(a.ToolUseID, rec.Timestamp)
		if a.Type == "hook_blocking_error" {
			s.stopBlock(a.BlockingError.Command, a.BlockingError.BlockingError, a.ToolUseID, rec.Timestamp)
		}
	case a.Type == "queued_command":
		s.readReports(a.Prompt)
		if id, ok := notification(a.Prompt); ok {
			s.close(id, rec.Timestamp)
		} else if isPrompt(a.Prompt) {
			// A prompt the user typed while the agent worked.
			s.newTurn()
		}
	}
}

// stopEvent ends the redo of the last block when the agent stops again: a
// Stop hook event of another stop than the block's (the block's sibling
// hooks share its toolUseID).
func (s *Session) stopEvent(stop string, at time.Time) {
	if s.Blocked == nil {
		return
	}
	if stop != "" && stop == s.Blocked.Stop {
		// A sibling hook of the blocked stop: the redo starts when the
		// last of them has finished.
		s.Blocked.At = at
		return
	}
	if d := at.Sub(s.Blocked.At).Seconds(); d > 0 && d <= idleGap.Seconds() {
		if s.StopBlockSeconds == nil {
			s.StopBlockSeconds = map[string]float64{}
		}
		s.StopBlockSeconds[s.Blocked.Kind] += d
	}
	s.Blocked = nil
}

func (s *Session) stopBlock(command, reason, stop string, at time.Time) {
	kind := blockKind(command, reason)
	s.StopBlocks++
	if s.StopBlockKinds == nil {
		s.StopBlockKinds, s.StopBlockSample = map[string]int{}, map[string]string{}
	}
	s.StopBlockKinds[kind]++
	if s.StopBlockSample == nil {
		s.StopBlockSample = map[string]string{}
	}
	if _, ok := s.StopBlockSample[kind]; !ok {
		r := []rune(reason)
		if len(r) > sampleLen {
			r = r[:sampleLen]
		}
		s.StopBlockSample[kind] = string(r)
	}
	if s.Blocked == nil || s.Blocked.Stop != stop || stop == "" {
		s.Blocked = &openBlock{Kind: kind, At: at, Stop: stop}
	}
}

// timedOut matches the shell tool's timeout result, "Command timed out
// after 10m 0s".
var timedOut = regexp.MustCompile(`Command timed out after (?:(\d+)h )?(?:(\d+)m )?(\d+)s`)

func timeoutSeconds(m []string) float64 {
	n := func(s string) float64 {
		v, _ := strconv.Atoi(s)
		return float64(v)
	}
	return 3600*n(m[1]) + 60*n(m[2]) + n(m[3])
}
