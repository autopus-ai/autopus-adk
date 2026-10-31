//go:build darwin

package cli

import (
	"context"
	"os"
	"os/exec"
)

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-OMP-004: Darwin build-selected verified-command state shared by constructors, sandbox validation, and startup.
// @AX:REASON [AUTO]: ptrace and inherited-path execution depend on this common type preserving identity, gate, and lifecycle callback fields.
type pipelineOMPVerifiedExecCommand struct {
	cmd                       *exec.Cmd
	parentFD                  *os.File
	expected                  pipelineOMPExecutableIdentity
	gateContext               context.Context
	afterFirstDarwinStop      func()
	afterInheritedDarwinStart func()
	inheritedDarwinPath       bool
	inheritedDarwinPrivate    bool
}
