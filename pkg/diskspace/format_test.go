package diskspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every size prints with one decimal in binary units (Definitions: GiB,
// MiB), so 45 GiB of disk.min_free_gb prints as df -g would show it.
func TestGiBString_OneDecimal(t *testing.T) {
	t.Parallel()
	for bytes, want := range map[int64]string{
		0:            "0.0",
		9 * GiB:      "9.0",
		45 * GiB:     "45.0",
		107374182400: "100.0",
		gibOf(59.9):  "59.9",
		GiB / 10:     "0.1",
		GiB/20 - 1:   "0.0",
	} {
		assert.Equal(t, want, GiBString(bytes), bytes)
	}
}

// gibOf converts a fractional GiB count at run time, as the CLI seam does.
func gibOf(v float64) int64 { return int64(v * float64(GiB)) }

func TestSizeString_PicksTheLargestUnitAtOrAboveOne(t *testing.T) {
	t.Parallel()
	for bytes, want := range map[int64]string{
		0:               "0 B",
		100:             "100 B",
		KiB:             "1.0 KiB",
		512 * KiB:       "512.0 KiB",
		MiB:             "1.0 MiB",
		65 * MiB:        "65.0 MiB",
		4*MiB + 12*KiB:  "4.0 MiB",
		GiB:             "1.0 GiB",
		33*GiB + GiB/10: "33.1 GiB",
	} {
		assert.Equal(t, want, SizeString(bytes), bytes)
	}
}
