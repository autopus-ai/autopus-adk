package checklist

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"time"
)

// Hit is one applied check in one run. Skipped checks are not hits: the
// retirement rule counts applications, not runs.
type Hit struct {
	Time   time.Time `json:"ts"`
	ID     string    `json:"check_id"`
	Mode   Mode      `json:"mode"`
	Base   string    `json:"base"`
	Result Result    `json:"result"`
}

// RecordHits appends the applied outcomes of rep.
func RecordHits(root string, rep Report, now time.Time) error {
	var buf bytes.Buffer
	for _, o := range rep.Outcomes {
		if o.Result == ResultSkipped {
			continue
		}
		line, err := json.Marshal(Hit{Time: now.UTC(), ID: o.ID, Mode: o.Mode, Base: rep.Base, Result: o.Result})
		if err != nil {
			return err
		}
		buf.Write(append(line, '\n'))
	}
	if buf.Len() == 0 {
		return nil
	}
	return appendFile(root, HitsFile, buf.Bytes())
}

// ReadHits returns every recorded hit; a missing file is no hits and an
// unreadable line is skipped.
func ReadHits(root string) ([]Hit, error) {
	data, err := readFile(root, HitsFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hits []Hit
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var h Hit
		if json.Unmarshal(sc.Bytes(), &h) == nil && h.ID != "" {
			hits = append(hits, h)
		}
	}
	return hits, nil
}
