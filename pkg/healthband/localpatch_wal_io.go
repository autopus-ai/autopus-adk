package healthband

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// LocalPatchLog is this SPEC's records and state as one read saw them. Reads
// need no lock, because every rewrite is an atomic rename; every append goes
// through the store lock.
type LocalPatchLog struct {
	Records []LocalPatchRecord // valid records in seq order
	State   LocalPatchState    // the state file with every newer record folded in
	Skipped int                // lines the tolerant read skipped
	written []byte             // state file bytes on disk; nil when there are none
	nextSeq int64
}

// ReadLocalPatchLog reads localpatch-state.json and localpatch-events.jsonl
// and folds every record newer than the state into it, so a crash between
// an append and the state rename loses nothing. Missing files are empty.
func (s *Store) ReadLocalPatchLog() (*LocalPatchLog, error) {
	state, written, err := readLocalPatchState(s.Path(LocalPatchStateFile))
	if err != nil {
		return nil, err
	}
	lines, err := readStoreLines(s.Path(LocalPatchEventsFile))
	if err != nil {
		return nil, err
	}
	log := &LocalPatchLog{State: state, written: written, nextSeq: state.LastSeq + 1}
	for _, line := range lines {
		record, ok := parseLocalPatchLine(line)
		if !ok {
			log.Skipped++
			continue
		}
		log.Records = append(log.Records, record)
	}
	slices.SortStableFunc(log.Records, func(a, b LocalPatchRecord) int { return cmp.Compare(a.Seq, b.Seq) })
	folded := state.LastSeq
	for _, record := range log.Records {
		if record.Seq > folded {
			log.State.apply(record)
		}
		log.nextSeq = max(log.nextSeq, record.Seq+1)
	}
	return log, nil
}

func parseLocalPatchLine(line []byte) (LocalPatchRecord, bool) {
	var record LocalPatchRecord
	if json.Unmarshal(line, &record) != nil || !record.valid() {
		return LocalPatchRecord{}, false
	}
	return record, true
}

// Result returns the claim's result record.
func (l *LocalPatchLog) Result(claimID string) (LocalPatchRecord, bool) {
	for _, record := range l.Records {
		if record.Kind == LocalPatchKindResult && record.ClaimID == claimID {
			return record, true
		}
	}
	return LocalPatchRecord{}, false
}

// Ended reports whether the claim has a result: a result record, or a
// terminal status that the state kept after compaction.
func (l *LocalPatchLog) Ended(claimID string) bool {
	if _, found := l.Result(claimID); found {
		return true
	}
	claim, found := l.State.claim(claimID)
	return found && claim.Status != ClaimClaimed
}

// ClaimRecords returns every record whose RecordClaimID is claimID, in seq
// order.
func (l *LocalPatchLog) ClaimRecords(claimID string) []LocalPatchRecord {
	var records []LocalPatchRecord
	for _, record := range l.Records {
		if record.RecordClaimID() == claimID {
			records = append(records, record)
		}
	}
	return records
}

// AppendLocalPatch appends phase A's decision and claim records as one
// fsynced write while the caller holds the store lock (after wal.Commit and
// before Unlock), writes the state, and compacts the log. It returns the
// records with their seq. Any other kind, or a record outside the contract,
// is refused before anything is written.
func (l *Locked) AppendLocalPatch(records ...LocalPatchRecord) ([]LocalPatchRecord, error) {
	for _, record := range records {
		if record.Kind != LocalPatchKindDecision && record.Kind != LocalPatchKindClaim {
			return nil, ErrInvalidLocalPatchRecord
		}
	}
	log, err := l.store.ReadLocalPatchLog()
	if err != nil {
		return nil, err
	}
	appended, err := log.append(l.store, records, true)
	if err != nil {
		return nil, err
	}
	return appended, l.compactLocalPatch(log)
}

