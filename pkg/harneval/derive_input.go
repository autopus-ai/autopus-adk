package harneval

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// OracleResultsDir is the live result directory holding one
// harness_oracle_result.v1 file per trial the oracle harness wrote a result
// for. File names carry no meaning: a record names its result by digest.
const OracleResultsDir = "oracle-results"

// maxOracleResults bounds the oracle result files read: a session has at
// most max_agent_runs trials, which a valid policy keeps within
// SignedLaneAgentSeconds.
const maxOracleResults = SignedLaneAgentSeconds

// Attestation predicate types and events of a signed-lane session
// (SPEC-HARNEVAL-003 REQ-HR-07, Wire Contracts).
const (
	PredicateTypeBound         = "https://autopus.ai/harness-eval/bound/v1"
	PredicateTypeSessionResult = "https://autopus.ai/harness-eval/session-result/v1"

	EventBound         = "bound"
	EventSessionResult = "session_result"
)

// BoundPredicate is the predicate of a bound attestation: the bind job of
// the run attempt fixed this binding digest before any trial.
type BoundPredicate struct {
	RunID         int64  `json:"run_id"`
	RunAttempt    int    `json:"run_attempt"`
	BindingDigest string `json:"binding_digest"`
	Event         string `json:"event"`
}

// SessionResultPredicate is the predicate of a session_result attestation:
// the live-eval job's digests of the documents it produced, after every
// agent process ended. OracleResultSHA256 lists the digest of each oracle
// result file; identical results may share one digest.
type SessionResultPredicate struct {
	BoundPredicate
	ProtocolSHA256     string   `json:"protocol_sha256"`
	RecordsSHA256      string   `json:"records_sha256"`
	OracleResultSHA256 []string `json:"oracle_result_sha256"`
	CalibrationSHA256  string   `json:"calibration_sha256"`
}

// AttestedSession is what the bound and session_result attestations of one
// run attempt establish once the caller has verified each bundle: its
// signature, transparency log proof, and the workflow identity of the bind
// and live-eval jobs on main. A log time is the verified timestamp of its
// bundle (the Rekor v1 integrated time or the RFC 3161 TSA time), never a
// value read from the bundle unverified; the zero time means none.
type AttestedSession struct {
	Bound         BoundPredicate
	BoundLogTime  time.Time
	Result        SessionResultPredicate
	ResultLogTime time.Time
}

// SignerInput is a received live result as raw bytes. A nil document was
// absent from the result; an empty one is present and empty. OracleResults
// holds every oracle result file of the result.
type SignerInput struct {
	Protocol      []byte
	Records       []byte
	Calibration   []byte
	OracleResults [][]byte
}

// checkAttestation ties the received documents to the verified attestations
// of the run attempt before any of them is decoded (REQ-HR-09): both
// attestations name the run meta's run and attempt and the trusted binding
// and carry a verified log time, and every document has exactly the attested
// digest. Oracle results compare as digest sets, so an added, changed, or
// removed result file is a mismatch.
func checkAttestation(in SignerInput, binding string, meta RunMeta, a AttestedSession) error {
	for _, field := range []struct {
		path  string
		equal bool
	}{
		{"bound.event", a.Bound.Event == EventBound},
		{"bound.run_id", a.Bound.RunID == meta.RunID},
		{"bound.run_attempt", a.Bound.RunAttempt == meta.RunAttempt},
		{"bound.binding_digest", a.Bound.BindingDigest == binding},
		{"bound.log_time", !a.BoundLogTime.IsZero()},
		{"session_result.event", a.Result.Event == EventSessionResult},
		{"session_result.run_id", a.Result.RunID == meta.RunID},
		{"session_result.run_attempt", a.Result.RunAttempt == meta.RunAttempt},
		{"session_result.binding_digest", a.Result.BindingDigest == binding},
		{"session_result.log_time", !a.ResultLogTime.IsZero()},
	} {
		if !field.equal {
			return trustErr(ReasonAttestationDigestMismatch, "%s", field.path)
		}
	}
	for _, doc := range []struct {
		name, field, want string
		data              []byte
	}{
		{ProtocolFile, "protocol_sha256", a.Result.ProtocolSHA256, in.Protocol},
		{RecordsFile, "records_sha256", a.Result.RecordsSHA256, in.Records},
		{CalibrationFile, "calibration_sha256", a.Result.CalibrationSHA256, in.Calibration},
	} {
		switch {
		case doc.data == nil:
			return trustErr(ReasonAttestationDigestMismatch, "%s is missing", doc.name)
		case sha256Hex(doc.data) != doc.want:
			return trustErr(ReasonAttestationDigestMismatch, "%s differs from %s", doc.name, doc.field)
		}
	}
	attested := make(map[string]bool, len(a.Result.OracleResultSHA256))
	for _, digest := range a.Result.OracleResultSHA256 {
		attested[digest] = true
	}
	received := make(map[string]bool, len(in.OracleResults))
	for _, data := range in.OracleResults {
		digest := sha256Hex(data)
		if !attested[digest] {
			return trustErr(ReasonAttestationDigestMismatch, "oracle result %s is not attested", digest)
		}
		received[digest] = true
	}
	for _, digest := range a.Result.OracleResultSHA256 {
		if !received[digest] {
			return trustErr(ReasonAttestationDigestMismatch, "oracle result %s is missing", digest)
		}
	}
	return nil
}

