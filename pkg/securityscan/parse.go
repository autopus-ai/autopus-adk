package securityscan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// gitleaksFinding is the part of a gitleaks JSON report entry the scan
// keeps. Secret and Match are deliberately absent, so the value cannot be
// carried any further than the decoder.
type gitleaksFinding struct {
	RuleID      string `json:"RuleID"`
	Description string `json:"Description"`
	StartLine   int    `json:"StartLine"`
	File        string `json:"File"`
	Commit      string `json:"Commit"`
	Fingerprint string `json:"Fingerprint"`
}

func parseGitleaks(data []byte) ([]Finding, error) {
	var raw []gitleaksFinding
	if err := json.Unmarshal(bytes.TrimSpace(data), &raw); err != nil {
		return nil, fmt.Errorf("gitleaks report: %w", err)
	}
	findings := make([]Finding, 0, len(raw))
	for _, r := range raw {
		loc := fmt.Sprintf("%s:%d", r.File, r.StartLine)
		if len(r.Commit) >= 7 {
			loc += " @" + r.Commit[:7]
		}
		fp := r.Fingerprint
		if fp == "" {
			fp = fmt.Sprintf("%s:%s:%d", r.File, r.RuleID, r.StartLine)
		}
		findings = append(findings, Finding{Tool: "gitleaks", Kind: KindSecret, ID: r.RuleID, Severity: SeverityHigh,
			File: r.File, Location: loc, Summary: r.Description, Fingerprint: fp})
	}
	return findings, nil
}

// govulncheck -format json writes a stream of messages, not one document.
type govulnMessage struct {
	OSV *struct {
		ID      string   `json:"id"`
		Summary string   `json:"summary"`
		Aliases []string `json:"aliases"`
	} `json:"osv"`
	Finding *struct {
		OSV          string        `json:"osv"`
		FixedVersion string        `json:"fixed_version"`
		Trace        []govulnFrame `json:"trace"`
	} `json:"finding"`
}

type govulnFrame struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Package  string `json:"package"`
	Function string `json:"function"`
	Position *struct {
		Filename string `json:"filename"`
		Line     int    `json:"line"`
	} `json:"position"`
}

// parseGovulncheck keeps only symbol-level findings, the ones whose
// vulnerable function the code can reach, as govulncheck's own text output
// does; module- and package-level matches are imported but never called.
// One finding per advisory, located at the first call site that has one.
func parseGovulncheck(data []byte) ([]Finding, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	summaries := map[string]string{}
	var findings []Finding
	seen := map[string]bool{}
	messages := 0
	for {
		var msg govulnMessage
		err := dec.Decode(&msg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("govulncheck output: %w", err)
		}
		messages++
		if msg.OSV != nil {
			summary := msg.OSV.Summary
			if len(msg.OSV.Aliases) > 0 {
				summary += " (" + strings.Join(msg.OSV.Aliases, ", ") + ")"
			}
			summaries[msg.OSV.ID] = summary
		}
		f := msg.Finding
		if f == nil || len(f.Trace) == 0 || f.Trace[0].Function == "" || seen[f.OSV] {
			continue
		}
		seen[f.OSV] = true
		top := f.Trace[0]
		pkg := top.Package
		if pkg == "" {
			pkg = top.Module
		}
		if top.Version != "" {
			pkg += "@" + top.Version
		}
		findings = append(findings, Finding{Tool: "govulncheck", Kind: KindVuln, ID: f.OSV, Severity: SeverityUnknown,
			Package: pkg, Location: callSite(f.Trace), FixedVersion: f.FixedVersion})
	}
	if messages == 0 {
		return nil, errors.New("govulncheck output: empty")
	}
	for i := range findings {
		findings[i].Summary = summaries[findings[i].ID]
	}
	return findings, nil
}

// callSite is the first frame past the vulnerable symbol that has a source
// position: the project code that reaches it.
func callSite(trace []govulnFrame) string {
	for _, fr := range trace[1:] {
		if fr.Position != nil && fr.Position.Filename != "" {
			return fmt.Sprintf("%s:%d", fr.Position.Filename, fr.Position.Line)
		}
	}
	return ""
}
