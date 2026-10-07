package cli

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
		return "Issues found — review the warnings above"
	}
}
