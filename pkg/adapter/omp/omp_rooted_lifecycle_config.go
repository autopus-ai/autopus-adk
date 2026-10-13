package omp

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// @AX:WARN [AUTO]: legacy bridge migration contains eight conditional branches.
// @AX:REASON [AUTO]: manifest ownership, marker policy, bounded rooted reads, managed-section parsing, removal, and user-remainder preservation must fail closed.
func migrateOMPLegacyBridgeConfigAt(
	workspace *ompRootedWorkspace,
	oldManifest *adapter.Manifest,
	files []adapter.FileMapping,
	writes []adapter.TransactionWrite,
	removes []adapter.TransactionRemove,
) ([]adapter.TransactionWrite, []adapter.TransactionRemove, error) {
	if oldManifest == nil || ompMappingsContainPath(files, configFile) {
		return writes, removes, nil
	}
	previous, managed := oldManifest.Files[configFile]
	if !managed {
		return writes, removes, nil
	}
	removes = filterOMPTransactionRemove(removes, configFile)
	if previous.Policy != adapter.OverwriteMarker {
		return writes, removes, nil
	}
	data, info, err := workspace.readFile(configFile, 4<<20)
	if errors.Is(err, fs.ErrNotExist) {
		return writes, removes, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read legacy %s: %w", configFile, err)
	}
	remainder, found, err := stripOMPManagedDocument(string(data))
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return writes, removes, nil
	}
	if strings.TrimSpace(remainder) == "" {
		removes = append(removes, adapter.TransactionRemove{Path: configFile})
		return writes, removes, nil
	}
	writes = append(writes, adapter.TransactionWrite{
		Path: configFile, Content: []byte(remainder), Perm: info.Mode().Perm(),
	})
	return writes, removes, nil
}

// releaseOMPProjectManagedConfigAt restores the ledger preimage of
// .omp/config.yml when the new file set no longer manages it: the claim set
// became empty because the profile left project-managed mode, native mode
// dropped the agent surface claim, or the user took task.disabledAgents over.
// The legacy migration above keeps the file out of the prune set, so without
// this step the managed keys would stay behind while the ownership ledger is
// pruned, and the next claim would fail on a prior-fingerprint mismatch. A
// config that drifted from the last emitted bytes fails closed exactly like
// Clean.
func (a *Adapter) releaseOMPProjectManagedConfigAt(
	workspace *ompRootedWorkspace,
	files []adapter.FileMapping,
	writes []adapter.TransactionWrite,
	removes []adapter.TransactionRemove,
) ([]adapter.TransactionWrite, []adapter.TransactionRemove, error) {
	if ompMappingsContainPath(files, configFile) {
		return writes, removes, nil
	}
	state, err := a.prepareOMPProjectCleanStateAt(workspace)
	if err != nil || state == nil {
		return writes, removes, err
	}
	if state.missing {
		return writes, append(removes, adapter.TransactionRemove{Path: configFile}), nil
	}
	return append(writes, adapter.TransactionWrite{
		Path: configFile, Content: state.preimage, Perm: state.mode.Perm(),
	}), removes, nil
}

// ompReleasedLedgerRemoves removes a recorded ledger the new file set no
// longer carries. Update's manifest diff prunes it through the prune roots;
// Generate retires only agent definitions, so it asks for this explicitly.
func ompReleasedLedgerRemoves(
	oldManifest *adapter.Manifest,
	files []adapter.FileMapping,
	removes []adapter.TransactionRemove,
) []adapter.TransactionRemove {
	if oldManifest == nil || ompMappingsContainPath(files, OMPModelProjectOwnershipRelativePath) {
		return removes
	}
	if _, recorded := oldManifest.Files[OMPModelProjectOwnershipRelativePath]; !recorded {
		return removes
	}
	for _, remove := range removes {
		if filepath.ToSlash(remove.Path) == OMPModelProjectOwnershipRelativePath {
			return removes
		}
	}
	return append(removes, adapter.TransactionRemove{Path: OMPModelProjectOwnershipRelativePath})
}

func ompMappingsContainPath(files []adapter.FileMapping, path string) bool {
	for _, file := range files {
		if filepath.ToSlash(file.TargetPath) == path {
			return true
		}
	}
	return false
}

func filterOMPTransactionRemove(removes []adapter.TransactionRemove, path string) []adapter.TransactionRemove {
	filtered := removes[:0]
	for _, remove := range removes {
		if filepath.ToSlash(remove.Path) != path {
			filtered = append(filtered, remove)
		}
	}
	return filtered
}
