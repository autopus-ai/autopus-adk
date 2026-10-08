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

// applyProviderEnv starts the provider with only the inherited variables
// that keep names, when keep is not empty, and without the ones that unset
// names. Nothing changes when both are empty; a command that cannot take an
// environment fails closed.
func applyProviderEnv(cmd command, keep, unset []string) error {
	if len(keep) == 0 && len(unset) == 0 {
		return nil
	}
	setter, ok := cmd.(envSetter)
	if !ok {
		return errProviderEnvUnsupported
	}
	env := os.Environ()
	if len(keep) > 0 {
		env = EnvironOnly(env, keep)
	}
	setter.SetEnv(EnvironWithout(env, unset))
	return nil
}

// EnvironOnly returns the entries of env whose name equals one of names,
// letter case included, the stricter reading for a keep list. A name that
// ends in * keeps every variable that starts with the text before it, such
// as LC_*; a bare * keeps nothing.
func EnvironOnly(env, names []string) []string {
	var kept []string
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		for _, name := range names {
			prefix, family := strings.CutSuffix(name, "*")
			if family && prefix != "" && strings.HasPrefix(key, prefix) || !family && key == name {
				kept = append(kept, entry)
				break
			}
		}
	}
	return kept
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
