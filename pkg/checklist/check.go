// Package checklist is the harness lesson loop: escape lessons become
// candidate checks, another provider tries to refute them, a person approves
// them, and approved checks run only on diffs that match their conditions.
package checklist

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mode is how a check's failure counts.
type Mode string

const (
	// ModeObserve records a flagged result without failing verification.
	ModeObserve Mode = "observe"
	// ModeEnforce fails verification when the check flags the change.
	ModeEnforce Mode = "enforce"
	// ModeRetired keeps the file but never selects it.
	ModeRetired Mode = "retired"
)

// When is a check's diff condition. Each list is OR; when both are set, a
// keyword must appear in an added line of a file that matched Paths.
type When struct {
	Paths    []string `yaml:"paths,omitempty" json:"paths,omitempty"`
	Keywords []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
}

// Empty reports a condition that could never select anything.
func (w When) Empty() bool { return len(w.Paths) == 0 && len(w.Keywords) == 0 }

// Check is one conditional check, stored as .autopus/checklist/<ID>.md.
type Check struct {
	ID      string   `yaml:"id" json:"id"`
	Mode    Mode     `yaml:"mode" json:"mode"`
	When    When     `yaml:"when" json:"when"`
	Verify  string   `yaml:"verify,omitempty" json:"verify,omitempty"`
	Sources []string `yaml:"sources" json:"sources"`
	Created string   `yaml:"created" json:"created"`
	// Body is the judgement criterion, the Markdown after the front matter.
	Body string `yaml:"-" json:"body"`
}

var checkIDPattern = regexp.MustCompile(`^CHK-\d{3,}$`)

// ValidID reports a well-formed check id.
func ValidID(id string) bool { return checkIDPattern.MatchString(id) }

var errNoFrontMatter = errors.New("missing front matter")

// ParseCheck decodes a check file and validates it.
func ParseCheck(data []byte) (Check, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Check{}, errNoFrontMatter
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return Check{}, errNoFrontMatter
	}
	var c Check
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &c); err != nil {
		return Check{}, fmt.Errorf("front matter: %w", err)
	}
	c.Body = strings.TrimSpace(strings.TrimPrefix(text[4+end+4:], "\n"))
	return c, c.Validate()
}

// Validate reports the first problem that keeps a check from being selected.
func (c Check) Validate() error {
	switch {
	case !ValidID(c.ID):
		return fmt.Errorf("id %q is not CHK-NNN", c.ID)
	case c.Mode != ModeObserve && c.Mode != ModeEnforce && c.Mode != ModeRetired:
		return fmt.Errorf("unknown mode: %s", c.Mode)
	case c.When.Empty():
		return errors.New("when needs paths or keywords")
	case containsEmpty(c.When.Paths) || containsEmpty(c.When.Keywords):
		return errors.New("when holds an empty path or keyword")
	case len(c.Sources) == 0:
		return errors.New("sources required")
	case c.Body == "" && c.Verify == "":
		return errors.New("a check needs a verify command or a body")
	}
	return nil
}

// Format encodes the check as its Markdown file.
func (c Check) Format() ([]byte, error) {
	var head bytes.Buffer
	enc := yaml.NewEncoder(&head)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return []byte("---\n" + head.String() + "---\n\n" + c.Body + "\n"), nil
}

func containsEmpty(list []string) bool {
	for _, s := range list {
		if s == "" {
			return true
		}
	}
	return false
}
