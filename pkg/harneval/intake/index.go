package intake

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// fpKey is the duplicate key: a fingerprint is only comparable within its
// algorithm version.
type fpKey struct {
	version     int
	fingerprint string
}

// index is what intake already knows about each fingerprint: open
// candidates, promotions (link records and active incident tasks), and
// rejections, each mapped to the id a matching row reports.
type index struct {
	open     map[fpKey]string
	openIDs  map[string]bool
	promoted map[fpKey]string
	rejected map[fpKey]string
}

// match reports the first record of the current fingerprint version that
// already covers fingerprint: a promotion, then a rejection, then an open
// candidate.
func (x *index) match(fingerprint string) (result, id string, ok bool) {
	key := fpKey{FingerprintVersion, fingerprint}
	if id, ok := x.promoted[key]; ok {
		return ResultAlreadyPromoted, id, true
	}
	if id, ok := x.rejected[key]; ok {
		return ResultAlreadyRejected, id, true
	}
	if id, ok := x.open[key]; ok {
		return ResultDuplicateCandidate, id, true
	}
	return "", "", false
}

// loadIndex checks the intake layout and reads every intake record and
// active task. Any unreadable, malformed, or unsafe file fails the load.
func loadIndex(a *area) (*index, error) {
	if err := a.checkLayout(); err != nil {
		return nil, err
	}
	x := &index{open: map[fpKey]string{}, openIDs: map[string]bool{}, promoted: map[fpKey]string{}, rejected: map[fpKey]string{}}
	scans := []struct {
		dir   string
		valid func(string) bool
		add   func(id string, data []byte) error
	}{
		{IntakeDir, ValidCandidateID, x.addCandidate},
		{PromotedDir, ValidTaskID, x.addLink},
		{RejectedDir, ValidCandidateID, x.addRejection},
	}
	for _, scan := range scans {
		if err := scanRecords(a, scan.dir, scan.valid, scan.add); err != nil {
			return nil, err
		}
	}
	return x, scanActiveTasks(a, x.addTask)
}

// scanRecords visits each <id>.json directly in dir whose id passes valid;
// other names such as .gitkeep, a README, or a temp file are ignored.
func scanRecords(a *area, dir string, valid func(string) bool, add func(id string, data []byte) error) error {
	entries, err := a.listDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		id, isJSON := strings.CutSuffix(entry.Name(), ".json")
		if !isJSON || !valid(id) {
			continue
		}
		rel := dir + "/" + entry.Name()
		data, err := a.readFile(rel)
		if err == nil {
			err = add(id, data)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
	}
	return nil
}

// recordKey checks the fields every intake record shares.
func recordKey(schema, want string, version int, fingerprint string) (fpKey, error) {
	if schema != want || version < 1 || !fingerprintPattern.MatchString(fingerprint) {
		return fpKey{}, fmt.Errorf("record needs %s, a fingerprint_version, and a 64-hex fingerprint", want)
	}
	return fpKey{version, fingerprint}, nil
}

// remember keeps the first id seen for key.
func remember(ids map[fpKey]string, key fpKey, id string) {
	if _, seen := ids[key]; !seen {
		ids[key] = id
	}
}

func (x *index) addCandidate(id string, data []byte) error {
	var record Candidate
	if err := decodeStrict(data, &record); err != nil {
		return err
	}
	key, err := recordKey(record.SchemaVersion, CandidateSchemaV1, record.FingerprintVersion, record.Fingerprint)
	if err == nil {
		x.openIDs[id] = true
		remember(x.open, key, id)
	}
	return err
}

func (x *index) addLink(id string, data []byte) error {
	var record Link
	if err := decodeStrict(data, &record); err != nil {
		return err
	}
	key, err := recordKey(record.SchemaVersion, LinkSchemaV1, record.FingerprintVersion, record.Fingerprint)
	if err == nil {
		remember(x.promoted, key, id)
	}
	return err
}

func (x *index) addRejection(id string, data []byte) error {
	var record Rejection
	if err := decodeStrict(data, &record); err != nil {
		return err
	}
	key, err := recordKey(record.SchemaVersion, RejectionSchemaV1, record.FingerprintVersion, record.Fingerprint)
	if err == nil {
		remember(x.rejected, key, id)
	}
	return err
}

// taskRef is the part of an active task the duplicate and prune scans read.
// Other fields are ignored, so SPEC-HARNEVAL-001 semantic checks never block
// these scans; malformed JSON still does.
type taskRef struct {
	ID         string `json:"id"`
	Provenance struct {
		Kind        string `json:"kind"`
		Ref         string `json:"ref"`
		Fingerprint string `json:"fingerprint"`
	} `json:"provenance"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

// scanActiveTasks decodes every task file below the manifest's active paths;
// without a manifest there is no active task.
func scanActiveTasks(a *area, add func(taskRef)) error {
	actives, err := a.activePaths()
	if err != nil {
		return err
	}
	for _, active := range actives {
		err := a.walkJSON(active, func(rel string, data []byte) error {
			var task taskRef
			if err := json.Unmarshal(data, &task); err != nil {
				return fmt.Errorf("decode %s: %w", rel, err)
			}
			add(task)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// addTask indexes an active incident task under its provenance fingerprint,
// which intake writes with the current fingerprint version. The task id is
// reported as a row's match, so a task whose id breaks the grammar is not
// indexed and its text never reaches stdout.
func (x *index) addTask(task taskRef) {
	if task.Provenance.Kind == "incident" && task.Status.State == harneval.StateActive &&
		task.Provenance.Fingerprint != "" && ValidTaskID(task.ID) {
		remember(x.promoted, fpKey{FingerprintVersion, task.Provenance.Fingerprint}, task.ID)
	}
}
