package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

const specReviewFingerprintSchema = "spec_review_fingerprint.v1"

// specReviewFingerprint digests every input a review round reads, component
// by component, so a later invocation can tell an unchanged input from a
// changed one and name what changed (SPEC-REVIEWREUSE-001 REQ-RR-REUSE-01).
type specReviewFingerprint struct {
	Schema     string                       `json:"schema"`
	Documents  map[string]string            `json:"documents"`
	References map[string]string            `json:"references,omitempty"`
	Code       specReviewCodeDigest         `json:"code"`
	Checklist  string                       `json:"checklist"`
	Policy     specReviewPolicyIdentity     `json:"policy"`
	Contract   string                       `json:"contract"`
	Providers  []specReviewProviderIdentity `json:"providers"`
	Evidence   specReviewEvidenceDigest     `json:"evidence"`
	Complete   bool                         `json:"complete"`
	Incomplete []string                     `json:"incomplete,omitempty"`
}

type specReviewCodeDigest struct {
	Head     string `json:"head"`
	Worktree string `json:"worktree"`
	Context  string `json:"context"`
}

// specReviewPolicyIdentity is every setting that changes what reviewers see
// or how their answers resolve.
type specReviewPolicyIdentity struct {
	Profile             string   `json:"profile"`
	JudgeMode           string   `json:"judge_mode"`
	VerifyInput         string   `json:"verify_input"`
	Strategy            string   `json:"strategy"`
	VerdictThreshold    float64  `json:"verdict_threshold"`
	MinProviders        int      `json:"min_providers"`
	ExcludeFailed       bool     `json:"exclude_failed_from_denom"`
	MaxRevisions        int      `json:"max_revisions"`
	PassCriteria        string   `json:"pass_criteria"`
	DocContextMaxLines  int      `json:"doc_context_max_lines"`
	ConditionalProfiles []string `json:"conditional_profiles,omitempty"`
}

type specReviewProviderIdentity struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Backend     string `json:"backend,omitempty"`
	Binary      string `json:"binary,omitempty"`
	Model       string `json:"model,omitempty"`
	Args        string `json:"args"`
	ModelPolicy string `json:"model_policy,omitempty"`
	Pinned      bool   `json:"pinned"`
}

type specReviewEvidenceDigest struct {
	ReviewFindings string `json:"review_findings,omitempty"`
	ReviewMD       string `json:"review_md,omitempty"`
}

// specReviewFingerprintInput is what one round reads besides the files.
type specReviewFingerprintInput struct {
	projectRoot       string
	specDir           string
	codeContext       string
	requiredDocuments []string
	delivery          map[string]string
	policy            specReviewPolicyIdentity
	reviewers         []orchestra.ProviderConfig
	judge             *orchestra.ProviderConfig
}

// specReviewChecklistDigest digests the checklist the prompt injects; tests
// replace it to simulate a changed checklist.
var specReviewChecklistDigest = func() (string, error) {
	body, err := spec.ReviewChecklistBody()
	if err != nil {
		return "", err
	}
	return spec.SHA256Hex([]byte(body)), nil
}

var specReviewContractDigest = sync.OnceValue(func() string {
	return spec.SHA256Hex([]byte(spec.ReviewPromptContract() + "\x00" + specReviewSchemaContract()))
})

// specReviewSchemaContract is the structured response contract and reviewer
// schema the reviewers receive, plus the judge prompt for fixed data.
func specReviewSchemaContract() string {
	schema := &orchestra.SchemaBuilder{}
	reviewer, _ := schema.EmbedInPrompt("reviewer")
	judgeSchema, _ := schema.EmbedInPrompt("review_judge")
	judge := ""
	if builder, err := orchestra.NewPromptBuilder(); err == nil {
		participants, _ := orchestra.AnonymizeReviewParticipants([]orchestra.ProviderResponse{{Provider: "probe", Output: "{}"}})
		judge, _ = builder.BuildReviewJudge(orchestra.ReviewJudgeData{SpecID: "SPEC-PROBE-001", Mode: "discover",
			Participants: participants, SchemaJSON: judgeSchema})
	}
	return buildStructuredSpecReviewPrompt("", reviewer, true) + "\x00" + judge
}

