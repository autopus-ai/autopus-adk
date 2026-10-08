//go:build !race

package secretscan

// corpusScale divides the random corpus sizes; a plain run checks them whole.
const corpusScale = 1

// slowdown multiplies the timing bounds; a plain run keeps them as written.
const slowdown = 1
