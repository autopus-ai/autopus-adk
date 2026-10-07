package secretscan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/evidence"
	"github.com/insajin/autopus-adk/pkg/worker/security"
)

// TestDetectorTable_EqualsUpstreamSources pins the table to both upstream
// detectors: a regex added, removed, reordered, or edited on either side
// fails here instead of drifting silently.
func TestDetectorTable_EqualsUpstreamSources(t *testing.T) {
	t.Parallel()
	want := append(evidence.SecretDetectorSources(), security.DefaultPatternSources()...)
	got := make([]string, 0, len(detectors))
	for _, d := range detectors {
		got = append(got, d.re.String())
	}
	require.Len(t, want, 25, "14 qa evidence regexes plus 11 worker default patterns")
	assert.Equal(t, want, got)
}

// TestRedact_CoversQAAndWorkerRedactions is the superset check: every byte
// that evidence.RedactText or the worker SecretScanner replaces in an S11
// fixture is also replaced by Redact. Coverage is read from each redactor's
// real output, independent of this package's span selection.
func TestRedact_CoversQAAndWorkerRedactions(t *testing.T) {
	t.Parallel()
	ours := []string{PlaceholderSecret, PlaceholderPrivateNote, PlaceholderUser}
	worker := security.NewSecretScanner()
	for _, fx := range s11Fixtures() {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			redacted, _ := Redact(fx.in)
			covered := replacedMask(t, fx.in, redacted, ours)
			upstream := map[string][]bool{
				"qa evidence":    replacedMask(t, fx.in, evidence.RedactText(fx.in), ours),
				"worker scanner": replacedMask(t, fx.in, worker.Scan(fx.in), []string{"***REDACTED***"}),
			}
			for name, mask := range upstream {
				for i, replaced := range mask {
					if replaced && !covered[i] {
						t.Fatalf("%s replaces byte %d of %q but Redact keeps it: %q", name, i, fx.in, redacted)
					}
				}
			}
		})
	}
}

// replacedMask aligns the literal runs a redactor kept against original and
// marks the original bytes it replaced with a placeholder.
func replacedMask(t *testing.T, original, redacted string, placeholders []string) []bool {
	t.Helper()
	literals := keptLiterals(redacted, placeholders)
	mask := make([]bool, len(original))
	if len(literals) == 1 {
		require.Equal(t, original, redacted, "redactor changed text without a placeholder")
		return mask
	}
	positions, ok := alignLiterals(original, literals, 0, 0)
	require.True(t, ok, "cannot align %q against %q", redacted, original)
	for i := 1; i < len(literals); i++ {
		for b := positions[i-1] + len(literals[i-1]); b < positions[i]; b++ {
			mask[b] = true
		}
	}
	return mask
}

// keptLiterals splits redacted around placeholders; consecutive placeholders
// collapse into one gap, so n gaps always yield n+1 literals.
func keptLiterals(redacted string, placeholders []string) []string {
	literals := []string{""}
	for i := 0; i < len(redacted); {
		matched := ""
		for _, p := range placeholders {
			if strings.HasPrefix(redacted[i:], p) {
				matched = p
				break
			}
		}
		if matched == "" {
			literals[len(literals)-1] += redacted[i : i+1]
			i++
			continue
		}
		if last := literals[len(literals)-1]; last != "" || len(literals) == 1 {
			literals = append(literals, "")
		}
		i += len(matched)
	}
	return literals
}

// alignLiterals places literals[idx:] in original from offset at, the first
// as a prefix, the last as a suffix, and each one after at least one replaced
// byte. It backtracks so a repeated literal cannot misalign the rest.
func alignLiterals(original string, literals []string, idx, at int) ([]int, bool) {
	lit := literals[idx]
	if idx == 0 {
		if !strings.HasPrefix(original, lit) {
			return nil, false
		}
		rest, ok := alignLiterals(original, literals, 1, len(lit))
		return append([]int{0}, rest...), ok
	}
	if idx == len(literals)-1 {
		pos := len(original) - len(lit)
		if pos <= at || original[pos:] != lit {
			return nil, false
		}
		return []int{pos}, true
	}
	for pos := at + 1; pos+len(lit) <= len(original); pos++ {
		if original[pos:pos+len(lit)] != lit {
			continue
		}
		if rest, ok := alignLiterals(original, literals, idx+1, pos+len(lit)); ok {
			return append([]int{pos}, rest...), true
		}
	}
	return nil, false
}
