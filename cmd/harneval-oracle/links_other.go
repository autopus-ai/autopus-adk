//go:build !unix

package main

import "os"

// openFlags is a plain read: without a link count no output is ever accepted.
const openFlags = os.O_RDONLY

// linkCount is unknown off Unix, so every output file is rejected and the
// harness fails closed; the signed lane runs on macOS only.
func linkCount(os.FileInfo) (uint64, bool) {
	return 0, false
}
