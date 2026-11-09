package testscenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateExec_ServiceCases(t *testing.T) {
	t.Parallel()
	ok := 200
	base := func() Exec {
		return Exec{Build: []string{"go", "build", "-o", "$BIN/api", "."}, Argv: []string{"$BIN/api", "--port", "$PORT"},
			Ready: "/healthz", Requests: []HTTPRequest{{Method: "get", Path: "/items", Expect: HTTPExpect{Status: &ok}}}}
	}
	x := base()
	assert.NoError(t, ValidateExec(&x))

	for name, tc := range map[string]struct {
		mutate func(*Exec)
		want   string
	}{
		"url instead of path": {func(x *Exec) { x.Requests[0].Path = "http://example.com/items" }, "must be a path on the case's own server"},
		"unknown method":      {func(x *Exec) { x.Requests[0].Method = "TRACE" }, "method"},
		"empty expect":        {func(x *Exec) { x.Requests[0].Expect = HTTPExpect{} }, "requests[0].expect must state"},
		"top-level expect":    {func(x *Exec) { x.Expect.Stdout = new(string) }, "a case with requests states what it expects on each request"},
		"stdin":               {func(x *Exec) { x.Stdin = "x" }, "send input to a service as a request body"},
		"ready not a path":    {func(x *Exec) { x.Ready = "healthz" }, "must be a path"},
		"contradiction": {func(x *Exec) {
			x.Requests[0].Expect.BodyContains = []string{"apple pie"}
			x.Requests[0].Expect.BodyNotContains = []string{"apple"}
		}, "no response satisfies both"},
		"system binary": {func(x *Exec) { x.Argv = []string{"nc", "-l"} }, "exec.argv[0]"},
	} {
		x := base()
		tc.mutate(&x)
		err := ValidateExec(&x)
		if assert.Error(t, err, name) {
			assert.Contains(t, err.Error(), tc.want, name)
		}
	}

	oneShot := Exec{Argv: []string{"./tool"}, Ready: "/x", Expect: ExecExpect{ExitCode: &ok}}
	assert.ErrorContains(t, ValidateExec(&oneShot), "exec.ready only applies to a case with requests")
}
