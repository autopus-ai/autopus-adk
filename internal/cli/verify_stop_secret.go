package cli

import (
	"context"
	"os"
	"strings"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// maxSecretHitsShown keeps the block reason short on a large change.
const maxSecretHitsShown = 10

// secretStopEnabled reports whether the stop gate checks added lines for
// credentials: on unless AUTOPUS_SECRET_STOP=off or hooks.secret_stop: false.
func secretStopEnabled(root string) bool {
	if strings.EqualFold(os.Getenv("AUTOPUS_SECRET_STOP"), "off") {
		return false
	}
	cfg := nearestConfig(root)
	return cfg == nil || cfg.Hooks.IsSecretStopEnabled()
}

// secretGate runs before the build: it costs one git diff and a regex pass,
// and a leaked credential is worth reporting even when the build would fail
// too. A new set of hits blocks once; the same set again, or a session past
// the block limit, ends with a warning instead, so the gate never loops on
// a value the agent decided to keep. The reason names file:line and rule,
// never the value.
func secretGate(ctx context.Context, session, root string) (block *stopHookOutput, warning string) {
	if !secretStopEnabled(root) {
		return nil, ""
	}
	hits := verifygate.AddedSecrets(ctx, root)
	if len(hits) == 0 {
		return nil, ""
	}
	where := describeSecretHits(hits)
	fingerprint := verifygate.SecretHitsFingerprint(hits)
	warning = "auto verify stop: added lines still look like credentials: " + where
	if readStopState(session, "secrets") == fingerprint || countStopBlock(session) > verifyStopMaxBlocks {
		return nil, warning
	}
	writeStopState(session, "secrets", fingerprint)
	return &stopHookOutput{Decision: "block", Reason: "Possible credential added: " + where + ".\n\n" +
		"Move the value to an environment variable or secret storage and read it from there. If it is a " +
		"deliberate test fixture, end that line with an `autopus:allow-secret` comment (or add its fingerprint " +
		"`file:rule:line` to .gitleaksignore). Do not repeat the value in your reply."}, ""
}

func describeSecretHits(hits []verifygate.SecretHit) string {
	parts := make([]string, 0, min(len(hits), maxSecretHitsShown)+1)
	for i, h := range hits {
		if i == maxSecretHitsShown {
			parts = append(parts, "and more")
			break
		}
		parts = append(parts, h.String())
	}
	return strings.Join(parts, ", ")
}

// withSecretWarning carries a warning onto the output as a system message,
// also when the output blocks: the version warning is recorded as shown once
// per session before it is returned, so a block that dropped it (the
// verification round, say) would lose it for good.
func withSecretWarning(out stopHookOutput, warning string) stopHookOutput {
	if warning == "" {
		return out
	}
	if out.Decision == "block" && strings.Contains(out.Reason, warning) {
		return out
	}
	if out.SystemMessage == "" {
		out.SystemMessage = warning
	} else {
		out.SystemMessage = warning + "\n" + out.SystemMessage
	}
	return out
}
