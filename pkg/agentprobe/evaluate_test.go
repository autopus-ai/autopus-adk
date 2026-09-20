package agentprobe

import "testing"

func fixture() Evidence {
	e := Evidence{Version: 1, Platform: "codex", RuntimeVersion: "1.2.3", RunID: "run", SupervisorID: "parent", Challenge: "challenge"}
	add := func(kind, child, workerCase string) {
		e.Events = append(e.Events, Event{Sequence: len(e.Events) + 1, Kind: kind, Source: "native_protocol", RunID: e.RunID, ParentID: e.SupervisorID, ChildID: child, Case: workerCase})
	}
	add("spawn", "one", "success")
	add("result", "one", "success")
	e.Events[1].ResultCode = e.Challenge
	add("spawn", "two", "cancel")
	add("cancel_requested", "two", "cancel")
	add("cancel_ack", "two", "cancel")
	add("terminal", "two", "cancel")
	e.Events[5].Status = "cancelled"
	add("inventory", "", "")
	e.Events[6].OwnedIDs = []string{}
	add("session_closed", "", "")
	return e
}

func TestEvaluateCompleteNativeChains(t *testing.T) {
	r, err := Evaluate(fixture())
	if err != nil || r.Overall != "pass" {
		t.Fatalf("%+v %v", r, err)
	}
	for _, gate := range r.Gates {
		if gate.Status != "pass" {
			t.Fatal(gate)
		}
	}
}

func TestEvaluateMissingAndNegativeEvidence(t *testing.T) {
	e := fixture()
	e.Events = e.Events[:5]
	r, err := Evaluate(e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Gates[2].Status != "unknown" || r.Gates[3].Status != "unknown" {
		t.Fatal(r)
	}
	e = fixture()
	e.Events[6].OwnedIDs = []string{"two"}
	r, err = Evaluate(e)
	if err != nil || r.Gates[3].Status != "fail" {
		t.Fatalf("%+v %v", r, err)
	}
	e = fixture()
	e.Events[1].ResultCode = "wrong"
	r, err = Evaluate(e)
	if err != nil || r.Gates[1].Status != "fail" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestEvaluateRejectsBrokenChains(t *testing.T) {
	cases := []func(*Evidence){
		func(e *Evidence) { e.Events[1].RunID = "other" }, func(e *Evidence) { e.Events[1].ParentID = "other" },
		func(e *Evidence) { e.Events[1].ChildID = "foreign" }, func(e *Evidence) { e.Events[1].Source = "model_claim" },
		func(e *Evidence) { e.Events[1].Sequence = 1 }, func(e *Evidence) { e.Events[5].Kind = "cancel_ack" },
		func(e *Evidence) { e.Events[6].OwnedIDs = nil }, func(e *Evidence) { e.Events[1].ResultCode = "raw text" },
	}
	for i, mutate := range cases {
		e := fixture()
		mutate(&e)
		if _, err := Evaluate(e); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestEvaluateCannotConfuseAckOrHostExitWithCleanup(t *testing.T) {
	e := fixture()
	e.Events = e.Events[:5]
	e.Events = append(e.Events, Event{Sequence: 6, Kind: "inventory", Source: "native_protocol", RunID: e.RunID, ParentID: e.SupervisorID, OwnedIDs: []string{}}, Event{Sequence: 7, Kind: "session_closed", Source: "native_protocol", RunID: e.RunID, ParentID: e.SupervisorID})
	r, err := Evaluate(e)
	if err != nil || r.Gates[2].Status != "unknown" || r.Gates[3].Status != "unknown" {
		t.Fatalf("ack mistaken for terminal: %+v %v", r, err)
	}
	e = fixture()
	e.Events[7].Kind = "host_process_exit"
	if _, err := Evaluate(e); err == nil {
		t.Fatal("host exit mistaken for session cleanup")
	}
	e = fixture()
	e.Events[5].Status = "completed"
	r, err = Evaluate(e)
	if err != nil || r.Gates[2].Status != "fail" {
		t.Fatalf("natural completion mistaken for cancellation: %+v %v", r, err)
	}
}

func TestEvaluateRejectsNativeOrderingAndIdentityConflicts(t *testing.T) {
	for _, mutate := range []func(*Evidence){
		func(e *Evidence) {
			e.Events[0], e.Events[1] = e.Events[1], e.Events[0]
			e.Events[0].Sequence = 1
			e.Events[1].Sequence = 2
		},
		func(e *Evidence) {
			e.Events[3], e.Events[4] = e.Events[4], e.Events[3]
			e.Events[3].Sequence = 4
			e.Events[4].Sequence = 5
		},
		func(e *Evidence) { e.Events[2].ChildID = "one" },
		func(e *Evidence) { e.Events[0].ChildID = e.SupervisorID },
		func(e *Evidence) { e.Events[6].OwnedIDs = []string{"two", "two"} },
		func(e *Evidence) { e.Events[7].ChildID = "foreign" },
		func(e *Evidence) { e.Events = append(e.Events, e.Events[7]); e.Events[8].Sequence = 9 },
	} {
		e := fixture()
		mutate(&e)
		if _, err := Evaluate(e); err == nil {
			t.Fatal("conflicting native event accepted")
		}
	}
}
