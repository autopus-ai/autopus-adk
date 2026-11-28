package harnessfb

import (
	"fmt"
	"html"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/secretscan"
)

// A review's findings are the mistakes a lint rule or a test could have
// caught. The same kind of finding in the same part of the code across
// sessions is the signal "All Code Is Slop" turns into a check; it is read
// from the reviewer's report with no model call.

// Finding is one file:line a reviewer pointed at, with its line of text.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Summary string `json:"summary"`
}

const (
	maxReportFindings = 20
	// A session keeps the latest maxAreaFindings findings of each area, in
	// at most maxAreas areas: a pattern shows five per session, and a
	// session-wide cap filled in the first day of a weeks-long session.
	maxAreaFindings = 5
	maxAreas        = 60
	maxSummary      = 160
	// reviewAreaPrefix names the pattern of findings in one area.
	reviewAreaPrefix = "review-area:"
)

var (
	fileLineRef      = regexp.MustCompile(`([A-Za-z0-9_.@/\-]+\.[A-Za-z0-9]{1,8}):(\d+)`)
	agentMessageFrom = regexp.MustCompile(`<agent-message from="([^"]+)">`)
	taskID           = regexp.MustCompile(`<task-id>([^<\s]+)</task-id>`)
	teammateMessage  = regexp.MustCompile(`(?s)<teammate-message teammate_id="([^"]+)"[^>]*>(.*?)</teammate-message>`)
	// markupTag is the harness's own envelope tags, not a generic in code.
	markupTag = regexp.MustCompile(`</?(?:result|task-notification|task-id|tool-use-id|output-file|status|summary|usage|agent-message|teammate-message)\b[^>]*>`)
)

// reviewFindings reads the file:line lines of a review report. Paths are
// resolved against the repository (or made relative to cwd without an
// index); a path outside the project, a fenced code block (a reproduction
// command) and a table row (a re-review's status of findings already
// counted) are skipped.
func reviewFindings(text, cwd string, idx *PathIndex) []Finding {
	var out []Finding
	seen := map[string]bool{}
	fenced := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced || strings.HasPrefix(line, "|") {
			continue
		}
		loc := fileLineRef.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}
		m := []string{line[loc[0]:loc[1]], line[loc[2]:loc[3]], line[loc[4]:loc[5]]}
		file, ok := projectPath(m[1], cwd, idx)
		if !ok {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		key := file + ":" + m[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		// The summary names the file as recorded, not by a home path.
		cited := line[:loc[2]] + file + line[loc[3]:]
		out = append(out, Finding{File: file, Line: n, Summary: summarize(cited)})
		if len(out) == maxReportFindings {
			break
		}
	}
	return out
}

func projectPath(p, cwd string, idx *PathIndex) (string, bool) {
	if strings.HasPrefix(p, "...") || strings.Contains(p, "/.../") {
		return "", false // an abbreviated path names no file
	}
	if idx != nil {
		var ok bool
		if p, ok = idx.resolve(p); !ok {
			return "", false
		}
	} else if strings.HasPrefix(p, "/") {
		if cwd == "" || !strings.HasPrefix(p, strings.TrimSuffix(cwd, "/")+"/") {
			return "", false
		}
		p = strings.TrimPrefix(p, strings.TrimSuffix(cwd, "/")+"/")
	}
	p = path.Clean(strings.TrimPrefix(p, "./"))
	ext := path.Ext(p)
	if p == "." || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/") || strings.Trim(ext, ".0123456789") == "" {
		return "", false // not a file: a version number or a time
	}
	return p, true
}

func summarize(line string) string {
	line = html.UnescapeString(markupTag.ReplaceAllString(line, ""))
	line, _ = secretscan.Redact(line)
	line = strings.TrimLeft(line, "-*#>|0123456789. ")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.Join(strings.Fields(line), " ")
	if r := []rune(line); len(r) > maxSummary {
		line = string(r[:maxSummary]) + "…"
	}
	return line
}

