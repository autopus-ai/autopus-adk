package healthband

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Store files under <project>/.autopus/metrics/ (REQ-01, REQ-10, REQ-24).
const (
	CIRunsFile     = "ci-runs.jsonl"
	CanaryRunsFile = "canary-runs.jsonl"
	EventsFile     = "band-events.jsonl"
	StateFile      = "band-state.json"
	LockFile       = ".lock"
	PendingDir     = "pending"
)

// Retention and lock waits (REQ-02, Durability Protocol items 2 and 4).
const (
	MaxObservationsPerSeries = 512
	MaxEvents                = 2048
	StoreLockWait            = 5 * time.Second
	ResultLockWait           = 60 * time.Second
	// MaxStoreFileBytes bounds every store file read: compaction keeps far
	// less, so a larger file was planted or corrupted.
	MaxStoreFileBytes = 64 << 20
)

var (
	// ErrStoreLocked reports that another process held the store lock past
	// the wait limit; callers record ReasonStoreLocked and exit 0.
	ErrStoreLocked = errors.New("healthband: metric store is locked")
	// ErrInvalidObservation rejects an observation at ingest (invalid_value).
	ErrInvalidObservation = errors.New("healthband: invalid observation")
	// ErrStoreTooLarge refuses a store file above MaxStoreFileBytes.
	ErrStoreTooLarge   = errors.New("healthband: metric store file exceeds its size bound")
	errUnsafeStorePath = errors.New("healthband: metric store path must be a regular file or directory, not a symlink")
)

var (
	sampleKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	sourcePattern    = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)
)

// requiredObservationKeys must all be present: a missing value would
// otherwise decode as 0 and count as a success.
var requiredObservationKeys = []string{"series", "sample_key", "observed_at", "tiebreak", "value", "attempt", "source"}

// Validate reports the first field that breaks the observation contract.
// NaN and ±Inf fail the 0-or-1 value check.
func (o Observation) Validate() error {
	field := ""
	switch {
	case o.Schema != SchemaObservation:
		field = "schema"
	case !ValidSeriesID(o.Series):
		field = "series"
	case !sampleKeyPattern.MatchString(o.SampleKey):
		field = "sample_key"
	case o.ObservedAt.IsZero():
		field = "observed_at"
	case o.Tiebreak < 0:
		field = "tiebreak"
	case o.Value != 0 && o.Value != 1:
		field = "value"
	case o.Attempt < 1:
		field = "attempt"
	case !sourcePattern.MatchString(o.Source):
		field = "source"
	default:
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidObservation, field)
}

// Store is the metric directory of one project. Reads need no lock because
// every rewrite is an atomic rename; every mutation goes through Locked.
type Store struct{ dir string }

// NewStore addresses <projectDir>/.autopus/metrics without touching disk.
func NewStore(projectDir string) *Store {
	return &Store{dir: filepath.Join(projectDir, ".autopus", "metrics")}
}

// Dir returns the store directory, <projectDir>/.autopus/metrics.
func (s *Store) Dir() string { return s.dir }

// Path returns the path of a store file.
func (s *Store) Path(name string) string { return filepath.Join(s.dir, name) }

// ReadCounts counts the lines a tolerant read skipped, per class.
type ReadCounts struct {
	Malformed     int `json:"malformed"`
	UnknownSchema int `json:"unknown_schema"`
	InvalidValue  int `json:"invalid_value"`
}

// Skipped is the total number of skipped lines.
func (c ReadCounts) Skipped() int { return c.Malformed + c.UnknownSchema + c.InvalidValue }

// ReadObservations returns the valid observations of a store file in file
// order and counts the skipped lines. A missing file is an empty store.
func (s *Store) ReadObservations(name string) ([]Observation, ReadCounts, error) {
	lines, err := readStoreLines(s.Path(name))
	if err != nil {
		return nil, ReadCounts{}, err
	}
	var observations []Observation
	var counts ReadCounts
	for _, line := range lines {
		observation, class := parseObservationLine(line)
		switch class {
		case ReasonMalformed:
			counts.Malformed++
		case ReasonUnknownSchema:
			counts.UnknownSchema++
		case ReasonInvalidValue:
			counts.InvalidValue++
		default:
			observations = append(observations, observation)
		}
	}
	return observations, counts, nil
}

