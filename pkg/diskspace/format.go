package diskspace

import "strconv"

// Binary units. Every *_gb key is in GiB, as df -g prints, and every *_mb key
// in MiB.
const (
	KiB int64 = 1 << 10
	MiB int64 = 1 << 20
	GiB int64 = 1 << 30
)

// GiBString prints bytes in GiB with one decimal, without the unit: "45.0".
func GiBString(bytes int64) string {
	return oneDecimal(bytes, GiB)
}

// SizeString prints bytes in the largest binary unit that keeps the value at
// or above one, with one decimal: "65.0 MiB", "33.1 GiB", "0 B".
func SizeString(bytes int64) string {
	switch {
	case bytes >= GiB:
		return oneDecimal(bytes, GiB) + " GiB"
	case bytes >= MiB:
		return oneDecimal(bytes, MiB) + " MiB"
	case bytes >= KiB:
		return oneDecimal(bytes, KiB) + " KiB"
	default:
		return strconv.FormatInt(bytes, 10) + " B"
	}
}

func oneDecimal(bytes, unit int64) string {
	return strconv.FormatFloat(float64(bytes)/float64(unit), 'f', 1, 64)
}
