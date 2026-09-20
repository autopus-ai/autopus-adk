package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillAuditReadOnly(t *testing.T) {
	root := t.TempDir()
	data := []byte("---\nname: same\ndescription: test\n---\nbody\n")
	for _, name := range []string{"one", "two"} {
		dir := filepath.Join(root, ".codex", "skills", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, ".codex", "skills", "one"), filepath.Join(root, ".codex", "skills", "linked")); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"skill", "audit", "--dir", root, "--platform", "codex", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report skillAuditReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SessionLoaded != "UNKNOWN" || len(report.Installed) != 2 {
		t.Fatalf("%+v", report)
	}
	if report.Summary.LocalBodyBytes != 2*len("body\n") || report.Summary.LocalBodyTokenEstimate != 4 {
		t.Fatalf("summary %+v", report.Summary)
	}
	if len(report.Duplicates) != 1 || len(report.Aliases) != 1 {
		t.Fatalf("duplicates=%v aliases=%v", report.Duplicates, report.Aliases)
	}
	if len(report.Missing) == 0 || report.FullVisible < report.DefaultVisible {
		t.Fatal("missing comparison")
	}
	if _, err := os.Stat(filepath.Join(root, "autopus.yaml")); !os.IsNotExist(err) {
		t.Fatal("audit wrote config")
	}
	for _, name := range []string{"one", "two"} {
		actual, err := os.ReadFile(filepath.Join(root, ".codex", "skills", name, "SKILL.md"))
		if err != nil || !bytes.Equal(actual, data) {
			t.Fatal("audit mutated file")
		}
	}
	if bytes.Contains(out.Bytes(), []byte("\\nbody")) {
		t.Fatal("body leaked")
	}
}

func TestSkillAuditScanBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".codex", "skills")); err != nil {
		t.Fatal(err)
	}
	files, skipped, err := scanSkillAuditFiles(root, []string{".codex/skills"})
	if err != nil || len(files) != 0 || len(skipped) != 1 {
		t.Fatalf("%v %v %v", files, skipped, err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "large"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills", "large", "SKILL.md"), make([]byte, skillAuditFileLimit+1), 0644); err != nil {
		t.Fatal(err)
	}
	files, skipped, err = scanSkillAuditFiles(root, []string{".agents/skills"})
	if err != nil || len(files) != 0 || len(skipped) != 1 {
		t.Fatalf("%v %v %v", files, skipped, err)
	}
}

func TestSkillAuditFormatsAndErrors(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"skill", "audit", "--dir", t.TempDir(), "--format", format})
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out.Bytes(), []byte("UNKNOWN")) {
			t.Fatal(out.String())
		}
	}
	for _, args := range [][]string{{"--platform", "invalid"}, {"--format", "invalid"}} {
		cmd := NewRootCmd()
		cmd.SetArgs(append([]string{"skill", "audit", "--dir", t.TempDir()}, args...))
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
}

func TestSkillAuditPartsUTF8AndCRLF(t *testing.T) {
	metadata, body, name, err := skillAuditParts([]byte("---\r\nname: test\r\n---\r\n한글\r\n"))
	if err != nil || name != "test" || len(metadata) != len("name: test\r\n") || len(body) != len("한글\r\n") {
		t.Fatalf("%q %q %s %v", metadata, body, name, err)
	}
}

func TestSkillAuditSummaryAndConfigSymlink(t *testing.T) {
	root := t.TempDir()
	report, err := buildSkillAudit(root, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.Summary.Registered != len(report.Catalog) || report.Summary.ConfiguredCompiled == 0 {
		t.Fatalf("%+v", report.Summary)
	}
	if report.Catalog[0].MetadataTokenEstimate != (report.Catalog[0].MetadataBytes+3)/4 {
		t.Fatal("estimate mismatch")
	}
	external := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(external, []byte("invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "autopus.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := buildSkillAudit(root, "codex"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink config: %v", err)
	}
	for _, platform := range []string{"gemini-cli", "antigravity-cli"} {
		report, err := buildSkillAudit(t.TempDir(), platform)
		if err != nil || report.Summary.ConfiguredCompiled == 0 {
			t.Fatalf("alias %s %v", platform, err)
		}
	}
}

func TestSkillAuditRejectsNonRegularOrOversizeConfig(t *testing.T) {
	for _, kind := range []string{"directory", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "autopus.yaml")
			if kind == "directory" {
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, make([]byte, skillAuditFileLimit+1), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := buildSkillAudit(root, "codex"); err == nil || !strings.Contains(err.Error(), "regular file at most") {
				t.Fatalf("unsafe config: %v", err)
			}
		})
	}
}
