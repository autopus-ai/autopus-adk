package omp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const (
	OMPModelProjectOwnershipRelativePath = ".autopus/omp-model-project-ownership-v1.json"
	ompModelProjectOwnershipSchema       = "autopus.omp-model-project-ownership.v1"
)

// ompAgentSurfaceOwner owns the task.disabledAgents claim autopus mode makes on
// its own; a claim without an owner belongs to the project-managed profile.
// Every field added for the agent surface is omitempty, so a ledger with
// profile claims only keeps the bytes v0.50.125 writes and reads.
const ompAgentSurfaceOwner = "agent_surface"

type ompModelProjectOwnedKey struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
	Owner       string `json:"owner,omitempty"`
	// PriorFingerprint is the key's value in the preimage. Only the agent
	// surface claim records it, so a release restores that key without
	// replaying the transaction chain.
	PriorFingerprint string `json:"prior_fingerprint,omitempty"`
}

// ompAgentSurfaceValue is the user-owned task.disabledAgents value the last
// emission rendered, kept so a later edit of that key alone can still be told
// apart from an edit anywhere else.
type ompAgentSurfaceValue struct {
	Present bool `json:"present"`
	Value   any  `json:"value,omitempty"`
}

type ompModelProjectOwnership struct {
	SchemaVersion            string                    `json:"schema_version"`
	Salt                     string                    `json:"salt"`
	OriginalMissing          bool                      `json:"original_missing"`
	OriginalCommitment       string                    `json:"original_commitment"`
	InitialEmittedCommitment string                    `json:"initial_emitted_commitment"`
	InitialManagedKeys       []ompModelProjectOwnedKey `json:"initial_managed_keys"`
	LastEmittedCommitment    string                    `json:"last_emitted_commitment"`
	ManagedKeys              []ompModelProjectOwnedKey `json:"managed_keys"`
	// AgentSurfaceUserOwned marks task.disabledAgents as the user's: ADK never
	// claims it again until its value returns to the default set. A ledger
	// holding only this mark has no salt, commitment, or claim.
	AgentSurfaceUserOwned bool                  `json:"agent_surface_user_owned,omitempty"`
	AgentSurfaceEmitted   *ompAgentSurfaceValue `json:"agent_surface_emitted,omitempty"`
	LedgerDigest          string                `json:"ledger_digest"`
}

type ompModelProjectOwnershipBody struct {
	SchemaVersion            string                    `json:"schema_version"`
	Salt                     string                    `json:"salt"`
	OriginalMissing          bool                      `json:"original_missing"`
	OriginalCommitment       string                    `json:"original_commitment"`
	InitialEmittedCommitment string                    `json:"initial_emitted_commitment"`
	InitialManagedKeys       []ompModelProjectOwnedKey `json:"initial_managed_keys"`
	LastEmittedCommitment    string                    `json:"last_emitted_commitment"`
	ManagedKeys              []ompModelProjectOwnedKey `json:"managed_keys"`
	AgentSurfaceUserOwned    bool                      `json:"agent_surface_user_owned,omitempty"`
	AgentSurfaceEmitted      *ompAgentSurfaceValue     `json:"agent_surface_emitted,omitempty"`
}

func newOMPModelProjectOwnership(
	original []byte,
	originalMissing bool,
	emitted []byte,
	fingerprints map[string]string,
) (ompModelProjectOwnership, []byte, error) {
	saltBytes := make([]byte, 32)
	if _, err := rand.Read(saltBytes); err != nil {
		return ompModelProjectOwnership{}, nil, fmt.Errorf("generate project ownership salt: %w", err)
	}
	keys := ompModelProjectOwnedKeys(fingerprints)
	ownership := ompModelProjectOwnership{
		SchemaVersion: ompModelProjectOwnershipSchema,
		Salt:          hex.EncodeToString(saltBytes), OriginalMissing: originalMissing,
		OriginalCommitment:       ompModelProjectCommitment(saltBytes, original, originalMissing),
		InitialEmittedCommitment: ompModelProjectCommitment(saltBytes, emitted, false),
		InitialManagedKeys:       append([]ompModelProjectOwnedKey(nil), keys...),
		LastEmittedCommitment:    ompModelProjectCommitment(saltBytes, emitted, false),
		ManagedKeys:              keys,
	}
	return canonicalOMPModelProjectOwnership(ownership)
}

