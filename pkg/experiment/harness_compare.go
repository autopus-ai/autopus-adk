package experiment

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

var harnessArms = []string{"native", "current", "reduced"}

type harnessMeasured struct {
	observation HarnessObservation
	tokens      *int64
	knownTokens int64
}

// CompareHarness reports supplied observations, never a promotion or winner.
// Pair deltas include rejected tasks and every retry, not only accepted tasks.
func CompareHarness(e HarnessEvidence) (HarnessReport, error) {
	r := HarnessReport{Version: 1, Mode: "observational", Complete: true}
	if err := validateHarness(e); err != nil {
		return r, err
	}
	measured := make(map[string]map[string]harnessMeasured)
	for _, arm := range harnessArms {
		measured[arm] = make(map[string]harnessMeasured)
	}
	for _, o := range e.Observations {
		s := telemetry.SummarizeEfficiency(o.Runs)
		var tokens *int64
		completeCalls := true
		for _, run := range o.Runs {
			if len(run.Usage) == 0 {
				completeCalls = false
			}
		}
		if completeCalls && s.ActualCoverage == 1 && s.UniqueModelCallCount > 0 && !s.PromotionBlocked {
			v := s.RawTokens
			tokens = &v
		}
		measured[o.Arm][o.TaskID] = harnessMeasured{o, tokens, s.RawTokens}
	}
	for _, arm := range harnessArms {
		a := HarnessArmReport{Arm: arm, Expected: len(e.ExpectedTaskIDs), ActualTokens: intPtr(0), ElapsedMS: intPtr(0), HumanCorrections: intPtr(0), MissingTasks: []string{}}
		for _, task := range e.ExpectedTaskIDs {
			m, ok := measured[arm][task]
			if !ok {
				a.MissingTasks = append(a.MissingTasks, task)
				a.UnknownAcceptance++
				a.ActualTokens = nil
				a.ElapsedMS = nil
				a.HumanCorrections = nil
				r.Complete = false
				continue
			}
			a.Observed++
			a.KnownActualTokens += m.knownTokens
			if m.tokens != nil {
				a.MeasuredTasks++
			}
			a.HarnessRevision = m.observation.HarnessRevision
			a.HarnessConfigHash = m.observation.HarnessConfigHash
			if m.observation.Accepted == nil {
				a.UnknownAcceptance++
				r.Complete = false
			} else if *m.observation.Accepted {
				a.Accepted++
			} else {
				a.Rejected++
			}
			addNullable(&a.ActualTokens, m.tokens)
			addNullable(&a.ElapsedMS, m.observation.ElapsedMS)
			addNullable(&a.HumanCorrections, m.observation.HumanCorrections)
			if m.tokens == nil || m.observation.ElapsedMS == nil || m.observation.HumanCorrections == nil {
				r.Complete = false
			}
		}
		r.Arms = append(r.Arms, a)
	}
	for i := 0; i < len(harnessArms); i++ {
		for j := i + 1; j < len(harnessArms); j++ {
			p := HarnessPairReport{Baseline: harnessArms[i], Candidate: harnessArms[j], TaskIDs: []string{}, ExcludedTasks: []string{}, ExclusionReasons: map[string]string{}}
			for _, task := range e.ExpectedTaskIDs {
				a, aok := measured[p.Baseline][task]
				b, bok := measured[p.Candidate][task]
				if !aok || !bok || a.observation.Identity != b.observation.Identity || a.tokens == nil || b.tokens == nil || a.observation.ElapsedMS == nil || b.observation.ElapsedMS == nil || a.observation.Accepted == nil || b.observation.Accepted == nil {
					p.ExcludedTasks = append(p.ExcludedTasks, task)
					reason := "incomplete_measurement_or_acceptance"
					if !aok || !bok {
						reason = "missing_observation"
					} else if a.observation.Identity != b.observation.Identity {
						reason = "incompatible_identity"
					}
					p.ExclusionReasons[task] = reason
					continue
				}
				if p.TokenDelta == nil {
					p.TokenDelta = intPtr(0)
					p.ElapsedDeltaMS = intPtr(0)
				}
				*p.TokenDelta += *b.tokens - *a.tokens
				*p.ElapsedDeltaMS += *b.observation.ElapsedMS - *a.observation.ElapsedMS
				p.TaskIDs = append(p.TaskIDs, task)
				p.TaskCount++
			}
			p.Complete = p.TaskCount == len(e.ExpectedTaskIDs)
			if !p.Complete {
				r.Complete = false
			}
			r.Pairs = append(r.Pairs, p)
		}
	}
	return r, nil
}
func intPtr(v int64) *int64 { return &v }
func addNullable(total **int64, v *int64) {
	if v == nil {
		*total = nil
	} else if *total != nil {
		**total += *v
	}
}

