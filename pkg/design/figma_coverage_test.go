package design

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuditFigma_DefaultLimitAndRefCap pins the two bounds an unbounded audit
// would blow: a non-positive maxRefs falls back to the documented default, and
// an explicit cap truncates the collected refs instead of scanning forever.
func TestAuditFigma_DefaultLimitAndRefCap(t *testing.T) {
	root := t.TempDir()
	var sb strings.Builder
	for i := range 40 {
		sb.WriteString("https://www.figma.com/design/key" + string(rune('a'+i%26)) + "/f?node-id=1-" + string(rune('0'+i%10)) + "\n")
	}
	writeFile(t, root, "design.md", sb.String())

	capped, err := AuditFigma(root, 2)
	if err != nil {
		t.Fatalf("AuditFigma: %v", err)
	}
	if len(capped.FigmaRefs) != 2 {
		t.Errorf("maxRefs=2 collected %d refs, want 2", len(capped.FigmaRefs))
	}

	defaulted, err := AuditFigma(root, 0)
	if err != nil {
		t.Fatalf("AuditFigma(0): %v", err)
	}
	if len(defaulted.FigmaRefs) != 30 {
		t.Errorf("maxRefs=0 collected %d refs, want the default cap of 30", len(defaulted.FigmaRefs))
	}
}

// TestAuditFigma_RefsSortedBySourceThenURL keeps the audit output stable across
// filesystem walk order, which generated reports and hashes depend on.
func TestAuditFigma_RefsSortedBySourceThenURL(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "b.md", "https://www.figma.com/design/k2/b\n")
	writeFile(t, root, "a.md", "https://www.figma.com/design/k1/zz\nhttps://www.figma.com/design/k1/aa\n")

	audit, err := AuditFigma(root, 10)
	if err != nil {
		t.Fatalf("AuditFigma: %v", err)
	}
	var got []string
	for _, ref := range audit.FigmaRefs {
		got = append(got, ref.SourcePath+" "+ref.URL)
	}
	want := []string{
		"a.md https://www.figma.com/design/k1/aa",
		"a.md https://www.figma.com/design/k1/zz",
		"b.md https://www.figma.com/design/k2/b",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("refs = %v, want %v", got, want)
	}
}

// TestAuditFigma_MissingRootIsAnError keeps a wrong workspace argument from
// being reported as a clean "no Figma references" audit.
func TestAuditFigma_MissingRootIsAnError(t *testing.T) {
	if _, err := AuditFigma(filepath.Join(t.TempDir(), "absent"), 5); err == nil {
		t.Error("AuditFigma on a missing root = nil error, want failure")
	}
}

// TestAuditFigma_SetupGapsReflectDetection pins the gap vocabulary consumers
// branch on: a mapping file flips Code Connect to detected, and each absent
// signal contributes exactly one gap.
func TestAuditFigma_SetupGapsReflectDetection(t *testing.T) {
	bare := t.TempDir()
	writeFile(t, bare, "notes.txt", "no design links here\n")
	audit, err := AuditFigma(bare, 5)
	if err != nil {
		t.Fatalf("AuditFigma: %v", err)
	}
	if audit.CodeConnect.Status != "missing" {
		t.Errorf("status = %q, want missing", audit.CodeConnect.Status)
	}
	if strings.Join(audit.SetupGaps, ",") != "code_connect_mapping_missing,figma_reference_missing" {
		t.Errorf("gaps = %v, want both the mapping and reference gaps", audit.SetupGaps)
	}

	full := t.TempDir()
	writeFile(t, full, "Button.figma.tsx", "export default {}\n")
	writeFile(t, full, "package.json", `{"dependencies":{"@figma/code-connect":"1.0.0"}}`)
	writeFile(t, full, "design.md", "https://www.figma.com/design/abc/Spec?node-id=12-34\n")
	audit, err = AuditFigma(full, 5)
	if err != nil {
		t.Fatalf("AuditFigma: %v", err)
	}
	if audit.CodeConnect.Status != "detected" || len(audit.SetupGaps) != 0 {
		t.Errorf("audit = %+v, want detected with no gaps", audit.CodeConnect)
	}
	if len(audit.CodeConnect.MappingRefs) != 1 || len(audit.CodeConnect.PackageRefs) != 1 {
		t.Errorf("mapping/package refs = %+v, want one each", audit.CodeConnect)
	}
}