func updateOMPModelProjectOwnership(
	ownership ompModelProjectOwnership,
	emitted []byte,
	fingerprints map[string]string,
) (ompModelProjectOwnership, []byte, error) {
	salt, err := decodeOMPModelProjectSalt(ownership.Salt)
	if err != nil {
		return ompModelProjectOwnership{}, nil, err
	}
	ownership.LastEmittedCommitment = ompModelProjectCommitment(salt, emitted, false)
	ownership.ManagedKeys = ompModelProjectOwnedKeys(fingerprints)
	return canonicalOMPModelProjectOwnership(ownership)
}

func canonicalOMPModelProjectOwnership(
	ownership ompModelProjectOwnership,
) (ompModelProjectOwnership, []byte, error) {
	ownership.SchemaVersion = ompModelProjectOwnershipSchema
	ownership.LedgerDigest = ""
	sort.Slice(ownership.InitialManagedKeys, func(i, j int) bool {
		return ownership.InitialManagedKeys[i].Path < ownership.InitialManagedKeys[j].Path
	})
	sort.Slice(ownership.ManagedKeys, func(i, j int) bool {
		return ownership.ManagedKeys[i].Path < ownership.ManagedKeys[j].Path
	})
	if err := validateOMPModelProjectOwnership(ownership); err != nil {
		return ompModelProjectOwnership{}, nil, err
	}
	body := ompModelProjectOwnershipBody{
		SchemaVersion: ownership.SchemaVersion, Salt: ownership.Salt,
		OriginalMissing: ownership.OriginalMissing, OriginalCommitment: ownership.OriginalCommitment,
		InitialEmittedCommitment: ownership.InitialEmittedCommitment,
		InitialManagedKeys:       ownership.InitialManagedKeys,
		LastEmittedCommitment:    ownership.LastEmittedCommitment,
		ManagedKeys:              ownership.ManagedKeys,
		AgentSurfaceUserOwned:    ownership.AgentSurfaceUserOwned,
		AgentSurfaceEmitted:      ownership.AgentSurfaceEmitted,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ompModelProjectOwnership{}, nil, fmt.Errorf("encode project ownership body: %w", err)
	}
	ownership.LedgerDigest = OMPModelSHA256(bodyBytes)
	data, err := json.MarshalIndent(ownership, "", "  ")
	if err != nil {
		return ompModelProjectOwnership{}, nil, fmt.Errorf("encode project ownership ledger: %w", err)
	}
	return ownership, append(data, '\n'), nil
}

