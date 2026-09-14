package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/spf13/cobra"
)

const (
	contextIntegrityVerified = "verified"
	contextIntegrityMissing  = "missing"
	contextIntegrityFailed   = "failed"
)

type workflowBindingContextOptions struct {
	manifest            string
	root                string
	command             string
	specDir             string
	requiredDocuments   []string
	conditionalProfiles []string
}

func registerWorkflowBindingContextFlags(cmd *cobra.Command, opts *workflowBindingContextOptions) {
	cmd.Flags().StringVar(&opts.manifest, "context-manifest", "", "Verified required-context manifest JSON")
	cmd.Flags().StringVar(&opts.root, "context-root", "", "Project root for context verification (default: manifest directory)")
	cmd.Flags().StringVar(&opts.command, "context-command", "go", "Expected context command profile")
	cmd.Flags().StringVar(&opts.specDir, "context-spec-dir", "", "Expected root-relative SPEC directory")
	cmd.Flags().StringArrayVar(&opts.requiredDocuments, "context-required-document", nil, "Expected task-specific required document")
	cmd.Flags().StringArrayVar(&opts.conditionalProfiles, "context-conditional-profile", nil, "Expected declared conditional context profile")
}

func verifyWorkflowContextManifest(opts workflowBindingContextOptions) string {
	path := strings.TrimSpace(opts.manifest)
	if path == "" {
		return contextIntegrityMissing
	}
	data, err := readBoundedJSONFile(path)
	if err != nil {
		return contextIntegrityFailed
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt promptlayer.ContextDeliveryResult
	if decoder.Decode(&receipt) != nil || ensureBindingJSONEOF(decoder) != nil {
		return contextIntegrityFailed
	}
	expectedCommand := strings.ToLower(strings.TrimSpace(opts.command))
	expectedSpecDir := filepath.ToSlash(filepath.Clean(strings.TrimSpace(opts.specDir)))
	if expectedCommand == "" || strings.TrimSpace(opts.specDir) == "" ||
		receipt.Command != expectedCommand || receipt.SpecDir != expectedSpecDir {
		return contextIntegrityFailed
	}
	root := strings.TrimSpace(opts.root)
	if root == "" {
		root = filepath.Dir(path)
	}
	if promptlayer.VerifyContextDeliveryForOptions(promptlayer.ContextDeliveryOptions{
		Root: root, Command: expectedCommand, SpecDir: expectedSpecDir,
		RequiredReferences:   opts.requiredDocuments,
		ConditionalProfiles:  contextProfileNames(opts.conditionalProfiles),
		ExplicitArchitecture: true,
	}, receipt) != nil {
		return contextIntegrityFailed
	}
	return contextIntegrityVerified
}
