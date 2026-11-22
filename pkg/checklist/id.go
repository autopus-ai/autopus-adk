package checklist

import (
	"fmt"
	"regexp"
	"strconv"
)

// A check id is CHK-<sequence>, optionally followed by -<6 hex>. Checks and
// candidates are committed, so two branches cut from the same main draft the
// same next number; the suffix is the head of the candidate's lesson
// fingerprint, so different defects get different ids, and the same defect
// drafted on both branches gets the same id, which a merge then shows as one
// file. Ids without a suffix (CHK-001) remain valid.
var checkIDPattern = regexp.MustCompile(`^CHK-(\d{3,})(?:-([0-9a-f]{6}))?$`)

// idForm names the accepted id forms in error messages.
const idForm = "CHK-NNN or CHK-NNN-xxxxxx"

// ValidID reports a well-formed check id.
func ValidID(id string) bool { return checkIDPattern.MatchString(id) }

// parseID splits a valid id into its sequence number and suffix.
func parseID(id string) (int, string, bool) {
	m := checkIDPattern.FindStringSubmatch(id)
	if m == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", false
	}
	return n, m[2], true
}

// idLess orders ids by sequence number, then suffix, then text, so CHK-999
// precedes CHK-1000 and the two CHK-004 ids of a merge sit together.
func idLess(a, b string) bool {
	na, sa, okA := parseID(a)
	nb, sb, okB := parseID(b)
	switch {
	case okA != okB:
		return okA
	case !okA || (na == nb && sa == sb):
		return a < b
	case na != nb:
		return na < nb
	}
	return sa < sb
}

// nextID is one past the highest sequence used by any check or candidate,
// suffixed with the head of the new candidate's fingerprint.
func nextID(checks []Check, cands []Candidate, fingerprint string) string {
	maxN := 0
	bump := func(id string) {
		if n, _, ok := parseID(id); ok && n > maxN {
			maxN = n
		}
	}
	for _, c := range checks {
		bump(c.ID)
	}
	for _, c := range cands {
		bump(c.ID)
	}
	id := fmt.Sprintf("CHK-%03d", maxN+1)
	if len(fingerprint) >= 6 && checkIDPattern.MatchString(id+"-"+fingerprint[:6]) {
		id += "-" + fingerprint[:6]
	}
	return id
}
