package healthband

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Codes of the Git Execution Policy checks (Local Patch Flow step 1). A
// config finding is GitConfigUnsafePrefix followed by the key, such as
// git_config_unsafe:filter.mark.clean.
const (
	GitConfigUnsafePrefix       = "git_config_unsafe:"
	GitVersionUnsupportedPrefix = "git_version_unsupported:"
	GitBaseUnavailable          = "base_unavailable"
	// GitConfigInfoAttributes is the key of a non-empty info/attributes.
	GitConfigInfoAttributes = "info_attributes"
	// GitConfigUnreadable is the key when git cannot list the configuration
	// or the listing does not parse: band fails closed.
	GitConfigUnreadable = "config_unreadable"
	// gitInfoAttributesBytes bounds the info/attributes read; a larger file
	// counts as unsafe.
	gitInfoAttributesBytes = 1 << 20
	gitCodeKeyBytes        = 128
)

// gitLFSValuePattern is item 3's byte-exact allowlist of a git-lfs filter
// value: single spaces and no shell metacharacter, because git runs filter
// commands through the shell. Item 2 blanks even an accepted value.
var gitLFSValuePattern = regexp.MustCompile(`^(git-lfs|/[A-Za-z0-9._/-]+/git-lfs) (filter-process|clean -- %f|smudge -- %f)$`)

// gitConfigEntry is one item of git config --list --show-scope
// --show-origin -z: scope NUL origin NUL key [LF value] NUL. A key without a
// value is an implicit true.
type gitConfigEntry struct {
	scope, key, value string
	hasValue          bool
}

// CheckConfig is Git Execution Policy item 3 in the runner's directory: the
// user's checkout at step 1, a new worktree before its checkout at step 2,
// the worktree again at step 8, and Cleanup Rule 3 before it reads a
// worktree. It reads info/attributes of $GIT_COMMON_DIR, which a linked
// worktree uses, and of $GIT_DIR, then the configuration of every scope,
// with every include and includeIf evaluated for the directory's own git
// directory. It returns "" when nothing unsafe is configured, else the code
// git_config_unsafe:<key>; a configuration git cannot list fails closed as
// git_config_unsafe:config_unreadable. Only a done context gives an error.
func (r GitPolicyRunner) CheckConfig(ctx context.Context) (string, error) {
	for _, dirFlag := range []string{"--git-common-dir", "--git-dir"} {
		out, err := r.Run(ctx, "rev-parse", "--path-format=absolute", dirFlag)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		dir := strings.TrimSuffix(string(out), "\n")
		if err != nil || !filepath.IsAbs(dir) {
			return GitConfigUnsafePrefix + GitConfigUnreadable, nil
		}
		if !gitInfoAttributesSafe(filepath.Join(dir, "info", "attributes")) {
			return GitConfigUnsafePrefix + GitConfigInfoAttributes, nil
		}
	}
	listing, err := r.Run(ctx, "config", "--list", "--show-scope", "--show-origin", "-z")
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	entries, ok := parseGitConfigListing(listing)
	if err != nil || !ok {
		return GitConfigUnsafePrefix + GitConfigUnreadable, nil
	}
	if key := gitConfigFinding(entries); key != "" {
		return GitConfigUnsafePrefix + key, nil
	}
	return "", nil
}

// gitInfoAttributesSafe reports that path is absent or holds only blank and
// comment lines, by git's own rule (leading " \t\r\n" skipped, then "#").
// Probe A3 showed that a linked worktree runs the filter that the common
// directory's info/attributes selects under every flag, so this refusal is
// mandatory. A symlink, a non-regular file, a read error, or a file above
// the bound is unsafe.
func gitInfoAttributesSafe(path string) bool {
	file, err := OpenRegular(path)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, gitInfoAttributesBytes+1))
	if err != nil || len(data) > gitInfoAttributesBytes {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimLeft(line, " \t\r\n"); line != "" && !strings.HasPrefix(line, "#") {
			return false
		}
	}
	return true
}

// parseGitConfigListing splits a -z listing; anything but whole entries is
// not a listing.
func parseGitConfigListing(listing []byte) ([]gitConfigEntry, bool) {
	fields := strings.Split(string(listing), "\x00")
	if fields[len(fields)-1] != "" {
		return nil, false
	}
	fields = fields[:len(fields)-1]
	if len(fields)%3 != 0 {
		return nil, false
	}
	entries := make([]gitConfigEntry, 0, len(fields)/3)
	for i := 0; i < len(fields); i += 3 {
		key, value, hasValue := strings.Cut(fields[i+2], "\n")
		if key == "" {
			return nil, false
		}
		entries = append(entries, gitConfigEntry{scope: fields[i], key: key, value: value, hasValue: hasValue})
	}
	return entries, true
}

