package diskspace

import (
	"context"
	"encoding/json"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Sample log files and bounds (SPEC-DISK-002 REQ-DISK2-LOG-03, -04).
const (
	samplesFile     = "samples.jsonl"
	samplesLockFile = "samples.lock"
	samplesMaxBytes = 256 << 10
	samplesMaxCount = 2000
	sampleInterval  = 5 * time.Minute
	sampleRetention = 7 * 24 * time.Hour
	trendMinWindow  = 10 * time.Minute
	trendMaxWindow  = 6 * time.Hour
)

// Sample is one free-space reading.
type Sample struct {
	Time      time.Time `json:"time"`
	FreeBytes int64     `json:"free_bytes"`
	Mount     string    `json:"mount"`
}

// Trend is the rate free space changed at, negative while it falls.
type Trend struct {
	WindowMinutes int64 `json:"window_minutes"`
	BytesPerHour  int64 `json:"bytes_per_hour"`
}

// RecordSample appends s unless the newest sample is of the same mount and
// less than five minutes old, and rewrites the log without samples older
// than seven days once it passes 256 KiB.
func RecordSample(state StateDir, s Sample) error {
	lock, err := filelock.Acquire(context.Background(), state.Join(samplesLockFile), state.logLockWait())
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	samples := LoadSamples(state)
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].Mount != s.Mount {
			continue
		}
		if s.Time.Sub(samples[i].Time) < sampleInterval {
			return nil
		}
		break
	}
	line, err := json.Marshal(s)
	if err != nil {
		return err
	}
	file, err := state.OpenAppend(samplesFile)
	if err != nil {
		return err
	}
	_, err = file.Write(append(line, '\n'))
	size := int64(-1)
	if info, statErr := file.Stat(); statErr == nil {
		size = info.Size()
	}
	_ = file.Close()
	if err != nil || size <= samplesMaxBytes {
		return err
	}
	// Age alone may not shrink the log when several mounts sample often, so
	// the rewrite also keeps at most the newest samplesMaxCount samples.
	recent := append(samples, s)
	recent = recent[max(len(recent)-samplesMaxCount, 0):]
	var kept []byte
	for _, old := range recent {
		if s.Time.Sub(old.Time) <= sampleRetention {
			data, _ := json.Marshal(old)
			kept = append(append(kept, data...), '\n')
		}
	}
	return state.WriteFileAtomic(samplesFile, kept)
}

// LoadSamples reads the sample log, skipping lines that do not parse.
func LoadSamples(state StateDir) []Sample {
	data, err := state.ReadFile(samplesFile)
	if err != nil {
		return nil
	}
	var samples []Sample
	for _, line := range splitLines(data) {
		var s Sample
		if json.Unmarshal(line, &s) == nil && !s.Time.IsZero() {
			samples = append(samples, s)
		}
	}
	return samples
}

// ComputeTrend measures from the oldest sample of mount aged between ten
// minutes and six hours to the current reading, or returns nil.
func ComputeTrend(samples []Sample, mount string, now time.Time, freeNow int64) *Trend {
	for _, s := range samples {
		age := now.Sub(s.Time)
		if s.Mount != mount || age < trendMinWindow || age > trendMaxWindow {
			continue
		}
		perHour := float64(freeNow-s.FreeBytes) / age.Hours()
		return &Trend{WindowMinutes: int64(age / time.Minute), BytesPerHour: int64(perHour)}
	}
	return nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