func validateHarness(e HarnessEvidence) error {
	if e.Version != 1 || len(e.ExpectedTaskIDs) == 0 || len(e.ExpectedTaskIDs) > 1000 || len(e.Observations) > 3000 {
		return fmt.Errorf("invalid harness evidence version or size")
	}
	tasks := map[string]bool{}
	rows := map[string]bool{}
	calls := map[string]bool{}
	configs := map[string]string{}
	totalCalls := 0
	for _, t := range e.ExpectedTaskIDs {
		if !validHarnessIdentity(t) || tasks[t] {
			return fmt.Errorf("empty or duplicate task")
		}
		tasks[t] = true
	}
	for _, o := range e.Observations {
		if !tasks[o.TaskID] || (o.Arm != "native" && o.Arm != "current" && o.Arm != "reduced") {
			return fmt.Errorf("unexpected task or arm")
		}
		key := o.Arm + "\x00" + o.TaskID
		if rows[key] {
			return fmt.Errorf("duplicate observation")
		}
		rows[key] = true
		if !validHarnessIdentity(o.HarnessRevision) || !validHarnessIdentity(o.HarnessConfigHash) {
			return fmt.Errorf("missing harness identity")
		}
		config := o.HarnessRevision + "\x00" + o.HarnessConfigHash
		if prior, ok := configs[o.Arm]; ok && prior != config {
			return fmt.Errorf("inconsistent arm configuration")
		}
		configs[o.Arm] = config
		id := o.Identity
		for _, v := range []string{id.Provider, id.ProviderVersion, id.Model, id.ModelVersion, id.Effort, id.CacheStratum, id.TaskHash, id.OracleHash, id.EnvironmentHash, id.BudgetHash, id.Revision} {
			if !validHarnessIdentity(v) {
				return fmt.Errorf("incomplete comparison identity")
			}
		}
		for _, v := range []*int64{o.ElapsedMS, o.HumanCorrections} {
			if v != nil && (*v < 0 || *v > 1e12) {
				return fmt.Errorf("invalid observation count")
			}
		}
		if len(o.Runs) > 1000 {
			return fmt.Errorf("too many runs")
		}
		for _, run := range o.Runs {
			if run.TaskID != o.TaskID || (run.Status != telemetry.StatusPass && run.Status != telemetry.StatusFail) || len(run.Usage) > 1000 {
				return fmt.Errorf("invalid run")
			}
			for _, u := range run.Usage {
				if !validHarnessIdentity(u.RunID) || !validHarnessIdentity(u.CallID) {
					return fmt.Errorf("invalid usage identity")
				}
				totalCalls++
				if totalCalls > 10000 {
					return fmt.Errorf("too many model calls")
				}
				if err := telemetry.ValidateUsageEnvelope(u); err != nil {
					return fmt.Errorf("invalid usage envelope")
				}
				if u.TaskID != o.TaskID || u.Provider != id.Provider || u.ProviderVersion != id.ProviderVersion || u.Model != id.Model || u.ModelVersion != id.ModelVersion || u.Effort != id.Effort || u.CacheStratum != id.CacheStratum {
					return fmt.Errorf("usage identity mismatch")
				}
				if u.UsageStatus == telemetry.UsageStatusActual && u.UsageSource != telemetry.UsageSourceProvider {
					return fmt.Errorf("actual usage requires provider source")
				}
				if u.RawTotalTokens != nil && *u.RawTotalTokens > 1e12 {
					return fmt.Errorf("usage exceeds limit")
				}
				key := u.RunID + "\x00" + u.CallID
				if calls[key] {
					return fmt.Errorf("duplicate model call")
				}
				calls[key] = true
			}
		}
	}
	return nil
}

func validHarnessIdentity(v string) bool {
	if v == "" || len(v) > 256 || !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}
