package harneval

// Advisory verdicts in precedence order, and the reason each one carries
// (REQ-HE-10).
const (
	VerdictVacuous    = "vacuous"
	VerdictIncomplete = "incomplete"
	VerdictRegression = "regression"
	VerdictOK         = "ok"

	ReasonOracleCalibrationFailed = "oracle_calibration_failed"
	ReasonOracleNotRun            = "oracle_not_run"
	ReasonCompletenessBelowFloor  = "completeness_below_floor"
	ReasonNoValidTrial            = "no_valid_trial"
	ReasonHardFlip                = "hard_flip"
	ReasonPassRateRegression      = "pass_rate_regression"
	ReasonWithinThreshold         = "within_threshold"
)

// ArmTotals counts one arm. Valid excludes error trials; PassRate is
// passes/valid, or nil (JSON null) when the arm has no valid trial.
type ArmTotals struct {
	Passes   int      `json:"passes"`
	Valid    int      `json:"valid"`
	PassRate *float64 `json:"pass_rate"`
}

// Arms holds the totals of both arms.
type Arms struct {
	Baseline  ArmTotals `json:"baseline"`
	Candidate ArmTotals `json:"candidate"`
}

// LiveVerdict is the advisory judgement of one live session. HardFlips lists
// task ids in ascending order; Calibration summarizes calibration.json.
type LiveVerdict struct {
	Arms            Arms             `json:"arms"`
	RegressionDelta float64          `json:"regression_delta"`
	HardFlips       []string         `json:"hard_flips"`
	Completeness    float64          `json:"completeness"`
	Calibration     CalibrationPhase `json:"calibration"`
	Verdict         string           `json:"verdict"`
	Reason          string           `json:"reason"`
}

// armTally accumulates the records of one arm, overall and per task.
type armTally struct {
	ran        int
	totals     ArmTotals
	taskValid  map[string]int
	taskPasses map[string]int
}

// ComputeVerdict judges a session whose documents passed their decoders, as
// LoadSession returns them. The documents are reconciled first: a mismatch is
// ErrRecordsProtocolMismatch, and a calibration.json that does not belong to
// the protocol is an *InvalidError. Neither yields a verdict.
func ComputeVerdict(s *Session) (LiveVerdict, error) {
	if err := reconcile(s); err != nil {
		return LiveVerdict{}, err
	}
	tallies := map[string]*armTally{}
	for _, arm := range []string{ArmBaseline, ArmCandidate} {
		tallies[arm] = &armTally{taskValid: map[string]int{}, taskPasses: map[string]int{}}
	}
	for _, record := range s.Records {
		tally := tallies[record.Arm]
		if record.Oracle != nil && record.Oracle.Ran {
			tally.ran++
		}
		if record.Outcome == OutcomeError {
			continue
		}
		tally.totals.Valid++
		tally.taskValid[record.TaskID]++
		if record.Outcome == OutcomePass {
			tally.totals.Passes++
			tally.taskPasses[record.TaskID]++
		}
	}
	baseline, candidate := tallies[ArmBaseline], tallies[ArmCandidate]
	for _, tally := range tallies {
		tally.totals.PassRate = ratio(tally.totals.Passes, tally.totals.Valid)
	}
	verdict := LiveVerdict{
		Arms:        Arms{Baseline: baseline.totals, Candidate: candidate.totals},
		HardFlips:   hardFlips(s.Protocol, baseline, candidate),
		Calibration: calibrationSummary(s.Calibration),
	}
	if b, c := baseline.totals.PassRate, candidate.totals.PassRate; b != nil && c != nil {
		verdict.RegressionDelta = *c - *b
	}
	if completeness := ratio(baseline.totals.Valid+candidate.totals.Valid, len(s.Protocol.Order)); completeness != nil {
		verdict.Completeness = *completeness
	}
	verdict.Verdict, verdict.Reason = decide(verdict, baseline, candidate, s.Protocol.Policy)
	return verdict, nil
}

// hardFlips lists the tasks with K valid trials in both arms that the
// baseline passed every time and the candidate never passed.
func hardFlips(protocol Protocol, baseline, candidate *armTally) []string {
	k := protocol.Policy.K
	flips := []string{}
	for _, task := range scheduledTasks(protocol.Order) {
		if baseline.taskValid[task] == k && baseline.taskPasses[task] == k &&
			candidate.taskValid[task] == k && candidate.taskPasses[task] == 0 {
			flips = append(flips, task)
		}
	}
	return flips
}

// decide applies the REQ-HE-10 precedence: vacuous, then incomplete, then
// regression (a hard flip before a pass-rate drop), then ok. A session whose
// oracle calibration did not pass at both ends, or whose oracle never ran in
// an arm, cannot be ok.
func decide(verdict LiveVerdict, baseline, candidate *armTally, policy LivePolicy) (string, string) {
	switch {
	case verdict.Calibration.Status != CalibrationPassed:
		return VerdictVacuous, ReasonOracleCalibrationFailed
	case baseline.ran == 0 || candidate.ran == 0:
		return VerdictVacuous, ReasonOracleNotRun
	case verdict.Completeness < policy.CompletenessFloor:
		return VerdictIncomplete, ReasonCompletenessBelowFloor
	case baseline.totals.Valid == 0 || candidate.totals.Valid == 0:
		return VerdictIncomplete, ReasonNoValidTrial
	case len(verdict.HardFlips) > 0:
		return VerdictRegression, ReasonHardFlip
	case regressed(baseline.totals, candidate.totals, policy.ThresholdBP):
		return VerdictRegression, ReasonPassRateRegression
	}
	return VerdictOK, ReasonWithinThreshold
}

// regressed is delta < threshold_bp/10000 in integers, free of rounding:
// 10000·(cp·bv − bp·cv) < threshold_bp·bv·cv. A delta exactly at the threshold
// is no regression.
func regressed(baseline, candidate ArmTotals, thresholdBP int) bool {
	bp, bv := int64(baseline.Passes), int64(baseline.Valid)
	cp, cv := int64(candidate.Passes), int64(candidate.Valid)
	return 10000*(cp*bv-bp*cv) < int64(thresholdBP)*bv*cv
}

// calibrationSummary is the session's calibration: passed only when the
// before and after calibrations both passed, missing when calibration.json or
// its after phase is absent, failed otherwise. Tasks come from the latest
// phase, which is the one that decided the status.
func calibrationSummary(calibration *Calibration) CalibrationPhase {
	switch {
	case calibration == nil:
		return CalibrationPhase{Status: CalibrationMissing, Tasks: []CalibrationTask{}}
	case calibration.After != nil:
		return *calibration.After
	case calibration.Before.Status == CalibrationFailed:
		return calibration.Before
	}
	return CalibrationPhase{Status: CalibrationMissing, Tasks: calibration.Before.Tasks}
}
