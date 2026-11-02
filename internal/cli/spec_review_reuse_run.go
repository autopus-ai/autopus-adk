package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// specReviewReuseRequest is what the reuse check reads before any readiness
// probe or model call.
type specReviewReuseRequest struct {
	cfg              *config.HarnessConfig
	gate             config.ReviewGateConf
	opts             specReviewOptions
	multi            bool
	requestedTimeout int
	specID           string
	specDir          string
	codeContext      string
	policy           specReviewPolicyIdentity
	configDigest     string
	ledger           *specReviewLedger
}

// reuseSpecReviewResult returns true when it returned the previous result:
// no probe, no call, and no file rewritten except the session's reused
// attempt (SPEC-REVIEWREUSE-001 REQ-RR-REUSE-02 to REQ-RR-REUSE-07). Without
// a previous receipt it does nothing; --force-review records itself and runs
// the review.
func reuseSpecReviewResult(ctx context.Context, req specReviewReuseRequest) (bool, error) {
	if req.opts.forceReview {
		return false, req.ledger.markForcedReview()
	}
	receiptPath := filepath.Join(req.specDir, "review-receipt.json")
	if _, err := os.Stat(receiptPath); err != nil {
		return false, nil
	}
	decision := decideSpecReviewReuse(req.specDir, currentSpecReviewFingerprint(ctx, req), time.Now())
	if !decision.found {
		return false, nil
	}
	if !decision.reuse {
		fmt.Fprintf(os.Stderr, "리뷰 재사용 안 함: %s\n", strings.Join(decision.reasons, "; "))
		return false, nil
	}
	findings, _ := spec.LoadFindings(req.specDir)
	if _, err := req.ledger.finish(specReviewEndReasonReused); err != nil {
		return false, fmt.Errorf("리뷰 세션 저장 실패 (SPEC: %s): %w", req.specID, err)
	}
	printSpecReviewReuse(os.Stdout, req.specID, receiptPath, decision, findings)
	return true, nil
}

// currentSpecReviewFingerprint assembles the planned reviewers and judge the
// way the attempt will, without probing them, and fingerprints the inputs.
func currentSpecReviewFingerprint(ctx context.Context, req specReviewReuseRequest) *specReviewFingerprint {
	projectRoot, _ := os.Getwd()
	reviewers, err := specReviewProviderAssembly(ctx, specReviewProviderRequest{
		Config: req.cfg, FlagProviders: req.opts.providers, Multi: req.multi, RequestedTimeout: req.requestedTimeout, Warnings: io.Discard,
	})
	if err != nil {
		return &specReviewFingerprint{Incomplete: []string{"providers: " + err.Error()}}
	}
	judge, err := specReviewJudgeAssembly(ctx, req.cfg, reviewers.Providers, req.gate.Judge, req.requestedTimeout)
	if err != nil {
		return &specReviewFingerprint{Incomplete: []string{"judge: " + err.Error()}}
	}
	in := specReviewFingerprintInput{
		projectRoot: projectRoot, specDir: req.specDir, codeContext: req.codeContext, requiredDocuments: req.opts.requiredDocuments,
		policy: req.policy, configDigest: req.configDigest, reviewers: reviewers.Providers, judge: judge,
	}
	delivery, err := prepareSpecReviewContextDelivery(req.specDir, reviewers.Providers, req.opts)
	if err == nil && delivery != nil {
		var receipt promptlayer.ContextDeliveryResult
		if receipt, err = delivery.buildVerified(); err == nil {
			in.delivery = specReviewDeliveryDigests(&receipt)
		}
	}
	if err != nil {
		return &specReviewFingerprint{Incomplete: []string{"references: " + err.Error()}}
	}
	return computeSpecReviewFingerprint(in)
}

// specReviewRoundFingerprint fingerprints the inputs of the round whose
// prompt was just built from delivery; the last one is written into the
// receipt.
func specReviewRoundFingerprint(p specReviewLoopParams, delivery map[string]string) *specReviewFingerprint {
	projectRoot, _ := os.Getwd()
	return computeSpecReviewFingerprint(specReviewFingerprintInput{
		projectRoot: projectRoot, specDir: p.specDir, codeContext: p.codeContext, delivery: delivery, requiredDocuments: p.requiredDocuments,
		policy: p.policy, configDigest: p.configDigest, reviewers: p.providers, judge: p.judgeConfig,
	})
}

// withSpecReviewEvidence completes the final round's fingerprint with the
// digests of the evidence files as the invocation leaves them.
func withSpecReviewEvidence(fp *specReviewFingerprint, specDir string) *specReviewFingerprint {
	if fp == nil {
		return nil
	}
	final := *fp
	for name, target := range map[string]*string{"review-findings.json": &final.Evidence.ReviewFindings, "review.md": &final.Evidence.ReviewMD} {
		if data, err := os.ReadFile(filepath.Join(specDir, name)); err == nil {
			*target = spec.SHA256Hex(data)
		}
	}
	return &final
}
