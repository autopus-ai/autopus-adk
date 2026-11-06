package config

import (
	"path/filepath"
	"slices"
	"strings"
)

const (
	// OutputFormatFlag is the flag agy and claude both take for their stdout format.
	OutputFormatFlag = "--output-format"
	// AgyOutputFormat is the stdout format the shipped agy provider asks for.
	// stream-json ends in a terminal result event, so the orchestra can tell a
	// finished answer from a process that only failed to exit (agy 1.5.0 can
	// keep its language server up after the answer), and the result carries
	// the token usage that text output lacks.
	AgyOutputFormat = "stream-json"
	// agyLegacyOutputFormat is the format the shipped agy default used to run:
	// it has no completion marker and no usage, so migration replaces it.
	agyLegacyOutputFormat = "text"
)

// DefaultGeminiProviderEntry returns the canonical gemini (agy) orchestra
// provider entry. SPEC-ORCH-021 REQ-014/015: the prompt is the value of
// --print, injected into the "" slot via PromptViaArgs. Subprocess
// OutputFormat describes the response text inside the stream envelope.
func DefaultGeminiProviderEntry() ProviderEntry {
	return ProviderEntry{
		Binary:        "agy",
		Args:          []string{"--print", "", OutputFormatFlag, AgyOutputFormat},
		PromptViaArgs: true,
		Subprocess:    SubprocessProvConf{OutputFormat: "text", Timeout: GeminiOrchestraTimeoutSeconds},
	}
}

// outputFormatValue finds the last --output-format value before `--`, in
// separated or inline form. index is the element holding the value.
func outputFormatValue(args []string) (value string, index int, found bool) {
	index = -1
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		switch {
		case args[i] == OutputFormatFlag && i+1 < len(args):
			value, index, found = args[i+1], i+1, true
			i++
		case strings.HasPrefix(args[i], OutputFormatFlag+"="):
			value, index, found = strings.TrimPrefix(args[i], OutputFormatFlag+"="), i, true
		}
	}
	return value, index, found
}

// withOutputFormat sets the --output-format of args to format: it adds the
// pair at outputFormatInsertAt when absent and rewrites the value in place when it equals replace.
// Every other element, including a user's --model, keeps its place. Any other
// explicit value is a user choice and is kept.
func withOutputFormat(args []string, format, replace string) ([]string, bool) {
	value, index, found := outputFormatValue(args)
	switch {
	case !found:
		at := outputFormatInsertAt(args)
		return slices.Insert(slices.Clone(args), at, OutputFormatFlag, format), true
	case strings.TrimSpace(value) == replace && replace != format:
		next := slices.Clone(args)
		if strings.HasPrefix(next[index], OutputFormatFlag+"=") {
			next[index] = OutputFormatFlag + "=" + format
		} else {
			next[index] = format
		}
		return next, true
	}
	return args, false
}

// migrateAgyOutputFormat moves an agy entry onto the stream-json envelope.
// Only the agy binary itself is touched: a wrapper's flag contract is not the
// harness's to rewrite.
func migrateAgyOutputFormat(entry ProviderEntry) (ProviderEntry, bool) {
	if entry.Backend != "" || strings.TrimSuffix(filepath.Base(entry.Binary), ".exe") != "agy" {
		return entry, false
	}
	args, changed := withOutputFormat(entry.Args, AgyOutputFormat, agyLegacyOutputFormat)
	entry.Args = args
	return entry, changed
}

// outputFormatInsertAt is where an absent --output-format pair goes: the end
// of the flags, which is before a `--` separator (flags after it are
// positional, so the pair would never be found again and every migration
// would add another) and before a trailing print flag whose value the prompt
// fills (agy `--print` with no "" slot takes the prompt appended after it).
func outputFormatInsertAt(args []string) int {
	at := slices.Index(args, "--")
	if at < 0 {
		at = len(args)
	}
	if at > 0 && (args[at-1] == "--print" || args[at-1] == "-p") {
		at--
	}
	return at
}
