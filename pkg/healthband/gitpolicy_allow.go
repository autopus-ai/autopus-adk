package healthband

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Command allowlist of Git Execution Policy item 6. Each form is one exact
// argv after the policy flags: literal tokens match byte for byte, and a
// placeholder admits one well-formed value. Paths reach git only after --
// or behind :(literal), and every other value is an OID, a band ref, a
// branch name, or an absolute path, so no value can be read as an option. No
// network command (fetch, push, ls-remote, remote), no worktree prune, lock,
// or unlock, no second --force, and no ref write outside
// refs/heads/autopus/band/ is admitted.

// gitIndexRule says whether a form takes GIT_INDEX_FILE.
type gitIndexRule int

const (
	gitIndexNever    gitIndexRule = iota
	gitIndexRequired              // read-tree and apply --cached touch only the band temp index
	gitIndexAllowed               // write-tree reads either index
)

type gitPolicyForm struct {
	tokens []string
	index  gitIndexRule
}

// gitPolicyPathsToken is a trailing placeholder for one or more paths.
const gitPolicyPathsToken = "<paths...>"

// gitPolicyForms are the git commands this SPEC names: the Git Execution
// Policy, the Local Patch Flow, the Patch Policy, the Cleanup Rules, and the
// removal of a stopped checkout.
var gitPolicyForms = []gitPolicyForm{
	{tokens: []string{"version"}},
	{tokens: []string{"config", "--list", "--show-scope", "--show-origin", "-z"}},
	{tokens: []string{"rev-parse", "--path-format=absolute", "--git-common-dir"}},
	{tokens: []string{"rev-parse", "--path-format=absolute", "--git-dir"}},
	{tokens: []string{"rev-parse", "--path-format=absolute", "--show-toplevel"}},
	{tokens: []string{"rev-parse", "HEAD"}},
	{tokens: []string{"rev-parse", "--verify", "<rev>"}},
	{tokens: []string{"rev-parse", "--verify", "--quiet", "<rev>"}},
	{tokens: []string{"symbolic-ref", "--short", "refs/remotes/origin/HEAD"}},
	{tokens: []string{"symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"}},
	{tokens: []string{"symbolic-ref", "-q", "--no-recurse", "<band-ref>"}},
	{tokens: []string{"check-ref-format", "--branch", "<branch>"}},
	// The base listing with modes, OIDs, and sizes: the checkout preflight,
	// and Patch Policy items 3-4 (folded collisions against every tracked
	// path and directory) and 6 (the base manifests of RR-7).
	{tokens: []string{"ls-tree", "-r", "-l", "-z", "<oid>"}},
	{tokens: []string{"ls-tree", "-r", "-z", "--name-only", "<oid>"}},
	{tokens: []string{"ls-tree", "-z", "<oid>", "--", "<literal-path>"}},
	// Patch Policy item 6 reads the base manifests by the listing's blob
	// OIDs, one per stdin line; cat-file applies no filter or textconv.
	{tokens: []string{"cat-file", "--batch"}},
	{tokens: []string{"check-attr", "-z", "<source>", "filter", "--", gitPolicyPathsToken}},
	{tokens: []string{"apply", "--numstat", "--summary", "-z", "--check"}},
	{tokens: []string{"read-tree", "<oid>"}, index: gitIndexRequired},
	{tokens: []string{"apply", "--cached"}, index: gitIndexRequired},
	{tokens: []string{"write-tree"}, index: gitIndexAllowed},
	{tokens: []string{"apply", "--index"}},
	{tokens: []string{"apply", "--index", "<abs>"}},
	{tokens: []string{"worktree", "add", "--no-checkout", "--detach", "<abs>", "<oid>"}},
	{tokens: []string{"worktree", "list", "--porcelain", "-z"}},
	{tokens: []string{"worktree", "remove", "--force", "<abs>"}},
	{tokens: []string{"reset", "--hard", "--no-recurse-submodules", "--quiet"}},
	{tokens: []string{"status", "--porcelain", "-z", "--untracked-files=all", "--ignored"}},
	{tokens: []string{"diff", "--no-ext-diff", "--no-textconv", "--binary"}},
	{tokens: []string{"ls-files", "-v", "-z"}},
	{tokens: []string{"ls-files", "-s", "-z"}},
	{tokens: []string{"commit", "--no-verify", "--cleanup=verbatim", "-F", "<abs>"}},
	{tokens: []string{"cat-file", "commit", "<oid>"}},
	{tokens: []string{"update-ref", "--no-deref", "<band-ref>", "<oid>", ""}},
	{tokens: []string{"update-ref", "--no-deref", "-d", "<band-ref>", "<oid>"}},
	{tokens: append(gitFormatPatchFlags(), "<range>")},
}

// gitFormatPatchFlags are the canonical format-patch flags of Local Patch
// Flow step 11, which remove every effect of format.* settings.
func gitFormatPatchFlags() []string {
	return []string{
		"format-patch", "--stdout", "--no-signature", "--no-thread", "--no-numbered",
		"--no-cover-letter", "--no-notes", "--no-attach", "--no-add-header", "--no-to", "--no-cc",
		"--no-from", "--no-base", "--no-signoff", "--subject-prefix=PATCH", "--full-index",
		"--no-textconv", "--no-ext-diff",
	}
}

