package healthband

import (
	"math"
	"sort"
)

// seriesSampleKey identifies one sample across attempts.
type seriesSampleKey struct{ series, sampleKey string }

// Collapse keeps one observation per (series, sample_key): the highest
// attempt wins and equal attempts keep the first written line. Input is in
// file order; output keeps the position of each key's first line.
func Collapse(observations []Observation) []Observation {
	index := make(map[seriesSampleKey]int, len(observations))
	out := make([]Observation, 0, len(observations))
	for _, observation := range observations {
		key := seriesSampleKey{observation.Series, observation.SampleKey}
		at, seen := index[key]
		if !seen {
			index[key] = len(out)
			out = append(out, observation)
			continue
		}
		if observation.Attempt > out[at].Attempt {
			out[at] = observation
		}
	}
	return out
}

// SortObservations orders observations by (ObservedAt, Tiebreak) ascending,
// comparing instants and the numeric tiebreak. The sort is stable, so equal
// keys keep their input order.
func SortObservations(observations []Observation) {
	sort.SliceStable(observations, func(i, j int) bool { return orderKeyLess(observations[i], observations[j]) })
}

// orderKeyLess orders by instant first and the numeric tiebreak second.
func orderKeyLess(a, b Observation) bool {
	if !a.ObservedAt.Equal(b.ObservedAt) {
		return a.ObservedAt.Before(b.ObservedAt)
	}
	return a.Tiebreak < b.Tiebreak
}

// OrderedSeries collapses file-order observations and returns each series
// ordered oldest first, ready for EvaluateAt.
func OrderedSeries(observations []Observation) map[string][]Observation {
	series := make(map[string][]Observation)
	for _, observation := range Collapse(observations) {
		series[observation.Series] = append(series[observation.Series], observation)
	}
	for _, ordered := range series {
		SortObservations(ordered)
	}
	return series
}

// EvaluateAt evaluates position p of an ordered series using only the
// observations at or before p (Detector Contract item 8). A position outside
// the series has no current block.
func EvaluateAt(ordered []Observation, position int) Evaluation {
	if position < 0 || position >= len(ordered) {
		return EvaluateValues(nil)
	}
	values := make([]float64, position+1)
	for i := range values {
		values[i] = ordered[i].Value
	}
	evaluation := EvaluateValues(values)
	evaluation.Series = ordered[position].Series
	evaluation.SampleKey = ordered[position].SampleKey
	return evaluation
}

// EvaluateValues evaluates the newest position of one series whose 0/1
// values are ordered oldest first (Detector Contract items 3–7). Blocks of K
// are cut backwards from the newest value, so the oldest len mod K values are
// excluded; the current block is the newest K values and the baseline is up
// to W blocks immediately before it. Arithmetic is float64 two-pass.
func EvaluateValues(values []float64) Evaluation {
	constants := DefaultConstants()
	evaluation := Evaluation{Constants: &constants}
	if len(values) < BlockSize {
		evaluation.Reasons = []string{ReasonNoCurrentBlock}
		return evaluation
	}
	blocks := blockValues(values)
	current := blocks[len(blocks)-1]
	baseline := blocks[:len(blocks)-1]
	if len(baseline) > BaselineWindow {
		baseline = baseline[len(baseline)-BaselineWindow:]
	}
	n := len(baseline)
	evaluation.N, evaluation.X = &n, &current
	if n < MinBaseline {
		evaluation.Reasons = []string{ReasonInsufficientSamples}
		return evaluation
	}

	mu, sd := meanAndSampleSD(baseline)
	sdEff := math.Max(sd, VarianceFloor)
	var reasons []string
	switch {
	case sd == 0:
		reasons = append(reasons, ReasonZeroVariance)
	case sd < VarianceFloor:
		reasons = append(reasons, ReasonVarianceFloorApplied)
	}
	z := (current - mu) / sdEff
	tier := 0
	if z < 0 {
		// One-sided test: a falling failure rate never escalates.
		reasons = append(reasons, ReasonBelowBaseline)
	} else {
		tier = TierForZ(z)
	}
	evaluation.Mu, evaluation.SD, evaluation.SDEff = &mu, &sd, &sdEff
	evaluation.Z, evaluation.Tier, evaluation.Reasons = &z, &tier, reasons
	return evaluation
}

// blockValues returns failures / K per block, oldest first, after dropping
// the oldest len mod K values so the newest block is always complete.
func blockValues(values []float64) []float64 {
	start := len(values) % BlockSize
	blocks := make([]float64, 0, len(values)/BlockSize)
	for i := start; i < len(values); i += BlockSize {
		sum := 0.0
		for _, value := range values[i : i+BlockSize] {
			sum += value
		}
		blocks = append(blocks, sum/BlockSize)
	}
	return blocks
}

// meanAndSampleSD is the two-pass mean and Bessel-corrected standard
// deviation; callers guarantee at least N_min ≥ 2 blocks.
func meanAndSampleSD(blocks []float64) (float64, float64) {
	sum := 0.0
	for _, block := range blocks {
		sum += block
	}
	mean := sum / float64(len(blocks))
	squares := 0.0
	for _, block := range blocks {
		squares += (block - mean) * (block - mean)
	}
	return mean, math.Sqrt(squares / float64(len(blocks)-1))
}

// TierForZ returns the largest k in {1, 2, 3} with z ≥ k − ε, otherwise 0.
// Boundaries therefore belong to the upper tier, and NaN is tier 0 because
// every comparison with NaN is false.
func TierForZ(z float64) int {
	for k := maxTier; k >= 1; k-- {
		if z >= float64(k)-TierEpsilon {
			return k
		}
	}
	return 0
}
