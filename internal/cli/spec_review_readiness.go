package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// SPEC review readiness preflight (SPEC-REVIEWRO-001 REQ-09..REQ-12): status
// probes for the projected reviewers and the judge run before any provider
// executes. A not-ready judge fails the review; a not-ready reviewer is
// excluded, stays in the quorum denominator, and degrades promotion.

const (
	specReviewRoleReviewer = "reviewer"
	specReviewRoleJudge    = "judge"
)

// reviewReadinessProbe runs the status probes of the spec review preflight and
// of the auto doctor readiness checks; tests replace it.
var reviewReadinessProbe = probeProviderReadiness

// specReviewReadinessEntry is one probed reviewer or judge.
type specReviewReadinessEntry struct {
	Provider  orchestra.ProviderConfig
	Readiness providerReadinessResult
	// Excluded marks a not-ready reviewer that never executes.
	Excluded bool
}

// specReviewPreflight is the readiness outcome the loop and the receipt read.
type specReviewPreflight struct {
	// Reviewers are the assembled reviewers in selection order.
	Reviewers []specReviewReadinessEntry
	Judge     *specReviewReadinessEntry
	// Discovered are --multi providers the read-only gate excluded.
	Discovered []specReviewExclusion
}

// specReviewProviderPlan is what one review executes.
type specReviewProviderPlan struct {
	// Names is the quorum denominator; not-ready reviewers stay in it.
	Names     []string
	Providers []orchestra.ProviderConfig
	Judge     *orchestra.ProviderConfig
	Preflight *specReviewPreflight
}

// planSpecReviewProviders assembles the read-only reviewers and the judge and
// runs the readiness preflight, all before any provider executes.
func planSpecReviewProviders(
	ctx context.Context, cfg *config.HarnessConfig, judge string, opts specReviewOptions, multi bool, requestedTimeout int,
) (specReviewProviderPlan, error) {
	reviewers, err := specReviewProviderAssembly(ctx, specReviewProviderRequest{
		Config: cfg, FlagProviders: opts.providers, Multi: multi, RequestedTimeout: requestedTimeout, Warnings: os.Stderr,
	})
	if err != nil {
		return specReviewProviderPlan{}, err
	}
	if len(reviewers.Providers) == 0 {
		return specReviewProviderPlan{}, fmt.Errorf("사용 가능한 프로바이더가 없습니다. 설치를 확인하세요: %v", reviewers.Names)
	}
	if multi && len(reviewers.Providers) < 2 {
		fmt.Fprintf(os.Stderr, "경고: --multi review requested but only one provider is installed; falling back to single-provider review (resolved: %v)\n", reviewers.Names)
	}
	judgeConfig, err := specReviewJudgeAssembly(ctx, cfg, reviewers.Providers, judge, requestedTimeout)
	if err != nil {
		return specReviewProviderPlan{}, err
	}
	preflight, err := runSpecReviewPreflight(ctx, reviewers, judgeConfig, opts.skipProviderReadiness, os.Stderr)
	if err != nil {
		return specReviewProviderPlan{}, err
	}
	return specReviewProviderPlan{
		Names: reviewers.Names, Providers: preflight.readyReviewers(), Judge: judgeConfig, Preflight: preflight,
	}, nil
}

// runSpecReviewPreflight probes the reviewers and the judge in one call, so
// providers sharing a status command (every OMP-backed one) probe once.
func runSpecReviewPreflight(
	ctx context.Context, reviewers specReviewProviderSet, judge *orchestra.ProviderConfig, skip bool, w io.Writer,
) (*specReviewPreflight, error) {
	probed := append([]orchestra.ProviderConfig(nil), reviewers.Providers...)
	if judge != nil {
		probed = append(probed, *judge)
	}
	results := reviewReadinessProbe(ctx, probed, providerReadinessOptions{Skip: skip})
	preflight := &specReviewPreflight{Discovered: append([]specReviewExclusion(nil), reviewers.Excluded...)}
	for index, provider := range reviewers.Providers {
		preflight.Reviewers = append(preflight.Reviewers, specReviewReadinessEntry{
			Provider: provider, Readiness: results[index], Excluded: results[index].Status == providerReadinessNotReady,
		})
	}
	if judge != nil {
		entry := specReviewReadinessEntry{Provider: *judge, Readiness: results[len(reviewers.Providers)]}
		if entry.Readiness.Status == providerReadinessNotReady {
			return nil, errors.New(redactReadinessText(fmt.Sprintf("spec review: judge %q is not ready: %s; %s",
				judge.Name, entry.Readiness.Token(), entry.Readiness.RunRemedy())))
		}
		preflight.Judge = &entry
	}
	if len(preflight.readyReviewers()) == 0 {
		return nil, specReviewNoReadyReviewerError(preflight.Reviewers)
	}
	preflight.print(w)
	return preflight, nil
}

// specReviewNoReadyReviewerError names every distinct remedy in reviewer order.
func specReviewNoReadyReviewerError(reviewers []specReviewReadinessEntry) error {
	message := "spec review: no ready reviewer remains"
	var remedies []string
	for _, entry := range reviewers {
		if remedy := entry.Readiness.RunRemedy(); remedy != "" && !slices.Contains(remedies, remedy) {
			remedies = append(remedies, remedy)
			message += "; " + remedy
		}
	}
	return errors.New(redactReadinessText(message))
}

func (preflight *specReviewPreflight) readyReviewers() []orchestra.ProviderConfig {
	var ready []orchestra.ProviderConfig
	for _, entry := range preflight.Reviewers {
		if !entry.Excluded {
			ready = append(ready, entry.Provider)
		}
	}
	return ready
}

