package gates

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// A compact contract is a document, so this file treats it as untrusted
// input. Every field the route decision reads must be present, exactly once,
// in the shape `auto spec change` renders; anything else is refused rather
// than defaulted, because a defaulted field is how a contract ends up
// authorizing work it never described.

const changeContractFrontMatter = "---"

// changeContractHeader is the typed front matter of a compact contract.
type changeContractHeader struct {
	SpecID         string
	ChangeID       string
	DeclaredClass  ChangeKind
	EffectiveClass ChangeKind
	Tier           RiskTier
	Decision       ChangeDecision
	NewContract    bool
}

// changeContractBody is the referenced-SPEC binding, implementation surface,
// and verification plan without which a contract bounds nothing.
type changeContractBody struct {
	SpecBinding      string
	AcceptanceIDs    []string
	Surface          []string
	VerificationPlan []string
}

// parseChangeContract reads one contract document into its typed header and
// body, or names the first reason it cannot be trusted.
func parseChangeContract(document string) (changeContractHeader, changeContractBody, error) {
	fields, err := parseChangeContractFields(document)
	if err != nil {
		return changeContractHeader{}, changeContractBody{}, err
	}
	header, err := typedChangeContractHeader(fields)
	if err != nil {
		return changeContractHeader{}, changeContractBody{}, err
	}
	body, err := parseChangeContractBody(document)
	if err != nil {
		return changeContractHeader{}, changeContractBody{}, err
	}
	return header, body, nil
}

// parseChangeContractFields reads the `key: value` block delimited by the
// first pair of front-matter markers. A repeated key is an error: silently
// keeping the last one lets a second line override the risk decision.
func parseChangeContractFields(document string) (map[string]string, error) {
	fields := make(map[string]string, 8)
	inHeader := false
	for _, raw := range strings.Split(document, "\n") {
		line := strings.TrimSpace(raw)
		if line == changeContractFrontMatter {
			if inHeader {
				return fields, nil
			}
			inHeader = true
			continue
		}
		if !inHeader || line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("header line %q is not a key: value pair", line)
		}
		key = strings.TrimSpace(key)
		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("header key %q is declared more than once", key)
		}
		fields[key] = strings.TrimSpace(value)
	}
	return nil, errors.New("front matter header is unterminated")
}

func typedChangeContractHeader(fields map[string]string) (changeContractHeader, error) {
	if schema := fields["schema"]; schema != ChangeContractSchema {
		return changeContractHeader{}, fmt.Errorf("schema %q is not %s", schema, ChangeContractSchema)
	}
	header := changeContractHeader{
		SpecID:   fields["spec_id"],
		ChangeID: fields["change_id"],
	}
	if header.SpecID == "" {
		return changeContractHeader{}, errors.New("header names no spec_id")
	}
	declared, err := ParseChangeKind(fields["declared_class"])
	if err != nil {
		return changeContractHeader{}, fmt.Errorf("declared_class: %w", err)
	}
	effective, err := ParseChangeKind(fields["effective_class"])
	if err != nil {
		return changeContractHeader{}, fmt.Errorf("effective_class: %w", err)
	}
	header.DeclaredClass, header.EffectiveClass = declared, effective
	switch tier := RiskTier(fields["risk_tier"]); tier {
	case RiskLow, RiskHigh:
		header.Tier = tier
	default:
		return changeContractHeader{}, fmt.Errorf("risk_tier %q is neither %s nor %s", tier, RiskLow, RiskHigh)
	}
	switch decision := ChangeDecision(fields["decision"]); decision {
	case DecisionCompact, DecisionEscalate:
		header.Decision = decision
	default:
		return changeContractHeader{}, fmt.Errorf(
			"decision %q is neither %s nor %s", decision, DecisionCompact, DecisionEscalate)
	}
	switch fields["new_exported_contract"] {
	case "true":
		header.NewContract = true
	case "false":
		header.NewContract = false
	default:
		return changeContractHeader{}, fmt.Errorf(
			"new_exported_contract %q is neither true nor false", fields["new_exported_contract"])
	}
	return header, nil
}

