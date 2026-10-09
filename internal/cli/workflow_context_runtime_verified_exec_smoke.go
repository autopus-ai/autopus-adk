package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const workflowContextVerifiedExecSmokeSchemaV1 = "workflow-context-verified-exec-smoke/v1"

type workflowContextVerifiedExecSmokeV1 struct {
	SchemaVersion       string `json:"schema_version"`
	OMPVersion          string `json:"omp_version"`
	OMPExecutableSHA256 string `json:"omp_executable_sha256"`
	RPCReady            bool   `json:"rpc_ready"`
	ProviderCalls       int64  `json:"provider_calls"`
	UIDIsolated         bool   `json:"uid_isolated"`
	EffectiveUser       string `json:"effective_user"`
}

func newWorkflowContextVerifiedExecSmokeCmd() *cobra.Command {
	var executable, canaryRoot, format string
	cmd := &cobra.Command{
		Use:           "verified-exec-smoke",
		Short:         "Run the authority-free verified OMP executable smoke gate",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.ToLower(strings.TrimSpace(format)) != "json" {
				return errors.New("workflow context-runtime verified-exec-smoke requires --format json")
			}
			output, err := runWorkflowContextVerifiedExecSmoke(cmd, executable, canaryRoot)
			if err != nil {
				return fmt.Errorf("workflow context-runtime verified-exec-smoke: %w", err)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
		},
	}
	cmd.Flags().StringVar(&executable, "omp-executable", "", "Absolute non-symlink OMP executable path")
	cmd.Flags().StringVar(&canaryRoot, "canary-root", "", "Isolated nobody-owned release canary root")
	cmd.Flags().StringVar(&format, "format", "json", "Output format (json)")
	return cmd
}

func runWorkflowContextVerifiedExecSmoke(
	cmd *cobra.Command,
	executable string,
	canaryRoot string,
) (workflowContextVerifiedExecSmokeV1, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return workflowContextVerifiedExecSmokeV1{}, errors.New("verified OMP execution is unsupported")
	}
	expectedUID, err := workflowContextReleaseCanaryExpectedUID()
	if err != nil {
		return workflowContextVerifiedExecSmokeV1{}, err
	}
	if err := validateWorkflowContextReleaseCanaryIsolation(canaryRoot, executable, expectedUID, 0); err != nil {
		return workflowContextVerifiedExecSmokeV1{}, err
	}
	canonical, identity, err := canonicalPipelineOMPExecutable(executable)
	if err != nil || canonical != executable {
		return workflowContextVerifiedExecSmokeV1{}, errors.New("OMP executable identity is unsafe")
	}
	config := pipelineOMPBackendConfig{Executable: canonical, executableID: identity}
	ompVersion, err := probePipelineOMPActiveVersion(cmd.Context(), config, pipelineOMPActiveCurrentRuntimeHooks{})
	if err != nil {
		return workflowContextVerifiedExecSmokeV1{}, fmt.Errorf("authority-free OMP version probe failed: %w", err)
	}
	if err := verifyPipelineOMPExecutable(canonical, identity); err != nil {
		return workflowContextVerifiedExecSmokeV1{}, err
	}
	providerCalls, err := runWorkflowContextVerifiedExecRPCSmoke(cmd.Context(), canonical, identity)
	if err != nil {
		return workflowContextVerifiedExecSmokeV1{}, fmt.Errorf("authority-free OMP RPC readiness probe failed: %w", err)
	}
	if providerCalls != 0 {
		return workflowContextVerifiedExecSmokeV1{}, errors.New("OMP RPC readiness probe reached the provider endpoint")
	}
	return workflowContextVerifiedExecSmokeV1{
		SchemaVersion: workflowContextVerifiedExecSmokeSchemaV1,
		OMPVersion:    ompVersion, OMPExecutableSHA256: fmt.Sprintf("sha256:%x", identity.digest[:]),
		RPCReady: true, ProviderCalls: providerCalls, UIDIsolated: true, EffectiveUser: "nobody",
	}, nil
}
