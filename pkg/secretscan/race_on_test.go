//go:build race

package secretscan

// corpusScale divides the random corpus sizes. The race detector slows the
// regex engine about fifteenfold and these single-goroutine properties gain
// nothing from it, so a race run checks a tenth of the corpus.
const corpusScale = 10

// slowdown multiplies the timing bounds. The race detector slowed Redact of
// private key headers about 23-fold.
const slowdown = 25