func computeSpecReviewFingerprint(in specReviewFingerprintInput) *specReviewFingerprint {
	fp := &specReviewFingerprint{Schema: specReviewFingerprintSchema, Policy: in.policy, Contract: specReviewContractDigest()}
	incomplete := func(format string, args ...any) { fp.Incomplete = append(fp.Incomplete, fmt.Sprintf(format, args...)) }
	documents, err := spec.ReviewDocumentDigests(in.specDir)
	if err != nil {
		incomplete("documents: %v", err)
	}
	fp.Documents = documents
	fp.References = map[string]string{}
	for source, digest := range in.delivery {
		fp.References[source] = digest
	}
	for _, path := range in.requiredDocuments {
		data, err := os.ReadFile(filepath.Join(in.projectRoot, path))
		if err != nil {
			incomplete("references: %s unreadable", path)
			continue
		}
		fp.References[path] = spec.SHA256Hex(data)
	}
	if len(fp.References) == 0 {
		fp.References = nil
	}
	fp.Code.Context = spec.SHA256Hex([]byte(in.codeContext))
	fp.Code.Head, fp.Code.Worktree, err = specReviewGitBaseline(in.projectRoot, in.specDir)
	if err != nil {
		incomplete("code: %v", err)
	}
	if fp.Checklist, err = specReviewChecklistDigest(); err != nil {
		incomplete("checklist: %v", err)
	}
	for _, provider := range in.reviewers {
		fp.Providers = append(fp.Providers, specReviewProviderIdentityOf(provider, specReviewRoleReviewer))
	}
	if in.judge != nil {
		fp.Providers = append(fp.Providers, specReviewProviderIdentityOf(*in.judge, specReviewRoleJudge))
	}
	fp.Complete = len(fp.Incomplete) == 0
	return fp
}

func specReviewPolicyIdentityOf(plan specReviewPlan, gate config.ReviewGateConf, strategy string, threshold float64, maxRevisions int, opts specReviewOptions) specReviewPolicyIdentity {
	return specReviewPolicyIdentity{
		Profile: plan.Profile, JudgeMode: plan.JudgeMode, VerifyInput: plan.VerifyInput, Strategy: strategy, VerdictThreshold: threshold,
		MinProviders: gate.MinProviders, ExcludeFailed: gate.ExcludeFailedFromDenom, MaxRevisions: maxRevisions,
		PassCriteria: spec.SHA256Hex([]byte(gate.PassCriteria)), DocContextMaxLines: gate.DocContextMaxLines,
		ConditionalProfiles: slices.Clone(opts.conditionalProfiles),
	}
}

func specReviewProviderIdentityOf(provider orchestra.ProviderConfig, role string) specReviewProviderIdentity {
	selector := specReviewModelSelector(provider)
	return specReviewProviderIdentity{
		Name: provider.Name, Role: role, Backend: provider.Backend, Binary: provider.Binary, Model: selector,
		Args: spec.SHA256Hex([]byte(strings.Join(provider.Args, "\x00"))), ModelPolicy: provider.ModelPolicy,
		Pinned: specReviewModelPinned(selector),
	}
}

// specReviewModelSelector is the provider's model field, else the value
// after --model or -m (or --model=) in its arguments.
func specReviewModelSelector(provider orchestra.ProviderConfig) string {
	if model := strings.TrimSpace(provider.Model); model != "" {
		return model
	}
	for i, arg := range provider.Args {
		switch {
		case (arg == "--model" || arg == "-m") && i+1 < len(provider.Args):
			return strings.TrimSpace(provider.Args[i+1])
		case strings.HasPrefix(arg, "--model="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--model="))
		}
	}
	return ""
}

// specReviewModelPinned reports a selector that names a version: non-empty,
// with a digit, not default, not ending in latest. An alias such as opus may
// resolve to a newer model between two invocations (REQ-RR-REUSE-04).
func specReviewModelPinned(selector string) bool {
	lower := strings.ToLower(selector)
	return lower != "" && lower != "default" && !strings.HasSuffix(lower, "latest") && strings.ContainsAny(lower, "0123456789")
}
