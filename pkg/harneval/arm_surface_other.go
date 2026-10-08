//go:build !darwin

package harneval

import (
	"context"
	"errors"
)

// armSurfaceDigest refuses: the driver build and run need macOS sandbox-exec.
func armSurfaceDigest(context.Context, string, string, Pins, ArmSurfaceOptions) (string, error) {
	return "", errors.New("the surface driver build and run need macOS sandbox-exec")
}
