package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Refusal reasons of the trusted reconstruction beside the harneval signer
// reasons (attestation_digest_mismatch, protocol_mismatch,
// records_protocol_mismatch, outcome_derivation_mismatch,
// policy_out_of_range), which pass through unchanged.
const (
	exportRunMetaInvalid         = "run_meta_invalid"
	exportInputInvalid           = "input_invalid"
	exportTrustedProtocolInvalid = "trusted_protocol_invalid"
	exportBaselineSurfaceFailed  = "baseline_surface_failed"
)

// maxRunMetaBytes bounds the --run-meta file, four short fields.
const maxRunMetaBytes = 64 << 10

// harnessTrustSources are the trusted values the reconstruction takes from
// outside the received artifact. attestations verifies the bound and
// session_result bundles of the run attempt (signature, transparency log
// proof, the bind and live-eval job identity on main) and returns their
// predicates with the verified log times (SPEC-HARNEVAL-003 T8); this build
// has none, so it refuses. baselineSurface is the digest of the baseline arm
// surface that the surface driver built from the binding's baseline_commit
// generates; the default rebuilds it with harneval.ArmSurfaceDigest, the Go
// twin of the runner's golden_surface.arm_surface. oracleAssertions is the
// black-box assertion ids of each task in main's task definitions; the
// default reads them from the loaded set (T15).
type harnessTrustSources struct {
	attestations     func(context.Context, harnessExportRequest, harneval.RunMeta) (harneval.AttestedSession, error)
	baselineSurface  func(context.Context, harnessExportRequest) (string, error)
	oracleAssertions func(*harneval.Set) (map[string][]string, error)
}

func reconstructionUnavailable(what string) error {
	return exportRefusal(exportReconstructionUnavailable, "this build has no "+what+"; nothing was signed")
}

func (s harnessTrustSources) withDefaults() harnessTrustSources {
	if s.attestations == nil {
		s.attestations = func(context.Context, harnessExportRequest, harneval.RunMeta) (harneval.AttestedSession, error) {
			return harneval.AttestedSession{}, reconstructionUnavailable("attestation bundle verification (SPEC-HARNEVAL-003 T8)")
		}
	}
	if s.baselineSurface == nil {
		s.baselineSurface = rebuildBaselineSurface
	}
	if s.oracleAssertions == nil {
		s.oracleAssertions = func(set *harneval.Set) (map[string][]string, error) { return harneval.OracleAssertionIDs(set), nil }
	}
	return s
}

// rebuildBaselineSurface rebuilds the baseline arm surface of the binding's
// baseline_commit with main's pins and the allowlisted child environment,
// offline from the local module cache, and digests it.
func rebuildBaselineSurface(ctx context.Context, req harnessExportRequest) (string, error) {
	digest, err := harneval.ArmSurfaceDigest(ctx, req.Root, req.Binding.BaselineCommit, req.Binding.Pins,
		harneval.ArmSurfaceOptions{Env: req.Env})
	if err != nil {
		return "", exportRefusal(exportBaselineSurfaceFailed, err.Error())
	}
	return digest, nil
}

// reconstruct is the trusted reconstruction of REQ-HR-02 and REQ-HR-09. It
// reads the run meta and the received documents as untrusted bytes, rebuilds
// the trusted protocol from main's golden set with the binding computed from
// the same checkout (its digest, baseline_commit, runner tree and candidate
// surface digests) and the trust sources, and lets VerifySignedSession check
// the attestation digests first and then every semantic rule. The
// attestations are verified before the baseline surface is rebuilt, the one
// slow source. The accepted session carries main's signed-lane floor.
func (s harnessTrustSources) reconstruct(ctx context.Context, req harnessExportRequest) (harnessSignable, error) {
	s = s.withDefaults()
	meta, err := readRunMeta(req.RunMeta)
	if err != nil {
		return harnessSignable{}, exportRefusal(exportRunMetaInvalid, err.Error())
	}
	in, err := harneval.LoadSignerInput(req.Input)
	if err != nil {
		return harnessSignable{}, exportRefusal(exportInputInvalid, err.Error())
	}
	set, err := harneval.LoadSet(req.Root)
	if err != nil {
		return harnessSignable{}, exportRefusal(exportTrustedProtocolInvalid, err.Error())
	}
	assertions, err := s.oracleAssertions(set)
	if err != nil {
		return harnessSignable{}, err
	}
	attested, err := s.attestations(ctx, req, meta)
	if err != nil {
		return harnessSignable{}, err
	}
	baseline, err := s.baselineSurface(ctx, req)
	if err != nil {
		return harnessSignable{}, err
	}
	trusted, err := harneval.RebuildTrustedProtocol(set, harneval.TrustedInputs{
		BaselineCommit: req.Binding.BaselineCommit, RunnerTreeDigest: req.Binding.RunnerTreeDigest,
		BaselineSurfaceDigest: baseline, CandidateSurfaceDigest: req.Binding.CandidateSurfaceDigest,
		BindingDigest: req.BindingDigest, OracleAssertions: assertions,
	})
	if err != nil {
		return harnessSignable{}, signerRefusal(err, exportTrustedProtocolInvalid)
	}
	session, err := harneval.VerifySignedSession(in, trusted, meta, attested)
	if err != nil {
		return harnessSignable{}, signerRefusal(err, exportInputInvalid)
	}
	return harnessSignable{Session: session, SignedTaskFloor: set.Manifest.Floors.SignedAgentTasks}, nil
}

// signerRefusal keeps a signer refusal's own reason and files any other
// defect under fallback.
func signerRefusal(err error, fallback string) error {
	var refusal *harneval.TrustError
	if errors.As(err, &refusal) {
		return refusal
	}
	return exportRefusal(fallback, err.Error())
}

// readRunMeta reads and strictly decodes the --run-meta file.
func readRunMeta(path string) (harneval.RunMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return harneval.RunMeta{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRunMetaBytes+1))
	if err != nil {
		return harneval.RunMeta{}, err
	}
	if len(data) > maxRunMetaBytes {
		return harneval.RunMeta{}, errors.New("the run meta file is larger than 64 KiB")
	}
	return harneval.DecodeRunMeta(data)
}
