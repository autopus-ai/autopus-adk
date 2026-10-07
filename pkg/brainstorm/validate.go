package brainstorm

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Problem codes of Validate, in the order its rules run.
const (
	ProblemTitle            = "title"
	ProblemHeader           = "header"
	ProblemSectionOrder     = "section_order"
	ProblemLedgerColumns    = "ledger_columns"
	ProblemLedgerRows       = "ledger_rows"
	ProblemLedgerValue      = "ledger_value"
	ProblemLedgerConfidence = "ledger_confidence"
	ProblemLedgerIfWrong    = "ledger_if_wrong"
	ProblemQuestionAudit    = "question_audit"
	ProblemOutcomeLock      = "outcome_lock"
	ProblemNextStep         = "next_step"
)

// sectionOrder is the H2 order of the content/skills/idea.md BS format.
var sectionOrder = []string{"원본 아이디어", "Clarification Ledger", "Question Audit", "Outcome Lock", "Visual Brief",
	"프로바이더별 발산 결과", "ICE 스코어링 — Top N", "추천 방향", "Evolution Ideas", "다음 단계"}

// The idea.md contract: ledger columns and rows, value sets, and the
// evidence-first confidence rule (7+ needs user text or project/code
// evidence; inferred rows stay at 6 or lower with a non-empty If Wrong).
var (
	ledgerColumns      = []string{"Field", "Status", "Source", "Confidence", "Decision / Assumption", "If Wrong", "Plan Handoff"}
	ledgerFields       = []string{"goal", "scope_boundary", "constraints", "done_evidence", "brownfield_impact"}
	ledgerStatuses     = []string{"answered", "assumed", "deferred"}
	ledgerSources      = []string{"user", "project-doc", "code", "inferred", "none"}
	evidenceSources    = []string{"user", "project-doc", "code"}
	headerFields       = []string{"**Created**:", "**Strategy**:", "**Providers**:", "**Status**:"}
	questionTransports = []string{"AskUserQuestion", "request_user_input", "question", "plain_text", "none"}
	outcomeLabels      = []string{"User-visible outcome:", "Mandatory requirements:", "Accepted assumptions:",
		"Deferred decisions:", "Explicit non-goals:", "Completion evidence:"}
)

var (
	titlePattern      = regexp.MustCompile(`^# (BS-[A-Za-z0-9-]+): \S`)
	headingPattern    = regexp.MustCompile(`^ {0,3}## (.*?)\s*$`)
	fencePattern      = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	separatorPattern  = regexp.MustCompile(`^:?-{3,}:?$`)
	confidencePattern = regexp.MustCompile(`^(10|[1-9])$`)
	nextStepPattern   = regexp.MustCompile("^`?/auto plan --from-idea (BS-[A-Za-z0-9-]+) \"[^\"]+\"`?$")
)

// document is a BS file reduced to its lines outside fences.
type document struct {
	title    string              // BS ID of line 1; empty when line 1 is no title
	header   []string            // lines between line 1 and the first H2
	order    []string            // H2 titles in order
	sections map[string][]string // body lines of the first H2 of each title
}

// Validate returns the problem codes of a BS file in rule order, each once;
// none means the file follows the idea.md format. Headings, tables, and
// next-step lines inside a fence are content and never count.
func Validate(content []byte) []string {
	doc := parse(string(content))
	var problems []string
	add := func(code string, failed bool) {
		if failed && !slices.Contains(problems, code) {
			problems = append(problems, code)
		}
	}
	add(ProblemTitle, doc.title == "")
	add(ProblemHeader, !doc.headerComplete())
	add(ProblemSectionOrder, !slices.Equal(doc.order, sectionOrder))
	columns, rows, value, confidence, ifWrong := doc.ledgerProblems()
	add(ProblemLedgerColumns, columns)
	add(ProblemLedgerRows, rows)
	add(ProblemLedgerValue, value)
	add(ProblemLedgerConfidence, confidence)
	add(ProblemLedgerIfWrong, ifWrong)
	add(ProblemQuestionAudit, !doc.questionAuditValid())
	add(ProblemOutcomeLock, !doc.outcomeLockValid())
	add(ProblemNextStep, !doc.nextStepValid())
	return problems
}

