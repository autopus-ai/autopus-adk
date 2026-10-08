package learn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/secretscan"
)

// Store manages learning entries in a JSONL file.
type Store struct {
	path string // path to pipeline.jsonl
	mu   sync.Mutex
}

// SkipRecord represents a skipped line during tolerant read.
type SkipRecord struct {
	Line   int
	Raw    string
	Reason string
}

// NewStore creates a store rooted at dir, ensuring .autopus/learnings/ exists.
func NewStore(dir string) (*Store, error) {
	learningsDir := filepath.Join(dir, ".autopus", "learnings")
	if err := os.MkdirAll(learningsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create learnings dir: %w", err)
	}
	return &Store{
		path: filepath.Join(learningsDir, "pipeline.jsonl"),
	}, nil
}

// Append adds a new entry to the JSONL file.
func (s *Store) Append(entry LearningEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendUnlocked(entry)
}

func (s *Store) appendUnlocked(entry LearningEntry) error {
	entry, err := redactEntry(entry)
	if err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	f, err := openStoreFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return nil
}

// ReadTolerant reads all entries from the JSONL file tolerantly.
func (s *Store) ReadTolerant() ([]LearningEntry, []SkipRecord, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []LearningEntry{}, []SkipRecord{}, nil
		}
		return nil, nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var entries []LearningEntry
	var skips []SkipRecord
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry LearningEntry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			skips = append(skips, SkipRecord{
				Line:   lineNum,
				Raw:    line,
				Reason: err.Error(),
			})
			continue
		}
		entry.Line = lineNum
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan file: %w", err)
	}
	return entries, skips, nil
}

// Read reads all valid entries from the JSONL file.
func (s *Store) Read() ([]LearningEntry, error) {
	entries, _, err := s.ReadTolerant()
	return entries, err
}

// NextID generates the next L-{NNN} ID based on existing entries.
func (s *Store) NextID() (string, error) {
	entries, err := s.Read()
	if err != nil {
		return "", err
	}

	maxNum := 0
	for _, e := range entries {
		if strings.HasPrefix(e.ID, "L-") {
			numStr := e.ID[2:]
			if n, err := strconv.Atoi(numStr); err == nil && n > maxNum {
				maxNum = n
			}
		}
	}
	return fmt.Sprintf("L-%03d", maxNum+1), nil
}

// AppendAtomic atomically generates an ID and appends a new learning entry.
func (s *Store) AppendAtomic(entryType EntryType, opts RecordOpts) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.NextID()
	if err != nil {
		return fmt.Errorf("next id: %w", err)
	}
	return s.appendUnlocked(newEntry(id, entryType, opts))
}

// newEntry is the entry AppendAtomic writes for opts, stamped now.
func newEntry(id string, entryType EntryType, opts RecordOpts) LearningEntry {
	return LearningEntry{
		ID:         id,
		Timestamp:  time.Now(),
		Type:       entryType,
		Phase:      opts.Phase,
		SpecID:     opts.SpecID,
		Files:      opts.Files,
		Packages:   opts.Packages,
		Pattern:    opts.Pattern,
		Resolution: opts.Resolution,
		Expected:   opts.Expected,
		Actual:     opts.Actual,
		Repro:      opts.Repro,
		Severity:   opts.Severity,
	}
}

// redactEntry is the store's first-write boundary: every byte Append and
// AppendAtomic persist passes through it. Evidence fields are validated and
// redacted in the REQ-HC-01 order and the verbatim fields are checked (a
// rejection writes nothing), then the other free-text fields are redacted.
// rewriteStore never calls this, so a stored value and its fingerprint do not
// change when prune re-encodes the entry.
func redactEntry(entry LearningEntry) (LearningEntry, error) {
	evidence := []struct {
		field EvidenceField
		value *string
	}{
		{FieldExpected, &entry.Expected},
		{FieldActual, &entry.Actual},
		{FieldRepro, &entry.Repro},
	}
	for _, ev := range evidence {
		redacted, _, err := RedactEvidenceField(ev.field, *ev.value)
		if err != nil {
			return LearningEntry{}, err
		}
		*ev.value = redacted
	}
	if err := checkVerbatim(entry); err != nil {
		return LearningEntry{}, err
	}
	for _, text := range []*string{&entry.Pattern, &entry.Resolution, &entry.Phase, &entry.SpecID} {
		*text, _ = secretscan.Redact(*text)
	}
	return entry, nil
}

// checkVerbatim refuses the fields the writer stores as given: a severity
// outside the enum, and a files or packages item that redaction would change.
// Files and packages are fingerprint input, so they are refused rather than
// redacted.
func checkVerbatim(entry LearningEntry) error {
	switch entry.Severity {
	case "", SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
	default:
		return &FieldError{Field: FieldSeverity, Detail: DetailUnknownValue}
	}
	lists := []struct {
		field EvidenceField
		items []string
	}{{FieldFiles, entry.Files}, {FieldPackages, entry.Packages}}
	for _, list := range lists {
		for _, item := range list.items {
			if redactSecrets(item) != item {
				return &FieldError{Field: list.field, Detail: DetailNeedsRedaction}
			}
		}
	}
	return nil
}

// UpdateReuseCount increments reuse_count for the entry with the given ID.
func (s *Store) UpdateReuseCount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, skips, err := s.ReadTolerant()
	if err != nil {
		return err
	}

	found := false
	for i := range entries {
		if entries[i].ID == id {
			entries[i].ReuseCount++
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("entry not found: %s", id)
	}

	return rewriteStore(s, entries, skips)
}