// TestExtractFigmaRefs_DeduplicatesByNodeIdentity keeps a document that links
// the same frame twice from inflating the ref count, while two node ids on one
// file stay distinct.
func TestExtractFigmaRefs_DeduplicatesByNodeIdentity(t *testing.T) {
	content := strings.Join([]string{
		"https://www.figma.com/design/abc/Spec?node-id=12-34",
		"https://www.figma.com/design/abc/Spec?node-id=12-34&t=xyz",
		"https://www.figma.com/design/abc/Spec?node-id=99-1",
	}, "\n")

	refs := ExtractFigmaRefs("design.md", content)

	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2 after node-identity dedup: %+v", len(refs), refs)
	}
	if refs[0].NodeID != "12:34" || refs[1].NodeID != "99:1" {
		t.Errorf("node ids = %q,%q, want normalized colon form", refs[0].NodeID, refs[1].NodeID)
	}
	if refs[0].URLHash == refs[1].URLHash {
		t.Error("two distinct nodes share a URL hash")
	}
	if refs[0].URL != "https://www.figma.com/design/abc/Spec" {
		t.Errorf("URL = %q, want the query stripped", refs[0].URL)
	}
}

// TestParseFigmaURL_RejectsForeignAndMalformedURLs keeps a look-alike host or a
// non-https link from being reported as a Figma reference.
func TestParseFigmaURL_RejectsForeignAndMalformedURLs(t *testing.T) {
	for _, raw := range []string{
		"https://%zz/design/abc",
		"http://figma.com/design/abc",
		"https://evil-figma.com/design/abc",
		"https://figma.com.attacker.dev/design/abc",
	} {
		clean, kind, fileKey, nodeID := parseFigmaURL(raw)
		if clean != "" || kind != "" || fileKey != "" || nodeID != "" {
			t.Errorf("parseFigmaURL(%q) = (%q,%q,%q,%q), want all empty", raw, clean, kind, fileKey, nodeID)
		}
	}

	// A bare figma.com URL has no path segment, so the kind must fall back
	// rather than be reported as an empty string.
	clean, kind, fileKey, _ := parseFigmaURL("https://figma.com/")
	if kind != "unknown" || fileKey != "" || clean == "" {
		t.Errorf("parseFigmaURL(bare host) = (%q,%q,%q), want kind=unknown", clean, kind, fileKey)
	}
}

// TestIsCodeConnectMappingPath_Shapes pins which filenames count as Code
// Connect configuration; a false positive would report setup that is absent.
func TestIsCodeConnectMappingPath_Shapes(t *testing.T) {
	yes := []string{"src/Button.figma.ts", "a/b/Card.figma.tsx", "figma.config.json", "FIGMA.CONFIG.JS", "code-connect.config.json"}
	no := []string{"src/button.ts", "figma.ts", "config/figma.json", "code-connect.config.yaml"}
	for _, rel := range yes {
		if !isCodeConnectMappingPath(rel) {
			t.Errorf("isCodeConnectMappingPath(%q) = false, want true", rel)
		}
	}
	for _, rel := range no {
		if isCodeConnectMappingPath(rel) {
			t.Errorf("isCodeConnectMappingPath(%q) = true, want false", rel)
		}
	}
}

// TestFigmaAudit_MarkdownAndJSONRenderBothStates keeps the human report honest
// for an empty audit and for one carrying refs, mappings, and gaps.
func TestFigmaAudit_MarkdownAndJSONRenderBothStates(t *testing.T) {
	empty := FigmaAudit{CodeConnect: CodeConnectAudit{Status: "missing"}}
	md := empty.Markdown()
	if !strings.Contains(md, "- Figma refs: none") || !strings.Contains(md, "- Code Connect: missing") {
		t.Errorf("empty audit markdown missing its explicit none/status lines:\n%s", md)
	}
	if strings.Contains(md, "Setup gaps") {
		t.Errorf("empty audit rendered a gap section:\n%s", md)
	}

	full := FigmaAudit{
		FigmaRefs: []FigmaRef{{SourcePath: "design.md", Kind: "design", URLHash: "abc123"}},
		CodeConnect: CodeConnectAudit{
			Status:      "detected",
			MappingRefs: []SourceRef{{Path: "Button.figma.ts"}},
			PackageRefs: []SourceRef{{Path: "package.json"}},
		},
		SetupGaps: []string{"figma_reference_missing"},
	}
	md = full.Markdown()
	for _, want := range []string{
		"  - design.md (design, abc123)",
		"  - mapping: Button.figma.ts",
		"  - package: package.json",
		"- Setup gaps:\n  - figma_reference_missing",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}

	data, err := full.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("JSON output is not newline-terminated")
	}
	var round FigmaAudit
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("JSON output does not round-trip: %v", err)
	}
	if round.CodeConnect.Status != "detected" || len(round.FigmaRefs) != 1 {
		t.Errorf("round-tripped audit = %+v, want the original content", round)
	}
}
