package a2a

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/privatedir"
	"github.com/insajin/autopus-adk/pkg/worker/security"
)

// policyDir returns the private per-user directory for cached policy files,
// created with mode 0700 and verified to be the current user's own directory.
func policyDir() (string, error) {
	dir := security.PolicyCacheDir()
	if err := privatedir.Ensure(dir); err != nil {
		return "", fmt.Errorf("create policy dir: %w", err)
	}
	return dir, nil
}

// cacheSecurityPolicy writes the security policy to the private policy
// directory for subprocess access: a temp file renamed into place (SEC-001).
func cacheSecurityPolicy(taskID string, policy SecurityPolicy, signature string) error {
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}

	dir, err := policyDir()
	if err != nil {
		return err
	}
	name := fmt.Sprintf("autopus-policy-%s.json", taskID)
	if err := privatedir.WriteFile(dir, name, data); err != nil {
		return fmt.Errorf("write policy: %w", err)
	}
	return writePolicySignature(filepath.Join(dir, name), signature)
}

func removeIfExists(path string) {
	_ = os.Remove(path)
}

// marshalJSON marshals v to JSON, returning an error instead of panicking (SEC-002).
func marshalJSON(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return data, nil
}
