package orchestra

import "testing"

// S3: with no FastFailPatterns override, resolved fast-fail rules reproduce the
// exact legacy reason strings (INV-003 default-equivalence).
func TestResolveFastFailRules_DefaultEquivalence(t *testing.T) {
	rules := resolveFastFailRules(nil) // override unset

	cases := []struct {
		input string
		want  string
	}{
		{"stream error: MODEL_CAPACITY_EXHAUSTED", "provider capacity exhausted"},
		{"RESOURCE_EXHAUSTED", "provider resource exhausted"},
		{"No capacity available for model", "provider model capacity unavailable"},
		{"RateLimitExceeded", "provider rate limit exceeded"},
		{"some unrelated output", ""},
	}
	for _, c := range cases {
		if got := matchFastFailRules(c.input, rules); got != c.want {
			t.Fatalf("matchFastFailRules(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// detectProviderFastFail (legacy entry point) must keep returning the same reasons.
func TestDetectProviderFastFail_BackwardCompatible(t *testing.T) {
	if got := detectProviderFastFail("MODEL_CAPACITY_EXHAUSTED"); got != "provider capacity exhausted" {
		t.Fatalf("detectProviderFastFail capacity = %q", got)
	}
	if got := detectProviderFastFail("nothing here"); got != "" {
		t.Fatalf("detectProviderFastFail no-match = %q, want empty", got)
	}
}

// S4: a FastFailPatterns override changes the resolved reason for matching input.
func TestResolveFastFailRules_Override(t *testing.T) {
	override := []FastFailRule{{Substring: "my_custom_error", Reason: "custom stop"}}
	rules := resolveFastFailRules(override)

	if got := matchFastFailRules("prefix...my_custom_error...suffix", rules); got != "custom stop" {
		t.Fatalf("override match = %q, want %q", got, "custom stop")
	}
	// The default capacity rule must no longer apply under the override set.
	if got := matchFastFailRules("MODEL_CAPACITY_EXHAUSTED", rules); got != "" {
		t.Fatalf("override set must not match default rules, got %q", got)
	}
}

// The fast-fail buffer wired with a provider's override rules triggers on the
// custom substring (end-to-end through provider_runner wiring).
func TestFastFailBuffer_UsesProviderOverrideRules(t *testing.T) {
	detector := &fastFailDetector{}
	var triggered string
	rules := resolveFastFailRules([]FastFailRule{{Substring: "boom", Reason: "kaboom"}})
	buf := newFastFailBuffer(detector, rules, func(reason string) { triggered = reason })

	if _, err := buf.Write([]byte("everything is fine until BOOM happens")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if detector.Reason() != "kaboom" {
		t.Fatalf("detector reason = %q, want kaboom", detector.Reason())
	}
	if triggered != "kaboom" {
		t.Fatalf("onMatch reason = %q, want kaboom", triggered)
	}
}