// print writes one `preflight: <provider> <status>` line per reviewer and the
// judge, then each distinct advisory warning; no line carries probe output.
func (preflight *specReviewPreflight) print(w io.Writer) {
	entries := append([]specReviewReadinessEntry(nil), preflight.Reviewers...)
	for _, entry := range preflight.Reviewers {
		line := entry.Readiness.PreflightLine()
		if entry.Excluded {
			line += " (excluded; degraded: " + specReviewUnreadyReason(entry) + ")"
		}
		fmt.Fprintln(w, redactReadinessText(line))
	}
	if preflight.Judge != nil {
		entries = append(entries, *preflight.Judge)
		fmt.Fprintln(w, preflight.Judge.Readiness.PreflightLine()+" ("+specReviewRoleJudge+")")
	}
	var warnings []string
	for _, entry := range entries {
		for _, warning := range entry.Readiness.Warnings {
			if !slices.Contains(warnings, warning) {
				warnings = append(warnings, warning)
				fmt.Fprintln(w, "preflight: "+redactReadinessText(warning))
			}
		}
	}
}

// specReviewUnreadyReason is the degraded reason provider_unready:<provider>:<state>.
func specReviewUnreadyReason(entry specReviewReadinessEntry) string {
	return "provider_unready:" + entry.Provider.Name + ":" + entry.Readiness.Reason
}

// applySpecReviewReadiness records the preflight exclusions on one revision:
// Provider Health names each exclusion, and its provider_unready reason joins
// the degraded reasons after the coverage and quorum reasons, ordered by
// provider name. It must run after applyObservationIntegrity, which rewrites
// DegradedReasons every revision.
func applySpecReviewReadiness(result *spec.ReviewResult, preflight *specReviewPreflight) {
	if result == nil || preflight == nil {
		return
	}
	var excluded []specReviewReadinessEntry
	for _, entry := range preflight.Reviewers {
		if entry.Excluded {
			excluded = append(excluded, entry)
		}
	}
	slices.SortStableFunc(excluded, func(a, b specReviewReadinessEntry) int {
		return strings.Compare(a.Provider.Name, b.Provider.Name)
	})
	for _, entry := range excluded {
		result.DegradedReasons = append(result.DegradedReasons, specReviewUnreadyReason(entry))
		for index := range result.ProviderStatuses {
			if result.ProviderStatuses[index].Provider == entry.Provider.Name {
				result.ProviderStatuses[index].Status = "error"
				result.ProviderStatuses[index].Note = "excluded: " + entry.Readiness.Token()
			}
		}
	}
}

// specReviewProviderPolicyRows renders the receipt rows of one executed
// revision: reviewers in selection order, discovered exclusions, then the
// judge (REQ-07).
func specReviewProviderPolicyRows(preflight *specReviewPreflight, result *orchestra.OrchestraResult) []specReviewProviderPolicyRow {
	if preflight == nil {
		return nil
	}
	rows := make([]specReviewProviderPolicyRow, 0, len(preflight.Reviewers)+len(preflight.Discovered)+1)
	for _, entry := range preflight.Reviewers {
		row := entry.policyRow(specReviewRoleReviewer)
		if !entry.Excluded {
			row.SandboxMode = executedSandboxMode(entry.Provider, entry.Provider.Name, result)
		}
		rows = append(rows, row)
	}
	for _, exclusion := range preflight.Discovered {
		rows = append(rows, specReviewProviderPolicyRow{Provider: exclusion.Provider, Role: specReviewRoleReviewer, Excluded: true})
	}
	if judge := preflight.Judge; judge != nil {
		row := judge.policyRow(specReviewRoleJudge)
		row.SandboxMode = executedSandboxMode(judge.Provider, judge.Provider.Name+specReviewJudgeSuffix, result)
		rows = append(rows, row)
	}
	return rows
}

func (entry specReviewReadinessEntry) policyRow(role string) specReviewProviderPolicyRow {
	return specReviewProviderPolicyRow{
		Provider: entry.Provider.Name, Role: role, Readiness: redactReadinessText(entry.Readiness.Token()), Excluded: entry.Excluded,
	}
}

// executedSandboxMode judges a provider's sandbox from the launch a backend
// recorded. A provider absent from the run, or without a recorded launch
// because it never started, claims no mode. Only the OMP review backend, which
// records no process argv and is read-only by construction, falls back to the
// projected config.
func executedSandboxMode(provider orchestra.ProviderConfig, name string, result *orchestra.OrchestraResult) string {
	if result == nil {
		return ""
	}
	executed := false
	for _, response := range result.Responses {
		if response.Provider == name {
			executed = true
			if mode := recordedSandboxMode(provider, response.Execution); mode != "" {
				return mode
			}
		}
	}
	for _, failed := range result.FailedProviders {
		if failed.Name == name {
			executed = true
			if mode := recordedSandboxMode(provider, failed.Execution); mode != "" {
				return mode
			}
		}
	}
	if !executed || provider.Backend != config.ProviderBackendOMP {
		return ""
	}
	return orchestra.ProviderSandboxMode(provider, provider.Args)
}

func recordedSandboxMode(provider orchestra.ProviderConfig, execution *orchestra.ProviderExecution) string {
	if execution == nil || len(execution.Command) == 0 {
		return ""
	}
	return orchestra.ProviderSandboxMode(provider, execution.Command[1:])
}
