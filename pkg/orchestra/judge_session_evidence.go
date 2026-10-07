package orchestra

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FreshJudgeSessionEvidence records the observable logical session boundary
// without serializing participant or judge session identifiers. Isolated does
// not assert OS-level process, filesystem, or network sandboxing.
// @AX:ANCHOR: [AUTO] public JSON evidence contract for proving participant-to-judge session separation
// @AX:REASON: [AUTO] run receipts and external diagnostics depend on stable redacted fingerprints and verification semantics
type FreshJudgeSessionEvidence struct {
	Required                      bool   `json:"required"`
	Isolated                      bool   `json:"isolated"`
	Verified                      bool   `json:"verified"`
	Mechanism                     string `json:"mechanism"`
	ParticipantsTerminated        bool   `json:"participants_terminated"`
	ParticipantSessionFingerprint string `json:"participant_session_fingerprint"`
	JudgeSessionFingerprint       string `json:"judge_session_fingerprint"`
	Reason                        string `json:"reason"`
}

func newFreshSubprocessJudgeSessionEvidence() *FreshJudgeSessionEvidence {
	return &FreshJudgeSessionEvidence{
		Required:               true,
		Mechanism:              "fresh_backend_execution",
		ParticipantsTerminated: true,
		Reason:                 "fresh subprocess judge execution pending verification",
	}
}

func verifyFreshSubprocessJudgeSession(
	evidence *FreshJudgeSessionEvidence,
	response *ProviderResponse,
) {
	if evidence == nil {
		return
	}
	if response == nil {
		evidence.Reason = "fresh subprocess judge execution returned no response"
		return
	}
	if response.ExecutedBackend != "subprocess" {
		evidence.Reason = fmt.Sprintf(
			"fresh subprocess judge backend was not observed: %q",
			response.ExecutedBackend,
		)
		return
	}
	evidence.Isolated = true
	evidence.Verified = true
	evidence.Reason = "fresh subprocess backend execution verified"
}

func freshJudgeSessionFromResponses(responses []ProviderResponse) *FreshJudgeSessionEvidence {
	for i := len(responses) - 1; i >= 0; i-- {
		if responses[i].freshJudgeSession != nil {
			return responses[i].freshJudgeSession
		}
	}
	return nil
}

// @AX:NOTE: [AUTO] judge Args fail closed on provider-specific resume, continue, session, thread, or conversation tokens
func freshJudgeConfigError(provider ProviderConfig) error {
	identity := providerCanonicalName(provider.Name)
	if binaryIdentity := providerCanonicalName(filepath.Base(strings.TrimSpace(provider.Binary))); identity == "" {
		identity = binaryIdentity
	} else if !knownProviderIdentity(identity) && knownProviderIdentity(binaryIdentity) {
		identity = binaryIdentity
	}
	for _, arg := range provider.Args {
		if flag, blocked := freshJudgeResumeToken(identity, arg); blocked {
			return fmt.Errorf(
				"judge provider %q cannot prove a fresh session while resume/continue option %q is configured",
				provider.Name,
				flag,
			)
		}
	}
	return nil
}

func freshJudgeResumeToken(identity, arg string) (string, bool) {
	token := strings.ToLower(strings.TrimSpace(arg))
	if token == "" {
		return "", false
	}
	switch identity {
	case "claude":
		if token == "-c" || token == "-r" {
			return token, true
		}
	case "gemini", "opencode":
		if token == "-c" || token == "-s" {
			return token, true
		}
	}

	normalized := strings.TrimLeft(token, "-")
	if index := strings.IndexByte(normalized, '='); index >= 0 {
		normalized = normalized[:index]
	}
	switch normalized {
	case "resume", "continue",
		"session", "session-id",
		"thread", "thread-id",
		"chat", "chat-id",
		"conversation", "conversation-id",
		"from-pr", "fork-session":
		return "--" + normalized, true
	default:
		return "", false
	}
}

func knownProviderIdentity(identity string) bool {
	switch identity {
	case "claude", "codex", "gemini", "opencode":
		return true
	default:
		return false
	}
}

func freshJudgeSessionError(evidence *FreshJudgeSessionEvidence) error {
	switch {
	case evidence == nil:
		return fmt.Errorf("evidence is missing")
	case !evidence.Required:
		return fmt.Errorf("evidence is not marked required")
	case !evidence.ParticipantsTerminated:
		return fmt.Errorf("participant termination is not verified")
	case !evidence.Isolated:
		return fmt.Errorf("judge execution isolation is not verified")
	case !evidence.Verified:
		return fmt.Errorf("fresh judge execution is not verified")
	default:
		return nil
	}
}