// LoadSignerInput reads a downloaded live result directory without trusting
// its structure. It reads protocol.json, records.jsonl, calibration.json, and
// every file of oracle-results/ by name, and nothing else; a symlink, special
// file, or directory in their place is refused before anything is opened,
// and the whole input is bounded by the 64 MiB document cap. An absent
// document stays nil for the attestation check to report.
func LoadSignerInput(dir string) (SignerInput, error) {
	return loadSignerInput(dir, maxOracleResults)
}

func loadSignerInput(dir string, maxResults int) (SignerInput, error) {
	budget := &inputBudget{left: maxDocumentBytes}
	var in SignerInput
	for _, doc := range []struct {
		name   string
		target *[]byte
	}{{ProtocolFile, &in.Protocol}, {RecordsFile, &in.Records}, {CalibrationFile, &in.Calibration}} {
		data, err := budget.read(dir, doc.name)
		if err != nil {
			return SignerInput{}, err
		}
		*doc.target = data
	}
	names, err := oracleResultNames(dir, maxResults)
	if err != nil {
		return SignerInput{}, err
	}
	for _, name := range names {
		data, err := budget.read(dir, OracleResultsDir+"/"+name)
		if err != nil {
			return SignerInput{}, err
		}
		in.OracleResults = append(in.OracleResults, data)
	}
	return in, nil
}

// oracleResultNames lists the entries of oracle-results/ in name order, or
// none when the directory is absent. It must be a real directory, and more
// than maxResults entries are refused without listing the rest.
func oracleResultNames(dir string, maxResults int) ([]string, error) {
	path := filepath.Join(dir, OracleResultsDir)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, OracleResultsDir)
	case info.Mode()&fs.ModeSymlink != 0:
		return nil, withPath(invalidf(DetailSymlinkNotAllowed, "%s is a symlink", OracleResultsDir), OracleResultsDir)
	case !info.IsDir():
		return nil, withPath(invalidf(DetailReadFailed, "%s is not a directory", OracleResultsDir), OracleResultsDir)
	}
	listing, err := os.Open(path)
	if err != nil {
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, OracleResultsDir)
	}
	defer listing.Close()
	entries, err := listing.ReadDir(maxResults + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, OracleResultsDir)
	}
	if len(entries) > maxResults {
		return nil, withPath(invalidf(DetailReadFailed, "%s holds more than %d oracle result files", OracleResultsDir, maxResults), OracleResultsDir)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// inputBudget is the part of the document cap a live result has left.
type inputBudget struct{ left int64 }

// read returns the bytes of the regular file rel below dir, or nil when it
// does not exist. The file must still be the one Lstat saw once it is open,
// and it may not take more than the budget left.
func (b *inputBudget) read(dir, rel string) ([]byte, error) {
	path := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, rel)
	case info.Mode()&fs.ModeSymlink != 0:
		return nil, withPath(invalidf(DetailSymlinkNotAllowed, "%s is a symlink", rel), rel)
	case !info.Mode().IsRegular():
		return nil, withPath(invalidf(DetailReadFailed, "%s is not a regular file", rel), rel)
	}
	tooLarge := withPath(invalidf(DetailReadFailed, "the live result is larger than the 64 MiB document cap at %s", rel), rel)
	if info.Size() > b.left {
		return nil, tooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, rel)
	}
	defer file.Close()
	if opened, err := file.Stat(); err != nil || !os.SameFile(info, opened) {
		return nil, withPath(invalidf(DetailReadFailed, "%s changed while it was opened", rel), rel)
	}
	data, err := io.ReadAll(io.LimitReader(file, b.left+1))
	if err != nil {
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, rel)
	}
	if int64(len(data)) > b.left {
		return nil, tooLarge
	}
	b.left -= int64(len(data))
	return data, nil
}
