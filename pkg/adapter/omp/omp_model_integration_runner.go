package omp

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

type ompModelIntegrationExecRunner struct {
	process *OMPModelProbeProcess
	pinErr  error
}

func newOMPModelIntegrationExecRunner() *ompModelIntegrationExecRunner {
	process, err := NewOMPInstalledModelProbeProcess(cliBinary, defaultOMPModelProbeOutput)
	return &ompModelIntegrationExecRunner{process: process, pinErr: err}
}

func (r ompModelIntegrationExecRunner) Run(
	ctx context.Context,
	executable string,
	args ...string,
) ([]byte, error) {
	if executable != cliBinary || !safeOMPModelIntegrationArgs(args) {
		return nil, fmt.Errorf("unsafe OMP model metadata command")
	}
	if r.pinErr != nil {
		return nil, r.pinErr
	}
	return r.process.Run(ctx, args...)
}

func (r ompModelIntegrationExecRunner) RunWithInput(
	ctx context.Context,
	executable string,
	input []byte,
	args ...string,
) ([]byte, error) {
	if executable != cliBinary || !bytes.Equal(input, ompModelRPCStateRequest) ||
		!SafeOMPModelSelectorRPCArgs(args) {
		return nil, fmt.Errorf("unsafe OMP model selector command")
	}
	if r.pinErr != nil {
		return nil, r.pinErr
	}
	return r.process.RunInput(ctx, input, args...)
}

func safeOMPModelIntegrationArgs(args []string) bool {
	joined := strings.Join(args, " ")
	if joined == "--version" || joined == "models --json --no-extensions" {
		return true
	}
	keyIndex := 2
	if len(args) == 6 && args[0] == "--config" &&
		args[1] != "" && !strings.ContainsAny(args[1], "\x00\r\n") {
		keyIndex = 4
	}
	if len(args) != keyIndex+2 || args[keyIndex-2] != "config" ||
		args[keyIndex-1] != "get" || args[keyIndex+1] != "--json" {
		return false
	}
	_, allowed := ompRoutingSettingAllowlist[args[keyIndex]]
	return allowed
}
