package execcheck

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// itemsServer stores names POSTed to /items and lists them on GET. The bug
// it may carry is the one a user of the API would see: a stored name read
// back upper-cased.
func itemsServer(bug bool) string {
	transform := "name"
	if bug {
		transform = "strings.ToUpper(name)"
	}
	return `package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

func main() {
	var mu sync.Mutex
	var items []string
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	http.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			name := strings.TrimSpace(string(body))
			items = append(items, ` + transform + `)
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Join(items, ","))
	})
	_ = http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), nil)
}
`
}

func serviceCase() testscenario.Exec {
	created, ok := 201, 200
	return testscenario.Exec{
		Build: []string{"go", "build", "-o", "$BIN/items", "."},
		Argv:  []string{"$BIN/items"},
		Ready: "/healthz",
		Requests: []testscenario.HTTPRequest{
			{Method: "POST", Path: "/items", Body: "apple", Expect: testscenario.HTTPExpect{Status: &created}},
			{Method: "GET", Path: "/items", Expect: testscenario.HTTPExpect{Status: &ok, BodyContains: []string{"apple"},
				Headers: map[string]string{"Content-Type": "text/plain"}}},
		},
	}
}

func serviceProject(t *testing.T, source string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module items\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644))
	return dir
}

// A POST read back by a GET passes on the correct server and, with the bug,
// fails as a product defect naming the request and the body it got.
func TestRun_ServiceCaseSendsRequestsInOrder(t *testing.T) {
	res, err := Run(context.Background(), serviceProject(t, itemsServer(false)), serviceCase())
	require.NoError(t, err)
	assert.True(t, res.Passed(), "%+v", res.Mismatches)

	project := serviceProject(t, itemsServer(true))
	res, err = Run(context.Background(), project, serviceCase())
	require.NoError(t, err)
	require.Len(t, res.Mismatches, 1)
	assert.Equal(t, "request 2 GET /items body_contains", res.Mismatches[0].Field)
	assert.Equal(t, "APPLE", res.Mismatches[0].Received)

	var report bytes.Buffer
	WriteReport(&report, "post-then-list", "AC-1", res)
	v := triage.Classify(triage.Input{
		JourneyID: "compiled-test-scenarios-intent-items-post-then-list-custom-command",
		Adapter:   "custom-command", Status: "failed", FailureText: report.String(), ProjectDir: project,
	})
	assert.Equal(t, triage.ClassProductDefect, v.Class, "%+v", v)
}

// A server that crashes on start is what its user sees; one that never
// listens on $PORT more likely means the case did not pass the port.
func TestRun_ServiceThatCannotStart(t *testing.T) {
	crash := strings.Replace(itemsServer(false), `_ = http.ListenAndServe(`, `panic("boom"); _ = http.ListenAndServe(`, 1)
	res, err := Run(context.Background(), serviceProject(t, crash), serviceCase())
	require.NoError(t, err)
	require.Len(t, res.Mismatches, 1)
	assert.Equal(t, "service", res.Mismatches[0].Field)
	assert.Contains(t, res.Mismatches[0].Received, "boom")

	deaf := strings.Replace(itemsServer(false), `"127.0.0.1:"+os.Getenv("PORT")`, `"127.0.0.1:0"+os.Getenv("UNUSED")`, 1)
	x := serviceCase()
	x.Timeout = "3s"
	res, err = Run(context.Background(), serviceProject(t, deaf), x)
	require.Error(t, err, "%+v", res)
	assert.Contains(t, err.Error(), "$PORT")
}
