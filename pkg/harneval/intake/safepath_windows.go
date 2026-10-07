//go:build windows

package intake

import "errors"

// Intake, promote, and reject never write on Windows: directory fsync and the
// link-based publication have no portable equivalent there, so every write
// helper refuses with platform_unsupported. Reads, which the prune protection
// scan needs, use the shared os.Root helpers on every platform.

var errPlatformUnsupported = errors.New(ReasonPlatformUnsupported)

func writeSupported() error { return errPlatformUnsupported }

func (a *area) ensureDir(string) error { return errPlatformUnsupported }

func (a *area) createExclusive(string, []byte) error { return errPlatformUnsupported }

func (a *area) removeFile(string) error { return errPlatformUnsupported }

func (a *area) syncDir(string) error { return errPlatformUnsupported }
