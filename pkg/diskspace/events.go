package diskspace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Event log files and bounds (SPEC-DISK-002 REQ-DISK2-LOG-01).
const (
	eventsFile     = "events.jsonl"
	eventsRotated  = "events.jsonl.1"
	eventsLockFile = "events.lock"
	eventsMaxBytes = 1 << 20
	stateLockWait  = 2 * time.Second
)

// Event kinds.
const (
	EventTrimApply      = "trim_apply"
	EventReclaimApply   = "reclaim_apply"
	EventLeaseReclaimed = "lease_reclaimed"
	EventLeaseRefused   = "lease_refused"
	EventLowSpaceAlert  = "low_space_alert"
	EventAutoReclaim    = "auto_reclaim"
)

// Event is one line of the event log.
type Event struct {
	Time           time.Time `json:"time"`
	Kind           string    `json:"kind"`
	FreeBytes      int64     `json:"free_bytes"`
	ReclaimedBytes int64     `json:"reclaimed_bytes"`
	Count          int       `json:"count"`
	Message        string    `json:"message,omitempty"`
}

// AppendEvent appends e, first rotating the log to events.jsonl.1 when the
// line would take it past 1 MiB, all under the event lock, so concurrent
// writers never rotate twice. Callers ignore the error: a full or broken
// state dir must never change a command's outcome.
func AppendEvent(state StateDir, e Event) error {
	_, err := appendEvent(state, e, 0)
	return err
}

// AppendEventUnlessRecent appends e unless an event of the same kind was
// logged less than within before it. The check and the append run under the
// one event lock, so concurrent sessions log one event per window, and it
// reports whether it appended.
func AppendEventUnlessRecent(state StateDir, e Event, within time.Duration) (bool, error) {
	return appendEvent(state, e, within)
}

func appendEvent(state StateDir, e Event, within time.Duration) (bool, error) {
	line, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	line = append(line, '\n')
	lock, err := filelock.Acquire(context.Background(), state.Join(eventsLockFile), state.logLockWait())
	if err != nil {
		return false, err
	}
	defer func() { _ = lock.Unlock() }()
	if within > 0 {
		if last, ok := LastEvent(state, e.Kind); ok && e.Time.Sub(last.Time) < within {
			return false, nil
		}
	}
	if info, err := os.Lstat(state.Join(eventsFile)); err == nil && info.Mode().IsRegular() &&
		info.Size()+int64(len(line)) > eventsMaxBytes {
		if err := os.Rename(state.Join(eventsFile), state.Join(eventsRotated)); err != nil {
			return false, err
		}
	}
	file, err := state.OpenAppend(eventsFile)
	if err != nil {
		return false, err
	}
	defer file.Close()
	_, err = file.Write(line)
	return err == nil, err
}

// TailEvents returns the last n events of the rotated and current logs,
// oldest first, skipping lines that do not parse.
func TailEvents(state StateDir, n int) ([]Event, error) {
	var events []Event
	for _, name := range []string{eventsRotated, eventsFile} {
		data, err := state.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		events = append(events, parseEvents(data)...)
	}
	if n > 0 && len(events) > n {
		events = events[len(events)-n:]
	}
	return events, nil
}

// LastEvent returns the newest event of kind in the current log.
func LastEvent(state StateDir, kind string) (Event, bool) {
	data, err := state.ReadFile(eventsFile)
	if err != nil {
		return Event{}, false
	}
	events := parseEvents(data)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			return events[i], true
		}
	}
	return Event{}, false
}

func parseEvents(data []byte) []Event {
	var events []Event
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), eventsMaxBytes)
	for scanner.Scan() {
		var e Event
		if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Kind != "" {
			events = append(events, e)
		}
	}
	return events
}
