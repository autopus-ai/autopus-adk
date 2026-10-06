package healthband

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// renameFile is the atomic commit step of a rewrite; tests replace it to
// inject a crash before the rename.
var renameFile = os.Rename

// CompactObservations rewrites a store file to its valid lines, keeping per
// series only the line that wins the collapse of each sample and, of those,
// the newest perSeries by order key. Kept lines stay byte-identical and in
// file order. A file already within bounds is not rewritten, and a crash
// before the rename leaves the previous file intact (REQ-02).
func (l *Locked) CompactObservations(name string, perSeries int) error {
	path := l.store.Path(name)
	lines, err := readStoreLines(path)
	if err != nil || len(lines) == 0 {
		return err
	}
	type validLine struct {
		raw         []byte
		observation Observation
	}
	var valid []validLine
	for _, line := range lines {
		if observation, reason := parseObservationLine(line); reason == "" {
			valid = append(valid, validLine{line, observation})
		}
	}
	winner := make(map[seriesSampleKey]int, len(valid))
	for i, entry := range valid {
		key := seriesSampleKey{entry.observation.Series, entry.observation.SampleKey}
		if at, seen := winner[key]; !seen || entry.observation.Attempt > valid[at].observation.Attempt {
			winner[key] = i
		}
	}
	// Collect winners per series in file order so ties on the order key are
	// broken deterministically by the stable sort.
	bySeries := make(map[string][]int)
	for i, entry := range valid {
		if winner[seriesSampleKey{entry.observation.Series, entry.observation.SampleKey}] == i {
			bySeries[entry.observation.Series] = append(bySeries[entry.observation.Series], i)
		}
	}
	keep := make([]bool, len(valid))
	kept := 0
	for _, indexes := range bySeries {
		sort.SliceStable(indexes, func(a, b int) bool {
			return orderKeyLess(valid[indexes[a]].observation, valid[indexes[b]].observation)
		})
		for _, i := range indexes[max(len(indexes)-max(perSeries, 0), 0):] {
			keep[i] = true
			kept++
		}
	}
	if kept == len(lines) {
		return nil
	}
	var buf bytes.Buffer
	for i, entry := range valid {
		if keep[i] {
			buf.Write(entry.raw)
			buf.WriteByte('\n')
		}
	}
	return writeStoreFileAtomic(path, buf.Bytes())
}

// CompactEvents rewrites band-events.jsonl to the newest maxEvents events by
// seq plus every event newer than checkpointSeq, which is never dropped.
// Lines that are not events of the known schema are dropped.
func (l *Locked) CompactEvents(maxEvents int, checkpointSeq int64) error {
	path := l.store.Path(EventsFile)
	lines, err := readStoreLines(path)
	if err != nil || len(lines) == 0 {
		return err
	}
	type eventLine struct {
		raw []byte
		seq int64
	}
	var events []eventLine
	for _, line := range lines {
		var header struct {
			Schema string `json:"schema"`
			Seq    *int64 `json:"seq"`
		}
		if json.Unmarshal(line, &header) == nil && header.Schema == SchemaBandEvaluation && header.Seq != nil {
			events = append(events, eventLine{line, *header.Seq})
		}
	}
	maxEvents = max(maxEvents, 0)
	keepAll := len(events) <= maxEvents
	var cutoff int64 // seq of the newest event outside the bound
	if !keepAll {
		seqs := make([]int64, len(events))
		for i, event := range events {
			seqs[i] = event.seq
		}
		sort.Slice(seqs, func(a, b int) bool { return seqs[a] > seqs[b] })
		cutoff = seqs[maxEvents]
	}
	var buf bytes.Buffer
	kept := 0
	for _, event := range events {
		if keepAll || event.seq > cutoff || event.seq > checkpointSeq {
			buf.Write(event.raw)
			buf.WriteByte('\n')
			kept++
		}
	}
	if kept == len(lines) {
		return nil
	}
	return writeStoreFileAtomic(path, buf.Bytes())
}

// writeStoreFileAtomic replaces path through a fsynced temp file and a
// rename, so readers see the old or the new file and never a mix.
func writeStoreFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err = temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = renameFile(tempPath, path); err != nil {
		return err
	}
	// Best effort: the rename already committed; a directory that cannot be
	// fsynced (Windows, some filesystems) must not report a failed rewrite.
	if handle, openErr := os.Open(dir); openErr == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
	return nil
}

// Store file IO. A repository can commit a symlink under .autopus/metrics,
// so every store file is opened only as a regular file that the path still
// names after the open; nothing is read or written through a symlink.

// openStoreFile opens path with flag (O_CREATE allowed) as a regular file.
func openStoreFile(path string, flag int) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errUnsafeStorePath
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, flag, 0o600)
	if err != nil {
		return nil, err
	}
	descriptor, statErr := file.Stat()
	entry, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil || !descriptor.Mode().IsRegular() || !os.SameFile(descriptor, entry) {
		_ = file.Close()
		return nil, errUnsafeStorePath
	}
	return file, nil
}

// readStoreLines returns the non-blank lines of a store file; a missing file
// has none.
func readStoreLines(path string) ([][]byte, error) {
	file, err := openStoreFile(path, os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(file); err != nil {
		return nil, fmt.Errorf("read metric store: %w", err)
	}
	var lines [][]byte
	for _, line := range bytes.Split(buf.Bytes(), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) > 0 {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// appendStoreFile appends data in one fsynced write. A partial last line left
// by a crash gets a newline first, so the new lines never join it.
func appendStoreFile(path string, data []byte) error {
	file, err := openStoreFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if size := info.Size(); size > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, size-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			data = append([]byte{'\n'}, data...)
		}
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}
