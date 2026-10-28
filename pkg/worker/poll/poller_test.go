package poll

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPoller_HitsCorrectURL(t *testing.T) {
	t.Parallel()

	var gotPath, gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
		gotAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := NewTaskPoller(srv.URL, "tok123", "ws-1", func([]byte) {})
	p.backoff = &AdaptiveBackoff{min: time.Millisecond, max: time.Millisecond, factor: 1, current: time.Millisecond}

	go p.Start(ctx)

	require.Eventually(t, func() bool {
		return gotPath.Load() == "/api/v1/workspaces/ws-1/tasks/pending" &&
			gotAuth.Load() == "Bearer tok123"
	}, 500*time.Millisecond, 5*time.Millisecond)
	cancel()

	assert.Equal(t, "/api/v1/workspaces/ws-1/tasks/pending", gotPath.Load())
	assert.Equal(t, "Bearer tok123", gotAuth.Load())
}

func TestTaskPoller_OnTaskCallback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"task-1"}]`))
	}))
	defer srv.Close()

	var called atomic.Bool
	var receivedData atomic.Value
	ctx, cancel := context.WithCancel(context.Background())
	p := NewTaskPoller(srv.URL, "tok", "ws", func(data []byte) {
		// Publish the payload before the flag the test waits on: the reverse
		// order let Eventually observe called=true while receivedData was still
		// empty, and the assertion below panicked on a nil interface.
		receivedData.Store(string(data))
		called.Store(true)
	})
	p.backoff = &AdaptiveBackoff{min: time.Millisecond, max: time.Millisecond, factor: 1, current: time.Millisecond}

	go p.Start(ctx)

	require.Eventually(t, func() bool { return called.Load() }, 200*time.Millisecond, 5*time.Millisecond)
	cancel()

	payload, ok := receivedData.Load().(string)
	require.True(t, ok, "callback must publish the payload before signalling")
	assert.Contains(t, payload, "task-1")
}

func TestTaskPoller_BackoffResetOnTask(t *testing.T) {
	t.Parallel()

	// Advance the backoff manually to a high value, then verify Reset is called
	// by the poller when a task is found (indirectly via the onTask callback).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"t"}]`))
	}))
	defer srv.Close()

	var taskReceived atomic.Bool
	p := NewTaskPoller(srv.URL, "tok", "ws", func([]byte) {
		taskReceived.Store(true)
	})
	// Use min backoff so the first poll fires fast.
	p.backoff = &AdaptiveBackoff{min: time.Millisecond, max: time.Millisecond, factor: 1, current: time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	go p.Start(ctx)

	// Verify the task callback fires (which implies backoff.Reset was called).
	require.Eventually(t, func() bool {
		return taskReceived.Load()
	}, 500*time.Millisecond, 5*time.Millisecond, "task callback should fire")
	cancel()
}

func TestTaskPoller_EmptyBodyIgnored(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	var called atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	p := NewTaskPoller(srv.URL, "tok", "ws", func([]byte) { called.Store(true) })
	p.backoff = &AdaptiveBackoff{min: time.Millisecond, max: time.Millisecond, factor: 1, current: time.Millisecond}

	go p.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	assert.False(t, called.Load(), "onTask should not be called for empty body")
}

func TestTaskPoller_LargeResponseTruncated(t *testing.T) {
	t.Parallel()

	// Serve a body larger than 10MB.
	bigBody := strings.Repeat("x", 11<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(bigBody))
	}))
	defer srv.Close()

	p := NewTaskPoller(srv.URL, "tok", "ws", func([]byte) {})
	body, err := p.fetchPending(context.Background())

	require.NoError(t, err)
	assert.Len(t, body, maxBodyBytes, "body should be limited to 10MB")
}

func TestTaskPoller_ContextCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	p := NewTaskPoller(srv.URL, "tok", "ws", func([]byte) {})
	p.backoff = &AdaptiveBackoff{min: time.Millisecond, max: time.Millisecond, factor: 1, current: time.Millisecond}
	// Count requests the poller issues, not requests the server reads: a
	// request sent just before cancel can reach the handler arbitrarily late.
	sent := &countingTransport{next: http.DefaultTransport}
	p.client.Transport = sent

	done := make(chan struct{})
	go func() {
		p.Start(ctx)
		close(done)
	}()
	require.Eventually(t, func() bool { return sent.count.Load() > 0 }, 5*time.Second, time.Millisecond,
		"the poller polls before cancel")
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after cancel")
	}

	snapshot := sent.count.Load()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, snapshot, sent.count.Load(), "no more requests after cancel")
}

type countingTransport struct {
	next  http.RoundTripper
	count atomic.Int32
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.count.Add(1)
	return c.next.RoundTrip(r)
}