// parseChangeContractBody reads the three sections a contract must carry. A
// section may be stated once; a second heading of the same name is a document
// with two answers to the same question.
func parseChangeContractBody(document string) (changeContractBody, error) {
	var body changeContractBody
	seen := make(map[string]bool, 3)
	section := ""
	for _, raw := range strings.Split(document, "\n") {
		line := strings.TrimSpace(raw)
		if heading, ok := changeContractHeading(line); ok {
			if seen[heading] {
				return changeContractBody{}, fmt.Errorf("section %q appears more than once", heading)
			}
			seen[heading] = true
			section = heading
			continue
		}
		if err := collectChangeContractLine(&body, section, line); err != nil {
			return changeContractBody{}, err
		}
	}
	return body, validateChangeContractBody(body)
}

func changeContractHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "## ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "## ")), true
}

func collectChangeContractLine(body *changeContractBody, section, line string) error {
	switch section {
	case "Referenced SPEC":
		switch {
		case strings.HasPrefix(line, "- SPEC:"):
			tokens := backquotedTokens(line)
			if len(tokens) == 0 {
				return errors.New("referenced SPEC line names no SPEC id")
			}
			if body.SpecBinding != "" {
				return errors.New("referenced SPEC is named more than once")
			}
			body.SpecBinding = tokens[0]
		case strings.HasPrefix(line, "- Acceptance criteria:"):
			if len(body.AcceptanceIDs) > 0 {
				return errors.New("acceptance criteria are listed more than once")
			}
			body.AcceptanceIDs = backquotedTokens(line)
		}
	case "Intended Surface":
		if path, ok := changeContractSurfacePath(line); ok {
			if err := validateChangeContractPath(path); err != nil {
				return err
			}
			body.Surface = append(body.Surface, path)
		}
	case "Verification Plan":
		if entry, ok := changeContractPlanEntry(line); ok {
			body.VerificationPlan = append(body.VerificationPlan, entry)
		}
	}
	return nil
}

func validateChangeContractBody(body changeContractBody) error {
	switch {
	case body.SpecBinding == "":
		return errors.New("body names no referenced SPEC")
	case len(body.AcceptanceIDs) == 0:
		return errors.New("body references no acceptance criterion")
	case len(body.Surface) == 0:
		return errors.New("body declares no intended surface")
	case len(body.VerificationPlan) == 0:
		return errors.New("body declares no verification plan")
	}
	return nil
}

// changeContractSurfacePath accepts only a backquoted bullet, so the prose
// that follows the surface list is never read as part of it.
func changeContractSurfacePath(line string) (string, bool) {
	if !strings.HasPrefix(line, "- `") {
		return "", false
	}
	tokens := backquotedTokens(line)
	if len(tokens) != 1 {
		return "", false
	}
	return tokens[0], true
}

// changeContractPlanEntry accepts a numbered list row such as "1. go test ./...".
func changeContractPlanEntry(line string) (string, bool) {
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	if digits == 0 || !strings.HasPrefix(line[digits:], ". ") {
		return "", false
	}
	entry := strings.TrimSpace(line[digits+2:])
	return entry, entry != ""
}

// backquotedTokens returns the backquoted spans of a line. Splitting on the
// quote character puts every quoted span at an odd index, so the prose
// between spans is never mistaken for a token.
func backquotedTokens(line string) []string {
	parts := strings.Split(line, "`")
	tokens := make([]string, 0, len(parts)/2)
	for i := 1; i < len(parts); i += 2 {
		if token := strings.TrimSpace(parts[i]); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// validateChangeContractPath refuses a surface entry that cannot bound a
// change set: an absolute path, an unclean path, or one that climbs out.
func validateChangeContractPath(path string) error {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	switch {
	case path == "" || cleaned != path:
		return fmt.Errorf("intended surface entry %q is not a clean relative path", path)
	case filepath.IsAbs(path) || strings.HasPrefix(path, "/"):
		return fmt.Errorf("intended surface entry %q is absolute", path)
	case path == ".." || strings.HasPrefix(path, "../"):
		return fmt.Errorf("intended surface entry %q climbs out of the project", path)
	}
	return nil
}
