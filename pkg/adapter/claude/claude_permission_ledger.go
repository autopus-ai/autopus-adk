package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/content"
)

const (
	// PermissionLedgerPath is the project-relative path of the ledger, which
	// the managed .gitignore ignores: it records this machine's writes only.
	PermissionLedgerPath          = ".autopus/claude-code-permissions.json"
	claudePermissionLedgerPath    = PermissionLedgerPath
	claudePermissionLedgerVersion = 1
)

// claudePermissionLedger records the allow entries this machine's update added
// to .claude/settings.json, which clean retracts. Deny is what older ledgers
// recorded: clean never retracts a deny, so a forged ledger cannot weaken one
// and a leftover deny fails safe, and no write records one any more.
type claudePermissionLedger struct {
	Version int      `json:"version"`
	Allow   []string `json:"allow,omitempty"`
	Deny    []string `json:"deny,omitempty"`
}

func (a *Adapter) preparePermissionLedgerMapping(perms *adapter.PermissionSet) (adapter.FileMapping, error) {
	manifest, err := loadClaudeManifestSafely(a.root)
	if err != nil {
		return adapter.FileMapping{}, err
	}
	ledger, _, err := loadClaudePermissionLedger(a.root, manifest)
	if err != nil {
		return adapter.FileMapping{}, err
	}
	gone := ledger == nil && manifest != nil && manifestClaimsLedger(manifest)
	if ledger == nil {
		ledger = &claudePermissionLedger{Version: claudePermissionLedgerVersion}
	}

	allow, _, err := readClaudePermissionPreimage(a.root)
	if err != nil {
		return adapter.FileMapping{}, err
	}
	if perms != nil {
		// A retired default leaves settings.json on this update, so the ledger
		// stops claiming it: a later hand-added copy then belongs to the user.
		ledger.Allow = withoutRetiredUndesired(ledger.Allow, perms.Allow)
		ledger.Allow = appendUniqueStrings(ledger.Allow, missingStrings(perms.Allow, allow)...)
		if gone {
			// The claimed ledger took its record along: re-claim the allows
			// settings.json holds that this generator emits, so clean still
			// retracts them. Over-claiming an allow fails safe.
			ledger.Allow = appendUniqueStrings(ledger.Allow, presentStrings(perms.Allow, allow)...)
		}
	}
	content, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return adapter.FileMapping{}, fmt.Errorf("permission ledger 직렬화 실패: %w", err)
	}
	content = append(content, '\n')
	return adapter.FileMapping{
		TargetPath:      claudePermissionLedgerPath,
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(string(content)),
		Content:         content,
	}, nil
}

// markPermissionLedgerExact makes the ledger write leave the file exactly
// 0600 (claudeFileMode). The write that claims a ledger git checked out at
// 0644 must tighten it, or the next run's ownership check refuses the ledger
// the run itself wrote.
func markPermissionLedgerExact(writes []adapter.TransactionWrite) {
	for i := range writes {
		if filepath.ToSlash(filepath.Clean(writes[i].Path)) == claudePermissionLedgerPath {
			writes[i].ExactPerm = true
		}
	}
}

func loadClaudePermissionLedger(
	root string,
	manifest *adapter.Manifest,
) (*claudePermissionLedger, bool, error) {
	return loadClaudePermissionLedgerOn(runtime.GOOS, root, manifest)
}

