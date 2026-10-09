//go:build !unix

package secretscan

import "time"

var processStart = time.Now()

// processTime falls back to wall time where getrusage is unavailable.
func processTime() time.Duration { return time.Since(processStart) }
