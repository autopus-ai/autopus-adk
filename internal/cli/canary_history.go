package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Canary history (SPEC-SIGMABAND-001 REQ-03): every executed canary run
// appends one canary.failure_rate:<target> observation for auto react band.

// canaryLocalTarget is the target of a run given no URL.
const canaryLocalTarget = "local"

// canaryUnknownHost stands for a URL whose host cannot be parsed; the raw
// text is never used, because it may carry credentials.
const canaryUnknownHost = "unknown-host"

// canaryHistoryAppend is the append step behind recordCanaryHistory. The
// parity tests swap it for a no-op to compare runs with history enabled and
// disabled; production never replaces it.
var canaryHistoryAppend = appendCanaryHistory

// recordCanaryHistory appends the finished run to the metric history. It
// runs after runCanary returns and after the verdict is final, reads the
// result without changing it, and reports a failure only as one stderr line,
// so latest.json, stdout, the JSON envelope, and the exit code are unchanged.
func recordCanaryHistory(cmd *cobra.Command, opts canaryOptions, result canaryResult) {
	series, value, ok := canaryHistorySample(opts, result)
	if !ok {
		return
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	projectDir, err := filepath.Abs(defaultString(opts.projectDir, "."))
	if err == nil {
		err = canaryHistoryAppend(ctx, projectDir, series, value)
	}
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: canary history append failed: %v\n", err)
	}
}

// canaryHistorySample returns the series and value of a finished run, or
// false when the run executed no check: a dry run, or a run whose every
// check is SKIPPED or never set (no_checks_executed). The value is 1 only
// for verdict FAIL; WARN counts as 0 (research Q4).
func canaryHistorySample(opts canaryOptions, result canaryResult) (string, float64, bool) {
	if opts.dryRun || !canaryExecutedCheck(result) {
		return "", 0, false
	}
	series, _ := healthband.CanarySeriesID(canaryHistoryTarget(resolveCanaryTargets(opts)))
	if result.Verdict == "FAIL" {
		return series, 1, true
	}
	return series, 0, true
}

// canaryExecutedCheck reports whether any check status is PASS, WARN, or
// FAIL, which includes the build and harness early returns.
func canaryExecutedCheck(result canaryResult) bool {
	for _, status := range []string{result.Build, result.E2E, result.Doctor, result.Endpoint, result.Browser} {
		if status == "PASS" || status == "WARN" || status == "FAIL" {
			return true
		}
	}
	return false
}

// canaryHistoryTarget is the sorted, de-duplicated set of lowercase API and
// frontend hosts with default ports removed, joined by "+", or "local" when
// no URL is given (Detector Contract item 2). The caller still filters it
// as an untrusted identifier.
func canaryHistoryTarget(targets canaryTargets) string {
	var hosts []string
	for _, raw := range []string{targets.APIURL, targets.FrontendURL} {
		if raw != "" {
			hosts = append(hosts, canaryHistoryHost(raw))
		}
	}
	if len(hosts) == 0 {
		return canaryLocalTarget
	}
	slices.Sort(hosts)
	return strings.Join(slices.Compact(hosts), "+")
}

// canaryHistoryHost is the lowercase host of one URL, with the port kept
// unless it is the default of the scheme (443 for https, 80 for http). A
// URL without a scheme is read as host[:port][/path]. User info is dropped.
func canaryHistoryHost(raw string) string {
	if !strings.Contains(raw, "://") {
		raw = "//" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return canaryUnknownHost
	}
	host, port := strings.ToLower(parsed.Hostname()), parsed.Port()
	if port == "" || (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		return host
	}
	return net.JoinHostPort(host, port)
}

// appendCanaryHistory appends one observation under the store lock, waiting
// at most the store lock wait. Band holds the lock only for local file IO,
// so a canary run is never blocked by band's network or provider steps.
func appendCanaryHistory(ctx context.Context, projectDir, series string, value float64) error {
	locked, err := healthband.NewStore(projectDir).Lock(ctx, healthband.StoreLockWait)
	if err != nil {
		return err
	}
	_, appendErr := locked.AppendCanary(series, time.Now(), value)
	return errors.Join(appendErr, locked.Unlock())
}
