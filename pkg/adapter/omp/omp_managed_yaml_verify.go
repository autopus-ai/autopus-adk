package omp

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ompManagedEdit is one intended change of a managed edit: path set to
// value, or removed together with each parent the removal leaves empty.
type ompManagedEdit struct {
	path   []string
	value  any
	remove bool
}

// verifyOMPManagedEdit re-reads a textual managed edit before anything can
// write it: after has to parse as one block-mapping document and decode to
// before with exactly edits applied. A splice that touched anything else, or
// produced YAML that reads differently, is managed_key_conflict, so no span
// mistake can reach a write.
func verifyOMPManagedEdit(before, after []byte, edits []ompManagedEdit) error {
	if _, err := parseOMPManagedDocument(after); err != nil {
		return fmt.Errorf("managed_key_conflict: the edited project config does not parse: %w", err)
	}
	want, err := decodeOMPManagedConfig(before)
	if err != nil {
		return err
	}
	for _, edit := range edits {
		if err := applyOMPManagedEdit(want, edit); err != nil {
			return err
		}
	}
	got, err := decodeOMPManagedConfig(after)
	if err != nil {
		return err
	}
	wantBytes, wantErr := yaml.Marshal(want)
	gotBytes, gotErr := yaml.Marshal(got)
	if wantErr != nil || gotErr != nil || !bytes.Equal(wantBytes, gotBytes) {
		return fmt.Errorf("managed_key_conflict: editing the managed keys would change other project config")
	}
	return nil
}

func decodeOMPManagedConfig(data []byte) (map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("managed_key_conflict: decode project config: %w", err)
	}
	if document == nil {
		document = map[string]any{}
	}
	return document, nil
}

func applyOMPManagedEdit(document map[string]any, edit ompManagedEdit) error {
	if edit.remove {
		removeOMPManagedValue(document, edit.path)
		return nil
	}
	current := document
	for _, segment := range edit.path[:len(edit.path)-1] {
		next, exists := current[segment]
		if !exists {
			child := map[string]any{}
			current[segment], current = child, child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("managed_key_conflict: parent of %s is not a mapping", strings.Join(edit.path, "."))
		}
		current = child
	}
	current[edit.path[len(edit.path)-1]] = edit.value
	return nil
}

// removeOMPManagedValue deletes path, then each parent on it the deletion
// left empty, as removeOMPManagedPath does in the text.
func removeOMPManagedValue(mapping map[string]any, path []string) bool {
	if len(path) == 1 {
		_, exists := mapping[path[0]]
		delete(mapping, path[0])
		return exists
	}
	child, ok := mapping[path[0]].(map[string]any)
	if !ok || !removeOMPManagedValue(child, path[1:]) {
		return false
	}
	if len(child) == 0 {
		delete(mapping, path[0])
	}
	return true
}
