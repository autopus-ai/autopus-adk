package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// reviewWriteRefusal is a review edit the line splice writer cannot make
// safely (Line Splice Writer Contract). Nothing is written; key is the first
// refused path, which the write-refused line names.
type reviewWriteRefusal struct{ key string }

func (e reviewWriteRefusal) Error() string {
	return "refuse to edit " + e.key + " in autopus.yaml: the target is not a plain one-line value"
}

// composeReviewReloadError is a review write the run could not read back.
// Like composeReloadError, which it unwraps to, it ends the run: the run's
// config still holds the previous values, and a later save would write them
// back.
type composeReviewReloadError struct{ err error }

func (e composeReviewReloadError) Error() string {
	return "review settings were applied but could not be re-read: " + e.err.Error()
}

func (e composeReviewReloadError) Unwrap() error { return composeReloadError(e) }

// reviewTarget is one key an edit writes (Config Write Contract): the
// providers list, a scalar, or a CLI provider's args. A missing scalar or args
// key is added at the end of its mapping.
type reviewTarget struct {
	path   []string
	list   []string
	value  string
	isList bool
	// args is the args edit; its list is the planned raw list once the
	// splice has read the raw node.
	args *reviewArgsTarget
	// written marks the list elements the splice writes; nil means all of
	// them. An args element it leaves keeps its raw scalar and its tag.
	written []bool
	// remove deletes the key's line instead of writing a value
	// (SPEC-JUDGEMODEL-001); a missing key needs nothing.
	remove bool
}

// reviewArgsTarget sets a CLI provider's model and effort by row, starting
// from the migrated args when the raw args are absent or empty (REQ-RVM-25).
type reviewArgsTarget struct {
	row      reviewArgsRow
	want     reviewCLIValue
	migrated []string
}

func (t reviewTarget) key() string { return strings.Join(t.path, ".") }

// reviewTargets lists the keys whose working value differs from the
// baseline, in change line order: spec.review_gate.providers, then per
// provider its backend, model, model_policy, and args, then
// spec.review_gate.judge, judge_model, and judge_effort. Nothing equal to the
// baseline is written; a CLI change writes the flags whose raw value differs
// from the wanted one.
func reviewTargets(b reviewBaseline, working reviewComposition) []reviewTarget {
	var targets []reviewTarget
	if !slices.Equal(working.Providers, b.Providers) {
		targets = append(targets, reviewTarget{
			path: []string{"spec", "review_gate", "providers"}, list: slices.Clone(working.Providers), isList: true,
		})
	}
	for _, name := range b.Visible {
		at := func(key string) []string { return []string{"orchestra", "providers", name, key} }
		switch {
		case working.OMP[name] && !b.OMP[name]:
			targets = append(targets, reviewTarget{path: at("backend"), value: config.ProviderBackendOMP},
				reviewTarget{path: at("model"), value: working.Models[name]})
		case working.OMP[name]:
			if working.Models[name] != b.Models[name] {
				targets = append(targets, reviewTarget{path: at("model"), value: working.Models[name]})
			}
		case working.CLI[name] != b.CLI[name]:
			row := reviewArgsRowFor(b.Binaries[name])
			if row.kind == reviewRowCodex && b.Raw[name].Policy != config.ProviderModelPolicyPinned {
				targets = append(targets, reviewTarget{path: at("model_policy"), value: config.ProviderModelPolicyPinned})
			}
			targets = append(targets, reviewTarget{path: at("args"),
				args: &reviewArgsTarget{row: row, want: working.CLI[name], migrated: b.Migrated[name]}})
		}
	}
	if working.Judge != b.Judge {
		targets = append(targets, reviewTarget{path: []string{"spec", "review_gate", "judge"}, value: working.Judge})
	}
	return append(targets, reviewJudgeModelTargets(b, working)...)
}

