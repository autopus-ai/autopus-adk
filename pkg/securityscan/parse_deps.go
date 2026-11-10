package securityscan

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// npmAdvisory is the advisory shape of npm 6, pnpm, yarn 3, and yarn 1's
// auditAdvisory events.
type npmAdvisory struct {
	ID               json.Number `json:"id"`
	ModuleName       string      `json:"module_name"`
	Severity         string      `json:"severity"`
	Title            string      `json:"title"`
	URL              string      `json:"url"`
	GitHubAdvisoryID string      `json:"github_advisory_id"`
	PatchedVersions  string      `json:"patched_versions"`
	Findings         []struct {
		Version string `json:"version"`
	} `json:"findings"`
}

type nodeAuditDoc struct {
	Error *struct {
		Code    string `json:"code"`
		Summary string `json:"summary"`
	} `json:"error"`
	Vulnerabilities map[string]struct {
		Severity     string            `json:"severity"`
		Via          []json.RawMessage `json:"via"`
		Range        string            `json:"range"`
		FixAvailable json.RawMessage   `json:"fixAvailable"`
	} `json:"vulnerabilities"`
	Advisories map[string]npmAdvisory `json:"advisories"`
}

// parseNodeAudit reads npm 7+ (`vulnerabilities`), npm 6 / pnpm / yarn 3
// (`advisories`), yarn 1 (NDJSON auditAdvisory events) and yarn 4 (NDJSON
// value/children lines).
func parseNodeAudit(data []byte) ([]Finding, error) {
	var doc nodeAuditDoc
	if err := json.Unmarshal(bytes.TrimSpace(data), &doc); err == nil {
		switch {
		case doc.Error != nil:
			return nil, fmt.Errorf("audit failed: %s %s", doc.Error.Code, doc.Error.Summary)
		case doc.Vulnerabilities != nil:
			return npmV7Findings(doc), nil
		case doc.Advisories != nil:
			var out []Finding
			for _, key := range sortedKeys(doc.Advisories) {
				out = append(out, advisoryFinding(doc.Advisories[key]))
			}
			return out, nil
		}
	}
	return yarnLines(data)
}

func npmV7Findings(doc nodeAuditDoc) []Finding {
	var out []Finding
	for _, name := range sortedKeys(doc.Vulnerabilities) {
		v := doc.Vulnerabilities[name]
		f := Finding{Tool: "node-audit", Kind: KindVuln, Severity: NormalizeSeverity(v.Severity), Package: name + "@" + v.Range}
		for _, raw := range v.Via {
			var via struct {
				Source json.Number `json:"source"`
				Title  string      `json:"title"`
				URL    string      `json:"url"`
			}
			var dep string
			if json.Unmarshal(raw, &via) == nil && via.Title != "" {
				f.ID, f.Summary = advisoryID("", via.URL, via.Source.String()), via.Title
				break
			}
			if json.Unmarshal(raw, &dep) == nil && f.ID == "" {
				f.ID, f.Summary = "via:"+dep, "depends on a vulnerable "+dep
			}
		}
		var fix struct{ Name, Version string }
		var available bool
		if json.Unmarshal(v.FixAvailable, &fix) == nil && fix.Name != "" {
			f.FixedVersion = fix.Name + "@" + fix.Version
		} else if json.Unmarshal(v.FixAvailable, &available) == nil && available {
			f.FixedVersion = "available (audit fix)"
		}
		out = append(out, f)
	}
	return out
}

func advisoryFinding(a npmAdvisory) Finding {
	pkg := a.ModuleName
	if len(a.Findings) > 0 && a.Findings[0].Version != "" {
		pkg += "@" + a.Findings[0].Version
	}
	return Finding{Tool: "node-audit", Kind: KindVuln, ID: advisoryID(a.GitHubAdvisoryID, a.URL, a.ID.String()),
		Severity: NormalizeSeverity(a.Severity), Package: pkg, FixedVersion: a.PatchedVersions, Summary: a.Title}
}

