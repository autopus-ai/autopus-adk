package evidence

import "regexp"

// SecretDetectorSources returns the source text of every regex RedactText
// applies, in application order: the five token patterns, then the
// assignment, flag, JSON key, credential URL, query, two private note, and
// Unix and Windows user path detectors. Each call returns a fresh slice.
//
// pkg/secretscan keeps a verbatim copy of these regexes and its tests compare
// the copy with this list, so a detector added to RedactText must be listed
// here as well.
func SecretDetectorSources() []string {
	applied := make([]*regexp.Regexp, 0, len(secretPatterns)+9)
	applied = append(applied, secretPatterns...)
	applied = append(applied,
		sensitiveAssignmentRe, sensitiveFlagValueRe, jsonSensitiveRe,
		credentialURLRe, secretQueryRe, privateNoteRe, jsonPrivateNoteRe,
		userPathRe, windowsUserPathRe,
	)
	sources := make([]string, len(applied))
	for i, re := range applied {
		sources[i] = re.String()
	}
	return sources
}
