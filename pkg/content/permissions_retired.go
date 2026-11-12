package content

// retiredAllowGroup is one block of allow entries an earlier DetectPermissions
// emitted and the current one no longer does. Fingerprint is the subset every
// generator version that wrote the block emitted together: a settings file
// holding all of it carries the block the generator wrote, even on an install
// whose ownership ledger predates it.
type retiredAllowGroup struct {
	Entries     []string
	Fingerprint []string
}

var retiredAllowGroups = []retiredAllowGroup{
	{
		Entries: []string{
			"Bash(auto *)", "Bash(auto:*)", "Bash(git *)", "Bash(git:*)",
			"Bash(make:*)", "Bash(find:*)", "Bash(gh:*)",
		},
		Fingerprint: []string{
			"Bash(auto *)", "Bash(auto:*)", "Bash(git *)", "Bash(git:*)", "Bash(make:*)",
		},
	},
	{
		Entries: []string{
			"Bash(go run:*)", "Bash(go tool:*)", "Bash(go get:*)", "Bash(go install:*)",
		},
		Fingerprint: []string{
			"Bash(go build:*)", "Bash(go run:*)", "Bash(go tool:*)",
			"Bash(go get:*)", "Bash(go install:*)",
		},
	},
	{
		Entries: []string{
			"Bash(npm *)", "Bash(npx *)", "Bash(node *)", "Bash(pnpm *)", "Bash(yarn *)",
		},
		Fingerprint: []string{
			"Bash(npm *)", "Bash(npx *)", "Bash(node *)", "Bash(pnpm *)", "Bash(yarn *)",
		},
	},
}

// IsRetiredDefaultAllow reports whether value is an allow entry an earlier
// DetectPermissions emitted and the current defaults dropped.
func IsRetiredDefaultAllow(value string) bool {
	for _, group := range retiredAllowGroups {
		for _, entry := range group.Entries {
			if entry == value {
				return true
			}
		}
	}
	return false
}

// WithoutRetiredDefaultAllows drops from existing the retired default allow
// entries the generator wrote: those claimed lists (an ownership ledger), and
// every entry of a retired block whose whole fingerprint existing holds. An
// entry in desired (current defaults plus the user's ExtraAllow) is kept, and
// so is a retired entry neither claimed nor part of a fingerprinted block,
// which reads as one the user added by hand.
func WithoutRetiredDefaultAllows(existing, desired, claimed []string) []string {
	held := stringSet(existing)
	keep := stringSet(desired)
	owned := stringSet(claimed)
	retire := make(map[string]bool)
	for _, group := range retiredAllowGroups {
		fingerprinted := true
		for _, entry := range group.Fingerprint {
			if !held[entry] {
				fingerprinted = false
				break
			}
		}
		for _, entry := range group.Entries {
			if !keep[entry] && (fingerprinted || owned[entry]) {
				retire[entry] = true
			}
		}
	}
	result := make([]string, 0, len(existing))
	for _, value := range existing {
		if !retire[value] {
			result = append(result, value)
		}
	}
	return result
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}