// advisoryID prefers a GHSA id, then one in the advisory URL, then the
// registry's numeric id.
func advisoryID(ghsa, url, numeric string) string {
	if ghsa != "" {
		return ghsa
	}
	if i := strings.LastIndex(url, "/"); i >= 0 && strings.HasPrefix(url[i+1:], "GHSA-") {
		return url[i+1:]
	}
	return numeric
}

func yarnLines(data []byte) ([]Finding, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil // yarn 4 prints nothing when there is nothing to report
	}
	var out []Finding
	parsed := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for sc.Scan() {
		var line struct {
			Type string `json:"type"`
			Data struct {
				Advisory *npmAdvisory `json:"advisory"`
			} `json:"data"`
			Value    string `json:"value"`
			Children *struct {
				ID           any      `json:"ID"`
				Issue        string   `json:"Issue"`
				Severity     string   `json:"Severity"`
				Vulnerable   string   `json:"Vulnerable Versions"`
				TreeVersions []string `json:"Tree Versions"`
			} `json:"children"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}
		parsed++
		switch {
		case line.Type == "auditAdvisory" && line.Data.Advisory != nil:
			out = append(out, advisoryFinding(*line.Data.Advisory))
		case line.Value != "" && line.Children != nil:
			c := line.Children
			pkg := line.Value
			if len(c.TreeVersions) > 0 {
				pkg += "@" + c.TreeVersions[0]
			}
			out = append(out, Finding{Tool: "node-audit", Kind: KindVuln, ID: fmt.Sprint(c.ID), Severity: NormalizeSeverity(c.Severity),
				Package: pkg, Summary: fmt.Sprintf("%s (vulnerable %s)", c.Issue, c.Vulnerable)})
		}
	}
	if parsed == 0 {
		return nil, errors.New("audit output is not JSON")
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func parseCargoAudit(data []byte) ([]Finding, error) {
	var doc struct {
		Vulnerabilities *struct {
			List []struct {
				Advisory struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				} `json:"advisory"`
				Versions struct {
					Patched []string `json:"patched"`
				} `json:"versions"`
				Package struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"package"`
			} `json:"list"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &doc); err != nil || doc.Vulnerabilities == nil {
		return nil, fmt.Errorf("cargo audit output: %v", err)
	}
	var out []Finding
	for _, v := range doc.Vulnerabilities.List {
		out = append(out, Finding{Tool: "cargo-audit", Kind: KindVuln, ID: v.Advisory.ID, Severity: SeverityUnknown,
			Package: v.Package.Name + "@" + v.Package.Version, FixedVersion: strings.Join(v.Versions.Patched, ", "), Summary: v.Advisory.Title})
	}
	return out, nil
}

type pipDependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Vulns   []struct {
		ID          string   `json:"id"`
		FixVersions []string `json:"fix_versions"`
		Aliases     []string `json:"aliases"`
		Description string   `json:"description"`
	} `json:"vulns"`
}

// parsePipAudit reads pip-audit's JSON: an object with `dependencies`, or
// the bare list older releases print.
func parsePipAudit(data []byte) ([]Finding, error) {
	data = bytes.TrimSpace(data)
	var doc struct {
		Dependencies []pipDependency `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		if err := json.Unmarshal(data, &doc.Dependencies); err != nil {
			return nil, fmt.Errorf("pip-audit output: %w", err)
		}
	}
	var out []Finding
	for _, dep := range doc.Dependencies {
		for _, v := range dep.Vulns {
			summary := v.Description
			if len(v.Aliases) > 0 {
				summary += " (" + strings.Join(v.Aliases, ", ") + ")"
			}
			out = append(out, Finding{Tool: "pip-audit", Kind: KindVuln, ID: v.ID, Severity: SeverityUnknown,
				Package: dep.Name + "@" + dep.Version, FixedVersion: strings.Join(v.FixVersions, ", "), Summary: summary})
		}
	}
	return out, nil
}
