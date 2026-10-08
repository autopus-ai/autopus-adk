package healthband

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// maxPendingBytes bounds a pending result read; a result is far smaller.
const maxPendingBytes = 1 << 20

// ErrInvalidResult refuses a result outside the contract before anything is
// written: a bad claim, series, sample key, episode, status, or BS ID.
var ErrInvalidResult = errors.New("healthband: claim result outside the contract")

// NewOwner returns the owner token of one run, <hostname>:<pid>:<64-bit
// random hex> (Durability item 6), with the hostname filtered to
// [A-Za-z0-9.-] and at most 64 bytes.
func NewOwner() string {
	host, _ := os.Hostname()
	host = strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, host)
	if host = host[:min(len(host), 64)]; host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s:%d:%s", host, os.Getpid(), randomHex(8))
}

// NewClaimID returns a random 128-bit claim id in lowercase hex.
func NewClaimID() string { return randomHex(16) }

func randomHex(size int) string {
	buf := make([]byte, size)
	_, _ = rand.Read(buf) // crypto/rand.Read never fails and always fills buf
	return hex.EncodeToString(buf)
}

// ClaimOutcome is what executing one claim produced. An empty Status means
// done; a failure is ClaimFailedPrefix followed by its reason.
type ClaimOutcome struct {
	Status          string                      `json:"status,omitempty"`
	DiagnosisStatus string                      `json:"diagnosis_status,omitempty"`
	BSID            string                      `json:"bs_id,omitempty"`
	BSStatus        string                      `json:"bs_status,omitempty"`
	PromptManifest  []promptlayer.ManifestEntry `json:"prompt_manifest,omitempty"`
}

// Result is one claim's outcome addressed to that claim (phase C).
type Result struct {
	Claim     Claim  `json:"claim"`
	Series    string `json:"series"`
	SampleKey string `json:"sample_key"`
	EpisodeID string `json:"episode_id"`
	ClaimOutcome
}

// Recorded reports how phase C recorded one result.
type Recorded struct {
	ClaimID   string
	Seq       int64  // seq of the appended action_result event; 0 when none was appended
	Reason    string // late_result, claim_unknown, or empty
	Pending   string // pending file written because the store lock stayed busy
	Duplicate bool   // the claim already had a result, so nothing was appended
}

// ClaimRunner executes one claim without the store lock (phase B).
type ClaimRunner func(ctx context.Context, claim DueClaim) ClaimOutcome

// ExecuteOptions configures phase B and C.
type ExecuteOptions struct {
	Clock      func() time.Time // nil uses time.Now
	ResultWait time.Duration    // phase C lock wait; zero uses ResultLockWait
	// AfterRecord, when set, runs after phase C has recorded a claim's
	// result and before the next claim starts, with the outcome its runner
	// returned and how phase C recorded it (SPEC-SIGMABAND-002: the
	// local_patch claim of a diagnose claim runs here, under the same owner).
	AfterRecord func(ctx context.Context, claim DueClaim, outcome ClaimOutcome, recorded Recorded)
}

// ExecuteClaims runs the claims one at a time in order, without the store
// lock, and records each result (phase C) before the next claim starts, so a
// finished claim is never claimed while a later one runs (Durability items 3
// and 4). The runner cannot misaddress a result: ExecuteClaims addresses it.
// A failed record does not stop later claims; a cancelled ctx does.
func (s *Store) ExecuteClaims(ctx context.Context, claims []DueClaim, run ClaimRunner, opts ExecuteOptions) ([]Recorded, error) {
	clock, wait := opts.Clock, opts.ResultWait
	if clock == nil {
		clock = time.Now
	}
	if wait <= 0 {
		wait = ResultLockWait
	}
	var recorded []Recorded
	var errs []error
	for _, claim := range claims {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		result := Result{Claim: claim.Claim, Series: claim.Series, SampleKey: claim.SampleKey, EpisodeID: claim.EpisodeID, ClaimOutcome: run(ctx, claim)}
		record, err := s.RecordResult(ctx, result, clock(), wait)
		recorded = append(recorded, record)
		if err != nil {
			errs = append(errs, err)
		}
		if opts.AfterRecord != nil {
			opts.AfterRecord(ctx, claim, result.ClaimOutcome, record)
		}
	}
	return recorded, errors.Join(errs...)
}

