package healthband_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// S1: tier boundaries belong to the upper tier within ε = 1e-9, so the pair
// 2 - 1e-10 / 2 - 1e-8 straddles exactly one tier change.
func TestTierForZ_EpsilonBoundaryPairAndInclusiveUpperTiers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		z    float64
		want int
	}{
		{name: "just inside epsilon belongs to tier 2", z: 2 - 1e-10, want: 2},
		{name: "just outside epsilon stays tier 1", z: 2 - 1e-8, want: 1},
		{name: "exact 1 is tier 1", z: 1, want: 1},
		{name: "exact 2 is tier 2", z: 2, want: 2},
		{name: "exact 3 is tier 3", z: 3, want: 3},
		{name: "far above 3 caps at tier 3", z: 9.5, want: 3},
		{name: "below 1 is tier 0", z: 0.999, want: 0},
		{name: "zero is tier 0", z: 0, want: 0},
		{name: "negative is tier 0", z: -2.5, want: 0},
		{name: "NaN is tier 0", z: math.NaN(), want: 0},
	} {
		assert.Equal(t, tc.want, healthband.TierForZ(tc.z), tc.name)
	}
}

// oracleTolerance is the acceptance tolerance against 6-decimal expectations.
const oracleTolerance = 5e-7

type oracleStats struct {
	mu, sd, sdEff, z float64
	tier             int
}

func repeat(value float64, count int) []float64 {
	out := make([]float64, count)
	for i := range out {
		out[i] = value
	}
	return out
}

