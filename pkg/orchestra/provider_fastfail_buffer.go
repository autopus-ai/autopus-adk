package orchestra

import (
	"bytes"
	"sync"
	"time"
)

type fastFailDetector struct {
	mu     sync.Mutex
	reason string
	once   sync.Once
}

func (d *fastFailDetector) Trigger(reason string, terminate func(string)) {
	if reason == "" {
		return
	}
	d.once.Do(func() {
		d.mu.Lock()
		d.reason = reason
		d.mu.Unlock()
		terminate(reason)
	})
}

func (d *fastFailDetector) Reason() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reason
}

// fastFailBufferCap bounds every provider stream without MaxOutputBytes, so
// a runaway provider cannot grow the capture without limit.
const fastFailBufferCap = 64 << 20

type fastFailBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	lastWrite time.Time
	detector  *fastFailDetector
	rules     []FastFailRule
	onMatch   func(string)
	limit     int // bytes kept; zero uses fastFailBufferCap
}

func newFastFailBuffer(detector *fastFailDetector, rules []FastFailRule, onMatch func(string)) *fastFailBuffer {
	return &fastFailBuffer{
		detector: detector,
		rules:    rules,
		onMatch:  onMatch,
	}
}

// Write keeps the head of the stream up to the limit and drains the rest
// without failing, so the provider runs to its end; a write that adds no
// byte skips the fast-fail scan, whose input then cannot change.
func (b *fastFailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.lastWrite = time.Now()
	limit := b.limit
	if limit <= 0 {
		limit = fastFailBufferCap
	}
	room := limit - b.buf.Len()
	if room <= 0 {
		b.mu.Unlock()
		return len(p), nil
	}
	b.buf.Write(p[:min(room, len(p))])
	snapshot := b.buf.String()
	b.mu.Unlock()

	if reason := matchFastFailRules(snapshot, b.rules); reason != "" {
		b.detector.Trigger(reason, b.onMatch)
	}
	return len(p), nil
}

func (b *fastFailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
