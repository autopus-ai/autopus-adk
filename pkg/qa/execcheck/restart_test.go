package execcheck

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// notesServer stores POSTed notes either in memory or in a file in its work
// directory, the difference a restart step is there to tell apart.
func notesServer(persist bool) string {
	store := `var notes []string`
	load := ``
	save := ``
	if persist {
		load = `if data, err := os.ReadFile("notes.txt"); err == nil && len(data) > 0 { notes = strings.Split(strings.TrimSpace(string(data)), "\n") }`
		save = `_ = os.WriteFile("notes.txt", []byte(strings.Join(notes, "\n")+"\n"), 0o600)`
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
	` + store + `
	` + load + `
	http.HandleFunc("/notes", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			notes = append(notes, strings.TrimSpace(string(body)))
			` + save + `
			w.WriteHeader(http.StatusCreated)
			return
		}
		fmt.Fprint(w, strings.Join(notes, ","))
	})
	_ = http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), nil)
}
`
}

func restartCase() testscenario.Exec {
	created, ok := 201, 200
	return testscenario.Exec{
		Build: []string{"go", "build", "-o", "$BIN/notes", "."},
		Argv:  []string{"$BIN/notes"},
		Requests: []testscenario.HTTPRequest{
			{Method: "POST", Path: "/notes", Body: "apple", Expect: testscenario.HTTPExpect{Status: &created}},
			{Restart: true},
			{Method: "GET", Path: "/notes", Expect: testscenario.HTTPExpect{Status: &ok, BodyContains: []string{"apple"}}},
		},
	}
}

// A note written before a restart is read back after it from a server that
// keeps it, and missing from one that does not, which is a product verdict.
func TestRun_ServiceRestartStep(t *testing.T) {
	res, err := Run(context.Background(), serviceProject(t, notesServer(true)), restartCase())
	require.NoError(t, err)
	assert.True(t, res.Passed(), "%+v", res.Mismatches)
	assert.Contains(t, res.Stdout, "step 2: restarted the server")

	res, err = Run(context.Background(), serviceProject(t, notesServer(false)), restartCase())
	require.NoError(t, err)
	require.Len(t, res.Mismatches, 1)
	assert.Equal(t, "request 3 GET /notes body_contains", res.Mismatches[0].Field)
	assert.Empty(t, strings.TrimSpace(res.Mismatches[0].Received))
}
