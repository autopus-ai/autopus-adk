package omp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func readOMPModelProjectOwnershipAt(workspace *ompRootedWorkspace) (ompModelProjectOwnership, bool, error) {
	data, _, err := workspace.readOwnerOnlyFile(OMPModelProjectOwnershipRelativePath, 4<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return ompModelProjectOwnership{}, false, nil
	}
	if err != nil {
		return ompModelProjectOwnership{}, false, fmt.Errorf("read project ownership ledger: %w", err)
	}
	var ownership ompModelProjectOwnership
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ownership); err != nil || requireOMPModelDoctorJSONEOF(decoder) != nil {
		return ompModelProjectOwnership{}, false, fmt.Errorf("project ownership ledger invalid")
	}
	wantDigest := ownership.LedgerDigest
	canonical, _, err := canonicalOMPModelProjectOwnership(ownership)
	if err != nil || canonical.LedgerDigest != wantDigest {
		return ompModelProjectOwnership{}, false, fmt.Errorf("project ownership ledger invalid")
	}
	return canonical, true, nil
}

func readOMPModelProjectConfigAt(workspace *ompRootedWorkspace) ([]byte, bool, fs.FileMode, error) {
	data, info, err := workspace.readFile(configFile, 4<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, true, 0, nil
	}
	if err != nil {
		return nil, false, 0, fmt.Errorf("read %s: %w", configFile, err)
	}
	return data, false, info.Mode().Perm(), nil
}

func validateCurrentOMPModelProjectConfigAt(
	workspace *ompRootedWorkspace,
	ownership ompModelProjectOwnership,
) ([]byte, error) {
	current, missing, _, err := readOMPModelProjectConfigAt(workspace)
	if err != nil {
		return nil, fmt.Errorf("managed_key_conflict: current project config is unavailable")
	}
	anchor, _, err := ompProjectConfigAnchor(current, missing, ownership)
	if err != nil {
		return nil, err
	}
	if anchor == nil {
		if missing {
			return nil, fmt.Errorf("managed_key_conflict: current project config is unavailable")
		}
		return nil, fmt.Errorf("managed_key_conflict: current project config differs from last emitted bytes")
	}
	return current, nil
}

// ompProjectConfigAnchor returns the bytes ADK last emitted when the current
// file is them, or is them with only task.disabledAgents changed; edited
// reports the second case. Any other difference returns nil: an edit outside
// the claim set that the transaction chain cannot be walked back from.
func ompProjectConfigAnchor(
	current []byte,
	missing bool,
	ownership ompModelProjectOwnership,
) (anchor []byte, edited bool, err error) {
	salt, err := decodeOMPModelProjectSalt(ownership.Salt)
	if err != nil {
		return nil, false, err
	}
	if !missing && ompModelProjectCommitment(salt, current, false) == ownership.LastEmittedCommitment {
		return current, false, nil
	}
	if _, claimed := ownership.agentSurfaceKey(); !claimed && !ownership.AgentSurfaceUserOwned {
		return nil, false, nil
	}
	value, present := any(config.OMPAutopusDisabledAgents()), true
	if emitted := ownership.AgentSurfaceEmitted; emitted != nil {
		value, present = emitted.Value, emitted.Present
	}
	canonical, renderErr := setOMPDisabledAgents(current, value, present)
	if renderErr != nil || ompModelProjectCommitment(salt, canonical, false) != ownership.LastEmittedCommitment {
		return nil, false, nil
	}
	return canonical, true, nil
}

func loadOMPModelProjectOriginalPreimageAt(
	workspace *ompRootedWorkspace,
	ownership ompModelProjectOwnership,
) ([]byte, bool, fs.FileMode, error) {
	originData, err := ompModelProjectOriginLedgerBytes(ownership)
	if err != nil {
		return nil, false, 0, err
	}
	journals, err := loadOMPTransactionJournalsAt(workspace, adapterName)
	if err != nil {
		return nil, false, 0, fmt.Errorf("load project ownership transaction: %w", err)
	}
	wantLedgerChecksum := adapter.Checksum(string(originData))
	hasOrigin := false
	for _, journal := range journals {
		hasOrigin = hasOrigin || journalContainsOriginLedger(journal, wantLedgerChecksum) ||
			journalWritesOMPProjectLedger(journal)
	}
	if !hasOrigin {
		return nil, false, 0, fmt.Errorf("project ownership origin transaction missing")
	}
	current, missing, _, err := readOMPModelProjectConfigAt(workspace)
	if err != nil {
		return nil, false, 0, err
	}
	anchor, _, err := ompProjectConfigAnchor(current, missing, ownership)
	if err != nil || anchor == nil {
		_, validateErr := validateCurrentOMPModelProjectConfigAt(workspace, ownership)
		return nil, false, 0, validateErr
	}
	return walkOMPModelProjectPreimageAt(workspace, ownership, journals, wantLedgerChecksum, anchor)
}