// parseObservationLine returns the observation or the reason it was skipped:
// not a JSON object is malformed, a missing or other schema is
// unknown_schema, and a contract violation is invalid_value.
func parseObservationLine(line []byte) (Observation, string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		return Observation{}, ReasonMalformed
	}
	var schema string
	if raw, ok := fields["schema"]; !ok || json.Unmarshal(raw, &schema) != nil || schema != SchemaObservation {
		return Observation{}, ReasonUnknownSchema
	}
	for _, key := range requiredObservationKeys {
		if _, ok := fields[key]; !ok {
			return Observation{}, ReasonInvalidValue
		}
	}
	var observation Observation
	if err := json.Unmarshal(line, &observation); err != nil || observation.Validate() != nil {
		return Observation{}, ReasonInvalidValue
	}
	return observation, ""
}

// Locked is the store while this process holds .autopus/metrics/.lock.
type Locked struct {
	store *Store
	lock  *filelock.Lock
}

// Lock takes the cross-process store lock, waiting at most wait. A symlinked
// .autopus or .autopus/metrics directory is refused before anything is
// created. Contention past the wait returns ErrStoreLocked.
func (s *Store) Lock(ctx context.Context, wait time.Duration) (*Locked, error) {
	for _, dir := range []string{filepath.Dir(s.dir), s.dir} {
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, errUnsafeStorePath
		}
	}
	lock, err := filelock.Acquire(ctx, s.Path(LockFile), wait)
	if errors.Is(err, filelock.ErrTimeout) {
		return nil, ErrStoreLocked
	}
	if err != nil {
		return nil, err
	}
	return &Locked{store: s, lock: lock}, nil
}

// Unlock releases the store lock.
func (l *Locked) Unlock() error { return l.lock.Unlock() }

// Store returns the locked store for reads.
func (l *Locked) Store() *Store { return l.store }

// AppendObservations validates every observation and then appends them as
// one fsynced write, so an invalid batch writes nothing. Times are stored in
// UTC.
func (l *Locked) AppendObservations(name string, observations []Observation) error {
	if len(observations) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, observation := range observations {
		if err := observation.Validate(); err != nil {
			return err
		}
		observation.ObservedAt = observation.ObservedAt.UTC()
		data, err := json.Marshal(observation)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return appendStoreFile(l.store.Path(name), buf.Bytes())
}

// AppendCanary appends one canary observation with sample key c<sequence>,
// where the sequence is one above the highest in canary-runs.jsonl. A time
// earlier than the newest stored one is clamped to it, so the order key never
// contradicts the sequence and compaction always keeps the highest sequence.
func (l *Locked) AppendCanary(series string, observedAt time.Time, value float64) (Observation, error) {
	if !strings.HasPrefix(series, SeriesPrefixCanary) {
		return Observation{}, fmt.Errorf("%w: series", ErrInvalidObservation)
	}
	existing, _, err := l.store.ReadObservations(CanaryRunsFile)
	if err != nil {
		return Observation{}, err
	}
	var sequence int64
	at := observedAt.UTC()
	for _, observation := range existing {
		sequence = max(sequence, observation.Tiebreak)
		if observation.ObservedAt.After(at) {
			at = observation.ObservedAt.UTC()
		}
	}
	sequence++
	observation := Observation{
		Schema: SchemaObservation, Series: series, SampleKey: "c" + strconv.FormatInt(sequence, 10),
		ObservedAt: at, Tiebreak: sequence, Value: value, Attempt: 1, Source: SourceCanary,
	}
	if err := l.AppendObservations(CanaryRunsFile, []Observation{observation}); err != nil {
		return Observation{}, err
	}
	return observation, nil
}
