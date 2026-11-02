package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

type specReviewLedgerKey struct{}

// specReviewCallScope is the call metadata the review loop puts on the
// context: the round, whether the attempt is a lean fallback, and an explicit
// reason such as json_repair. pkg/orchestra's request types are not this
// package's to extend, so the metadata travels on the context instead.
type specReviewCallScope struct {
	round    int
	fallback bool
	reason   string
}

type specReviewCallScopeKey struct{}

func withSpecReviewLedger(ctx context.Context, ledger *specReviewLedger) context.Context {
	return context.WithValue(ctx, specReviewLedgerKey{}, ledger)
}

func specReviewLedgerFrom(ctx context.Context) *specReviewLedger {
	if ctx == nil {
		return nil
	}
	ledger, _ := ctx.Value(specReviewLedgerKey{}).(*specReviewLedger)
	return ledger
}

func specReviewScopeFrom(ctx context.Context) specReviewCallScope {
	scope, _ := ctx.Value(specReviewCallScopeKey{}).(specReviewCallScope)
	return scope
}

func withSpecReviewRound(ctx context.Context, round int) context.Context {
	scope := specReviewScopeFrom(ctx)
	scope.round = round
	return context.WithValue(ctx, specReviewCallScopeKey{}, scope)
}

func withSpecReviewFallback(ctx context.Context) context.Context {
	scope := specReviewScopeFrom(ctx)
	scope.fallback = true
	return context.WithValue(ctx, specReviewCallScopeKey{}, scope)
}

func withSpecReviewCallReason(ctx context.Context, reason string) context.Context {
	scope := specReviewScopeFrom(ctx)
	scope.reason = reason
	return context.WithValue(ctx, specReviewCallScopeKey{}, scope)
}

// specReviewCallSpecFor names the role and reason of one request.
func specReviewCallSpecFor(ctx context.Context, req orchestra.ProviderRequest) specReviewCallSpec {
	scope := specReviewScopeFrom(ctx)
	reason := scope.reason
	switch {
	case reason != "":
	case req.Role == specReviewRoleJudge:
		reason = specReviewReasonAdjudicate
	case scope.fallback:
		reason = specReviewReasonFallback
	case scope.round > 0:
		reason = specReviewReasonVerify
	default:
		reason = specReviewReasonReview
	}
	role := req.Role
	if role == "" {
		role = specReviewRoleReviewer
	}
	return specReviewCallSpec{Round: scope.round, Role: role, Reason: reason, Provider: req.Provider}
}

// meteredSpecReviewBackend reserves budget before every model call and
// settles it after (REQ-RP-BUD-03). It is the one seam every reviewer, JSON
// repair, and judge call passes through.
type meteredSpecReviewBackend struct {
	inner  orchestra.ExecutionBackend
	ledger *specReviewLedger
}

// meterSpecReviewBackend wraps backend when ctx carries a review session.
func meterSpecReviewBackend(ctx context.Context, backend orchestra.ExecutionBackend) orchestra.ExecutionBackend {
	ledger := specReviewLedgerFrom(ctx)
	if ledger == nil || backend == nil {
		return backend
	}
	return &meteredSpecReviewBackend{inner: backend, ledger: ledger}
}

func (b *meteredSpecReviewBackend) Name() string { return b.inner.Name() }

// BackendNameFor delegates so the wrapper never changes the backend name a
// provider is reported under.
func (b *meteredSpecReviewBackend) BackendNameFor(provider orchestra.ProviderConfig) string {
	if resolver, ok := b.inner.(orchestra.ProviderBackendResolver); ok {
		return resolver.BackendNameFor(provider)
	}
	return b.inner.Name()
}

func (b *meteredSpecReviewBackend) Execute(ctx context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	index, err := b.ledger.reserve(specReviewCallSpecFor(ctx, req))
	if err != nil {
		return nil, err
	}
	started := b.ledger.now()
	resp, execErr := b.inner.Execute(ctx, req)
	b.ledger.settle(ctx, index, started, resp, execErr)
	return resp, execErr
}

// settle records how the call ended. A provider that reported no usage stays
// "unknown" with null counters.
func (l *specReviewLedger) settle(ctx context.Context, index int, started time.Time, resp *orchestra.ProviderResponse, execErr error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	row := &l.file.Calls[index]
	row.DurationMS = l.now().Sub(started).Milliseconds()
	row.EndReason = l.endReasonLocked(ctx, resp, execErr)
	row.Usage, row.Model, row.Effort = specReviewUsageFrom(resp)
	_ = l.persistLocked()
}

func (l *specReviewLedger) endReasonLocked(ctx context.Context, resp *orchestra.ProviderResponse, execErr error) string {
	if execErr == nil && resp != nil && !resp.TimedOut && resp.ExitCode == 0 && !resp.EmptyOutput {
		return specReviewCallCompleted
	}
	timedOut := (resp != nil && resp.TimedOut) || errors.Is(execErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)
	switch {
	case timedOut && !l.deadline.IsZero() && !l.now().Before(l.deadline):
		return specReviewCallExhausted
	case errors.Is(execErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		return specReviewCallCancelled
	case timedOut:
		return specReviewCallTimeout
	}
	return specReviewCallFailed
}

// specReviewUsageFrom maps the response's usage envelopes. Envelopes of one
// status are summed; any missing counter makes the sum unknown.
func specReviewUsageFrom(resp *orchestra.ProviderResponse) (specReviewCallUsage, string, string) {
	usage := specReviewCallUsage{Status: "unknown"}
	model, effort := "unknown", "unknown"
	if resp == nil {
		return usage, model, effort
	}
	for _, envelope := range resp.Usage {
		if model == "unknown" && strings.TrimSpace(envelope.Model) != "" {
			model = strings.TrimSpace(envelope.Model)
		}
		if effort == "unknown" && strings.TrimSpace(envelope.Effort) != "" {
			effort = strings.TrimSpace(envelope.Effort)
		}
		status, cost := "", envelope.EstimatedCostUSD
		switch envelope.UsageStatus {
		case telemetry.UsageStatusActual, telemetry.UsageStatusCostOnly:
			status, cost = "actual", envelope.ActualCostUSD
		case telemetry.UsageStatusEstimated:
			status = "estimated"
		default:
			continue
		}
		next := specReviewCallUsage{Status: status, InputTokens: envelope.InputTokensTotal, OutputTokens: envelope.OutputTokensTotal, CostUSD: cost}
		usage = mergeSpecReviewUsage(usage, next)
	}
	return usage, model, effort
}

// mergeSpecReviewUsage folds one envelope into the running usage. Actual wins
// over estimated; two envelopes of the same status add.
func mergeSpecReviewUsage(current, next specReviewCallUsage) specReviewCallUsage {
	switch {
	case current.Status == "unknown", current.Status == "estimated" && next.Status == "actual":
		return next
	case current.Status != next.Status:
		return current
	}
	return specReviewCallUsage{
		Status: current.Status, InputTokens: addOptionalInt(current.InputTokens, next.InputTokens),
		OutputTokens: addOptionalInt(current.OutputTokens, next.OutputTokens), CostUSD: addOptionalFloat(current.CostUSD, next.CostUSD),
	}
}

func addOptionalInt(a, b *int64) *int64 {
	if a == nil || b == nil {
		return nil
	}
	sum := *a + *b
	return &sum
}

func addOptionalFloat(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	sum := *a + *b
	return &sum
}