// walkOMPModelProjectPreimageAt follows the exact-byte chain of committed
// config writes back from anchor to the origin whose salted commitment the
// ledger holds. The origin is the transaction that created this ledger, or
// one that re-based it on a file that already existed: the commitment, not
// the journal shape, proves which before-bytes are the preimage. A claim of
// task.disabledAgents recorded at the origin is restored to its preimage
// state first, because the re-based file may already carry ADK's value.
// @AX:WARN [AUTO]: preimage chain recovery has high branch complexity.
// @AX:REASON [AUTO]: exact-byte chain links, origin evidence, rollback artifacts, and commitment checks must all fail closed.
func walkOMPModelProjectPreimageAt(
	workspace *ompRootedWorkspace,
	ownership ompModelProjectOwnership,
	journals []*adapter.TransactionJournal,
	wantLedgerChecksum string,
	anchor []byte,
) ([]byte, bool, fs.FileMode, error) {
	salt, err := decodeOMPModelProjectSalt(ownership.Salt)
	if err != nil {
		return nil, false, 0, err
	}
	initialSurface, surfaceClaimed := ompAgentSurfaceKeyIn(ownership.InitialManagedKeys)
	current := anchor
	for _, journal := range journals {
		var configEntry *adapter.TransactionJournalEntry
		currentChecksum := adapter.Checksum(string(current))
		for index := range journal.Entries {
			entry := &journal.Entries[index]
			if filepath.ToSlash(entry.Path) == configFile && entry.AfterChecksum == currentChecksum {
				configEntry = entry
				break
			}
		}
		if configEntry == nil {
			continue
		}
		origin := journalContainsOriginLedger(journal, wantLedgerChecksum)
		rebase := !origin && journalWritesOMPProjectLedger(journal)
		if configEntry.MissingBefore {
			if !(origin || rebase) || !ownership.OriginalMissing {
				return nil, false, 0, fmt.Errorf("project ownership preimage mismatch")
			}
			return nil, true, 0, nil
		}
		if configEntry.BackupPath == "" {
			return nil, false, 0, fmt.Errorf("read project ownership preimage: rollback artifact missing")
		}
		backup, info, readErr := workspace.readFile(
			configEntry.BackupPath, maxOMPConfigRollbackArtifact,
		)
		if readErr != nil {
			return nil, false, 0, fmt.Errorf("read project ownership preimage: %w", readErr)
		}
		artifact, artifactErr := decodeOMPConfigRollbackArtifact(backup)
		var before []byte
		if artifactErr == nil {
			if info.Mode().Perm() != 0o600 || artifact.AfterChecksum != configEntry.AfterChecksum {
				return nil, false, 0, fmt.Errorf("project ownership rollback artifact invalid")
			}
			before, err = applyOMPConfigRollbackArtifact(artifact, current)
		} else {
			before = append([]byte(nil), backup...)
			migrated, migrationErr := encodeOMPConfigRollbackArtifact(before, current)
			if migrationErr != nil {
				return nil, false, 0, fmt.Errorf("migrate project ownership preimage: %w", migrationErr)
			}
			if migrationErr = workspace.atomicWrite(configEntry.BackupPath, migrated, 0o600); migrationErr != nil {
				return nil, false, 0, fmt.Errorf("migrate project ownership preimage: %w", migrationErr)
			}
		}
		if err != nil {
			return nil, false, 0, fmt.Errorf("read project ownership preimage: %w", err)
		}
		current = before
		if !origin && !rebase {
			continue
		}
		candidate := current
		if surfaceClaimed {
			if candidate, err = applyOMPAgentSurfacePrior(current, initialSurface.PriorFingerprint); err != nil {
				return nil, false, 0, err
			}
		}
		switch {
		case ownership.OriginalMissing && ompBlankConfig(candidate):
			return nil, true, fs.FileMode(configEntry.Mode), nil
		case !ownership.OriginalMissing &&
			ompModelProjectCommitment(salt, candidate, false) == ownership.OriginalCommitment:
			return candidate, false, fs.FileMode(configEntry.Mode), nil
		case origin:
			return nil, false, 0, fmt.Errorf("project ownership preimage mismatch")
		}
	}
	return nil, false, 0, fmt.Errorf("project ownership origin transaction missing")
}

// journalWritesOMPProjectLedger reports a transaction that wrote the ledger,
// the one shape a re-based origin can have.
func journalWritesOMPProjectLedger(journal *adapter.TransactionJournal) bool {
	for _, entry := range journal.Entries {
		if filepath.ToSlash(entry.Path) == OMPModelProjectOwnershipRelativePath && entry.AfterChecksum != "" {
			return true
		}
	}
	return false
}