// AppendLocalPatchPrep is the end of Local Patch Flow step 1: under the
// store lock (StoreLockWait) it re-reads this SPEC's records and appends the
// prep record only while its claim has no result. appended false means that
// a result exists, so the live claim stops with no further record (CD-3 M7).
func (s *Store) AppendLocalPatchPrep(ctx context.Context, prep LocalPatchRecord) (appended bool, err error) {
	if prep.Kind != LocalPatchKindPrep {
		return false, ErrInvalidLocalPatchRecord
	}
	err = s.withLocalPatchLog(ctx, StoreLockWait, func(log *LocalPatchLog) error {
		if log.Ended(prep.RecordClaimID()) {
			return nil
		}
		_, err := log.append(s, []LocalPatchRecord{prep}, false)
		appended = err == nil
		return err
	})
	return appended, err
}

// AppendLocalPatchStage appends one stage record under the store lock,
// waiting at most StoreLockWait inside ctx; an error ends the live claim
// failed:record_unavailable before its next artifact step.
func (s *Store) AppendLocalPatchStage(ctx context.Context, stage LocalPatchRecord) error {
	if stage.Kind != LocalPatchKindStage {
		return ErrInvalidLocalPatchRecord
	}
	return s.withLocalPatchLog(ctx, StoreLockWait, func(log *LocalPatchLog) error {
		_, err := log.append(s, []LocalPatchRecord{stage}, false)
		return err
	})
}

// AppendLocalPatchResult appends a claim's result under the store lock
// (ResultLockWait) only while the claim has none, so a live claim and
// recovery never both end one claim (CD-3 M7, F-011). appended false means
// that the claim had already ended and nothing was written.
func (s *Store) AppendLocalPatchResult(ctx context.Context, result LocalPatchRecord) (appended bool, err error) {
	if result.Kind != LocalPatchKindResult {
		return false, ErrInvalidLocalPatchRecord
	}
	err = s.withLocalPatchLog(ctx, ResultLockWait, func(log *LocalPatchLog) error {
		if log.Ended(result.ClaimID) {
			return nil
		}
		_, err := log.append(s, []LocalPatchRecord{result}, true)
		appended = err == nil
		return err
	})
	return appended, err
}

// ReadLocalPatchLocked reads this SPEC's records under the store lock
// (StoreLockWait), the fresh read that recovery acts on.
func (s *Store) ReadLocalPatchLocked(ctx context.Context) (*LocalPatchLog, error) {
	var fresh *LocalPatchLog
	err := s.withLocalPatchLog(ctx, StoreLockWait, func(log *LocalPatchLog) error {
		fresh = log
		return nil
	})
	return fresh, err
}

// withLocalPatchLog runs fn under the store lock with a fresh read.
func (s *Store) withLocalPatchLog(ctx context.Context, wait time.Duration, fn func(*LocalPatchLog) error) (err error) {
	locked, err := s.Lock(ctx, wait)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, locked.Unlock()) }()
	log, err := s.ReadLocalPatchLog()
	if err != nil {
		return err
	}
	return fn(log)
}

// append validates records, gives them the next seqs, appends them as one
// fsynced write, folds them, and, with writeState, renames the new state
// into place. The caller holds the store lock.
func (l *LocalPatchLog) append(store *Store, records []LocalPatchRecord, writeState bool) ([]LocalPatchRecord, error) {
	out := slices.Clone(records)
	var buf bytes.Buffer
	for i := range out {
		out[i].Schema, out[i].Seq = SchemaLocalPatch, l.nextSeq+int64(i)
		if !out[i].LeaseUntil.IsZero() {
			out[i].LeaseUntil = out[i].LeaseUntil.UTC()
		}
		if !out[i].valid() {
			return nil, ErrInvalidLocalPatchRecord
		}
		data, err := json.Marshal(out[i])
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if len(out) == 0 {
		return nil, nil
	}
	if err := appendStoreFile(store.Path(LocalPatchEventsFile), buf.Bytes()); err != nil {
		return nil, err
	}
	for _, record := range out {
		l.Records = append(l.Records, record)
		l.State.apply(record)
	}
	l.nextSeq += int64(len(out))
	if !writeState {
		return out, nil
	}
	return out, l.writeState(store)
}
