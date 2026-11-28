package harnessfb

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// foldMeta reads a harness-written user record only for a background
// reviewer's report, which Claude Code delivers as one.
func (s *Session) foldMeta(rec record) {
	text, list := blocks(rec.Message)
	for _, b := range list {
		if b.Type == "text" {
			text += b.Text
		}
	}
	s.readReports(text)
}

var (
	agentIDLine = regexp.MustCompile(`agentId: ([A-Za-z0-9_-]+)`)
	// A named teammate's spawn result names it instead of an agentId.
	teammateIDLine = regexp.MustCompile(`(?m)^\s*(?:agent_id|name): ([A-Za-z0-9_.-]+)(@[A-Za-z0-9_.-]+)?\s*$`)
)

func (s *Session) result(b block, at time.Time) {
	var text string
	if json.Unmarshal(b.Content, &text) != nil {
		var parts []block
		_ = json.Unmarshal(b.Content, &parts)
		for _, p := range parts {
			text += p.Text
		}
	}
	sp, ok := s.Open[b.ToolUseID]
	if !ok {
		return
	}
	if m := timedOut.FindStringSubmatch(text); m != nil && (sp.Kind == "test" || sp.Kind == "bash") {
		s.Timeouts++
		s.TimeoutSeconds += timeoutSeconds(m)
	}
	if sp.Kind == "review" {
		if s.Reviewers == nil {
			s.Reviewers = map[string]bool{}
		}
		if m := agentIDLine.FindStringSubmatch(text); m != nil {
			s.Reviewers[m[1]] = true
		}
		for _, m := range teammateIDLine.FindAllStringSubmatch(text, -1) {
			s.Reviewers[m[1]] = true
		}
	}
	// A background command or agent ends when its notification arrives.
	background := strings.Contains(text, "running in background with ID") || strings.Contains(text, "Async agent launched") ||
		strings.Contains(text, `"resumedAgentId"`)
	if sp.Kind == "review" && !background {
		s.addFindings(reviewFindings(text, s.Cwd, s.Paths))
	}
	if background {
		return
	}
	s.close(b.ToolUseID, at)
}

func (s *Session) close(id string, at time.Time) {
	sp, ok := s.Open[id]
	if !ok {
		return
	}
	delete(s.Open, id)
	secs := at.Sub(sp.Start).Seconds()
	if secs < 0 {
		secs = 0
	}
	switch sp.Kind {
	case "test":
		s.TestRuns++

	case "agent":
		// A sub-agent's own edits are not in this transcript; one that
		// ran may have fixed what a review found.
		s.LastEdit = at
	case "review":
		s.TurnRevs++
		s.Reviews++

		if sp.Repeat {
			s.RepeatReviews++
			s.RepeatReviewSeconds += secs
			s.Pending = append(s.Pending, pendingRepeat{End: at, Seconds: secs})
		}
	}
}