// saveReviewComposition writes working to dir's autopus.yaml for update and
// quality (REQ-RVM-12): the bytes after pruneRetiredConfig, with only the
// planned value spans replaced, checked by verifyReviewLineEdit and written
// in one atomic replace that keeps the file mode. A refusal or failure
// leaves the file and cfg as they were; after the write cfg takes the
// orchestra and review gate of the re-read file, so no later save of the
// run writes the previous values back.
func saveReviewComposition(dir string, cfg *config.HarnessConfig, baseline reviewBaseline, working reviewComposition) error {
	targets := reviewTargets(baseline, working)
	if len(targets) == 0 {
		return nil
	}
	for _, target := range targets {
		if target.path[0] == "orchestra" && target.path[3] == "model" {
			if _, _, ok := orchestra.SplitModelSelector(target.value); !ok {
				return reviewWriteRefusal{key: target.key()}
			}
		}
	}
	path := filepath.Join(dir, "autopus.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	source, _, err := pruneRetiredConfig(original)
	if err != nil {
		return err
	}
	edited, targets, err := spliceReviewTargets(source, targets)
	if err != nil {
		return err
	}
	if err := verifyReviewLineEdit(source, edited, targets); err != nil {
		return err
	}
	if err := atomicWriteQualityConfig(path, edited); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	reloaded, err := composeReloadConfig(dir)
	if err != nil {
		return composeReviewReloadError{err: err}
	}
	cfg.Orchestra = reloaded.Orchestra
	cfg.Spec.ReviewGate = reloaded.Spec.ReviewGate
	return nil
}

// applyReviewCompositionMemory applies working to init's in-memory config,
// which init's save writes (REQ-RVM-13). The candidate is validated before
// anything is set, so a refused edit changes nothing.
func applyReviewCompositionMemory(cfg *config.HarnessConfig, baseline reviewBaseline, working reviewComposition) error {
	candidate := *cfg
	candidate.Orchestra.Providers = maps.Clone(cfg.Orchestra.Providers)
	candidate.Spec.ReviewGate.Providers = slices.Clone(working.Providers)
	candidate.Spec.ReviewGate.Judge = working.Judge
	candidate.Spec.ReviewGate.JudgeModel, candidate.Spec.ReviewGate.JudgeEffort = working.JudgeModel.Model, working.JudgeModel.Effort
	if err := applyReviewEntries(candidate.Orchestra.Providers, baseline, working); err != nil {
		return err
	}
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	cfg.Orchestra.Providers = candidate.Orchestra.Providers
	cfg.Spec.ReviewGate = candidate.Spec.ReviewGate
	return nil
}

// applyReviewEntries sets every provider entry value working changes, as the
// line writer writes it: a switch's backend and model, an OMP model, and a
// CLI change's model_policy and args, edited from the entry's args or, when
// they are empty, the migrated ones. A CLI value the planner refuses keeps
// its entry and refuses with the args key. Init's config and the live check
// candidate share it.
func applyReviewEntries(providers map[string]config.ProviderEntry, b reviewBaseline, working reviewComposition) error {
	for _, name := range b.Visible {
		entry := providers[name]
		switch {
		case working.OMP[name] && (!b.OMP[name] || working.Models[name] != b.Models[name]):
			entry.Backend, entry.Model = config.ProviderBackendOMP, working.Models[name]
		case !working.OMP[name] && working.CLI[name] != b.CLI[name]:
			row := reviewArgsRowFor(b.Binaries[name])
			if row.kind == reviewRowCodex {
				entry.ModelPolicy = config.ProviderModelPolicyPinned
			}
			args := entry.Args
			if len(args) == 0 {
				args = b.Migrated[name]
			}
			edit, ok := planReviewArgsEdit(args, row, working.CLI[name])
			if !ok {
				return reviewWriteRefusal{key: "orchestra.providers." + name + ".args"}
			}
			entry.Args = edit.apply(args)
		default:
			continue
		}
		providers[name] = entry
	}
	return nil
}
