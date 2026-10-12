package cli

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// initComposeOMPDeps is the OMP catalog runner init's OMP tab reads with
// (SPEC-MODELUX-002 REQ-MUX2-22). Tests replace it and restore it with
// t.Cleanup; its activation is never used, because init's own OMP
// generation activates what init saves.
var initComposeOMPDeps = defaultOMPPlatformDependencies()

// composeOMPCatalogError is a failed read of the installed OMP catalog while
// the tab listed models; its text is the cause.
type composeOMPCatalogError struct{ err error }

func (e composeOMPCatalogError) Error() string { return e.err.Error() }

func (e composeOMPCatalogError) Unwrap() error { return e.err }

// composeOMPCatalogFailure reports an error of reading the catalog rather
// than a refusal of the choice (REQ-MUX2-23): a failed listing, or the
// unavailable catalog a plan's probe reports. Plan errors, blockers,
// validation failures, and the placeholder refusal are refusals.
func composeOMPCatalogFailure(err error) bool {
	var listing composeOMPCatalogError
	var unavailable ompCatalogUnavailableError
	return errors.As(err, &listing) || errors.As(err, &unavailable)
}

// planInitOMP plans opts on a detached copy of init's config under the file
// path's catalog, attestation, capability, and blocker rules, then refuses a
// candidate role_model_policy holding a ${NAME} placeholder: init's
// config.Save would write it, and the next load would expand it. init's
// config itself is not touched.
func planInitOMP(
	ctx context.Context,
	cfg *config.HarnessConfig,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) (ompProfileApplyPreviewPayload, config.RoleModelPolicyConf, error) {
	detached, err := cloneHarnessConfig(cfg)
	if err != nil {
		return ompProfileApplyPreviewPayload{}, config.RoleModelPolicyConf{}, err
	}
	preview, err := planOMPProfileForConfig(ctx, detached, opts, runner)
	if err != nil {
		return ompProfileApplyPreviewPayload{}, config.RoleModelPolicyConf{}, err
	}
	if err := checkInitOMPPolicy(detached.RoleModelPolicy); err != nil {
		return ompProfileApplyPreviewPayload{}, config.RoleModelPolicyConf{}, err
	}
	return preview, detached.RoleModelPolicy, nil
}

// checkInitOMPPolicy applies validateOMPProfileSource's rule to an
// in-memory candidate: no scalar of it may hold a ${NAME} placeholder.
func checkInitOMPPolicy(policy config.RoleModelPolicyConf) error {
	var node yaml.Node
	if err := node.Encode(policy); err != nil {
		return fmt.Errorf("encode role_model_policy: %w", err)
	}
	if yamlNodeContainsOMPProfilePlaceholder(&node, make(map[*yaml.Node]bool)) {
		return errors.New("role_model_policy_placeholder_unsupported")
	}
	return nil
}

// applyInitOMPPolicy sets only init's role_model_policy to candidate once it
// passes the placeholder rule; a refused candidate changes nothing.
func applyInitOMPPolicy(cfg *config.HarnessConfig, candidate config.RoleModelPolicyConf) error {
	if err := checkInitOMPPolicy(candidate); err != nil {
		return err
	}
	cfg.RoleModelPolicy = candidate
	return nil
}

// ompApplyInit puts the confirmed choice into init's config; init's save
// and OMP generation write and activate it under
// saveConfigWithGenerationRollback, which restores everything when the
// generation fails.
func (f *composeFormRun) ompApplyInit(candidate config.RoleModelPolicyConf) error {
	if err := applyInitOMPPolicy(f.shared.cfg, candidate); err != nil {
		return f.ompFailed(err)
	}
	fmt.Fprintln(f.out, f.form.InitOMPApplied)
	f.applied = true
	return nil
}
