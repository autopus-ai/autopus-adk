package harneval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/insajin/autopus-adk/pkg/content"
)

// RunOptions holds the seams of one deterministic run. A nil field selects
// the production behavior.
type RunOptions struct {
	// Adapters builds the platform adapters; the default is PinnedAdapters.
	Adapters AdapterFactory
	// StaleCheck returns the committed template paths a regeneration of
	// content/ would change; the default is content.DetectTemplateRegenDrift.
	StaleCheck func(root string) ([]string, error)
	// Mutate edits the generated surfaces after generation and before any
	// assertion; the seeded-mutation self-test uses it (REQ-HE-12).
	Mutate func(*Generation) error
	// Evaluate evaluates one active surface task and reports whether it ran;
	// the default is EvaluateTask, which always runs.
	Evaluate func(Task, *Generation) (TaskOutcome, bool)
	// Now stamps produced_at; the default is time.Now.
	Now func() time.Time
	// Baseline loads the committed baseline; the default is LoadBaseline.
	// `auto eval harness baseline --init` substitutes an empty baseline,
	// since the first baseline comes from a run that has none to compare.
	Baseline func(root string) (*Baseline, error)
}

func (o RunOptions) withDefaults() RunOptions {
	if o.Adapters == nil {
		o.Adapters = PinnedAdapters
	}
	if o.StaleCheck == nil {
		o.StaleCheck = content.DetectTemplateRegenDrift
	}
	if o.Evaluate == nil {
		o.Evaluate = func(task Task, generation *Generation) (TaskOutcome, bool) {
			return EvaluateTask(task, generation), true
		}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Baseline == nil {
		o.Baseline = LoadBaseline
	}
	return o
}

// Run is the deterministic lane over the repository tree at root, in the
// REQ-HE-04 order: load the set and baseline, refuse stale templates,
// generate every variant surface under the sentinel, apply the mutation seam,
// evaluate the active surface tasks, then check vacuity and compare with the
// baseline. A precondition stops the run with its reason alone. An error is
// returned only when no result document can be produced at all.
func Run(ctx context.Context, root string, opts RunOptions) (*Result, error) {
	opts = opts.withDefaults()
	result, err := run(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	result.ProducedAt = opts.Now().UTC().Format(time.RFC3339)
	return result, nil
}

func run(ctx context.Context, root string, opts RunOptions) (*Result, error) {
	set, err := LoadSet(root)
	if err != nil {
		return loadFailure(err)
	}
	baseline, err := opts.Baseline(root)
	if err != nil {
		return loadFailure(err)
	}
	stale, err := opts.StaleCheck(root)
	if err != nil {
		return precondition(ReasonTemplatesStale, []string{err.Error()}, DetailRegenFailed), nil
	}
	if len(stale) > 0 {
		return precondition(ReasonTemplatesStale, nil, stale...), nil
	}
	generation, err := Generate(ctx, set, opts.Adapters)
	if err != nil {
		return generationFailure(err)
	}
	defer func() { _ = generation.Close() }()
	if opts.Mutate != nil {
		if err := opts.Mutate(generation); err != nil {
			return nil, fmt.Errorf("mutate surface: %w", err)
		}
	}
	surfaceDigest, err := SurfaceDigest(generation.Surfaces[""].Root)
	if err != nil {
		return nil, err
	}
	result := compare(set, baseline, evaluate(set, generation, opts.Evaluate))
	result.SurfaceDigest = surfaceDigest
	return result, nil
}

// evaluate runs every active surface task and returns the pass/fail result of
// each task that was executed.
func evaluate(set *Set, generation *Generation, evaluator func(Task, *Generation) (TaskOutcome, bool)) map[string]bool {
	results := map[string]bool{}
	for _, task := range set.Tasks {
		if task.Kind != KindSurface || task.Status.State != StateActive {
			continue
		}
		if outcome, executed := evaluator(task, generation); executed {
			results[task.ID] = outcome.Passed
		}
	}
	return results
}

// loadFailure maps a load error to its precondition: baseline_missing, or
// invalid with the defect's detail code. Notes keep the full loader message.
func loadFailure(err error) (*Result, error) {
	if errors.Is(err, ErrBaselineMissing) {
		return precondition(ReasonBaselineMissing, nil), nil
	}
	var invalid *InvalidError
	if errors.As(err, &invalid) {
		return precondition(ReasonInvalid, []string{err.Error()}, invalid.Detail), nil
	}
	return nil, err
}

// generationFailure maps a generation error to host_probe_unpinned with the
// probed binaries, or to generation_failed with the failed platform.
func generationFailure(err error) (*Result, error) {
	var probe *UnpinnedProbeError
	if errors.As(err, &probe) {
		result := precondition(ReasonHostProbeUnpinned, nil, probe.Binaries...)
		result.SentinelLog = probe.Invocations
		return result, nil
	}
	var failed *GenerationError
	if errors.As(err, &failed) {
		return precondition(ReasonGenerationFailed, []string{err.Error()}, failed.Detail()), nil
	}
	return nil, err
}

// precondition is a failed result carrying one reason and its details only.
func precondition(reason string, notes []string, details ...string) *Result {
	return &Result{
		SchemaVersion:  ResultSchemaV1,
		Status:         StatusFail,
		FailureReasons: []string{reason},
		Details:        sortedUnique(details),
		Notes:          notes,
	}
}

func sortedUnique(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	sort.Strings(unique)
	return unique
}
