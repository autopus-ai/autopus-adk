package workflow

// This file guards the JS-injection trust boundary (REQ-011, S6). Phase model
// and effort strings are interpolated into the generated workflow JS, so they
// are validated against closed whitelists at the parse boundary. Anything
// outside the whitelist is rejected fail-closed before it can reach the JS
// surface.

// safeAgentModels is the closed set of agent model identifiers allowed to be
// interpolated into generated workflow JS. Extend deliberately; never widen to
// free-form input.
var safeAgentModels = map[string]bool{
	"claude-fable-5-1":  true,
	"claude-opus-5-5":   true,
	"claude-opus-5":     true,
	"claude-opus-4-8":   true,
	"claude-opus-4-7":   true,
	"claude-sonnet-5":   true,
	"claude-sonnet-4-6": true,
	"claude-haiku-4-5":  true,
	"claude-fable-5":    true,
	"fable":             true,
	"best":              true,
}

// isSafeAgentModel reports whether m is a whitelisted model. The empty string is
// allowed because deterministic gate phases carry no model.
func isSafeAgentModel(m string) bool {
	return m == "" || safeAgentModels[m]
}

// safeEfforts is the closed set of effort tiers allowed in generated JS.
var safeEfforts = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

// isSafeEffort reports whether e is a whitelisted effort tier. The empty string
// is allowed because phases without an agent carry no effort.
func isSafeEffort(e string) bool {
	return e == "" || safeEfforts[e]
}

// isSafeResultType reports whether rt is a whitelisted verdict_source. The
// result_type is interpolated into a single-line comment in the generated
// workflow JS, so a newline-bearing value could terminate the comment and emit
// an executable statement. Restrict it to the closed set; the empty string is
// allowed because non-gate phases carry no verdict source.
func isSafeResultType(rt string) bool {
	return rt == "" || rt == VerdictSourceExitCode
}