func concat(parts ...[]float64) []float64 {
	var out []float64
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

// valuesFromBlocks expands baseline block values and the current block value
// x into 0/1 observations, oldest first, K = 4 per block.
func valuesFromBlocks(blocks []float64, x float64) []float64 {
	var values []float64
	for _, block := range append(append([]float64(nil), blocks...), x) {
		failures := int(math.Round(block * 4))
		values = append(values, concat(repeat(1, failures), repeat(0, 4-failures))...)
	}
	return values
}

func assertNear(t *testing.T, want float64, got *float64, field string) {
	t.Helper()
	if assert.NotNil(t, got, field) {
		assert.InDelta(t, want, *got, oracleTolerance, field)
	}
}

func reasonsOf(evaluation healthband.Evaluation) []string {
	if evaluation.Reasons == nil {
		return []string{}
	}
	return evaluation.Reasons
}

// S1: the numeric oracle O1–O10 (acceptance.md), recomputed independently.
// O10 also rejects a window-less implementation, which would report n=35,
// sd=0.355036, z=1.005935, tier 1.
func TestEvaluateValues_NumericOracleO1ToO10(t *testing.T) {
	t.Parallel()
	o5Baseline := concat(repeat(0.25, 4), repeat(0.5, 1), repeat(0, 15))
	var o7Baseline []float64 // (0.0, 0.75) ×10
	for range 10 {
		o7Baseline = append(o7Baseline, 0, 0.75)
	}
	for _, tc := range []struct {
		id      string
		blocks  []float64
		x       float64
		n       int
		stats   *oracleStats
		reasons []string
	}{
		{"O1", repeat(0, 20), 0.25, 20, &oracleStats{0, 0, 0.25, 1.0, 1}, []string{"zero_variance"}},
		{"O2", repeat(0, 20), 0.50, 20, &oracleStats{0, 0, 0.25, 2.0, 2}, []string{"zero_variance"}},
		{"O3", repeat(0, 20), 0.75, 20, &oracleStats{0, 0, 0.25, 3.0, 3}, []string{"zero_variance"}},
		{"O4", repeat(0, 19), 1.00, 19, nil, []string{"insufficient_samples"}},
		{"O5", o5Baseline, 0.75, 20, &oracleStats{0.075, 0.142810, 0.25, 2.7, 2}, []string{"variance_floor_applied"}},
		{"O6", o5Baseline, 1.00, 20, &oracleStats{0.075, 0.142810, 0.25, 3.7, 3}, []string{"variance_floor_applied"}},
		{"O7", o7Baseline, 1.00, 20, &oracleStats{0.375, 0.384742, 0.384742, 1.624466, 1}, []string{}},
		{"O8", o7Baseline, 0.00, 20, &oracleStats{0.375, 0.384742, 0.384742, -0.974679, 0}, []string{"below_baseline"}},
		{"O9", concat(repeat(0.25, 1), repeat(0, 19)), 0.50, 20, &oracleStats{0.0125, 0.055902, 0.25, 1.95, 1}, []string{"variance_floor_applied"}},
		{"O10", concat(repeat(1, 5), repeat(0, 30)), 0.50, 30, &oracleStats{0, 0, 0.25, 2.0, 2}, []string{"zero_variance"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			got := healthband.EvaluateValues(valuesFromBlocks(tc.blocks, tc.x))

			if assert.NotNil(t, got.N) {
				assert.Equal(t, tc.n, *got.N, "n")
			}
			assertNear(t, tc.x, got.X, "x")
			assert.Equal(t, tc.reasons, reasonsOf(got), "reasons")
			if tc.stats == nil {
				assert.Nil(t, got.Mu, "mu")
				assert.Nil(t, got.SD, "sd")
				assert.Nil(t, got.SDEff, "sd_eff")
				assert.Nil(t, got.Z, "z")
				assert.Nil(t, got.Tier, "tier")
				return
			}
			assertNear(t, tc.stats.mu, got.Mu, "mu")
			assertNear(t, tc.stats.sd, got.SD, "sd")
			assertNear(t, tc.stats.sdEff, got.SDEff, "sd_eff")
			assertNear(t, tc.stats.z, got.Z, "z")
			if assert.NotNil(t, got.Tier) {
				assert.Equal(t, tc.stats.tier, *got.Tier, "tier")
			}
		})
	}
}

func valuesFromString(t *testing.T, digits string) []float64 {
	t.Helper()
	values := make([]float64, len(digits))
	for i, digit := range digits {
		switch digit {
		case '0':
		case '1':
			values[i] = 1
		default:
			t.Fatalf("value string holds %q", digit)
		}
	}
	return values
}

// S3 detector part: R1 and R2 are the acceptance value strings of the
// 2026-10-06 replay (trusted push/schedule runs on main). L1 and L2 are the
// limit-1000 payload of the same repository re-fetched in Phase 1.9, where
// the W = 30 cap is active and two reasons coincide; their expected values
// come from an independent Python oracle (evidence/phase19-probes.txt).
func TestEvaluateValues_RealDataValueStrings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		id, digits string
		count, n   int
		x          float64
		stats      *oracleStats
		reasons    []string
	}{
		{id: "R1 ci.failure_rate:CI", count: 80, n: 19, reasons: []string{"insufficient_samples"},
			digits: "00101000000000000010000000000000000000001000000111010000000000000010000011100000"},
		{id: "R2 ci.failure_rate:Security Scan", count: 88, n: 21, x: 0,
			stats: &oracleStats{0.107143, 0.280306, 0.280306, -0.382235, 0}, reasons: []string{"below_baseline"},
			digits: "0000000000000000000000000000000000000000000000000111111111000000000000000000000000000000"},
		{id: "L1 ci.failure_rate:CI limit 1000", count: 274, n: 30,
			stats: &oracleStats{0.416667, 0.422091, 0.422091, -0.987149, 0}, reasons: []string{"below_baseline"},
			digits: "1011000000010000000000000010000000010001010100000000010010001001000010000000000100000001000010000000100000010001100000001001111101101000010100000001001111111111111111111111111111111111001110100000101000000000000010000000000000000000001000000111010000000000000010000011100000"},
		{id: "L2 ci.failure_rate:Security Scan limit 1000", count: 291, n: 30,
			stats:   &oracleStats{0.075, 0.238078, 0.25, -0.3, 0},
			reasons: []string{"variance_floor_applied", "below_baseline"},
			digits:  "000000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000000011000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000111111111000000000000000000000000000000"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			values := valuesFromString(t, tc.digits)
			require.Len(t, values, tc.count)

			got := healthband.EvaluateValues(values)

			if assert.NotNil(t, got.N) {
				assert.Equal(t, tc.n, *got.N)
			}
			assertNear(t, tc.x, got.X, "x")
			assert.Equal(t, tc.reasons, reasonsOf(got))
			if tc.stats == nil {
				assert.Nil(t, got.Z)
				assert.Nil(t, got.Tier)
				return
			}
			assertNear(t, tc.stats.mu, got.Mu, "mu")
			assertNear(t, tc.stats.sd, got.SD, "sd")
			assertNear(t, tc.stats.sdEff, got.SDEff, "sd_eff")
			assertNear(t, tc.stats.z, got.Z, "z")
			if assert.NotNil(t, got.Tier) {
				assert.Equal(t, tc.stats.tier, *got.Tier)
			}
		})
	}
}

// Detector Contract item 9: every evaluation carries the fixed constants.
func TestEvaluateValues_RecordsFixedConstants(t *testing.T) {
	t.Parallel()
	got := healthband.EvaluateValues(valuesFromBlocks(repeat(0, 20), 0.5))

	if assert.NotNil(t, got.Constants) {
		assert.Equal(t, healthband.Constants{K: 4, W: 30, NMin: 20, Floor: 0.25, Eps: 1e-9}, *got.Constants)
	}
}