// addFindings keeps a session's findings, each once: the latest
// maxAreaFindings of each area, in at most maxAreas areas.
func (s *Session) addFindings(found []Finding) {
	for _, f := range found {
		area := findingArea(f.File)
		var same []int
		areas := map[string]bool{}
		dup := false
		for i, g := range s.Findings {
			dup = dup || (g.File == f.File && g.Line == f.Line)
			ga := findingArea(g.File)
			areas[ga] = true
			if ga == area {
				same = append(same, i)
			}
		}
		switch {
		case dup:
		case len(same) >= maxAreaFindings:
			s.Findings = append(append(s.Findings[:same[0]:same[0]], s.Findings[same[0]+1:]...), f)
		case len(same) == 0 && len(areas) >= maxAreas:
		default:
			s.Findings = append(s.Findings, f)
		}
	}
}

// readReports adds the findings of every reviewer report text carries.
func (s *Session) readReports(text string) {
	for _, r := range s.reviewerReports(text) {
		s.addFindings(reviewFindings(r, s.Cwd, s.Paths))
	}
}

// reviewerReports are the reviewer reports a user-role text carries: a
// background reviewer's message or notification result, or the messages of
// reviewer teammates (several can arrive in one record).
func (s *Session) reviewerReports(text string) []string {
	if m := agentMessageFrom.FindStringSubmatch(text); m != nil && s.Reviewers[m[1]] {
		return []string{text}
	}
	if m := taskID.FindStringSubmatch(text); m != nil && s.Reviewers[m[1]] && strings.Contains(text, "<result>") {
		return []string{text}
	}
	var out []string
	for _, m := range teammateMessage.FindAllStringSubmatch(text, -1) {
		if s.Reviewers[m[1]] {
			out = append(out, m[2])
		}
	}
	return out
}

// findingArea is the part of the code a finding is in: the first two
// directories of its path, or the file itself at the top.
func findingArea(file string) string {
	dir := path.Dir(file)
	if dir == "." {
		return file
	}
	parts := strings.Split(dir, "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

// findingHits are session m's review findings grouped by area.
func findingHits(m Metrics) []namedHit {
	byArea := map[string][]Finding{}
	for _, f := range m.Findings {
		a := findingArea(f.File)
		byArea[a] = append(byArea[a], f)
	}
	areas := make([]string, 0, len(byArea))
	for a := range byArea {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	var out []namedHit
	for _, a := range areas {
		fs := byArea[a]
		h := Hit{SessionID: m.SessionID, Count: len(fs), Active: m.ActiveSeconds, End: m.End}
		for i, f := range fs {
			if i == 5 {
				break
			}
			ref := fmt.Sprintf("%s:%d", f.File, f.Line)
			if strings.Contains(f.Summary, ref) {
				h.Findings = append(h.Findings, f.Summary)
			} else {
				h.Findings = append(h.Findings, ref+" "+f.Summary)
			}
		}
		out = append(out, namedHit{reviewAreaPrefix + a, fmt.Sprintf("같은 영역(%s)에서 리뷰 지적이 반복됨", a), h})
	}
	return out
}

// isReviewArea reports a pattern made of review findings.
func isReviewArea(pattern string) bool { return strings.HasPrefix(pattern, reviewAreaPrefix) }

// interleave picks up to n findings from hits, one per session in turn,
// the most recent session first, so every session is seen.
func interleave(hits []Hit, n int) []string {
	hs := append([]Hit(nil), hits...)
	sort.SliceStable(hs, func(i, j int) bool { return hs[i].End.After(hs[j].End) })
	var out []string
	for round := 0; len(out) < n; round++ {
		added := false
		for _, h := range hs {
			if round < len(h.Findings) && len(out) < n {
				out = append(out, fmt.Sprintf("[session %s] %q", h.SessionID, h.Findings[round]))
				added = true
			}
		}
		if !added {
			break
		}
	}
	return out
}
