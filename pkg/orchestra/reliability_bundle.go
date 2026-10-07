package orchestra

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	defaultReliabilityRetentionRuns = 20
	defaultReliabilityRetentionAge  = 7 * 24 * time.Hour
	defaultReliabilityActiveGrace   = 6 * time.Hour
	reliabilityActiveMarkerName     = ".active"
)

type reliabilityStore struct {
	runID      string
	dir        string
	mu         sync.Mutex
	degraded   bool // set once when receipt persistence first fails (REQ-005)
	preflight  []ProviderPreflightReceipt
	prompt     []PromptTransportReceipt
	collection []CollectionReceipt
	events     []ReliabilityEvent
}

func newReliabilityStore(runID string) (*reliabilityStore, error) {
	root, err := reliabilityRuntimeRoot()
	if err != nil {
		return nil, err
	}
	runDir := filepath.Join(root, "runs", sanitizeProviderName(runID))
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, fmt.Errorf("create reliability run dir: %w", err)
	}
	store := &reliabilityStore{runID: runID, dir: runDir}
	if err := store.touchActiveMarker(); err != nil {
		return nil, fmt.Errorf("touch reliability active marker: %w", err)
	}
	_ = pruneReliabilityArtifacts(
		filepath.Join(root, "runs"),
		defaultReliabilityRetentionRuns,
		defaultReliabilityRetentionAge,
		defaultReliabilityActiveGrace,
	)
	return store, nil
}

func reliabilityRuntimeRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		root := filepath.Join(home, ".autopus", "runtime", "orchestra")
		if mkErr := os.MkdirAll(root, 0o700); mkErr == nil {
			return root, nil
		}
	}
	root := filepath.Join(os.TempDir(), "autopus-runtime", "orchestra")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create reliability root: %w", err)
	}
	return root, nil
}

func pruneReliabilityArtifacts(baseDir string, keepRuns int, maxAge, activeGrace time.Duration) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type dirInfo struct {
		name string
		mod  time.Time
	}
	var dirs []dirInfo
	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(baseDir, entry.Name())
		if isActiveReliabilityRun(path, now, activeGrace) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		if maxAge > 0 && now.Sub(info.ModTime()) > maxAge {
			_ = os.RemoveAll(path)
			continue
		}
		dirs = append(dirs, dirInfo{name: entry.Name(), mod: info.ModTime()})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].mod.After(dirs[j].mod) })
	for i := keepRuns; i < len(dirs); i++ {
		_ = os.RemoveAll(filepath.Join(baseDir, dirs[i].name))
	}
	return nil
}

func isActiveReliabilityRun(path string, now time.Time, grace time.Duration) bool {
	if grace <= 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(path, reliabilityActiveMarkerName))
	if err != nil || info.IsDir() {
		return false
	}
	return now.Sub(info.ModTime()) <= grace
}

func (s *reliabilityStore) artifactDir() string {
	return s.dir
}

func (s *reliabilityStore) recordPreflight(receipt ProviderPreflightReceipt) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preflight = append(s.preflight, receipt)
	return s.writeJSON(fmt.Sprintf("preflight-%s.json", sanitizeProviderName(receipt.Provider)), receipt)
}

func (s *reliabilityStore) recordPrompt(receipt PromptTransportReceipt) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompt = append(s.prompt, receipt)
	return s.writeJSON(fmt.Sprintf("prompt-%s-%s.json", sanitizeProviderName(receipt.Provider), sanitizeProviderName(receipt.Correlation.RoundID)), receipt)
}

func (s *reliabilityStore) recordCollection(receipt CollectionReceipt) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collection = append(s.collection, receipt)
	return s.writeJSON(fmt.Sprintf("collection-%s-%s.json", sanitizeProviderName(receipt.Provider), sanitizeProviderName(receipt.Correlation.RoundID)), receipt)
}

func (s *reliabilityStore) recordEvent(event ReliabilityEvent) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return s.writeJSON(fmt.Sprintf("event-%s-%s.json", sanitizeProviderName(event.Kind), sanitizeProviderName(event.Correlation.ProviderID)), event)
}

func (s *reliabilityStore) writeFailureBundle(summary, nextStep string, degraded bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle := FailureBundle{
		SchemaVersion:      reliabilitySchemaVersion,
		Timestamp:          time.Now().UTC(),
		RunID:              s.runID,
		Degraded:           degraded,
		Summary:            summary,
		NextStep:           nextStep,
		PreflightReceipts:  append([]ProviderPreflightReceipt(nil), s.preflight...),
		PromptReceipts:     append([]PromptTransportReceipt(nil), s.prompt...),
		CollectionReceipts: append([]CollectionReceipt(nil), s.collection...),
		Events:             append([]ReliabilityEvent(nil), s.events...),
	}
	return s.writeJSON("failure-bundle.json", bundle)
}

func (s *reliabilityStore) summary(bundlePath string) *ReliabilitySummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	summary := &ReliabilitySummary{
		RunID:         s.runID,
		ArtifactDir:   s.dir,
		FailureBundle: bundlePath,
		OpenEvents:    len(s.events),
	}
	for _, receipt := range s.preflight {
		if receipt.Status != "pass" {
			summary.PreflightFailures++
		}
	}
	for _, receipt := range s.prompt {
		if receipt.Status == "mismatch" || receipt.Status == "failed" {
			summary.PromptMismatches++
		}
	}
	for _, receipt := range s.collection {
		if receipt.Status != "pass" {
			summary.CollectionFailures++
		}
	}
	return summary
}

func (s *reliabilityStore) writeJSON(name string, payload any) string {
	path := filepath.Join(s.dir, name)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		s.markDegraded()
		return ""
	}
	if err := s.ensureWritableDir(); err != nil {
		s.markDegraded()
		return ""
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		if retryErr := s.ensureWritableDir(); retryErr != nil {
			s.markDegraded()
			return ""
		}
		if retryErr := os.WriteFile(path, data, 0o600); retryErr != nil {
			s.markDegraded()
			return ""
		}
	}
	_ = s.touchActiveMarker()
	return path
}

// markDegraded emits exactly one warning per store the first time receipt
// persistence fails, so an operator can see that reliability receipts are not being
// persisted. The caller already holds s.mu (writeJSON runs under the record* lock),
// so the degraded flag is read/written without additional locking. The caller's
// empty-string return contract is unchanged (REQ-005).
func (s *reliabilityStore) markDegraded() {
	if s.degraded {
		return
	}
	s.degraded = true
	log.Printf("[WARN] reliability receipt persistence degraded for run %s; receipts will not be persisted to %s", s.runID, s.dir)
}

func (s *reliabilityStore) ensureWritableDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	return s.touchActiveMarker()
}

func (s *reliabilityStore) touchActiveMarker() error {
	return os.WriteFile(
		filepath.Join(s.dir, reliabilityActiveMarkerName),
		[]byte(time.Now().UTC().Format(time.RFC3339Nano)),
		0o600,
	)
}
