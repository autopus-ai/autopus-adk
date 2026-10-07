package harneval

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// surfaceEditor applies one seeded mutation to every generated variant
// surface, the way a change to a canonical source reaches every variant, and
// records how to undo each edit so the next mutation starts from the clean
// surface. An edit that does not find its target exactly as written fails the
// test: a mutation that silently stopped applying would prove nothing.
type surfaceEditor struct {
	t          *testing.T
	generation *Generation
	undo       []func() error
}

// surfaces returns each distinct generated surface once, in variant order.
func (e *surfaceEditor) surfaces() []*Surface {
	var surfaces []*Surface
	seen := map[*Surface]bool{}
	for _, key := range e.generation.VariantKeys() {
		if surface := e.generation.Surfaces[key]; !seen[surface] {
			seen[surface] = true
			surfaces = append(surfaces, surface)
		}
	}
	return surfaces
}

// edit rewrites a file the platform generated on every surface.
func (e *surfaceEditor) edit(platform, rel string, change func([]byte) []byte) {
	e.t.Helper()
	for _, surface := range e.surfaces() {
		data, problem := surfaceFile(surface, platform, rel)
		require.Empty(e.t, problem, "%s %s is not on the generated surface", platform, rel)
		full := filepath.Join(surface.Root, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		require.NoError(e.t, err)
		e.undo = append(e.undo, func() error { return restoreFile(full, data, info.Mode().Perm()) })
		require.NoError(e.t, os.WriteFile(full, change(data), info.Mode().Perm()))
	}
}

// replace swaps the single occurrence of old for replacement.
func (e *surfaceEditor) replace(platform, rel, old, replacement string) {
	e.t.Helper()
	e.edit(platform, rel, func(data []byte) []byte {
		require.Equal(e.t, 1, bytes.Count(data, []byte(old)), "%s %s must hold the target exactly once: %q", platform, rel, old)
		return bytes.Replace(data, []byte(old), []byte(replacement), 1)
	})
}

// editJSON lets change edit the decoded document and re-encodes it the way
// the generator does. The unedited document must round-trip byte for byte,
// so the edit is the only difference; change reports whether it edited.
func (e *surfaceEditor) editJSON(platform, rel string, change func(doc map[string]any) bool) {
	e.t.Helper()
	e.edit(platform, rel, func(data []byte) []byte {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var doc map[string]any
		require.NoError(e.t, decoder.Decode(&doc))
		require.Equal(e.t, string(data), encodeIndented(e.t, doc), "%s %s does not round-trip", platform, rel)
		require.True(e.t, change(doc), "%s %s: the mutation found nothing to edit", platform, rel)
		return []byte(encodeIndented(e.t, doc))
	})
}

func encodeIndented(t *testing.T, doc any) string {
	data, err := json.MarshalIndent(doc, "", "  ")
	require.NoError(t, err)
	return string(data) + "\n"
}

// remove deletes a file the platform generated and drops it from the
// platform's ownership, as an adapter that stopped generating it would.
func (e *surfaceEditor) remove(platform, rel string) {
	e.t.Helper()
	for _, surface := range e.surfaces() {
		data, problem := surfaceFile(surface, platform, rel)
		require.Empty(e.t, problem, "%s %s is not on the generated surface", platform, rel)
		full := filepath.Join(surface.Root, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		require.NoError(e.t, err)
		e.undo = append(e.undo, func() error {
			surface.Owned[platform][rel] = true
			return restoreFile(full, data, info.Mode().Perm())
		})
		delete(surface.Owned[platform], rel)
		require.NoError(e.t, os.Remove(full))
	}
}

// leak writes a copy of one platform's file onto another platform's surface,
// as an adapter that lost its platform gating would. The target must not be
// on that surface yet.
func (e *surfaceEditor) leak(fromPlatform, fromRel, toPlatform, toRel string) {
	e.t.Helper()
	for _, surface := range e.surfaces() {
		data, problem := surfaceFile(surface, fromPlatform, fromRel)
		require.Empty(e.t, problem, "%s %s is not on the generated surface", fromPlatform, fromRel)
		require.NotNil(e.t, surface.Owned[toPlatform], "%s generated nothing", toPlatform)
		require.False(e.t, surface.Owned[toPlatform][toRel], "%s already generates %s", toPlatform, toRel)
		full := filepath.Join(surface.Root, filepath.FromSlash(toRel))
		created := missingDirs(e.t, filepath.Dir(full))
		e.undo = append(e.undo, func() error {
			delete(surface.Owned[toPlatform], toRel)
			err := os.Remove(full)
			for _, dir := range created {
				err = errors.Join(err, os.Remove(dir))
			}
			return err
		})
		require.NoError(e.t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(e.t, os.WriteFile(full, data, 0o644))
		surface.Owned[toPlatform][toRel] = true
	}
}

// missingDirs lists dir and each missing parent, deepest first, so an undo
// removes exactly the directories a leak created.
func missingDirs(t *testing.T, dir string) []string {
	var missing []string
	for {
		_, err := os.Lstat(dir)
		if err == nil {
			return missing
		}
		require.ErrorIs(t, err, fs.ErrNotExist)
		missing = append(missing, dir)
		dir = filepath.Dir(dir)
	}
}

// swapVariant makes a variant resolve to another variant's surface, so its
// tasks are evaluated on bytes generated without its overrides.
func (e *surfaceEditor) swapVariant(variant, source string) {
	e.t.Helper()
	original, replacement := e.generation.Surfaces[variant], e.generation.Surfaces[source]
	require.NotNil(e.t, original, "variant %q was not generated", variant)
	require.NotNil(e.t, replacement, "variant %q was not generated", source)
	require.NotSame(e.t, original, replacement)
	e.undo = append(e.undo, func() error {
		e.generation.Surfaces[variant] = original
		return nil
	})
	e.generation.Surfaces[variant] = replacement
}

// restore undoes every recorded edit in reverse order.
func (e *surfaceEditor) restore() error {
	var errs []error
	for index := len(e.undo) - 1; index >= 0; index-- {
		errs = append(errs, e.undo[index]())
	}
	e.undo = nil
	return errors.Join(errs...)
}

func restoreFile(full string, data []byte, mode fs.FileMode) error {
	if err := os.WriteFile(full, data, mode); err != nil {
		return err
	}
	return os.Chmod(full, mode)
}