// loadClaudePermissionLedgerOn is loadClaudePermissionLedger under the file
// mode rules of goos, so a test can run the Windows rules on any host.
func loadClaudePermissionLedgerOn(
	goos, root string,
	manifest *adapter.Manifest,
) (*claudePermissionLedger, bool, error) {
	if manifest == nil {
		return nil, false, nil
	}
	claim, owned := manifest.Files[claudePermissionLedgerPath]
	if !owned {
		return nil, false, nil
	}
	data, exists, err := readClaudeCleanFile(root, claudePermissionLedgerPath)
	if err != nil {
		return nil, false, fmt.Errorf("permission ledger 읽기 실패: %w", err)
	}
	if !exists {
		// A claimed ledger that is gone, as git deletes it from a clone that
		// pulls the commit untracking it, reads as none: clean retracts
		// nothing, and the next update writes a fresh ledger that re-claims
		// the generated allows settings.json holds.
		return nil, false, nil
	}
	target, pathErr := adapter.SafePruneFilePath(root, claudePermissionLedgerPath)
	if pathErr != nil {
		return nil, false, fmt.Errorf("permission ledger path 확인 실패: %w", pathErr)
	}
	info, statErr := os.Lstat(target)
	if statErr != nil {
		return nil, false, fmt.Errorf("permission ledger stat 실패: %w", statErr)
	}
	// Only a ledger an Autopus write left owner-only is trusted to retract
	// allow entries: a checkout, a copy, or a forged ledger arrives 0644.
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("permission ledger must be an owner-only regular file: %s is not a regular file",
			claudePermissionLedgerPath)
	}
	if perm := info.Mode().Perm(); !permissionLedgerModeTrusted(goos, perm) {
		return nil, false, fmt.Errorf("permission ledger must be an owner-only regular file: %s has mode %04o, not 0600; "+
			"delete it and the next auto update writes a fresh ledger", claudePermissionLedgerPath, perm)
	}
	actual := adapter.Checksum(string(data))
	if actual != claim.Checksum {
		return nil, false, fmt.Errorf("permission ledger checksum mismatch")
	}
	if err := verifyClaudePermissionLedgerTransaction(root, actual); err != nil {
		return nil, false, err
	}
	var ledger claudePermissionLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return nil, false, fmt.Errorf("permission ledger 파싱 실패: %w", err)
	}
	if ledger.Version != claudePermissionLedgerVersion {
		return nil, false, fmt.Errorf("unsupported permission ledger version %d", ledger.Version)
	}
	ledger.Allow = appendUniqueStrings(nil, ledger.Allow...)
	ledger.Deny = nil
	return &ledger, true, nil
}

// manifestClaimsLedger reports whether manifest owns the ledger path.
func manifestClaimsLedger(manifest *adapter.Manifest) bool {
	_, claimed := manifest.Files[claudePermissionLedgerPath]
	return claimed
}

// permissionLedgerModeTrusted reports whether perm marks a ledger an Autopus
// write left owner-only. Windows has no such mark: Go reports only 0666 or
// 0444 there (the read-only attribute), so the regular-file, no-symlink,
// manifest checksum, and committed-transaction checks decide alone.
func permissionLedgerModeTrusted(goos string, perm os.FileMode) bool {
	return goos == "windows" || perm == 0o600
}

func verifyClaudePermissionLedgerTransaction(root, checksum string) error {
	if err := adapter.RejectSymlinkComponents(root, filepath.Join(".autopus", "txns")); err != nil {
		return fmt.Errorf("permission ledger transaction 확인 실패: %w", err)
	}
	journals, err := adapter.ListCommittedTransactions(root)
	if err != nil {
		return fmt.Errorf("permission ledger transaction 로드 실패: %w", err)
	}
	for _, journal := range journals {
		if journal.Platform != adapterName || journal.Status != adapter.TransactionStatusCommitted {
			continue
		}
		for _, entry := range journal.Entries {
			if filepath.ToSlash(entry.Path) != claudePermissionLedgerPath ||
				entry.Operation != "write" || entry.AfterChecksum != checksum {
				continue
			}
			rel, relErr := filepath.Rel(root, journal.Path)
			if relErr != nil {
				return fmt.Errorf("permission ledger transaction path 확인 실패: %w", relErr)
			}
			if err := adapter.RejectSymlinkComponents(root, rel); err != nil {
				return fmt.Errorf("permission ledger transaction 확인 실패: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("permission ledger has no committed transaction ownership")
}

func readClaudePermissionPreimage(root string) ([]string, []string, error) {
	data, exists, err := readClaudeCleanFile(root, ".claude/settings.json")
	if err != nil {
		return nil, nil, fmt.Errorf("settings.json permission preimage 읽기 실패: %w", err)
	}
	if !exists {
		return nil, nil, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, nil, fmt.Errorf("settings.json permission preimage 파싱 실패: %w", err)
	}
	permissions, _ := settings["permissions"].(map[string]any)
	return toStringSlice(permissions["allow"]), toStringSlice(permissions["deny"]), nil
}

// ledgerClaimedAllows returns the allow entries a trusted ledger claims, or
// none when the ledger is absent or fails its ownership checks: retraction
// then falls back to the retired-block fingerprints alone.
func (a *Adapter) ledgerClaimedAllows() []string {
	manifest, err := loadClaudeManifestSafely(a.root)
	if err != nil {
		return nil
	}
	ledger, _, err := loadClaudePermissionLedger(a.root, manifest)
	if err != nil || ledger == nil {
		return nil
	}
	return ledger.Allow
}

func withoutRetiredUndesired(values, desired []string) []string {
	keep := make(map[string]bool, len(desired))
	for _, value := range desired {
		keep[value] = true
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if keep[value] || !content.IsRetiredDefaultAllow(value) {
			result = append(result, value)
		}
	}
	return result
}
