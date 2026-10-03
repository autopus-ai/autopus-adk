package testscenario

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const codeParseInvalid = "qa_test_scenario_parse_invalid"

// ParseBytes decodes and validates one document. name becomes Document.Path
// and the error path; callers pass a base name or a label for agent output.
//
// Unknown keys are rejected because a misspelled field is otherwise dropped
// in silence, and a case that silently lost its check reads as verified.
func ParseBytes(name string, body []byte) (Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	var doc Document
	if err := decoder.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return Document{}, invalid(name, codeParseInvalid, "file holds no YAML document")
		}
		return Document{}, invalid(name, codeParseInvalid, "%s", err.Error())
	}
	// A second document would be dropped unread; only an empty trailing
	// separator is harmless.
	for {
		var extra any
		err := decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || extra != nil {
			return Document{}, invalid(name, codeParseInvalid, "file must hold exactly one YAML document")
		}
	}
	doc.Path = name
	normalize(&doc)
	if err := Validate(doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// normalize trims identity fields so downstream ids match what Validate
// accepted. argv is left alone: its bytes are what exec receives.
func normalize(doc *Document) {
	doc.SchemaVersion = strings.TrimSpace(doc.SchemaVersion)
	doc.Spec = strings.TrimSpace(doc.Spec)
	for index := range doc.Cases {
		c := &doc.Cases[index]
		c.ID = strings.TrimSpace(c.ID)
		c.Ac = strings.TrimSpace(c.Ac)
		c.Kind = strings.TrimSpace(c.Kind)
		c.Automation.Type = strings.TrimSpace(c.Automation.Type)
		c.Automation.Scenario = strings.TrimSpace(c.Automation.Scenario)
	}
}

// LoadFile reads one document; its Path is the file's base name.
func LoadFile(path string) (Document, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	return ParseBytes(filepath.Base(path), body)
}

// LoadDir reads every active document under DirRel.
func LoadDir(projectDir string) ([]Document, error) {
	return loadGlob(filepath.Join(projectDir, DirRel))
}

// LoadCandidates reads every generated document under CandidatesDirRel.
func LoadCandidates(projectDir string) ([]Document, error) {
	return loadGlob(filepath.Join(projectDir, CandidatesDirRel))
}

// loadGlob is non-recursive so the candidates directory never leaks into the
// active set. It fails closed: a partial set would report fewer cases than
// the project declared.
func loadGlob(dir string) ([]Document, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]Document, 0, len(paths))
	for _, path := range paths {
		doc, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}