// gitOwnConfig are the policy's -c values as git lists them (section and
// name lowercased), which the listing shows in the command scope.
var gitOwnConfig = func() map[string]bool {
	own := make(map[string]bool, len(gitPolicyConfig))
	for _, item := range gitPolicyConfig {
		key, value, _ := strings.Cut(item, "=")
		section, subsection, name := splitGitConfigKey(key)
		own[gitJoinConfigKey(strings.ToLower(section), subsection, strings.ToLower(name))+"\n"+value] = true
	}
	return own
}()

// gitConfigRules are item 3's refusals in the SPEC's order; each reports an
// unsafe entry from its lowercased section and name. Every scope counts, so a
// driver that only global or system configuration sets is refused too (probe
// A3, an intended fail-closed limitation).
var gitConfigRules = []func(section, subsection, name string, entry gitConfigEntry) bool{
	func(section, subsection, name string, entry gitConfigEntry) bool {
		if section != "filter" || subsection == "" || (name != "clean" && name != "smudge" && name != "process") {
			return false
		}
		return subsection != "lfs" || !entry.hasValue || !gitLFSValuePattern.MatchString(entry.value)
	},
	func(section, subsection, name string, _ gitConfigEntry) bool {
		return subsection != "" && (section == "diff" && (name == "textconv" || name == "command") ||
			section == "merge" && name == "driver")
	},
	func(section, subsection, _ string, _ gitConfigEntry) bool {
		lower := strings.ToLower(subsection)
		return section == "lfs" && (strings.HasPrefix(lower, "extension.") || strings.HasPrefix(lower, "customtransfer."))
	},
	func(section, subsection, name string, _ gitConfigEntry) bool {
		return section == "core" && subsection == "" && name == "alternaterefscommand"
	},
	func(section, subsection, name string, entry gitConfigEntry) bool {
		return section == "remote" && subsection != "" && name == "promisor" && gitConfigTrue(entry)
	},
	func(section, subsection, name string, _ gitConfigEntry) bool {
		return section == "remote" && subsection != "" && name == "partialclonefilter" ||
			section == "extensions" && subsection == "" && name == "partialclone"
	},
	// Beyond the SPEC list (flagged in the hand-off): a config-defined hook
	// command, which core.hooksPath does not cover in git versions that run
	// hook.<name>.command.
	func(section, subsection, name string, _ gitConfigEntry) bool {
		return section == "hook" && subsection != "" && name == "command"
	},
}

// gitConfigFinding returns the code key of the first unsafe entry by rule
// order, or "" when every entry is safe. The policy's own -c values are
// skipped: the scrubbed environment leaves no other command-scope source.
func gitConfigFinding(entries []gitConfigEntry) string {
	for _, rule := range gitConfigRules {
		for _, entry := range entries {
			if entry.scope == "command" && entry.hasValue && gitOwnConfig[entry.key+"\n"+entry.value] {
				continue
			}
			section, subsection, name := splitGitConfigKey(entry.key)
			if rule(strings.ToLower(section), subsection, strings.ToLower(name), entry) {
				return gitCodeKey(entry.key)
			}
		}
	}
	return ""
}

// splitGitConfigKey splits section.subsection.name; the subsection may hold
// dots and keeps its case.
func splitGitConfigKey(key string) (section, subsection, name string) {
	first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	if first < 0 {
		return key, "", ""
	}
	if first == last {
		return key[:first], "", key[last+1:]
	}
	return key[:first], key[first+1 : last], key[last+1:]
}

func gitJoinConfigKey(section, subsection, name string) string {
	if subsection == "" {
		return section + "." + name
	}
	return section + "." + subsection + "." + name
}

// gitConfigTrue reads a git boolean fail-closed: only an explicit false
// value (false, no, off, 0, or empty) is false.
func gitConfigTrue(entry gitConfigEntry) bool {
	if !entry.hasValue {
		return true
	}
	switch strings.ToLower(entry.value) {
	case "false", "no", "off", "0", "":
		return false
	}
	return true
}

// gitCodeKey spells a key for a code: core.alternateRefsCommand as the SPEC
// names it, every byte outside [A-Za-z0-9._-] as "_", at most 128 bytes, so a
// hostile subsection cannot put control text into a record or the output.
func gitCodeKey(key string) string {
	if strings.EqualFold(key, "core.alternateRefsCommand") {
		return "core.alternateRefsCommand"
	}
	safe := []byte(key)
	for i, c := range safe {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			safe[i] = '_'
		}
	}
	if len(safe) > gitCodeKeyBytes {
		safe = safe[:gitCodeKeyBytes]
	}
	return string(safe)
}
