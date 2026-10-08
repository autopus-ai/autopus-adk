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
)

// maxRunMetaBytes bounds the --run-meta file, four short fields.
const maxRunMetaBytes = 64 << 10

// harnessTrustSources are the trusted values the reconstruction takes from
// outside the received artifact. attestations verifies the bound and
// session_result bundles of the run attempt (signature, transparency log
// proof, the bind and live-eval job identity on main) and returns their
// predicates with the verified log times (SPEC-HARNEVAL-003 T8).
// baselineSurface is the digest of the baseline arm surface that the surface
// driver built from the binding's baseline_commit generates. oracleAssertions
// is the black-box assertion ids of each task in main's task definitions
// (T15). A nil source has no trusted value to give, so nothing is signed.
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
		s.baselineSurface = func(context.Context, harnessExportRequest) (string, error) {
			return "", reconstructionUnavailable("baseline arm surface rebuild from baseline_commit")
		}
	}
	if s.oracleAssertions == nil {
		s.oracleAssertions = func(*harneval.Set) (map[string][]string, error) {
			return nil, reconstructionUnavailable("black_box_oracle in the golden task schema (SPEC-HARNEVAL-003 T15)")
		}
	}
	return s
}

// reconstruct is the trusted reconstruction of REQ-HR-02 and REQ-HR-09. It
// reads the run meta and the received documents as untrusted bytes, rebuilds
// the trusted protocol from main's golden set with the binding computed from
// the same checkout (its digest, baseline_commit, runner tree and candidate
// surface digests) and the trust sources, and lets VerifySignedSession check
// the attestation digests first and then every semantic rule. The accepted
// session carries main's signed-lane floor.
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
	attested, err := s.attestations(ctx, req, meta)
	if err != nil {
		return harnessSignable{}, err
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
