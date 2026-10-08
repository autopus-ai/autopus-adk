package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// judge compares one artifact run with the bundle's pinned expectations.
// A timed-out run is not checked. A stdout over the limit is too_large. Every
// file assertion path is opened by the output rules below, in assertion
// order, and the first path that breaks them decides the check; a rejected
// run holds no assertion, so nothing artifact-shaped is ever compared.
func judge(in input, outputs string) result {
	res := result{SchemaVersion: ResultSchema, TaskID: in.TaskID, Assertions: []assertionResult{},
		ArtifactExit: in.ArtifactExit, TimedOut: in.TimedOut}
	switch {
	case in.TimedOut:
		res.OutputCheck = CheckNotChecked
		return res
	case in.StdoutOverflow:
		res.OutputCheck = CheckTooLarge
		return res
	}
	contents, check := readOutputs(outputs, in.Assertions)
	res.OutputCheck = check
	if check != CheckOK {
		return res
	}
	for _, item := range in.Assertions {
		res.Assertions = append(res.Assertions, assertionResult{ID: item.ID, Passed: item.matches(in, contents)})
	}
	return res
}

// readOutputs reads every file assertion path below the output root. A
// missing root or file is no output (nil content) and fails only its
// assertion; any other defect is the run's check.
func readOutputs(outputs string, assertions []assertion) (map[string][]byte, string) {
	contents := map[string][]byte{}
	var paths []string
	for _, item := range assertions {
		if item.Kind == KindFile {
			paths = append(paths, item.Path)
		}
	}
	if len(paths) == 0 {
		return contents, CheckOK
	}
	root, check := openRoot(outputs)
	if check != CheckOK || root == nil {
		return contents, check
	}
	defer func() { _ = root.Close() }()
	for _, name := range paths {
		data, check := readOutput(root, name)
		if check != CheckOK {
			return nil, check
		}
		contents[name] = data
	}
	return contents, CheckOK
}

// openRoot opens the output root as an os.Root. The root must be a real
// directory at its canonical path, so neither it nor an ancestor was swapped
// for a link; nil with ok means the artifact left no output root at all.
func openRoot(outputs string) (*os.Root, string) {
	info, err := os.Lstat(outputs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, CheckOK
	}
	if err != nil || !info.IsDir() {
		return nil, CheckLinkRejected
	}
	if real, err := filepath.EvalSymlinks(outputs); err != nil || real != outputs {
		return nil, CheckLinkRejected
	}
	root, err := os.OpenRoot(outputs)
	if err != nil {
		return nil, CheckLinkRejected
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, CheckLinkRejected
	}
	return root, CheckOK
}

// readOutput opens one fixed relative path below the root. Every intermediate
// element must be a directory that is not a symlink, and the final element a
// regular file by Lstat; the descriptor opened with O_NOFOLLOW|O_NONBLOCK must
// be a regular file with one link and the same file as that Lstat. os.Root
// alone rejects escapes but follows symlinks inside the root and reads hard
// links and FIFOs, hence the extra checks. At most OutputLimit bytes are read.
func readOutput(root *os.Root, name string) ([]byte, string) {
	elements := strings.Split(name, "/")
	var linked os.FileInfo
	for i := 1; i <= len(elements); i++ {
		info, err := root.Lstat(strings.Join(elements[:i], "/"))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, CheckOK
		}
		final := i == len(elements)
		if err != nil || (!final && !info.IsDir()) || (final && !info.Mode().IsRegular()) {
			return nil, CheckLinkRejected
		}
		linked = info
	}
	file, err := root.OpenFile(name, openFlags, 0)
	if err != nil {
		return nil, CheckLinkRejected
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(linked, opened) {
		return nil, CheckLinkRejected
	}
	if links, ok := linkCount(opened); !ok || links != 1 {
		return nil, CheckLinkRejected
	}
	if opened.Size() > OutputLimit {
		return nil, CheckTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, OutputLimit+1))
	if err != nil {
		return nil, CheckLinkRejected
	}
	if len(data) > OutputLimit {
		return nil, CheckTooLarge
	}
	return append([]byte{}, data...), CheckOK
}
