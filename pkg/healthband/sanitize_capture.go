package healthband

// Capture buffers for untrusted subprocess output (Untrusted Input Contract
// item 1). Both are io.Writers for exec.Cmd.Stdout that never fail a write,
// so the subprocess is drained to its end however much it prints. Each keeps
// one byte beyond its limit to tell whether the kept window meets a line
// boundary.

// TailBuffer keeps the last limit bytes written to it.
type TailBuffer struct {
	limit int
	buf   []byte
	total int
}

// NewTailBuffer returns a TailBuffer for the last limit bytes.
func NewTailBuffer(limit int) *TailBuffer { return &TailBuffer{limit: max(limit, 0)} }

// Write keeps at most twice the window in memory and never fails.
func (b *TailBuffer) Write(p []byte) (int, error) {
	b.total += len(p)
	b.buf = append(b.buf, p...)
	if keep := b.limit + 1; len(b.buf) > 2*keep {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-keep:]...)
	}
	return len(p), nil
}

// Captured returns the kept tail aligned forward to a line boundary and
// whether any byte was dropped (size_cap).
func (b *TailBuffer) Captured() (string, bool) {
	if b.total <= b.limit {
		return string(b.buf), false
	}
	window := string(b.buf[len(b.buf)-b.limit-1:])
	return window[alignTailStart(window, 1):], true
}

// HeadBuffer keeps the first limit bytes written to it.
type HeadBuffer struct {
	limit int
	buf   []byte
}

// NewHeadBuffer returns a HeadBuffer for the first limit bytes.
func NewHeadBuffer(limit int) *HeadBuffer { return &HeadBuffer{limit: max(limit, 0)} }

// Write keeps the first limit+1 bytes and discards the rest without failing.
func (b *HeadBuffer) Write(p []byte) (int, error) {
	if room := b.limit + 1 - len(b.buf); room > 0 {
		b.buf = append(b.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

// Captured returns the kept head aligned back to a line boundary, so a token
// split at the limit cannot leak a prefix, and whether any byte was dropped.
func (b *HeadBuffer) Captured() (string, bool) {
	if len(b.buf) <= b.limit {
		return string(b.buf), false
	}
	text := string(b.buf)
	return text[:alignHeadEnd(text, b.limit)], true
}
