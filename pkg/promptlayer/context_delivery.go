package promptlayer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ContextDeliverySchemaVersion = "autopus.context_delivery.v1"

type ContextDeliveryOptions struct {
	Root                string
	Command             string
	SpecDir             string
	RequiredReferences  []string
	ConditionalProfiles []ContextProfileName
	// ExplicitArchitecture loads optional architecture only when selected by the
	// task. The canonical OMP evidence path retains full delivery by default.
	ExplicitArchitecture bool
}

type ContextDeliveryDocument struct {
	SourceRef          string `json:"source_ref"`
	SourceHash         string `json:"source_hash"`
	PromptHash         string `json:"prompt_hash"`
	TokenEstimate      int    `json:"token_estimate"`
	Kind               Kind   `json:"kind"`
	RedactionStatus    string `json:"redaction_status"`
	InvalidationReason string `json:"invalidation_reason"`
	Complete           bool   `json:"complete"`
}

// ContextDeliveryResult is a body-free receipt. Prompt and Layers are retained
// only in-process; serialized handoffs carry stable references and hashes.
type ContextDeliveryResult struct {
	SchemaVersion         string                    `json:"schema_version"`
	Command               string                    `json:"command"`
	SpecDir               string                    `json:"spec_dir"`
	RequiredDocuments     []ContextDeliveryDocument `json:"required_documents"`
	RequiredTokenEstimate int                       `json:"required_token_estimate"`
	SnapshotHash          string                    `json:"snapshot_hash"`
	PromptManifestHash    string                    `json:"prompt_manifest_hash"`
	IntegrityStatus       string                    `json:"integrity_status"`
	PromptManifest        Manifest                  `json:"prompt_manifest"`
	Prompt                string                    `json:"-"`
	Layers                []Layer                   `json:"-"`
}

// BuildContextDelivery loads every required document without size truncation.
// The returned receipt can be passed between agents without duplicating bodies.
func BuildContextDelivery(opts ContextDeliveryOptions) (ContextDeliveryResult, error) {
	root, command, specDir, refs, err := normalizeContextDeliveryOptions(opts)
	if err != nil {
		return ContextDeliveryResult{}, err
	}

	layers := make([]Layer, 0, len(refs))
	rawHashes := make(map[string]string, len(refs))
	for i, ref := range refs {
		kind := KindSnapshot
		group := GroupFrozenSnapshot
		if ref == "AGENTS.md" {
			kind = KindStable
			group = GroupIdentityRules
		} else if !strings.HasPrefix(ref, specDir+"/") {
			group = GroupProjectContext
		}
		raw, readErr := readRequiredContextSource(root, ref)
		if readErr != nil {
			return ContextDeliveryResult{}, readErr
		}
		if ref == filepath.ToSlash(filepath.Join(specDir, "spec.md")) {
			if identityErr := VerifyContextSpecIdentity(specDir, raw); identityErr != nil {
				return ContextDeliveryResult{}, identityErr
			}
		}
		sanitized := SanitizeContent(string(raw), ContextOptions{
			Required: true, PreserveInjectionEvidence: true,
		})
		layer := Layer{
			ID: fmt.Sprintf("required:%03d:%s", i, ref), Kind: kind, Group: group,
			SourceRef: ref, Content: sanitized.Content,
			CacheEligible:   sanitized.RedactionStatus == RedactionPassed,
			RedactionStatus: sanitized.RedactionStatus, InvalidationReason: sanitized.InvalidationReason,
		}
		layers = append(layers, layer)
		rawHashes[ref] = canonicalHash(raw)
	}
	if err := verifyFrozenContextSources(root, refs, rawHashes); err != nil {
		return ContextDeliveryResult{}, err
	}

	rendered, err := Render(layers)
	if err != nil {
		return ContextDeliveryResult{}, err
	}
	documents := make([]ContextDeliveryDocument, 0, len(rendered.Manifest.Entries))
	totalTokens := 0
	for _, entry := range rendered.Manifest.Entries {
		documents = append(documents, ContextDeliveryDocument{
			SourceRef: entry.SourceRef, SourceHash: rawHashes[entry.SourceRef],
			PromptHash: "sha256:" + entry.Hash, TokenEstimate: entry.TokenEstimate,
			Kind: entry.Kind, RedactionStatus: entry.RedactionStatus,
			InvalidationReason: entry.InvalidationReason, Complete: true,
		})
		totalTokens += entry.TokenEstimate
	}
	manifestJSON, err := json.Marshal(rendered.Manifest)
	if err != nil {
		return ContextDeliveryResult{}, fmt.Errorf("encode context prompt manifest: %w", err)
	}
	return ContextDeliveryResult{
		SchemaVersion: ContextDeliverySchemaVersion, Command: command, SpecDir: specDir,
		RequiredDocuments: documents, RequiredTokenEstimate: totalTokens,
		SnapshotHash:       hashContextSnapshot(command, specDir, documents),
		PromptManifestHash: canonicalHash(manifestJSON), IntegrityStatus: "verified",
		PromptManifest: rendered.Manifest, Prompt: rendered.Prompt, Layers: layers,
	}, nil
}

