package harnessfb

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"time"
)

// idleGap is the longest gap between two transcript records that still
// counts as session time; a longer one is a user away.
const idleGap = 30 * time.Minute

// span is a tool call still running: a test, a review, or another call
// whose end only a later record shows.
type span struct {
	Kind   string    `json:"kind"` // test | review | other
	Start  time.Time `json:"start"`
	Repeat bool      `json:"repeat,omitempty"`
}

// pendingRepeat is a repeat review of the current turn whose verdict (did
// anything get edited after it?) waits for the turn to end.
type pendingRepeat struct {
	End     time.Time `json:"end"`
	Seconds float64   `json:"seconds"`
}

// Session is a session's metrics plus the parser state that lets the next
// stop read only the transcript bytes appended since.
type Session struct {
	Metrics
	Offset    int64           `json:"offset"`
	LastTS    time.Time       `json:"last_ts"`
	Open      map[string]span `json:"open,omitempty"`
	Reviewers map[string]bool `json:"reviewers,omitempty"`
	TurnRevs  int             `json:"change_reviews"` // finished reviews of the current change
	Pending   []pendingRepeat `json:"pending,omitempty"`
	LastEdit  time.Time       `json:"last_edit"`
	Cwd       string          `json:"cwd,omitempty"`
	// Paths resolves cited paths against the repository; Observe sets it.
	Paths *PathIndex `json:"-"`
	// Blocked is the Stop block whose redo runs until the next Stop event.
	Blocked *openBlock `json:"blocked,omitempty"`
}

// record is the part of a Claude Code transcript line the parser reads.
type record struct {
	Type          string          `json:"type"`
	Timestamp     time.Time       `json:"timestamp"`
	IsSidechain   bool            `json:"isSidechain"`
	IsMeta        bool            `json:"isMeta"`
	Cwd           string          `json:"cwd"`
	Message       json.RawMessage `json:"message"`
	Attachment    json.RawMessage `json:"attachment"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type block struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	Text      string          `json:"text"`
}

// ReadClaudeTranscript reads the transcript bytes s has not read yet and
// folds them into s. A partly written last line is left for the next
// read. A transcript shorter than s.Offset (rewritten) is read again from
// the start.
func ReadClaudeTranscript(path string, s *Session) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() < s.Offset {
		*s = Session{Metrics: Metrics{SessionID: s.SessionID}, Paths: s.Paths}
	}
	if s.Platform == "" {
		s.Platform = "claude-code"
	}
	if _, err := f.Seek(s.Offset, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		s.Offset += int64(len(line))
		var rec record
		if json.Unmarshal(line, &rec) != nil || rec.IsSidechain {
			continue
		}
		s.fold(rec)
	}
}

func (s *Session) fold(rec record) {
	if rec.Cwd != "" {
		s.Cwd = rec.Cwd
	}
	if !rec.Timestamp.IsZero() {
		if s.Start.IsZero() {
			s.Start = rec.Timestamp
		}
		if !s.LastTS.IsZero() {
			s.addGap(rec.Timestamp.Sub(s.LastTS), rec.Timestamp)
		}
		if rec.Timestamp.After(s.LastTS) {
			s.LastTS = rec.Timestamp
		}
		s.End = s.LastTS
	}
	switch rec.Type {
	case "assistant":
		s.foldAssistant(rec)
	case "user":
		if !rec.IsMeta {
			s.foldUser(rec)
		} else {
			s.foldMeta(rec)
		}
	case "attachment":
		s.foldAttachment(rec)
	}
}

// spanLimit is how long an open span keeps a gap from being idle: a
// background call whose end the transcript never shows must not make a
// night away count as work.
const spanLimit = 2 * time.Hour

// addGap counts the time between two records: as session time when it is
// short or a tool call was running, and as test or review time for each
// kind running through it, so spans that overlap count once.
func (s *Session) addGap(gap time.Duration, now time.Time) {
	if gap <= 0 {
		return
	}
	test, review, busy := false, false, false
	for _, sp := range s.Open {
		if now.Sub(sp.Start) > spanLimit {
			continue
		}
		busy = true
		test = test || sp.Kind == "test"
		review = review || sp.Kind == "review"
	}
	if gap > idleGap && !busy {
		return
	}
	s.ActiveSeconds += gap.Seconds()
	if test {
		s.TestWaitSeconds += gap.Seconds()
	}
	if review {
		s.ReviewSeconds += gap.Seconds()
	}
}

func blocks(msg json.RawMessage) (text string, list []block) {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return "", nil
	}
	if json.Unmarshal(m.Content, &text) == nil {
		return text, nil
	}
	_ = json.Unmarshal(m.Content, &list)
	return "", list
}

func (s *Session) foldAssistant(rec record) {
	_, list := blocks(rec.Message)
	for _, b := range list {
		if b.Type != "tool_use" {
			continue
		}
		var in toolInput
		_ = json.Unmarshal(b.Input, &in)
		if isEdit(b.Name, in.Command) {
			s.LastEdit = rec.Timestamp
		}
		if b.Name == "Bash" && gitCommit.MatchString(in.Command) {
			// A commit closes a change: a review after it reviews another.
			s.endChange()
		}
		kind := classify(b.Name, in, s.Reviewers)
		if kind == "" {
			continue
		}
		sp := span{Kind: kind, Start: rec.Timestamp}
		// A review is a repeat when one of the same change has finished
		// before it started; reviews run side by side are not.
		sp.Repeat = kind == "review" && s.TurnRevs > 0
		if s.Open == nil {
			s.Open = map[string]span{}
		}
		s.Open[b.ID] = sp
	}
}

func (s *Session) foldUser(rec record) {
	text, list := blocks(rec.Message)
	if text != "" {
		s.readReports(text)
		if id, ok := notification(text); ok {
			s.close(id, rec.Timestamp)
		} else if isPrompt(text) {
			s.newTurn()
		}
		return
	}
	prompt := false
	for _, b := range list {
		switch b.Type {
		case "tool_result":
			s.result(b, rec.Timestamp)
		case "text":
			s.readReports(b.Text)
			if id, ok := notification(b.Text); ok {
				s.close(id, rec.Timestamp)
			} else if isPrompt(b.Text) {
				prompt = true
			}
		}
	}
	if prompt {
		s.newTurn()
	}
}

var gitCommit = regexp.MustCompile(`\bgit( -C \S+)? commit\b`)

// newTurn starts a user turn, which also ends the change under work.
func (s *Session) newTurn() {
	s.endChange()
	s.Turns++
}

// endChange closes the change under work (a user turn or a commit): its
// repeat reviews get their verdict, and its review count resets.
func (s *Session) endChange() {
	for _, p := range s.Pending {
		if !s.LastEdit.After(p.End) {
			s.RepeatReviewsNoFix++
			s.RepeatNoFixSeconds += p.Seconds
		}
	}
	s.Pending = nil
	s.TurnRevs = 0
}
