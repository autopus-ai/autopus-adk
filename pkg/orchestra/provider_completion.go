package orchestra

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"
)

// providerCompletionGrace is how long a provider may keep running after its
// terminal result envelope has been read. agy 1.5.0 can finish its answer and
// then never exit (its language server stays up, SIGTERM is ignored), which
// used to hold a review until the full provider timeout and discard the
// answer. A normal exit lands well inside this window, so only a process
// that is genuinely stuck is reaped.
const providerCompletionGrace = 5 * time.Second

// errReapedAfterCompletion reports that the provider's process group was
// killed after its result was complete. It wraps errResultReady, so every
// caller that treats a ready result as success keeps doing so.
var errReapedAfterCompletion = fmt.Errorf("%w: reaped after completion", errResultReady)

// completionWatcher scans provider stdout line by line and closes Done once a
// complete terminal envelope line has been written.
type completionWatcher struct {
	kind    providerEnvelope
	mu      sync.Mutex
	partial []byte
	done    chan struct{}
	once    sync.Once
}

// newCompletionWatcher returns nil for providers without an envelope, whose
// completion is the process exit alone.
func newCompletionWatcher(kind providerEnvelope) *completionWatcher {
	if kind == envelopeNone {
		return nil
	}
	return &completionWatcher{kind: kind, done: make(chan struct{})}
}

// Write never fails, so it can sit beside the capture in an io.MultiWriter.
func (w *completionWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.partial = append(w.partial, p...)
	for {
		newline := bytes.IndexByte(w.partial, '\n')
		if newline < 0 {
			break
		}
		line := w.partial[:newline]
		if _, _, ok := terminalEnvelopeLine(w.kind, line); ok {
			w.once.Do(func() { close(w.done) })
		}
		w.partial = w.partial[newline+1:]
	}
	if len(w.partial) > fastFailBufferCap {
		// The capture keeps no more than this, so neither can the scan.
		w.partial = nil
	}
	if len(w.partial) == 0 {
		w.partial = nil
	}
	return len(p), nil
}

// Done is closed once the terminal envelope was seen; nil for a nil watcher,
// so a select on it never fires.
func (w *completionWatcher) Done() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.done
}

// syncBuffer is a bytes.Buffer safe to read while the exec copy goroutine
// may still write, which happens when a reaped process tree leaves Wait
// behind past providerWaitGracePeriod.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// markReaped records on the receipt that the provider was killed after its
// result was complete, so the evidence shows a reap and not a clean exit.
func (e *ProviderExecution) markReaped(waitErr error) {
	if e != nil && errors.Is(waitErr, errReapedAfterCompletion) {
		e.ReapedAfterCompletion = true
	}
}