func readOMPModelProjectOwnership(root string) (ownership ompModelProjectOwnership, exists bool, returnErr error) {
	workspace, err := openOMPRootedWorkspace(root)
	if err != nil {
		return ompModelProjectOwnership{}, false, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	return readOMPModelProjectOwnershipAt(workspace)
}

func ompModelProjectOriginLedgerBytes(ownership ompModelProjectOwnership) ([]byte, error) {
	ownership.LastEmittedCommitment = ownership.InitialEmittedCommitment
	ownership.ManagedKeys = append([]ompModelProjectOwnedKey(nil), ownership.InitialManagedKeys...)
	if _, claimed := ompAgentSurfaceKeyIn(ownership.InitialManagedKeys); claimed {
		// The origin claimed task.disabledAgents, so it carried no user mark.
		// A mark set later never matches the origin bytes; the preimage walk
		// then proves the origin by its commitment instead.
		ownership.AgentSurfaceUserOwned, ownership.AgentSurfaceEmitted = false, nil
	}
	_, data, err := canonicalOMPModelProjectOwnership(ownership)
	return data, err
}

func journalContainsOriginLedger(journal *adapter.TransactionJournal, checksum string) bool {
	for _, entry := range journal.Entries {
		if filepath.ToSlash(entry.Path) == OMPModelProjectOwnershipRelativePath &&
			entry.MissingBefore && entry.AfterChecksum == checksum {
			return true
		}
	}
	return false
}

func validateOMPModelProjectOwnership(ownership ompModelProjectOwnership) error {
	if ownership.SchemaVersion != ompModelProjectOwnershipSchema {
		return fmt.Errorf("project ownership schema invalid")
	}
	if ownership.markerOnly() {
		return nil
	}
	if _, err := decodeOMPModelProjectSalt(ownership.Salt); err != nil {
		return err
	}
	for _, value := range []string{ownership.OriginalCommitment, ownership.InitialEmittedCommitment, ownership.LastEmittedCommitment} {
		if !validOMPModelHash(value) {
			return fmt.Errorf("project ownership commitment invalid")
		}
	}
	for _, keys := range [][]ompModelProjectOwnedKey{ownership.InitialManagedKeys, ownership.ManagedKeys} {
		seen := make(map[string]bool, len(keys))
		for _, key := range keys {
			if !validOMPManagedPath(key.Path) || !validOMPModelHash(key.Fingerprint) || seen[key.Path] ||
				!validOMPModelProjectKeyOwner(key) {
				return fmt.Errorf("project ownership managed key invalid")
			}
			seen[key.Path] = true
		}
	}
	_, claimed := ownership.agentSurfaceKey()
	if (ownership.AgentSurfaceUserOwned && claimed) ||
		(ownership.AgentSurfaceEmitted != nil && !ownership.AgentSurfaceUserOwned) {
		return fmt.Errorf("project ownership agent surface mark invalid")
	}
	return nil
}

func validOMPModelProjectKeyOwner(key ompModelProjectOwnedKey) bool {
	switch key.Owner {
	case "":
		return key.PriorFingerprint == ""
	case ompAgentSurfaceOwner:
		return key.Path == ompDisabledAgentsKey && validOMPModelHash(key.PriorFingerprint)
	}
	return false
}

// markerOnly reports a ledger that only remembers a user-owned
// task.disabledAgents: it manages no key and so restores nothing.
func (o ompModelProjectOwnership) markerOnly() bool {
	return o.AgentSurfaceUserOwned && o.AgentSurfaceEmitted == nil && o.Salt == "" &&
		o.OriginalCommitment == "" && o.InitialEmittedCommitment == "" && o.LastEmittedCommitment == "" &&
		!o.OriginalMissing && len(o.InitialManagedKeys) == 0 && len(o.ManagedKeys) == 0
}

func newOMPMarkerOnlyOwnership() (ompModelProjectOwnership, []byte, error) {
	return canonicalOMPModelProjectOwnership(ompModelProjectOwnership{AgentSurfaceUserOwned: true})
}

func (o ompModelProjectOwnership) agentSurfaceKey() (ompModelProjectOwnedKey, bool) {
	return ompAgentSurfaceKeyIn(o.ManagedKeys)
}

func ompAgentSurfaceKeyIn(keys []ompModelProjectOwnedKey) (ompModelProjectOwnedKey, bool) {
	for _, key := range keys {
		if key.Owner == ompAgentSurfaceOwner {
			return key, true
		}
	}
	return ompModelProjectOwnedKey{}, false
}

// hasProfileClaims reports a ledger whose preimage is only recoverable from
// the transaction chain, because a profile claim may own any key's value.
func (o ompModelProjectOwnership) hasProfileClaims() bool {
	for _, key := range o.ManagedKeys {
		if key.Owner == "" {
			return true
		}
	}
	return false
}

func decodeOMPModelProjectSalt(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("project ownership salt invalid")
	}
	return decoded, nil
}

func ompModelProjectCommitment(salt, data []byte, missing bool) string {
	marker := []byte("present\x00")
	if missing {
		marker = []byte("missing\x00")
	}
	payload := append(append(append([]byte("autopus.omp.project-commitment.v1\x00"), salt...), marker...), data...)
	return OMPModelSHA256(payload)
}

func ompModelProjectOwnedKeys(values map[string]string) []ompModelProjectOwnedKey {
	keys := make([]ompModelProjectOwnedKey, 0, len(values))
	for path, fingerprint := range values {
		keys = append(keys, ompModelProjectOwnedKey{Path: path, Fingerprint: fingerprint})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Path < keys[j].Path })
	return keys
}
