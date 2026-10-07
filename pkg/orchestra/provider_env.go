package orchestra

import (
	"errors"
	"os"
	"strings"
)

// errProviderEnvUnsupported refuses a provider whose command cannot take a
// filtered environment: it would otherwise start with every variable.
var errProviderEnvUnsupported = errors.New("orchestra: provider command cannot drop inherited environment variables")

// envSetter is the optional command capability of a replaced environment.
type envSetter interface{ SetEnv(env []string) }

// applyProviderEnv starts the provider without the inherited variables that
// unset names. Nothing changes when unset is empty; a command that cannot
// take an environment fails closed.
func applyProviderEnv(cmd command, unset []string) error {
	if len(unset) == 0 {
		return nil
	}
	setter, ok := cmd.(envSetter)
	if !ok {
		return errProviderEnvUnsupported
	}
	setter.SetEnv(EnvironWithout(os.Environ(), unset))
	return nil
}

// EnvironWithout returns env without the entries whose name matches one of
// names, compared case-insensitively. A name that ends in * matches every
// variable that starts with the text before it, such as AWS_CONTAINER_*.
func EnvironWithout(env, names []string) []string {
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		drop := false
		for _, name := range names {
			if envNameMatches(key, name) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, entry)
		}
	}
	return kept
}

// envNameMatches compares key with name, or with its prefix when name ends
// in *, case-insensitively.
func envNameMatches(key, name string) bool {
	prefix, family := strings.CutSuffix(name, "*")
	if !family {
		return strings.EqualFold(key, name)
	}
	return prefix != "" && len(key) >= len(prefix) && strings.EqualFold(key[:len(prefix)], prefix)
}
