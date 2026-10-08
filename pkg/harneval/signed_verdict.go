package harneval

// ReasonSignedTasksBelowFloor is the vacuous reason of a signed-lane session
// that scheduled fewer black-box agent tasks than the manifest's
// floors.signed_agent_tasks (SPEC-HARNEVAL-003 REQ-HR-08). The signed report
// carries it as vacuous.
const ReasonSignedTasksBelowFloor = "signed_tasks_below_floor"

// SignedLaneVerdict judges a verified signed-lane session: the
// SPEC-HARNEVAL-001 verdict, made vacuous when the session's order schedules
// fewer black-box tasks than floor. A floor below one is no declared floor,
// so the session proves too little coverage to sign anything but vacuous. A
// verdict that is already vacuous keeps its reason, as REQ-HE-10 judges
// vacuity first.
func SignedLaneVerdict(s *Session, floor int) (LiveVerdict, error) {
	verdict, err := ComputeVerdict(s)
	if err != nil {
		return LiveVerdict{}, err
	}
	if verdict.Verdict != VerdictVacuous && (floor < 1 || len(scheduledTasks(s.Protocol.Order)) < floor) {
		verdict.Verdict, verdict.Reason = VerdictVacuous, ReasonSignedTasksBelowFloor
	}
	return verdict, nil
}
