package cli

import "strings"

// providerReadinessAdvice follows the not-ready lines under Provider
// Readiness: each line names its login command, and a status the probe
// misclassified has an explicit escape in spec review.
const providerReadinessAdvice = "Run the login command shown for each not-ready provider, then re-run 'auto doctor'; " +
	"if a status is misclassified, 'auto spec review --skip-provider-readiness' skips the preflight"

// retiredOrchestraRemedy repairs the surface SPEC-PANERM-001 retired: `auto
// update` prunes the legacy orchestra keys from autopus.yaml and retracts the
// stale completion hooks of every configured platform (REQ-14).
const retiredOrchestraRemedy = `run "auto update"`

// userLevelStaleHooksRemedy covers the user-level settings files that `auto
// update` reports but never edits (REQ-13).
const userLevelStaleHooksRemedy = "remove these handlers by hand; auto update never edits user-level settings"

// localStaleHooksRemedy covers .claude/settings.local.json, which `auto
// update` never edits; the scripts its handlers run stay until they are gone.
const localStaleHooksRemedy = "remove these handlers from .claude/settings.local.json by hand, then run \"auto update\"; " +
	"auto update never edits that file and keeps the scripts it names"

// openCodeHeldStaleHooksRemedy replaces retiredOrchestraRemedy while
// opencode.json names a listed script and opencode is not configured: no
// update edits that file, and every update keeps a script it names, so
// running update again changes nothing. Either removing the entries or
// configuring opencode, whose update retracts them, lets the next update
// delete the scripts.
func openCodeHeldStaleHooksRemedy(scripts []string) string {
	return "opencode is not configured, so auto update never edits opencode.json and keeps the scripts it loads (" +
		strings.Join(scripts, ", ") + "): remove those plugin entries from opencode.json by hand or run " +
		`"auto platform add opencode", then run "auto update"`
}

// doctorRemediationAdvice names the command that actually repairs what failed.
//
// The banner used to say "review warnings or run 'auto doctor --fix' where
// offered" for every failure. `--fix` installs missing dependencies and nothing
// else, so on a repo whose harness is gitignored -- the layout autopus-adk
// itself uses -- a fresh clone fails every platform check and the advice sends
// the operator to a command that installs zero files and reprints the same
// advice. `auto update` is the installer.
func doctorRemediationAdvice(platformFailed, depsMissing bool) string {
	switch {
	case platformFailed && depsMissing:
		return "Issues found — run 'auto update' to install the managed surface, then 'auto doctor --fix' for missing dependencies"
	case platformFailed:
		return "Issues found — the managed surface is missing or stale; run 'auto update'"
	case depsMissing:
		return "Issues found — run 'auto doctor --fix' to install missing dependencies"
	default:
		return "Issues found — review the [ERROR] lines above"
	}
}

// doctorWarningAdvice summarises a report with warnings and no failure. The
// banner used to say "Issues found — the managed surface is missing or
// stale; run 'auto update'" over a page whose only findings were degraded OMP
// model routing: no ERROR line said what failed, and `auto update` does not
// repair routing. Naming the sections points the reader at the lines to read.
func doctorWarningAdvice(sections []string, updateFixes bool) string {
	msg := "Warnings found, nothing failed — review the [WARN] lines above"
	if len(sections) > 0 {
		msg = "Warnings found, nothing failed — review the [WARN] lines in: " + strings.Join(sections, ", ")
	}
	if updateFixes {
		msg += "; 'auto update' removes the retired orchestra surface"
	}
	return msg
}

type doctorVerdictKind int

const (
	doctorVerdictPass doctorVerdictKind = iota
	doctorVerdictWarn
	doctorVerdictFail
)

type doctorVerdictInput struct {
	allOK              bool
	errorLines         int
	unhealthy          []string
	updateFixes        bool
	depsMissing        bool
	retiredUpdateFixes bool
}

type doctorVerdict struct {
	kind    doctorVerdictKind
	message string
}

// decideDoctorVerdict derives the result box from what the report printed.
// An ERROR line the reader saw always wins, even when no check marked the
// report unhealthy (a failed --fix install prints one and returns nil), so a
// PASS box can never sit under an ERROR line and a FAIL box always has one.
func decideDoctorVerdict(in doctorVerdictInput) doctorVerdict {
	switch {
	case in.errorLines > 0:
		return doctorVerdict{doctorVerdictFail, doctorRemediationAdvice(in.updateFixes, in.depsMissing)}
	case in.allOK:
		return doctorVerdict{doctorVerdictPass, "All checks passed"}
	default:
		return doctorVerdict{doctorVerdictWarn, doctorWarningAdvice(in.unhealthy, in.retiredUpdateFixes)}
	}
}
