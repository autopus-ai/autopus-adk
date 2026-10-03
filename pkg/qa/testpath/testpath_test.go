package testpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Review finding 6: the diff guard and triage share one answer to "is this a
// test", so a file the guard protects is the file triage blames.
func TestIsTestPath_NamesAndDirectories(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want bool
	}{
		{"pkg/auth/login_test.go", true},
		{"e2e/login.spec.ts", true},
		{"src/form.test.tsx", true},
		{"lib/user_spec.rb", true},
		{"app/test_login.py", true},
		{"app/login_test.py", true},
		{"server/handler_test.ts", true},
		{"web/cart_spec.js", true},
		{"web/__tests__/form.tsx", true},
		{"tests/helpers/session.ts", true},
		{"test/fixtures/data.json", true},
		{"e2e/support/commands.ts", true},
		{"spec/support/helpers.rb", true},
		{"pkg/a/b/__tests__/deep/x.ts", true},
		{"src/app.ts", false},
		{"src/contest.py", false},
		{"src/testing.py", false},
		{"src/openapi_spec.go", false},
		{"src/specs/openapi.yaml", false},
		{"attestation/report.go", false},
		{"qa/journeys/login.ts", false},
		{".autopus/specs/SPEC-LOGIN-001/spec.md", false},
		{"README.md", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, IsTestPath(tc.path), tc.path)
	}
}
