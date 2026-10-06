package cli

import "fmt"

// readOnlyViolationKind classifies why the read-only provider policy rejected
// a provider.
type readOnlyViolationKind int

const (
	readOnlyUnsupportedArgv readOnlyViolationKind = iota
	readOnlyUnsupportedProvider
	readOnlyNativeBinary
	readOnlyUnsafeArgv
	readOnlyIncompleteArgv
	readOnlyUnsafeValue
	readOnlyUnsupportedSchemaFlag
)

// Provider config fields a violation points at, named like the keys under
// orchestra.providers.<name> in autopus.yaml.
const (
	readOnlyFieldBinary     = "binary"
	readOnlyFieldArgs       = "args"
	readOnlyFieldPaneArgs   = "pane_args"
	readOnlyFieldSchemaFlag = "subprocess.schema_flag"
)

// readOnlyPolicyViolation is the typed error of the read-only provider policy.
// Error keeps the historical text so plan and brainstorm output stays
// byte-identical; callers that name a config key and a remedy read the fields.
type readOnlyPolicyViolation struct {
	Provider string
	// Field is the offending config field; empty for an unsupported provider.
	Field string
	// Item is the offending argv item, value flag, required binary, schema
	// flag, or provider name, depending on Kind.
	Item string
	// Value is the rejected flag value, or the configured binary for a
	// native-binary violation.
	Value string
	// Inline reports that Value was attached to Item with "=".
	Inline bool
	Kind   readOnlyViolationKind
}

func (v *readOnlyPolicyViolation) Error() string {
	switch v.Kind {
	case readOnlyUnsupportedProvider:
		return "read-only provider policy: " + v.Reason()
	case readOnlyUnsupportedSchemaFlag:
		return fmt.Sprintf("read-only provider policy: provider %q has %s", v.Provider, v.Reason())
	default:
		return fmt.Sprintf("read-only provider policy: provider %q %s", v.Provider, v.Reason())
	}
}

// Reason renders why the provider was rejected, without the policy prefix.
// Provider and native binary names are quoted as is; config argv items go
// through readOnlyViolationArgv.
func (v *readOnlyPolicyViolation) Reason() string {
	switch v.Kind {
	case readOnlyUnsupportedProvider:
		return fmt.Sprintf("unsupported provider %q", v.Item)
	case readOnlyNativeBinary:
		return fmt.Sprintf("requires native binary %q", v.Item)
	case readOnlyUnsafeArgv:
		return fmt.Sprintf("contains unsafe argv %q", readOnlyViolationArgv(v.Item))
	case readOnlyIncompleteArgv:
		return fmt.Sprintf("has incomplete argv %q", readOnlyViolationArgv(v.Item))
	case readOnlyUnsafeValue:
		return fmt.Sprintf("contains unsafe value for %q", readOnlyViolationArgv(v.Item))
	case readOnlyUnsupportedSchemaFlag:
		return fmt.Sprintf("unsupported schema flag %q", readOnlyViolationArgv(v.Item))
	default:
		return fmt.Sprintf("contains unsupported argv %q", readOnlyViolationArgv(v.Item))
	}
}

// readOnlyViolationArgvRunes bounds a config argv item or value quoted in a
// violation message.
const readOnlyViolationArgvRunes = 64

// readOnlyViolationArgv renders a config argv item or value for stderr and
// doctor output. Config argv may carry credentials (CWE-532), so it is
// redacted first, which keeps a token whole for its pattern, and then cut.
func readOnlyViolationArgv(text string) string {
	runes := []rune(redactReadinessText(text))
	if len(runes) > readOnlyViolationArgvRunes {
		runes = runes[:readOnlyViolationArgvRunes]
	}
	return string(runes)
}
