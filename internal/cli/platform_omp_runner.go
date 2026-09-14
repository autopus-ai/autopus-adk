package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

type ompOperatorExecRunner struct {
	process *omp.OMPModelProbeProcess
	pinErr  error
}

func newOMPOperatorExecRunner() *ompOperatorExecRunner {
	process, err := omp.NewOMPInstalledModelProbeProcess("omp", ompModelDoctorProbeOutput)
	return &ompOperatorExecRunner{process: process, pinErr: err}
}

func (runner ompOperatorExecRunner) Run(
	ctx context.Context,
	executable string,
	args ...string,
) ([]byte, error) {
	if executable != "omp" || !safeOMPOperatorProbeArgs(args) {
		return nil, errors.New("unsafe OMP operator command")
	}
	if runner.pinErr != nil {
		return nil, runner.pinErr
	}
	return runner.process.Run(ctx, args...)
}

func (runner ompOperatorExecRunner) RunWithInput(
	ctx context.Context,
	executable string,
	input []byte,
	args ...string,
) ([]byte, error) {
	if executable != "omp" || !bytes.Equal(input, []byte(`{"id":"autopus-model-state","type":"get_state"}`+"\n")) ||
		!omp.SafeOMPModelSelectorRPCArgs(args) {
		return nil, errors.New("unsafe OMP operator selector command")
	}
	if runner.pinErr != nil {
		return nil, runner.pinErr
	}
	return runner.process.RunInput(ctx, input, args...)
}

func safeOMPOperatorProbeArgs(args []string) bool {
	joined := strings.Join(args, " ")
	if joined == "--version" || joined == "models --json --no-extensions" ||
		joined == "config list --json" {
		return true
	}
	keyIndex := 2
	if len(args) == 6 && args[0] == "--config" && filepath.IsAbs(args[1]) &&
		!strings.ContainsAny(args[1], "\x00\r\n") {
		keyIndex = 4
	}
	if len(args) != keyIndex+2 || args[keyIndex-2] != "config" ||
		args[keyIndex-1] != "get" || args[keyIndex+1] != "--json" {
		return false
	}
	allowed := map[string]bool{
		config.OMPNativeAgentModelOverridesKey: true,
		"retry.fallbackChains":                 true,
		"retry.modelFallback":                  true,
		"tools.approvalMode":                   true,
		"task.isolation.mode":                  true,
	}
	return allowed[args[keyIndex]]
}
