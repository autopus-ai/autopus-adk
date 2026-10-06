package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// readinessProbeOMP is the one `omp usage` probe shared by every OMP-backed provider.
const readinessProbeOMP readinessProbeKind = "omp"

// ompUsageArrays are the top-level arrays the OMP readiness rules read.
var ompUsageArrays = []string{"reports", "accountsWithoutUsage", "disabledCredentials"}

// ompReadinessCommand resolves the executable and environment exactly as the
// OMP review backend does, so the probe sees the review's accounts.
func ompReadinessCommand(env []string, dir string) (providerReadinessCommand, error) {
	located, err := exec.LookPath("omp")
	if err != nil {
		return providerReadinessCommand{}, err
	}
	executable, identity, err := canonicalPipelineOMPExecutable(located)
	if err != nil {
		return providerReadinessCommand{}, err
	}
	normalized, err := normalizePipelineOMPEnvironment(env)
	if err != nil {
		return providerReadinessCommand{}, err
	}
	argv := append([]string{executable}, readinessStatusArgs[readinessProbeOMP]...)
	return providerReadinessCommand{Argv: argv, Env: normalized, Dir: dir, ompIdentity: &identity}, nil
}

// ompReadinessFamily is the model selector text before the first "/".
func ompReadinessFamily(model string) string {
	family, _, found := strings.Cut(strings.TrimSpace(model), "/")
	if !found {
		return ""
	}
	return family
}

func classifyOMPReadiness(evidence readinessEvidence, family string, env []string) providerReadinessResult {
	if evidence.Failure != "" {
		return unknownReadiness(evidence.Failure)
	}
	if evidence.ExitCode != 0 {
		return unknownReadiness(readinessReasonProbeFailed)
	}
	usage, parsed := parseOMPUsage(evidence.Stdout.Data)
	if !parsed {
		return unknownReadiness("unparsable")
	}
	usable := len(ompFamilyElements(usage["reports"], family)) +
		len(ompFamilyElements(usage["accountsWithoutUsage"], family))
	disabled := ompFamilyElements(usage["disabledCredentials"], family)
	state := ompDisabledState(disabled)
	switch {
	case usable > 0:
		result := providerReadinessResult{Status: providerReadinessReady}
		if len(disabled) > 0 {
			result.Warnings = []string{redactReadinessText(fmt.Sprintf(
				`omp %s: %d of %d accounts unusable (%s); run "omp usage --redact" for details`,
				family, len(disabled), usable+len(disabled), state))}
		}
		return result
	case len(disabled) > 0:
		return notReadyReadiness(state, ompReadinessRemedy(family, env))
	default:
		return unknownReadiness("no_account_evidence")
	}
}

// parseOMPUsage accepts only an object whose three account lists are JSON arrays.
func parseOMPUsage(stdout []byte) (map[string][]json.RawMessage, bool) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(stdout, &top); err != nil {
		return nil, false
	}
	usage := make(map[string][]json.RawMessage, len(ompUsageArrays))
	for _, key := range ompUsageArrays {
		var elements []json.RawMessage
		if err := json.Unmarshal(top[key], &elements); err != nil || elements == nil {
			return nil, false
		}
		usage[key] = elements
	}
	return usage, true
}

// ompFamilyElements keeps elements whose string field provider equals family.
func ompFamilyElements(elements []json.RawMessage, family string) []map[string]json.RawMessage {
	if family == "" {
		return nil
	}
	var matched []map[string]json.RawMessage
	for _, element := range elements {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(element, &fields); err != nil {
			continue
		}
		if provider, ok := readinessJSONString(fields["provider"]); ok && provider == family {
			matched = append(matched, fields)
		}
	}
	return matched
}

// ompDisabledState is auth_expired only when every disabled credential's
// failure text mentions expiry. omp names that text "cause" (CD-5 capture);
// "reason" is read only when no string cause is present.
func ompDisabledState(disabled []map[string]json.RawMessage) string {
	for _, fields := range disabled {
		cause, ok := readinessJSONString(fields["cause"])
		if !ok {
			cause, _ = readinessJSONString(fields["reason"])
		}
		if !strings.Contains(strings.ToLower(cause), "expired") {
			return "account_disabled"
		}
	}
	return "auth_expired"
}

func ompReadinessRemedy(family string, env []string) string {
	remedy := "omp login " + family
	if readinessEnvValue(env, "PI_CODING_AGENT_DIR") != "" {
		remedy += ompAgentDirRemedyNote
	}
	return remedy
}

// readinessJSONString decodes raw only when it is a JSON string.
func readinessJSONString(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	var value string
	if len(trimmed) == 0 || trimmed[0] != '"' || json.Unmarshal(trimmed, &value) != nil {
		return "", false
	}
	return value, true
}
