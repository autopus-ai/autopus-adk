package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Output of auto react band: one text row or one band.<series> check per
// planned series (REQ-20, FR-16). Every value is a number, a filtered ID, or
// a reason code (Untrusted Input Contract item 9); the JSON data, the check
// fields, and the text row show the same values.

// bandClaimPlanned is the claim status of a --dry-run, which runs no claim.
const bandClaimPlanned = "planned"

// bandReport is the outcome of one band run.
type bandReport struct {
	DryRun        bool                  `json:"dry_run"`
	CI            bandCIReport          `json:"ci"`
	Reasons       []string              `json:"reasons,omitempty"`   // run-level reason codes
	NotFound      []string              `json:"not_found,omitempty"` // filtered --series IDs that name no series
	Read          healthband.ReadCounts `json:"read"`
	SkippedEvents int                   `json:"skipped_events,omitempty"`
	Interrupted   int                   `json:"interrupted,omitempty"` // claims this run marked interrupted
	Constants     healthband.Constants  `json:"constants"`
	Series        []bandSeriesResult    `json:"series"`
}

// bandCIReport is the CI network step: the runs gh listed, those the trusted
// filter kept, and the observations appended (planned under --dry-run).
type bandCIReport struct {
	Fetched  bool   `json:"fetched"`
	Reason   string `json:"reason,omitempty"`
	Rows     int    `json:"rows"`
	Trusted  int    `json:"trusted"`
	Appended int    `json:"appended"`
}

// bandSeriesResult is one series: the newest position evaluated in this run
// (the checkpoint key when none was) and the claim this run planned for it.
type bandSeriesResult struct {
	healthband.Evaluation
	Events          int    `json:"events"`
	Action          string `json:"action,omitempty"`
	PlannedAction   string `json:"planned_action,omitempty"`
	EpisodeID       string `json:"episode_id,omitempty"`
	ClaimKey        string `json:"claim_key,omitempty"`
	ClaimStatus     string `json:"claim_status,omitempty"`
	DiagnosisStatus string `json:"diagnosis_status,omitempty"`
	BSID            string `json:"bs_id,omitempty"`
	ResultPending   bool   `json:"result_pending,omitempty"`
	open            bool   // an episode is open after the newest evaluated position
}

func (r *bandReport) setCI(fetched bool, fetch bandCIFetch) {
	r.CI = bandCIReport{Fetched: fetched, Reason: fetch.Reason, Rows: fetch.Rows, Trusted: len(fetch.Observations)}
	if fetch.Reason != "" {
		r.Reasons = append(r.Reasons, fetch.Reason)
	}
}

// addPlan adds the read counts, the unknown --series entries, and one result
// per planned series, in the plan's series order.
func (r *bandReport) addPlan(planned bandPlanned, fetch bandCIFetch) {
	r.CI.Appended, r.Read = planned.fresh, planned.counts
	r.SkippedEvents, r.Interrupted = planned.skipped, planned.interrupted
	for _, id := range planned.plan.NotFound {
		filtered, _ := healthband.SanitizeIdentifier(id)
		r.NotFound = append(r.NotFound, filtered)
	}
	if len(r.NotFound) > 0 {
		r.Reasons = append(r.Reasons, healthband.ReasonSeriesNotFound)
	}
	for _, series := range planned.plan.Series {
		result := bandSeriesResult{Evaluation: healthband.Evaluation{Series: series.Series}, Events: len(series.Events)}
		if n := len(series.Events); n > 0 {
			result.setEvent(series.Events[n-1], r.DryRun)
		} else {
			state := planned.state.Series[series.Series]
			result.SampleKey = state.LastKey
			if n := len(state.Episodes); n > 0 && state.Episodes[n-1].Open {
				result.EpisodeID, result.open = state.Episodes[n-1].ID, true
			}
		}
		if series.Late > 0 {
			result.addReason(healthband.ReasonLateObservation)
		}
		for _, reason := range fetch.SeriesReasons[series.Series] {
			result.addReason(reason)
		}
		r.Series = append(r.Series, result)
	}
}

func (r *bandSeriesResult) setEvent(event healthband.Event, dryRun bool) {
	r.Evaluation = event.Evaluation
	r.Constants, r.Reasons = nil, slices.Clone(event.Reasons)
	r.EpisodeID = event.EpisodeID
	r.open = event.EpisodeID != "" && !slices.Contains(event.Reasons, healthband.ReasonEpisodeClosed)
	if dryRun {
		r.PlannedAction = event.Action
	} else {
		r.Action = event.Action
	}
}

func (r *bandSeriesResult) addReason(reason string) {
	if !slices.Contains(r.Reasons, reason) {
		r.Reasons = append(r.Reasons, reason)
	}
}

// addClaims gives each claim's series its claim: planned under --dry-run,
// still claimed when the run ended before it, otherwise its outcome and how
// phase C recorded it. recorded[i] belongs to claims[i].
func (r *bandReport) addClaims(claims []healthband.DueClaim, outcomes map[string]healthband.ClaimOutcome, recorded []healthband.Recorded) {
	for i, claim := range claims {
		at := slices.IndexFunc(r.Series, func(result bandSeriesResult) bool { return result.Series == claim.Series })
		if at < 0 {
			continue
		}
		result := &r.Series[at]
		result.ClaimKey = claim.SampleKey
		outcome, ran := outcomes[claim.ID]
		switch {
		case r.DryRun:
			result.ClaimStatus = bandClaimPlanned
		case !ran:
			result.ClaimStatus = healthband.ClaimClaimed
		default:
			result.ClaimStatus = defaultString(outcome.Status, healthband.ClaimDone)
			result.DiagnosisStatus, result.BSID = outcome.DiagnosisStatus, outcome.BSID
		}
		if i < len(recorded) {
			result.ResultPending = recorded[i].Pending != ""
			if recorded[i].Reason != "" {
				result.addReason(recorded[i].Reason)
			}
		}
	}
}

