package diskspace

import (
	"strconv"
	"strings"
)

// longestMount returns the mount point of a /proc/self/mounts table that holds
// path: the longest one equal to path or a directory prefix of it. The table
// is parsed on every OS so it can be tested anywhere; only linux reads it.
func longestMount(table []byte, path string) string {
	best := ""
	for _, line := range strings.Split(string(table), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mount := unescapeMount(fields[1])
		if mountHolds(mount, path) && len(mount) > len(best) {
			best = mount
		}
	}
	if best == "" {
		return "/"
	}
	return best
}

func mountHolds(mount, path string) bool {
	return mount == "/" || path == mount || strings.HasPrefix(path, mount+"/")
}

// unescapeMount decodes the octal escapes the kernel writes for a space, tab,
// newline, or backslash in a mount point (\040, \011, \012, \134).
func unescapeMount(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] == '\\' && i+4 <= len(field) {
			if code, err := strconv.ParseUint(field[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(code))
				i += 3
				continue
			}
		}
		b.WriteByte(field[i])
	}
	return b.String()
}
