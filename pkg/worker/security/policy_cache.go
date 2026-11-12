package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/privatedir"
)

// PolicyCache provides file-based storage for SecurityPolicy objects.
// Policies are stored in a private per-user directory for isolation.
type PolicyCache struct {
	dir string
}

// PolicyCacheDir is the private per-user directory that holds the cached
// SecurityPolicy files of worker tasks and their signature sidecars.
func PolicyCacheDir() string { return privatedir.Path("worker-policy") }

// NewPolicyCache creates a PolicyCache using a secure per-user directory.
func NewPolicyCache() *PolicyCache {
	return &PolicyCache{dir: PolicyCacheDir()}
}

// Write atomically writes a SecurityPolicy to disk with mode 0600.
// Uses temp file + rename to prevent partial reads.
// Rejects writes to symlink targets (symlink defense).
func (c *PolicyCache) Write(taskID string, policy SecurityPolicy) error {
	return c.WriteWithLstatGuard(taskID, policy)
}

// WriteWithLstatGuard writes a SecurityPolicy after verifying the target
// path is not a symlink. Uses Lstat to detect symlinks before writing.
func (c *PolicyCache) WriteWithLstatGuard(taskID string, policy SecurityPolicy) error {
	target := c.PolicyPath(taskID)

	// Check directory for symlinks
	if err := checkSymlink(c.dir); err != nil {
		return err
	}

	// Check target path for symlinks (only if file already exists)
	if err := checkSymlink(target); err != nil {
		return err
	}

	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	// The directory must be a private directory of the current user; the
	// file goes through a temp file and a rename, so it is never followed.
	if err := privatedir.Ensure(c.dir); err != nil {
		return fmt.Errorf("create policy dir: %w", err)
	}
	if err := privatedir.WriteFile(c.dir, filepath.Base(target), data); err != nil {
		return fmt.Errorf("write policy: %w", err)
	}
	return nil
}

// checkSymlink uses Lstat to detect if a path is a symlink.
// Returns error only if the path is confirmed to be a symlink.
// Non-existent paths or other errors are treated as safe (non-symlink).
func checkSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		// Path doesn't exist or can't be stat'd — not a symlink concern
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink detected at policy path: refusing to write")
	}
	return nil
}

// Read loads a SecurityPolicy from disk for the given task ID.
// Returns an error if the file does not exist or cannot be parsed.
func (c *PolicyCache) Read(taskID string) (*SecurityPolicy, error) {
	data, err := privatedir.ReadFile(c.dir, filepath.Base(c.PolicyPath(taskID)))
	if err != nil {
		return nil, fmt.Errorf("read policy for %s: %w", taskID, err)
	}

	var policy SecurityPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("unmarshal policy for %s: %w", taskID, err)
	}
	return &policy, nil
}

// Delete removes the cached policy file for the given task ID.
// Ignores not-exist errors.
func (c *PolicyCache) Delete(taskID string) {
	_ = os.Remove(c.PolicyPath(taskID))
}

// PolicyPath returns the file path for the given task ID's policy.
// Uses filepath.Base to prevent path traversal via malicious task IDs.
func (c *PolicyCache) PolicyPath(taskID string) string {
	safe := filepath.Base(taskID)
	return filepath.Join(c.dir, fmt.Sprintf("autopus-policy-%s.json", safe))
}
