package omp

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"gopkg.in/yaml.v3"
)

type OMPManagedKeyClaim struct {
	Path               string
	Value              any
	Complete           bool
	FullArrayOwnership bool
	PriorFingerprint   string
}

type OMPProjectManagedInput struct {
	Existing []byte
	Mode     fs.FileMode
	Claims   []OMPManagedKeyClaim
}

type OMPProjectManagedMapping struct {
	RelativePath string
	Data         []byte
	Mode         fs.FileMode
}

type OMPProjectManagedResult struct {
	Bytes               []byte
	Mode                fs.FileMode
	Changed             bool
	ManagedFingerprints map[string]string
	Mapping             OMPProjectManagedMapping
}

func OMPMissingManagedValueFingerprint() string {
	return config.OMPMissingManagedValueFingerprint()
}

func FingerprintOMPManagedValue(value any) (string, error) {
	canonical, err := canonicalOMPManagedValueJSON(value)
	if err != nil {
		return "", err
	}
	return OMPModelSHA256(canonical), nil
}

func MergeOMPProjectManagedConfig(input OMPProjectManagedInput) (OMPProjectManagedResult, error) {
	result := originalOMPProjectManagedResult(input)
	claims := append([]OMPManagedKeyClaim(nil), input.Claims...)
	if err := validateOMPManagedClaims(claims); err != nil {
		return result, err
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].Path < claims[j].Path })
	data := append([]byte(nil), input.Existing...)
	fingerprints := make(map[string]string, len(claims))
	edits := make([]ompManagedEdit, 0, len(claims))
	for _, claim := range claims {
		updated, fingerprint, err := applyOMPManagedClaim(data, claim)
		if err != nil {
			return result, err
		}
		data = updated
		fingerprints[claim.Path] = fingerprint
		edits = append(edits, ompManagedEdit{path: strings.Split(claim.Path, "."), value: claim.Value})
	}
	if err := verifyOMPManagedEdit(input.Existing, data, edits); err != nil {
		return result, err
	}
	result.Bytes = data
	result.Changed = string(data) != string(input.Existing)
	result.ManagedFingerprints = fingerprints
	result.Mapping = OMPProjectManagedMapping{RelativePath: ".omp/config.yml", Data: append([]byte(nil), data...), Mode: input.Mode}
	return result, nil
}

func originalOMPProjectManagedResult(input OMPProjectManagedInput) OMPProjectManagedResult {
	data := append([]byte(nil), input.Existing...)
	return OMPProjectManagedResult{
		Bytes: data,
		Mode:  input.Mode,
		Mapping: OMPProjectManagedMapping{
			RelativePath: ".omp/config.yml",
			Data:         append([]byte(nil), data...),
			Mode:         input.Mode,
		},
	}
}

func validateOMPManagedClaims(claims []OMPManagedKeyClaim) error {
	if len(claims) == 0 {
		return fmt.Errorf("at least one complete managed key claim is required")
	}
	seen := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		if !claim.Complete || !validOMPManagedPath(claim.Path) {
			return fmt.Errorf("managed_key_conflict: claim must own one complete key path")
		}
		if !validOMPModelHash(claim.PriorFingerprint) {
			return fmt.Errorf("managed_key_conflict: prior fingerprint is required")
		}
		if _, exists := seen[claim.Path]; exists {
			return fmt.Errorf("managed_key_conflict: duplicate claim %s", claim.Path)
		}
		for prior := range seen {
			if strings.HasPrefix(claim.Path, prior+".") || strings.HasPrefix(prior, claim.Path+".") {
				return fmt.Errorf("managed_key_conflict: overlapping claims %s and %s", prior, claim.Path)
			}
		}
		seen[claim.Path] = struct{}{}
		valueNode, err := ompManagedValueNode(claim.Value)
		if err != nil {
			return err
		}
		if ompYAMLNodeContainsSequence(valueNode) && !claim.FullArrayOwnership {
			return fmt.Errorf("array_ownership_required: %s", claim.Path)
		}
	}
	return nil
}

// @AX:WARN [AUTO]: managed YAML path validation has cyclomatic complexity 16.
// @AX:REASON [AUTO]: gocyclo reports 16 across segment shape, whitespace, traversal, separator, and reserved-token checks.
func validOMPManagedPath(path string) bool {
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return false
	}
	for _, segment := range strings.Split(path, ".") {
		if segment == "" {
			return false
		}
		for i, r := range segment {
			if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
				return false
			}
		}
	}
	return true
}

// @AX:WARN [AUTO]: managed YAML claim application contains 11 if branches.
// @AX:REASON [AUTO]: path ownership, node shape, preimage, conflict, and mutation outcomes are checked independently.
func applyOMPManagedClaim(data []byte, claim OMPManagedKeyClaim) ([]byte, string, error) {
	document, err := parseOMPManagedDocument(data)
	if err != nil {
		return nil, "", err
	}
	if err := rejectDuplicateOMPManagedKeys(document); err != nil {
		return nil, "", err
	}
	segments := strings.Split(claim.Path, ".")
	location, err := locateOMPManagedPath(document, segments)
	if err != nil {
		return nil, "", err
	}
	currentFingerprint := OMPMissingManagedValueFingerprint()
	if location.exists {
		current, decodeErr := decodeOMPManagedNode(location.value)
		if decodeErr != nil {
			return nil, "", decodeErr
		}
		currentFingerprint, err = FingerprintOMPManagedValue(current)
		if err != nil {
			return nil, "", err
		}
	}
	if currentFingerprint != claim.PriorFingerprint {
		return nil, "", fmt.Errorf("managed_key_conflict: prior fingerprint mismatch for %s", claim.Path)
	}
	valueNode, err := ompManagedValueNode(claim.Value)
	if err != nil {
		return nil, "", err
	}
	if location.exists && ompYAMLNodeContainsSequence(location.value) && !claim.FullArrayOwnership {
		return nil, "", fmt.Errorf("array_ownership_required: %s", claim.Path)
	}
	var updated []byte
	if location.exists {
		updated, err = replaceOMPManagedEntry(data, document, location.key, valueNode)
	} else {
		updated, err = insertOMPManagedEntry(data, document, location.mapping, segments[location.missingIndex:], valueNode)
	}
	if err != nil {
		return nil, "", err
	}
	fingerprint, err := FingerprintOMPManagedValue(claim.Value)
	return updated, fingerprint, err
}

type ompManagedLocation struct {
	mapping      *yaml.Node
	key          *yaml.Node
	value        *yaml.Node
	exists       bool
	missingIndex int
}

func locateOMPManagedPath(root *yaml.Node, segments []string) (ompManagedLocation, error) {
	current := root
	for index, segment := range segments {
		if current.Kind != yaml.MappingNode || current.Style&yaml.FlowStyle != 0 {
			return ompManagedLocation{}, fmt.Errorf("managed_key_conflict: parent of %s is not a block mapping", strings.Join(segments[:index+1], "."))
		}
		key, value := findOMPManagedPair(current, segment)
		if key == nil {
			return ompManagedLocation{mapping: current, missingIndex: index}, nil
		}
		if index == len(segments)-1 {
			return ompManagedLocation{mapping: current, key: key, value: value, exists: true}, nil
		}
		current = value
	}
	return ompManagedLocation{}, fmt.Errorf("managed_key_conflict: empty managed path")
}

func findOMPManagedPair(mapping *yaml.Node, wanted string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == wanted {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}
	return nil, nil
}
