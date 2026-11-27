package harnessfb

import (
	"fmt"
	"sort"
)

// MinSessions is how many sessions a waste signal must show up in before
// it is a repeated pattern; one or two can be chance.
const MinSessions = 3

// Hit is one session's share of a pattern.
type Hit struct {
	SessionID string  `json:"session_id"`
	Count     int     `json:"count"`
	Seconds   float64 `json:"seconds"`
	Share     float64 `json:"share"` // of the session's active time
	// Active is the session's active time, for the share's scale.
	Active float64 `json:"active_seconds"`
	// Sample is an example of what the signal said (a block reason).
	Sample string `json:"sample,omitempty"`
}

// Pattern is a waste signal and the sessions that showed it.
type Pattern struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Hits     []Hit  `json:"hits"`
	Repeated bool   `json:"repeated"`
}

// detector names a waste signal and reports a session's hit, if any.
type detector struct {
	id, title string
	hit       func(Metrics) (Hit, bool)
}

var detectors = []detector{
	{"test-wait", "테스트 대기가 세션 시간의 30% 이상", func(m Metrics) (Hit, bool) {
		return Hit{Count: m.TestRuns, Seconds: m.TestWaitSeconds, Share: m.TestWaitShare()},
			m.TestWaitShare() >= 0.3 && m.TestWaitSeconds >= 300
	}},
	{"review-heavy", "리뷰가 세션 시간의 30% 이상", func(m Metrics) (Hit, bool) {
		return Hit{Count: m.Reviews, Seconds: m.ReviewSeconds, Share: m.ReviewShare()},
			m.ReviewShare() >= 0.3 && m.ReviewSeconds >= 600
	}},
	{"repeat-review", "같은 변경을 다시 리뷰했지만 그 뒤에 고친 것이 없음", func(m Metrics) (Hit, bool) {
		return Hit{Count: m.RepeatReviewsNoFix, Seconds: m.RepeatNoFixSeconds, Share: share(m.RepeatNoFixSeconds, m.ActiveSeconds)},
			m.RepeatReviewsNoFix >= 1
	}},
	{"timeout", "명령이 시간 제한에 걸려 끊김", func(m Metrics) (Hit, bool) {
		return Hit{Count: m.Timeouts, Seconds: m.TimeoutSeconds, Share: share(m.TimeoutSeconds, m.ActiveSeconds)}, m.Timeouts >= 1
	}},
}

// namedHit is one session's hit of one pattern.
type namedHit struct {
	id, title string
	Hit
}

// sessionHits are the waste signals session m shows.
func sessionHits(m Metrics) []namedHit {
	var out []namedHit
	for _, d := range detectors {
		if h, ok := d.hit(m); ok {
			h.SessionID, h.Active = m.SessionID, m.ActiveSeconds
			out = append(out, namedHit{d.id, d.title, h})
		}
	}
	kinds := make([]string, 0, len(m.StopBlockKinds))
	for k := range m.StopBlockKinds {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		if n := m.StopBlockKinds[k]; n >= 2 {
			secs := m.StopBlockSeconds[k]
			out = append(out, namedHit{"stop-block:" + k, fmt.Sprintf("Stop hook이 같은 이유(%s)로 한 세션에서 두 번 이상 막음", k),
				Hit{SessionID: m.SessionID, Count: n, Seconds: secs, Share: share(secs, m.ActiveSeconds),
					Active: m.ActiveSeconds, Sample: m.StopBlockSample[k]}})
		}
	}
	return out
}

// Detect finds every waste signal in sessions. A signal is Repeated when
// at least MinSessions distinct sessions show it. Repeated patterns come
// first, then by how many sessions show them.
func Detect(sessions []Metrics) []Pattern {
	byID := map[string]*Pattern{}
	var order []*Pattern
	add := func(id, title string, h Hit) {
		p := byID[id]
		if p == nil {
			p = &Pattern{ID: id, Title: title}
			byID[id] = p
			order = append(order, p)
		}
		p.Hits = append(p.Hits, h)
	}
	for _, m := range sessions {
		for _, h := range sessionHits(m) {
			add(h.id, h.title, h.Hit)
		}
	}
	out := make([]Pattern, 0, len(order))
	for _, p := range order {
		p.Repeated = len(p.Hits) >= MinSessions
		out = append(out, *p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Repeated != out[j].Repeated {
			return out[i].Repeated
		}
		return len(out[i].Hits) > len(out[j].Hits)
	})
	return out
}
