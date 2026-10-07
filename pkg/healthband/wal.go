package healthband

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"time"
)

// maxCheckpointBytes bounds a checkpoint read; band writes far less.
const maxCheckpointBytes = 16 << 20

var (
	errReadOnlyWAL = errors.New("healthband: the write-ahead log was loaded without the store lock")
	errStalePlan   = errors.New("healthband: the plan does not continue the write-ahead log")
)

// WAL is band-events.jsonl as a write-ahead log and band-state.json as its
// checkpoint, as one run sees them (REQ-10). Loading folds every event newer
// than the checkpoint into the checkpoint state, so a crash between the
// event append and the checkpoint rename loses and duplicates nothing.
type WAL struct {
	store   *Store
	locked  *Locked // nil for a read-only load
	now     time.Time
	state   Checkpoint
	written []byte // checkpoint bytes on disk; nil when there is none
	nextSeq int64
	results map[string]bool // claim ids that already have an action_result event
	pending []string        // pending result files to delete after the checkpoint
	// Interrupted lists the claims this load marked interrupted.
	Interrupted []string
	// Skipped counts event lines and pending files the tolerant read skipped.
	Skipped int
}

// LoadWAL loads the log and checkpoint read-only for --dry-run: it takes no
// lock, creates and writes nothing, and leaves pending results alone; lease
// expiry and retention apply in memory only.
func (s *Store) LoadWAL(now time.Time) (*WAL, error) {
	w, err := s.loadWAL(now)
	if err != nil {
		return nil, err
	}
	w.settle()
	return w, nil
}

// OpenWAL starts phase A under the store lock: it replays the events newer
// than the checkpoint, appends pending results, marks expired leases
// interrupted, and applies episode retention, in that order (Durability
// item 2), so a finished claim whose result was pending never expires.
func (l *Locked) OpenWAL(now time.Time) (*WAL, error) {
	w, err := l.store.loadWAL(now)
	if err != nil {
		return nil, err
	}
	w.locked = l
	if err := w.appendPending(); err != nil {
		return nil, err
	}
	w.settle()
	return w, nil
}

// settle marks expired leases interrupted and applies retention.
func (w *WAL) settle() {
	w.Interrupted = expireLeases(&w.state, w.now)
	retain(&w.state, w.now)
}

func (s *Store) loadWAL(now time.Time) (*WAL, error) {
	w := &WAL{store: s, now: now.UTC(), results: make(map[string]bool)}
	var err error
	if w.state, w.written, err = readCheckpoint(s.Path(StateFile)); err != nil {
		return nil, err
	}
	events, skipped, err := readEvents(s.Path(EventsFile))
	if err != nil {
		return nil, err
	}
	w.Skipped = skipped
	checkpointSeq := w.state.LastSeq
	w.nextSeq = checkpointSeq + 1
	for _, event := range events {
		if event.Seq > checkpointSeq {
			applyEvent(&w.state, event)
		}
		if event.Kind == EventKindActionResult {
			w.results[event.ClaimID] = true
		}
		w.nextSeq = max(w.nextSeq, event.Seq+1)
	}
	return w, nil
}

// Commit appends the plan's events as one fsynced write, folds them, writes
// the checkpoint by atomic rename, deletes the pending results appended at
// open, and, once every stored series was planned, compacts (REQ-02).
func (w *WAL) Commit(plan Plan) error {
	if w.locked == nil {
		return errReadOnlyWAL
	}
	events := plan.Events()
	if len(events) > 0 && plan.base != w.nextSeq {
		return errStalePlan
	}
	if err := w.append(events); err != nil {
		return err
	}
	retain(&w.state, w.now)
	if err := w.writeCheckpoint(); err != nil {
		return err
	}
	for _, path := range w.pending {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	w.pending = nil
	if !plan.complete {
		return nil
	}
	for _, name := range []string{CIRunsFile, CanaryRunsFile} {
		if err := w.locked.CompactObservations(name, MaxObservationsPerSeries); err != nil {
			return err
		}
	}
	return w.locked.CompactEvents(MaxEvents, w.state.LastSeq)
}

// Checkpoint returns a deep copy of the current checkpoint state.
func (w *WAL) Checkpoint() Checkpoint {
	state := w.state
	state.Series = make(map[string]SeriesState, len(w.state.Series))
	for id, series := range w.state.Series {
		state.Series[id] = cloneSeries(series)
	}
	return state
}

// append writes events as one fsynced append and then folds them.
func (w *WAL) append(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if err := appendStoreFile(w.store.Path(EventsFile), buf.Bytes()); err != nil {
		return err
	}
	for _, event := range events {
		applyEvent(&w.state, event)
		w.nextSeq = max(w.nextSeq, event.Seq+1)
		if event.Kind == EventKindActionResult {
			w.results[event.ClaimID] = true
		}
	}
	return nil
}

// writeCheckpoint renames a new checkpoint into place unless the bytes on
// disk already hold this state, so an idle run leaves the file untouched.
func (w *WAL) writeCheckpoint() error {
	data, err := json.Marshal(w.state)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if bytes.Equal(data, w.written) || (w.written == nil && w.state.LastSeq == 0 && len(w.state.Series) == 0) {
		return nil
	}
	if err := writeStoreFileAtomic(w.store.Path(StateFile), data); err != nil {
		return err
	}
	w.written = data
	return nil
}

// readCheckpoint returns the checkpoint and its bytes. A missing file is an
// empty checkpoint. An unparsable or invalid one is rebuilt by replaying the
// log from the start, because the checkpoint only caches the folded log.
func readCheckpoint(path string) (Checkpoint, []byte, error) {
	empty := Checkpoint{Schema: SchemaBandState, Series: make(map[string]SeriesState)}
	file, err := openStoreFile(path, os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil, nil
	}
	if err != nil {
		return Checkpoint{}, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCheckpointBytes))
	if err != nil {
		return Checkpoint{}, nil, err
	}
	var state Checkpoint
	if json.Unmarshal(data, &state) != nil || !validCheckpoint(state) {
		return empty, data, nil
	}
	if state.Series == nil {
		state.Series = make(map[string]SeriesState)
	}
	return state, data, nil
}

// readEvents returns the valid events of the log ordered by seq and counts
// the lines it skipped.
func readEvents(path string) ([]Event, int, error) {
	lines, err := readStoreLines(path)
	if err != nil {
		return nil, 0, err
	}
	events := make([]Event, 0, len(lines))
	skipped := 0
	for _, line := range lines {
		var event Event
		if json.Unmarshal(line, &event) != nil || !validEvent(event) {
			skipped++
			continue
		}
		events = append(events, event)
	}
	slices.SortStableFunc(events, func(a, b Event) int { return cmp.Compare(a.Seq, b.Seq) })
	return events, skipped, nil
}