// row is the REQ-20 text row; an absent value prints "-".
func (r bandSeriesResult) row() string {
	action := "action=" + defaultString(r.Action, "-")
	if r.PlannedAction != "" {
		action = "planned_action=" + r.PlannedAction
	}
	return fmt.Sprintf("%s n=%s/%d x=%s μ=%s sd_eff=%s z=%s tier=%s %s episode=%s",
		r.Series, bandIntText(r.N), healthband.MinBaseline, bandFloatText(r.X), bandFloatText(r.Mu),
		bandFloatText(r.SDEff), bandFloatText(r.Z), bandIntText(r.Tier), action, defaultString(r.EpisodeID, "-"))
}

func bandIntText(v *int) string {
	if v == nil {
		return "-"
	}
	return strconv.Itoa(*v)
}

func bandFloatText(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'f', 6, 64)
}

// fields flattens the result into check fields: a number keeps its JSON
// text, so it parses as the value data.series holds; a string is unquoted
// and a reason list comma-joined. A value JSON cannot hold yields nil.
func (r bandSeriesResult) fields() map[string]string {
	data, err := json.Marshal(r)
	var raw map[string]json.RawMessage
	if err != nil || json.Unmarshal(data, &raw) != nil {
		return nil
	}
	fields := make(map[string]string, len(raw))
	for key, value := range raw {
		var text string
		var list []string
		switch {
		case json.Unmarshal(value, &text) == nil:
			fields[key] = text
		case json.Unmarshal(value, &list) == nil:
			fields[key] = strings.Join(list, ",")
		default:
			fields[key] = string(value)
		}
	}
	return fields
}

// status is the check status: skip when this run evaluated nothing, warn
// inside an open episode, pass otherwise.
func (r bandSeriesResult) status() string {
	switch {
	case r.Events == 0:
		return "skip"
	case r.open:
		return "warn"
	}
	return "pass"
}

// status is the envelope status: warn on any run-level reason or check.
func (r bandReport) status() jsonEnvelopeStatus {
	if len(r.Reasons) > 0 || slices.ContainsFunc(r.Series, func(s bandSeriesResult) bool { return s.status() == "warn" }) {
		return jsonStatusWarn
	}
	return jsonStatusOK
}

func (r bandReport) checks() []jsonCheck {
	checks := make([]jsonCheck, 0, len(r.Series))
	for _, result := range r.Series {
		checks = append(checks, jsonCheck{ID: "band." + result.Series, Status: result.status(), Detail: result.row(), Fields: result.fields()})
	}
	return checks
}

// printBandText writes the source line, any skipped or interrupted counts,
// the run-level reasons the source line does not name, and the rows.
func printBandText(w io.Writer, r bandReport) {
	if r.DryRun {
		fmt.Fprintln(w, "band: dry run; nothing written, no lock taken, no provider called")
	}
	switch {
	case !r.CI.Fetched:
		fmt.Fprintln(w, "ci: not fetched (--no-fetch)")
	case r.CI.Reason != "":
		fmt.Fprintln(w, "ci: skipped "+r.CI.Reason)
	default:
		fmt.Fprintf(w, "ci: %d runs, %d trusted, %d new\n", r.CI.Rows, r.CI.Trusted, r.CI.Appended)
	}
	if r.Read.Skipped() > 0 {
		fmt.Fprintf(w, "store: skipped malformed=%d unknown_schema=%d invalid_value=%d\n", r.Read.Malformed, r.Read.UnknownSchema, r.Read.InvalidValue)
	}
	if r.SkippedEvents > 0 {
		fmt.Fprintf(w, "events: skipped %d\n", r.SkippedEvents)
	}
	if r.Interrupted > 0 {
		fmt.Fprintf(w, "claims: interrupted %d\n", r.Interrupted)
	}
	for _, reason := range r.Reasons {
		switch reason {
		case r.CI.Reason:
		case healthband.ReasonSeriesNotFound:
			fmt.Fprintln(w, "reason: "+reason+" "+strings.Join(r.NotFound, " "))
		default:
			fmt.Fprintln(w, "reason: "+reason)
		}
	}
	for _, result := range r.Series {
		printBandSeries(w, result)
	}
}

func printBandSeries(w io.Writer, r bandSeriesResult) {
	fmt.Fprintln(w, r.row())
	if len(r.Reasons) > 0 {
		fmt.Fprintln(w, "  reasons: "+strings.Join(r.Reasons, ", "))
	}
	if r.Events == 0 {
		fmt.Fprintln(w, "  no new observation after "+defaultString(r.SampleKey, "-"))
	}
	if r.ClaimKey == "" {
		return
	}
	line := "  claim: diagnose " + r.ClaimKey + " " + r.ClaimStatus
	if r.BSID != "" {
		line += " bs=" + r.BSID
	}
	if r.DiagnosisStatus != "" {
		line += " diagnosis=" + r.DiagnosisStatus
	}
	if r.ResultPending {
		line += " result=pending"
	}
	fmt.Fprintln(w, line)
}
