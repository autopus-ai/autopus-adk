package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"slices"
	"strings"
)

// FingerprintVersion is the fingerprint algorithm version. Every candidate,
// link, and rejection record stores it, and duplicates are matched on the
// (version, fingerprint) pair, so changing the algorithm means raising it.
const FingerprintVersion = 1

// fingerprintInput is the canonical hash input. It is a struct rather than a
// map so json.Marshal emits the keys in this fixed order (REQ-HC-02).
type fingerprintInput struct {
	V        int      `json:"v"`
	Type     string   `json:"type"`
	Pattern  string   `json:"pattern"`
	Files    []string `json:"files"`
	Packages []string `json:"packages"`
}

// CanonicalInput returns the exact bytes Fingerprint hashes for e. Only type,
// pattern, files, and packages take part; the values are the stored ones, and
// no secret detector runs, so a detector change never moves a fingerprint.
func CanonicalInput(e Entry) []byte {
	input := fingerprintInput{
		V:        FingerprintVersion,
		Type:     e.Type,
		Pattern:  normalizePattern(e.Pattern),
		Files:    normalizeFiles(e.Files),
		Packages: normalizePackages(e.Packages),
	}
	// json.Marshal cannot fail for a struct of strings, an int, and string
	// slices: there is no unsupported type, cycle, or custom marshaler.
	data, _ := json.Marshal(input)
	return data
}

// Fingerprint returns the lowercase hex SHA-256 of CanonicalInput(e).
func Fingerprint(e Entry) string {
	sum := sha256.Sum256(CanonicalInput(e))
	return hex.EncodeToString(sum[:])
}

// normalizePattern lowercases with strings.ToLower and then collapses every
// run of Unicode whitespace into one space with strings.Fields.
func normalizePattern(pattern string) string {
	return strings.Join(strings.Fields(strings.ToLower(pattern)), " ")
}

// normalizeFiles applies the fixed order: trim, drop empty, convert
// backslashes, path.Clean, drop ".", deduplicate, byte-sort. Empty items are
// dropped before path.Clean because path.Clean("") is ".".
func normalizeFiles(files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		file = path.Clean(strings.ReplaceAll(file, `\`, "/"))
		if file == "." {
			continue
		}
		out = append(out, file)
	}
	return sortedUnique(out)
}

// normalizePackages trims, drops empty items, deduplicates, and byte-sorts.
func normalizePackages(packages []string) []string {
	out := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if pkg = strings.TrimSpace(pkg); pkg != "" {
			out = append(out, pkg)
		}
	}
	return sortedUnique(out)
}

// sortedUnique byte-sorts items in place and drops repeats. The result is
// never nil, so an empty list encodes as [] and never as null.
func sortedUnique(items []string) []string {
	slices.Sort(items)
	return slices.Compact(items)
}

// candidateIDFor names the candidate of a fingerprint: GTC- and its first 12
// hex digits.
func candidateIDFor(fingerprint string) string {
	return "GTC-" + fingerprint[:12]
}

// taskIDFor names the golden task of a fingerprint: GT-INC- and its first 8
// hex digits in upper case, which fits the SPEC-HARNEVAL-001 id grammar.
func taskIDFor(fingerprint string) string {
	return "GT-INC-" + strings.ToUpper(fingerprint[:8])
}
