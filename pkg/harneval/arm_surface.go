package harneval

import (
	"context"
	"fmt"
	"regexp"
)

// SurfaceDriverPackage is the package the surface driver is built from in
// every arm tree; its only file is the checkout's main.go.
const SurfaceDriverPackage = "scripts/benchmarks/harness/surface_driver"

// linkableVersionPattern is a generator version the go tool passes to the
// linker as one -X value: no space or quote can split it.
var linkableVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]*$`)

// ArmSurfaceOptions are the inputs of ArmSurfaceDigest beside the tree. Env
// is the allowlisted environment the trusted steps (git, go env, go mod
// download) run with; PATH must find git and go. Proxy is the GOPROXY of the
// trusted module download; empty selects a file proxy over the local module
// cache, so the rebuild needs no network.
type ArmSurfaceOptions struct {
	Env   []string
	Proxy string
}

// ArmSurfaceDigest rebuilds one arm surface exactly as the trusted runner's
// golden_surface.arm_surface builds it and returns its SurfaceDigest: the
// commit of the repository at root extracted with git archive (ambient
// harness entries removed), the checkout's surface driver as the only file
// of its package there, the arm's modules downloaded outside any sandbox
// and verified by the arm's own go.sum, an offline `go build -trimpath`
// that links pins.GeneratorVersion into pkg/version under grader.sb, and
// the driver run under grader.sb with an empty PATH and HOME writing the
// default five-platform surface with pins. The signer computes the trusted
// baseline_surface_digest this way from the binding's baseline_commit
// (SPEC-HARNEVAL-003 REQ-HR-02). It needs macOS sandbox-exec and refuses
// elsewhere; the scratch tree is removed before it returns.
func ArmSurfaceDigest(ctx context.Context, root, commit string, pins Pins, opts ArmSurfaceOptions) (string, error) {
	if !revisionPattern.MatchString(commit) {
		return "", fmt.Errorf("arm surface: %q is not a 40-hex commit", commit)
	}
	if !linkableVersionPattern.MatchString(pins.GeneratorVersion) {
		return "", fmt.Errorf("arm surface: generator version %q cannot be linked with -X", pins.GeneratorVersion)
	}
	if pins.CodexModelCatalog != "" && !isCleanRelPath(pins.CodexModelCatalog) {
		return "", fmt.Errorf("arm surface: codex model catalog %q is not a clean relative path", pins.CodexModelCatalog)
	}
	digest, err := armSurfaceDigest(ctx, root, commit, pins, opts)
	if err != nil {
		return "", fmt.Errorf("arm surface at %s: %w", commit, err)
	}
	return digest, nil
}