func normalizeContextDeliveryOptions(opts ContextDeliveryOptions) (string, string, string, []string, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	command := strings.ToLower(strings.TrimSpace(opts.Command))
	profile, ok := ResolveCommandContextProfile(command)
	if !ok {
		return "", "", "", nil, fmt.Errorf("unknown context profile command: %s", command)
	}
	specDir, err := cleanContextReference(opts.SpecDir, profile.RelevantSpec)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("invalid spec directory: %w", err)
	}
	refs := append([]string(nil), profile.RequiredDocuments()...)
	conditionalProfiles, err := resolveConditionalContextProfiles(command, profile, opts.ConditionalProfiles)
	if err != nil {
		return "", "", "", nil, err
	}
	refs = append(refs, documentsForProfiles(conditionalProfiles)...)
	if !opts.ExplicitArchitecture {
		availableConditional, err := availableDefaultConditionalDocuments(root, command)
		if err != nil {
			return "", "", "", nil, err
		}
		refs = append(refs, availableConditional...)
	}
	if profile.RelevantSpec {
		for _, name := range relevantSpecDocuments(command) {
			refs = append(refs, filepath.ToSlash(filepath.Join(specDir, name)))
		}
	}
	additionalRefs, err := cleanUniqueContextReferences(opts.RequiredReferences)
	if err != nil {
		return "", "", "", nil, err
	}
	sort.Strings(additionalRefs)
	refs = append(refs, additionalRefs...)
	refs, err = cleanUniqueContextReferences(refs)
	if err != nil {
		return "", "", "", nil, err
	}
	return root, command, specDir, refs, nil
}

func relevantSpecDocuments(command string) []string {
	switch command {
	case "go":
		return []string{"spec.md", "plan.md", "acceptance.md"}
	case "review":
		return []string{"spec.md", "plan.md", "research.md", "acceptance.md"}
	case "plan":
		return []string{"spec.md"}
	default:
		return nil
	}
}

func cleanUniqueContextReferences(refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		clean, err := cleanContextReference(ref, true)
		if err != nil {
			return nil, fmt.Errorf("invalid required context reference %q: %w", ref, err)
		}
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out, nil
}

func cleanContextReference(ref string, required bool) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		if required {
			return "", fmt.Errorf("path is required")
		}
		return "", nil
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(filepath.FromSlash(ref))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root")
	}
	return filepath.ToSlash(clean), nil
}

func readRequiredContextSource(root, ref string) ([]byte, error) {
	direct := filepath.Join(root, filepath.FromSlash(ref))
	info, err := os.Lstat(direct)
	if err != nil {
		return nil, fmt.Errorf("required context unavailable %s: %w", ref, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("required context must be a regular file: %s", ref)
	}
	resolved, err := resolveContextPath(root, filepath.FromSlash(ref))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read required context %s: %w", ref, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("required context is empty: %s", ref)
	}
	return data, nil
}

func verifyFrozenContextSources(root string, refs []string, expected map[string]string) error {
	for _, ref := range refs {
		data, err := readRequiredContextSource(root, ref)
		if err != nil {
			return err
		}
		if canonicalHash(data) != expected[ref] {
			return fmt.Errorf("required context changed while snapshot was being built: %s", ref)
		}
	}
	return nil
}

func hashContextSnapshot(command, specDir string, documents []ContextDeliveryDocument) string {
	ordered := append([]ContextDeliveryDocument(nil), documents...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SourceRef < ordered[j].SourceRef })
	var material strings.Builder
	material.WriteString(command)
	material.WriteByte('\n')
	material.WriteString(specDir)
	for _, document := range ordered {
		material.WriteByte('\n')
		material.WriteString(document.SourceRef)
		material.WriteByte('\x00')
		material.WriteString(document.SourceHash)
		material.WriteByte('\x00')
		material.WriteString(document.PromptHash)
	}
	return canonicalHash([]byte(material.String()))
}

func canonicalHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