// GitFormatPatchArgs is the canonical format-patch of base..commit.
func GitFormatPatchArgs(base, commit string) []string {
	return append(gitFormatPatchFlags(), base+".."+commit)
}

// BandBranchRef is the local branch of a key: refs/heads/autopus/band/<key>.
func BandBranchRef(key string) string { return bandRefPrefix + key }

const bandRefPrefix = "refs/heads/autopus/band/"

var (
	gitOIDPattern     = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	bandKeyRefPattern = regexp.MustCompile(`^refs/heads/autopus/band/[a-z0-9-]{1,200}$`)
)

// gitPolicyPlaceholders validate the value of each placeholder token.
var gitPolicyPlaceholders = map[string]func(string) bool{
	"<oid>":      validGitOID,
	"<rev>":      validGitPolicyRev,
	"<band-ref>": bandKeyRefPattern.MatchString,
	"<branch>":   validGitBranchInput,
	"<abs>":      validGitPolicyAbs,
	"<range>": func(arg string) bool {
		base, commit, found := strings.Cut(arg, "..")
		return found && validGitOID(base) && validGitOID(commit)
	},
	"<source>": func(arg string) bool {
		oid, found := strings.CutPrefix(arg, "--source=")
		return found && validGitOID(oid)
	},
	"<literal-path>": func(arg string) bool {
		path, found := strings.CutPrefix(arg, ":(literal)")
		return found && validGitPolicyPath(path)
	},
}

// CheckGitCommand admits args, the git argv after the policy flags, only as
// one form of the allowlist with well-formed values. indexFile is the
// GIT_INDEX_FILE the command would get: read-tree and apply --cached require
// an absolute one, write-tree takes one or none, and every other form
// refuses one, so no command but the temp-index three sees GIT_INDEX_FILE.
func CheckGitCommand(args []string, indexFile string) error {
	for _, form := range gitPolicyForms {
		if form.index.admits(indexFile) && form.matches(args) {
			return nil
		}
	}
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	return fmt.Errorf("%w: git %q", ErrGitCommandNotAllowed, name)
}

func (rule gitIndexRule) admits(indexFile string) bool {
	switch rule {
	case gitIndexRequired:
		return validGitPolicyAbs(indexFile)
	case gitIndexAllowed:
		return indexFile == "" || validGitPolicyAbs(indexFile)
	default:
		return indexFile == ""
	}
}

func (form gitPolicyForm) matches(args []string) bool {
	tokens := form.tokens
	if last := len(tokens) - 1; tokens[last] == gitPolicyPathsToken {
		if len(args) <= last {
			return false
		}
		for _, path := range args[last:] {
			if !validGitPolicyPath(path) {
				return false
			}
		}
		tokens, args = tokens[:last], args[:last]
	}
	if len(tokens) != len(args) {
		return false
	}
	for i, token := range tokens {
		if check, placeholder := gitPolicyPlaceholders[token]; placeholder {
			if !check(args[i]) {
				return false
			}
		} else if token != args[i] {
			return false
		}
	}
	return true
}

func validGitOID(oid string) bool { return gitOIDPattern.MatchString(oid) }

// validGitPolicyRev admits the revisions band verifies: an OID's commit or
// tree, the base candidates of item 4, and a band branch.
func validGitPolicyRev(rev string) bool {
	if oid, found := strings.CutSuffix(rev, "^{commit}"); found {
		if validGitOID(oid) {
			return true
		}
		for _, prefix := range []string{"refs/remotes/origin/", "refs/heads/"} {
			if branch, isBase := strings.CutPrefix(oid, prefix); isBase && validGitBranchName(branch) {
				return true
			}
		}
		return false
	}
	if oid, found := strings.CutSuffix(rev, "^{tree}"); found {
		return validGitOID(oid)
	}
	return bandKeyRefPattern.MatchString(rev)
}

// validGitPolicyAbs admits a clean absolute path, with or without one
// trailing slash, so no constructed path climbs out with "..".
func validGitPolicyAbs(path string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	return trimmed != "" && filepath.IsAbs(trimmed) && filepath.Clean(trimmed) == trimmed &&
		!strings.ContainsAny(path, "\x00\n")
}

// validGitPolicyPath admits a non-empty path for a position behind -- or
// :(literal); the Patch Policy, not the allowlist, judges its content.
func validGitPolicyPath(path string) bool {
	return path != "" && !strings.ContainsRune(path, 0)
}

// validGitBranchInput admits the value of check-ref-format --branch, which
// git itself judges: non-empty, at most 1,024 bytes, no control character,
// and no leading "-".
func validGitBranchInput(name string) bool {
	if name == "" || len(name) > 1024 || strings.HasPrefix(name, "-") {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// validGitBranchName applies git's refname rules to a branch name, so a base
// candidate holds no revision syntax: no "..", "@{", lone "@", space, or one
// of ~^:?*[\, no empty or dot-led component, no ".lock" component end, and
// no trailing ".".
func validGitBranchName(name string) bool {
	if !validGitBranchInput(name) || name == "@" || strings.Contains(name, "..") ||
		strings.Contains(name, "@{") || strings.HasSuffix(name, ".") || strings.ContainsAny(name, " ~^:?*[\\") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	return true
}
