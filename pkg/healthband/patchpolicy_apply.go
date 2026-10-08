package healthband

import (
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/editguard"
)

// Patch Policy item 9 and the closing git apply cross-check of item 2.

// guardDenial is item 9: band applies the diff from the CLI process, which
// no pre-tool hook sees, so the policy asks the edit guard about the decoded
// paths where the manifests and fix locks live, the user's checkout. A deny
// gives path_denied:<class>; a guard fault, which the guard's own contract
// turns into an allow with a Diagnostic, and a panic give
// path_denied:guard_fault, so band fails closed where an agent edit would
// proceed (CD-3 L1).
func guardDenial(decide func(editguard.Call, editguard.Options) editguard.Decision, checkout string, files []*diffFile) (code string) {
	if decide == nil {
		decide = editguard.Decide
	}
	targets := make([]string, 0, len(files))
	for _, file := range files {
		targets = append(targets, file.path)
	}
	defer func() {
		if recover() != nil {
			code = PatchCodeGuardFault
		}
	}()
	decision := decide(editguard.Call{Cwd: checkout, Targets: targets}, editguard.Options{})
	switch {
	case decision.Deny && decision.Class != "":
		return PatchCodePathDenied + ":" + string(decision.Class)
	case decision.Deny || decision.Diagnostic != "":
		return PatchCodeGuardFault
	}
	return ""
}

// applyCheck runs `git apply --numstat --summary -z --check` with the diff
// on stdin; it writes no object. The decoded path set must equal the path
// set git reports, each file's counts must equal its --numstat counts, and
// the summary may hold only a `create mode 100644` line per added file, so
// a parse that differs from git's refuses the diff (patch_invalid).
func (r policyRun) applyCheck(files []*diffFile) (PatchVerdict, error) {
	out, err := r.git([]byte(r.diff), "apply", "--numstat", "--summary", "-z", "--check")
	if err != nil {
		return refusePatch(PatchCodeInvalid), nil
	}
	records := strings.Split(string(out), "\x00")
	summary := records[len(records)-1]
	records = records[:len(records)-1]
	if len(records) != len(files) {
		return refusePatch(PatchCodeInvalid), nil
	}
	byPath := make(map[string]*diffFile, len(files))
	creates := map[string]bool{}
	for _, file := range files {
		byPath[file.path] = file
		if file.isNew {
			creates[" create mode 100644 "+file.path] = true
		}
	}
	accepted := make([]PatchFile, 0, len(files))
	for _, record := range records {
		stat, ok := numstatRecord(record)
		file := byPath[stat.Path]
		if !ok || file == nil || stat.Added != file.added || stat.Removed != file.removed {
			return refusePatch(PatchCodeInvalid), nil
		}
		delete(byPath, stat.Path)
		accepted = append(accepted, stat)
	}
	for _, line := range strings.Split(strings.TrimSuffix(summary, "\n"), "\n") {
		if line != "" && !creates[line] {
			return refusePatch(PatchCodeInvalid), nil
		}
		delete(creates, line)
	}
	if len(creates) > 0 {
		return refusePatch(PatchCodeInvalid), nil
	}
	return PatchVerdict{Files: accepted}, nil
}

// numstatRecord reads `<added>\t<removed>\t<path>`; a binary file's `-`
// counts are refused.
func numstatRecord(record string) (PatchFile, bool) {
	fields := strings.SplitN(record, "\t", 3)
	if len(fields) != 3 {
		return PatchFile{}, false
	}
	added, errAdded := strconv.Atoi(fields[0])
	removed, errRemoved := strconv.Atoi(fields[1])
	return PatchFile{Path: fields[2], Added: added, Removed: removed}, errAdded == nil && errRemoved == nil
}
