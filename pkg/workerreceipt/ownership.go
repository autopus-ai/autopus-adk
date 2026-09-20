package workerreceipt

import (
	"fmt"
	"strings"
)

// ValidateOwnership checks lexical scope, not actual edits or OS permissions.
// A nil assignment checks only the worker's own declarations. A non-nil
// assignment additionally binds them to supervisor-supplied roots; empty means
// no write scope. Roots are literal paths or a literal subtree ending in /**.
func ValidateOwnership(receipt Receipt, assigned []string) error {
	owned, err := ownershipRoots(receipt.OwnedPaths)
	if err != nil {
		return err
	}
	if assigned != nil {
		approved, err := ownershipRoots(assigned)
		if err != nil {
			return err
		}
		for _, root := range owned {
			if !withinOwnership(root, approved) {
				return fmt.Errorf("worker receipt declares ownership outside supervisor assignment")
			}
		}
	}
	for _, changed := range receipt.ChangedFiles {
		if err := validateReceiptReference(changed); err != nil {
			return err
		}
		if strings.ContainsAny(changed, "*?[]\r\n") {
			return fmt.Errorf("worker receipt changed files must be literal paths")
		}
		if !withinOwnership(changed, owned) {
			return fmt.Errorf("worker receipt changed file is outside declared ownership")
		}
	}
	return nil
}

func ownershipRoots(paths []string) ([]string, error) {
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := validateReceiptReference(path); err != nil {
			return nil, err
		}
		root := strings.TrimSuffix(path, "/**")
		if strings.ContainsAny(root, "*?[]\r\n") {
			return nil, fmt.Errorf("worker receipt ownership must use literal roots or trailing /**")
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func withinOwnership(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}
