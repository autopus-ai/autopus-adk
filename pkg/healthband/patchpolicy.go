package healthband

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/insajin/autopus-adk/pkg/editguard"
)

// Patch Policy (SPEC-SIGMABAND-002 REQ-08). The policy reads a patch
// request's reply, keeps it in memory, and accepts the one diff it holds only
// when every item passes; the first item that fails gives the code. It writes
// no object and no file: its git commands are ls-tree, check-attr, cat-file
// --batch over the base manifests' OIDs (item 6, RR-7), and, last,
// `git apply --numstat --summary -z --check` with the diff on stdin.
//
// Command order: the in-process checks of items 2–3 run before any git
// command sees a decoded path, and the git apply cross-check of item 2 runs
// last (Local Patch Flow step 7, "ending with git apply"). Both orders give
// the same codes for every diff whose own items pass, and only the late
// cross-check lets item 4 refuse an NFD spelling of a tracked NFC file on a
// normalization-insensitive volume, where git apply fails first ("already
// exists in working directory").

// Patch Policy codes. A refusal ends the local_patch claim failed:<code>.
const (
	PatchCodeNoPatch       = "no_patch"
	PatchCodeInvalid       = "patch_invalid"
	PatchCodePathDenied    = "path_denied"
	PatchCodeFilter        = "path_denied:filter"
	PatchCodeCaseCollision = "path_denied:case_collision"
	PatchCodeGuardFault    = "path_denied:guard_fault"
	PatchCodeContentDenied = "patch_content_denied"
	PatchCodeControlChar   = "patch_content_denied:control_char"
	PatchCodeConfusable    = "patch_content_denied:confusable"
	PatchCodeTooLarge      = "patch_too_large"
)

// Patch Policy bounds (items 1 and 8).
const (
	PatchMaxDiffBytes    = 64 << 10
	PatchMaxFiles        = 10
	PatchMaxChangedLines = 400
)

// GitRunner runs one git command for the Patch Policy: args start with the
// git subcommand, so a hardened runner (Git Execution Policy, T4) prepends
// its own -c flags and environment and checks the subcommand against its
// allowlist. dir is the working directory and stdin, when non-nil, the
// command's standard input. It returns stdout; a non-zero exit is an error.
type GitRunner interface {
	Run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error)
}

// GitRunnerFunc adapts a function to GitRunner.
type GitRunnerFunc func(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error)

// Run calls f.
func (f GitRunnerFunc) Run(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	return f(ctx, dir, stdin, args...)
}

// PatchReply is the reply of a patch request (Local Patch Provider Contract
// item 7): the result text of its stream, and whether the stream passed its
// 8 MiB bound or the 1 MiB text capture dropped bytes.
type PatchReply struct {
	Text    string
	Dropped bool
}

// PatchPolicyInput is what one evaluation reads.
type PatchPolicyInput struct {
	Reply PatchReply
	// BaseSHA is the prep base commit (40 or 64 lowercase hex).
	BaseSHA string
	// Worktree is the claim's band worktree at BaseSHA; every git command
	// runs there.
	Worktree string
	// Checkout is the top level of the user's checkout, where editguard
	// finds the manifests and fix locks (item 9).
	Checkout string
}

// PatchFile is one accepted path with its git apply --numstat counts, the
// files[] entry of the result record.
type PatchFile struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// PatchVerdict is the policy's answer. An accepted verdict has no code and
// carries the diff, which stays in memory until the executor applies it.
type PatchVerdict struct {
	Code  string
	Diff  string
	Files []PatchFile
}

// Accepted reports whether the policy accepted the diff.
func (v PatchVerdict) Accepted() bool { return v.Code == "" }

// PatchPolicy evaluates patch replies. Git runs every git command; Decide is
// the edit guard of item 9 and nil means editguard.Decide (a test seam).
type PatchPolicy struct {
	Git    GitRunner
	Decide func(editguard.Call, editguard.Options) editguard.Decision
}

var (
	errPatchPolicyInput = errors.New("healthband: patch policy input outside the contract")
	baseSHAPattern      = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// Evaluate runs Patch Policy items 1–9 over in.Reply. It returns an error
// only for input outside the contract or a context that ended; every other
// git failure refuses the diff with patch_invalid, so no fault accepts one.
func (p PatchPolicy) Evaluate(ctx context.Context, in PatchPolicyInput) (PatchVerdict, error) {
	if p.Git == nil || !baseSHAPattern.MatchString(in.BaseSHA) || !filepath.IsAbs(in.Worktree) || !filepath.IsAbs(in.Checkout) {
		return PatchVerdict{}, errPatchPolicyInput
	}
	diff, code := extractDiff(in.Reply)
	if code != "" {
		return refusePatch(code), nil
	}
	files, ok := parseDiff(diff)
	if !ok || !validPaths(files) {
		return refusePatch(PatchCodeInvalid), nil
	}
	run := policyRun{policy: p, ctx: ctx, in: in, diff: diff}
	verdict, err := run.evaluate(files)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return PatchVerdict{}, fmt.Errorf("healthband: patch policy stopped: %w", ctxErr)
		}
		return refusePatch(PatchCodeInvalid), nil
	}
	if verdict.Accepted() {
		verdict.Diff = diff
	}
	return verdict, nil
}

// policyRun is one evaluation past the in-process items 1–3. diff is the
// text the sections came from, so git reads exactly the bytes the items
// checked.
type policyRun struct {
	policy PatchPolicy
	ctx    context.Context
	in     PatchPolicyInput
	diff   string
}

// evaluate runs the items that read the base (2–4, 6: the exact-path
// queries, the listing, check-attr, and the manifests), the in-process
// items 5–8, the edit guard (9), and the git apply cross-check of item 2, in
// that order. An error is a git fault.
func (r policyRun) evaluate(files []*diffFile) (PatchVerdict, error) {
	base, err := r.readBase(files)
	if err != nil {
		return PatchVerdict{}, err
	}
	if !allowedBaseEntries(files, base) {
		return refusePatch(PatchCodeInvalid), nil
	}
	filtered, err := r.filtered(files)
	if err != nil {
		return PatchVerdict{}, err
	}
	if filtered {
		return refusePatch(PatchCodeFilter), nil
	}
	built, err := r.buildDenials(base)
	if err != nil {
		return PatchVerdict{}, err
	}
	steps := []func() string{
		func() string { return caseCollision(files, base.folds) },
		func() string { return deniedPath(files, base.exact, built) },
		func() string { return deniedContent(files) },
		func() string { return tooLarge(files) },
		func() string { return guardDenial(r.policy.Decide, r.in.Checkout, files) },
	}
	for _, step := range steps {
		if code := step(); code != "" {
			return refusePatch(code), nil
		}
	}
	return r.applyCheck(files)
}

func refusePatch(code string) PatchVerdict { return PatchVerdict{Code: code} }

// git runs one policy command in the worktree.
func (r policyRun) git(stdin []byte, args ...string) ([]byte, error) {
	return r.policy.Git.Run(r.ctx, r.in.Worktree, stdin, args...)
}