func parse(text string) document {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	doc := document{sections: map[string][]string{}}
	if match := titlePattern.FindStringSubmatch(lines[0]); match != nil {
		doc.title = match[1]
	}
	fence, current, inHeader := "", "", true
	for _, line := range lines[1:] {
		if fence != "" {
			if closesFence(line, fence) {
				fence = ""
			}
			continue
		}
		if match := fencePattern.FindStringSubmatch(line); match != nil && !(match[1][0] == '`' && strings.Contains(match[2], "`")) {
			fence = match[1]
			continue
		}
		if match := headingPattern.FindStringSubmatch(line); match != nil {
			inHeader, current = false, match[1]
			doc.order = append(doc.order, current)
			if _, seen := doc.sections[current]; seen {
				current = "" // a repeated section keeps the first body
			} else {
				doc.sections[current] = []string{}
			}
			continue
		}
		if inHeader {
			doc.header = append(doc.header, line)
		} else if current != "" {
			doc.sections[current] = append(doc.sections[current], line)
		}
	}
	return doc
}

// closesFence reports whether line closes a fence opened by run: the same
// character, at least as long, and nothing else but trailing spaces.
func closesFence(line, run string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	trimmed = strings.TrimRight(trimmed, " \t")
	return len(trimmed) >= len(run) && strings.Trim(trimmed, run[:1]) == ""
}

func (d document) headerComplete() bool {
	for _, field := range headerFields {
		found := false
		for _, line := range d.header {
			if value, ok := strings.CutPrefix(strings.TrimSpace(line), field); ok && strings.TrimSpace(value) != "" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (d document) ledgerProblems() (columns, rows, value, confidence, ifWrong bool) {
	table := firstTable(d.sections["Clarification Ledger"])
	if len(table) < 2 || !slices.Equal(table[0], ledgerColumns) || !isSeparator(table[1]) {
		columns = true
	}
	var data [][]string
	if len(table) > 2 {
		data = table[2:]
	}
	var fields []string
	for _, row := range data {
		fields = append(fields, row[0])
		if len(row) != len(ledgerColumns) {
			rows = true
			continue
		}
		status, source, score, wrong := row[1], row[2], row[3], row[5]
		value = value || !slices.Contains(ledgerStatuses, status) || !slices.Contains(ledgerSources, source)
		level, _ := strconv.Atoi(score)
		confidence = confidence || !confidencePattern.MatchString(score) || (level >= 7 && !slices.Contains(evidenceSources, source))
		ifWrong = ifWrong || (source == "inferred" && wrong == "")
	}
	rows = rows || !slices.Equal(fields, ledgerFields)
	return columns, rows, value, confidence, ifWrong
}

// firstTable returns the cells of the first run of table lines.
func firstTable(lines []string) [][]string {
	var table [][]string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			if len(table) > 0 {
				break
			}
			continue
		}
		table = append(table, splitCells(trimmed))
	}
	return table
}

// splitCells splits a table row on unescaped pipes; cell values lose
// surrounding spaces and backticks.
func splitCells(row string) []string {
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	var cells []string
	var cell strings.Builder
	for i := 0; i < len(row); i++ {
		switch {
		case row[i] == '\\' && i+1 < len(row) && row[i+1] == '|':
			cell.WriteByte('|')
			i++
		case row[i] == '|':
			cells = append(cells, strings.Trim(strings.TrimSpace(cell.String()), "`"))
			cell.Reset()
		default:
			cell.WriteByte(row[i])
		}
	}
	return append(cells, strings.Trim(strings.TrimSpace(cell.String()), "`"))
}

func isSeparator(cells []string) bool {
	for _, cell := range cells {
		if !separatorPattern.MatchString(cell) {
			return false
		}
	}
	return len(cells) > 0
}

// item strips a bullet line to "key: value"; a bold key counts as plain.
func item(line string) string {
	return strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(line), "- "), "**", "")
}

// items returns the "key: value" bullets of a section, first value wins.
func items(lines []string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		if key, value, ok := strings.Cut(item(line), ":"); ok {
			if key = strings.TrimSpace(key); out[key] == "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	}
	return out
}

func firstToken(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func (d document) questionAuditValid() bool {
	audit := items(d.sections["Question Audit"])
	count, err := strconv.Atoi(firstToken(audit["question_count"]))
	return slices.Contains(questionTransports, firstToken(audit["question_transport"])) &&
		err == nil && count >= 0 && count <= 3 && audit["unresolved_fields"] != ""
}

func (d document) outcomeLockValid() bool {
	for _, label := range outcomeLabels {
		found := false
		for _, line := range d.sections["Outcome Lock"] {
			if rest, ok := strings.CutPrefix(item(line), label); ok && strings.TrimSpace(rest) != "" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// nextStepValid requires the idea.md next-step line naming the BS of line 1.
func (d document) nextStepValid() bool {
	for _, line := range d.sections["다음 단계"] {
		if match := nextStepPattern.FindStringSubmatch(strings.TrimSpace(line)); match != nil && (d.title == "" || match[1] == d.title) {
			return true
		}
	}
	return false
}
