package diskspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Consumer sample ledger: a ring buffer of per-consumer sizes, rewritten
// whole on every record so it never grows past consumerSamplesMax lines.
const (
	consumerSamplesFile     = "consumers.jsonl"
	consumerSamplesLockFile = "consumers.lock"
	consumerSamplesMax      = 288
	consumerSampleInterval  = 5 * time.Minute
	growthWindow            = time.Hour
)

// ConsumerSample is one reading of consumer sizes, keyed by consumer name.
type ConsumerSample struct {
	Time  time.Time        `json:"time"`
	Bytes map[string]int64 `json:"bytes"`
}

// Growth is how one consumer changed. DeltaBytes runs from the oldest sample
// of the last hour that has the consumer, or, when none does, from the newest
// older one; LastDeltaBytes runs from the newest earlier sample.
type Growth struct {
	Name           string    `json:"name"`
	Bytes          int64     `json:"bytes"`
	DeltaBytes     int64     `json:"delta_bytes"`
	Since          time.Time `json:"since"`
	LastDeltaBytes int64     `json:"last_delta_bytes"`
	LastSince      time.Time `json:"last_since"`
}

// ConsumerSampleDue reports whether a new sample would be kept: the newest
// is older than the sample interval, or there is none. Callers on a budget
// read it before measuring anything.
func ConsumerSampleDue(samples []ConsumerSample, now time.Time) bool {
	return len(samples) == 0 || now.Sub(samples[len(samples)-1].Time) >= consumerSampleInterval
}

// RecordConsumerSample appends s, keeping the newest consumerSamplesMax
// samples. A sample within the interval of the newest one is not appended;
// it only fills in the consumers the newest one lacks, so a cheap reading
// does not hide a fuller one taken a minute later. It returns the ledger as
// written.
func RecordConsumerSample(state StateDir, s ConsumerSample) ([]ConsumerSample, error) {
	if len(s.Bytes) == 0 {
		return LoadConsumerSamples(state), nil
	}
	lock, err := filelock.Acquire(context.Background(), state.Join(consumerSamplesLockFile), state.logLockWait())
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Unlock() }()
	samples := LoadConsumerSamples(state)
	if n := len(samples); n > 0 && !ConsumerSampleDue(samples, s.Time) {
		for name, bytes := range s.Bytes {
			if _, ok := samples[n-1].Bytes[name]; !ok {
				samples[n-1].Bytes[name] = bytes
			}
		}
	} else {
		samples = append(samples, s)
	}
	samples = samples[max(len(samples)-consumerSamplesMax, 0):]
	var data []byte
	for _, sample := range samples {
		line, err := json.Marshal(sample)
		if err != nil {
			return nil, err
		}
		data = append(append(data, line...), '\n')
	}
	return samples, state.WriteFileAtomic(consumerSamplesFile, data)
}

// LoadConsumerSamples reads the ledger oldest first, skipping lines that do
// not parse.
func LoadConsumerSamples(state StateDir) []ConsumerSample {
	data, err := state.ReadFile(consumerSamplesFile)
	if err != nil {
		return nil
	}
	var samples []ConsumerSample
	for _, line := range splitLines(data) {
		var s ConsumerSample
		if json.Unmarshal(line, &s) == nil && !s.Time.IsZero() && s.Bytes != nil {
			samples = append(samples, s)
		}
	}
	slices.SortStableFunc(samples, func(a, b ConsumerSample) int { return a.Time.Compare(b.Time) })
	return samples
}

// ComputeGrowth ranks the consumers of current by DeltaBytes, largest first,
// against the samples taken before now. A consumer no earlier sample has is
// left out.
func ComputeGrowth(samples []ConsumerSample, current map[string]int64, now time.Time) []Growth {
	var out []Growth
	for name, bytes := range current {
		var inWindow, older, last *ConsumerSample
		for i := range samples {
			s := &samples[i]
			if _, ok := s.Bytes[name]; !ok || !s.Time.Before(now) {
				continue
			}
			switch {
			case now.Sub(s.Time) > growthWindow:
				older = s
			case inWindow == nil:
				inWindow = s
			}
			last = s
		}
		base := inWindow
		if base == nil {
			base = older
		}
		if base == nil {
			continue
		}
		out = append(out, Growth{Name: name, Bytes: bytes, DeltaBytes: bytes - base.Bytes[name], Since: base.Time,
			LastDeltaBytes: bytes - last.Bytes[name], LastSince: last.Time})
	}
	slices.SortFunc(out, func(a, b Growth) int {
		if a.DeltaBytes != b.DeltaBytes {
			if a.DeltaBytes > b.DeltaBytes {
				return -1
			}
			return 1
		}
		if a.Name < b.Name {
			return -1
		}
		return 1
	})
	return out
}

// LedgerGrowth is ComputeGrowth with the newest sample as the current
// reading, for callers that only read the ledger.
func LedgerGrowth(samples []ConsumerSample) []Growth {
	if len(samples) < 2 {
		return nil
	}
	newest := samples[len(samples)-1]
	return ComputeGrowth(samples[:len(samples)-1], newest.Bytes, newest.Time)
}

// TopGrower is the first consumer that grew by at least minBytes.
func TopGrower(growth []Growth, minBytes int64) (Growth, bool) {
	if len(growth) > 0 && growth[0].DeltaBytes >= minBytes && growth[0].DeltaBytes > 0 {
		return growth[0], true
	}
	return Growth{}, false
}

// GrowthPhrase prints a growth as "colima +38.0 GiB in 52m".
func GrowthPhrase(g Growth, now time.Time) string {
	sign := "+"
	if g.DeltaBytes < 0 {
		sign = "-"
	}
	delta := g.DeltaBytes
	if delta < 0 {
		delta = -delta
	}
	return fmt.Sprintf("%s %s%s in %dm", g.Name, sign, SizeString(delta), int64(now.Sub(g.Since)/time.Minute))
}