// RecordResult is phase C for one claim (Durability item 4): under the
// store lock it replays the log, finds the claim across the checkpoint's
// episodes, appends its action_result event with the decided reason, applies
// retention, and writes the checkpoint. A result for an unknown claim or one
// past its late-result window is appended as claim_unknown and changes no
// state; a second result for an ended claim appends nothing.
//
// When the lock stays busy past wait, or ctx ends while waiting, the result
// is persisted as pending/<claim-id>.json for the next phase A instead. A
// result outside the contract is refused before anything is written.
func (s *Store) RecordResult(ctx context.Context, result Result, now time.Time, wait time.Duration) (recorded Recorded, err error) {
	if result.Status == "" {
		result.Status = ClaimDone
	}
	if !result.valid() {
		return recorded, ErrInvalidResult
	}
	recorded.ClaimID = result.Claim.ID
	locked, err := s.Lock(ctx, wait)
	if errors.Is(err, ErrStoreLocked) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		recorded.Pending, err = s.writePending(result)
		return recorded, err
	}
	if err != nil {
		return recorded, err
	}
	defer func() { err = errors.Join(err, locked.Unlock()) }()
	w, err := s.loadWAL(now)
	if err != nil {
		return recorded, err
	}
	w.locked = locked
	reason, duplicate := resultReason(w.state, result, w.now)
	if duplicate || w.results[result.Claim.ID] {
		recorded.Duplicate = true
		return recorded, w.writeCheckpoint()
	}
	event := resultEvent(w.nextSeq, result, reason)
	if err := w.append([]Event{event}); err != nil {
		return recorded, err
	}
	recorded.Seq, recorded.Reason = event.Seq, reason
	retain(&w.state, w.now)
	return recorded, w.writeCheckpoint()
}

// writePending persists a result that could not take the store lock as
// pending/<claim-id>.json by exclusive create (Durability item 4). The claim
// id is validated hex, so the name cannot leave the pending directory.
func (s *Store) writePending(result Result) (string, error) {
	dir := s.Path(PendingDir)
	if err := ensureRealDir(dir); err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, result.Claim.ID+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = file.Write(append(data, '\n'))
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		_ = os.Remove(path) // never leave a partial result for the next run
		return "", err
	}
	return path, nil
}

// appendPending appends the action_result of every pending result once, at
// the start of phase A (Durability item 2). A file whose claim already has
// an action_result event was appended before a crash and is only deleted;
// files are deleted after the checkpoint write, and a file outside the
// contract is skipped, counted, and kept.
func (w *WAL) appendPending() error {
	dir := w.store.Path(PendingDir)
	if info, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	} else if !info.IsDir() {
		return errUnsafeStorePath
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		claimID, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !claimIDPattern.MatchString(claimID) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		result, ok := readPending(path, claimID)
		if !ok {
			w.Skipped++
			continue
		}
		w.pending = append(w.pending, path)
		if w.results[claimID] {
			continue
		}
		if reason, duplicate := resultReason(w.state, result, w.now); !duplicate {
			if err := w.append([]Event{resultEvent(w.nextSeq, result, reason)}); err != nil {
				return err
			}
		}
	}
	return nil
}

// readPending reads one pending result; it must parse, name the claim of its
// file name, and hold only contract values.
func readPending(path, claimID string) (Result, bool) {
	file, err := openStoreFile(path, os.O_RDONLY)
	if err != nil {
		return Result{}, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPendingBytes))
	var result Result
	if err != nil || json.Unmarshal(data, &result) != nil || result.Claim.ID != claimID || !result.valid() {
		return Result{}, false
	}
	return result, true
}

// ensureRealDir creates dir (0700) and refuses a symlink or non-directory at
// .autopus, .autopus/metrics, or dir itself.
func ensureRealDir(dir string) error {
	for _, path := range []string{filepath.Dir(filepath.Dir(dir)), filepath.Dir(dir), dir} {
		if info, err := os.Lstat(path); err == nil && !info.IsDir() {
			return errUnsafeStorePath
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		return errors.Join(errUnsafeStorePath, err)
	}
	return nil
}

// resultEvent is the action_result event of one result: the claim with its
// result status, the result fields, and the decided reason.
func resultEvent(seq int64, result Result, reason string) Event {
	claim := result.Claim
	claim.Status, claim.InterruptedAt = result.Status, nil
	event := Event{
		Schema: SchemaBandEvaluation, Seq: seq, Kind: EventKindActionResult,
		Evaluation: Evaluation{Series: result.Series, SampleKey: result.SampleKey},
		Action:     claim.Kind, EpisodeID: result.EpisodeID, Claims: []Claim{claim}, ClaimID: claim.ID,
		DiagnosisStatus: result.DiagnosisStatus, BSID: result.BSID, BSStatus: result.BSStatus, PromptManifest: result.PromptManifest,
	}
	if reason != "" {
		event.Reasons = []string{reason}
	}
	return event
}
