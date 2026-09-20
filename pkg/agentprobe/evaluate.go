package agentprobe

import "fmt"

type workerState struct {
	id                               string
	result, requested, ack, terminal bool
	resultCode, status               string
}

// Evaluate checks ordered native event chains. Source labels are declarations;
// callers must distinguish imported traces from their own runtime collection.
func Evaluate(e Evidence) (Report, error) {
	r := Report{Version: 1, Platform: e.Platform, RuntimeVersion: e.RuntimeVersion, RunID: e.RunID, Overall: "unknown", Gates: []Gate{{"spawn", "unknown", "missing_spawn"}, {"result", "unknown", "missing_result"}, {"cancel", "unknown", "missing_terminal"}, {"cleanup", "unknown", "missing_inventory_or_close"}}}
	if err := validateEvidence(e); err != nil {
		return r, err
	}
	workers := map[string]*workerState{}
	inventory, closed := false, false
	residual := 0
	sequence := 0
	for _, event := range e.Events {
		if event.Sequence <= sequence || closed {
			return r, fmt.Errorf("non-monotonic event or event after session close")
		}
		sequence = event.Sequence
		if event.Kind == "inventory" {
			if inventory {
				return r, fmt.Errorf("duplicate inventory")
			}
			inventory = true
			for _, id := range event.OwnedIDs {
				if id != e.SupervisorID {
					residual++
				} else {
					return r, fmt.Errorf("supervisor cannot be an owned child")
				}
			}
			continue
		}
		if event.Kind == "session_closed" {
			closed = true
			continue
		}
		if inventory {
			return r, fmt.Errorf("worker event after final inventory")
		}
		worker := workers[event.Case]
		if event.Kind == "spawn" {
			if worker != nil {
				return r, fmt.Errorf("duplicate worker spawn")
			}
			for _, prior := range workers {
				if prior.id == event.ChildID {
					return r, fmt.Errorf("child reused across cases")
				}
			}
			if event.ChildID == e.SupervisorID {
				return r, fmt.Errorf("child equals supervisor")
			}
			workers[event.Case] = &workerState{id: event.ChildID}
			continue
		}
		if worker == nil || worker.id != event.ChildID {
			return r, fmt.Errorf("event has no matching spawn")
		}
		switch event.Kind {
		case "result":
			if worker.result || worker.terminal || event.Case != "success" {
				return r, fmt.Errorf("invalid result order")
			}
			worker.result = true
			worker.resultCode = event.ResultCode
		case "cancel_requested":
			if worker.requested || event.Case != "cancel" {
				return r, fmt.Errorf("invalid cancel request")
			}
			worker.requested = true
		case "cancel_ack":
			if !worker.requested || worker.ack || worker.terminal {
				return r, fmt.Errorf("invalid cancel acknowledgement")
			}
			worker.ack = true
		case "terminal":
			if !worker.ack || worker.terminal || event.Case != "cancel" {
				return r, fmt.Errorf("invalid cancellation terminal order")
			}
			worker.terminal = true
			worker.status = event.Status
		}
	}
	success, cancel := workers["success"], workers["cancel"]
	if success != nil && cancel != nil {
		r.Gates[0] = Gate{"spawn", "pass", "two_distinct_workers_observed"}
	}
	if success != nil && success.result {
		r.Gates[1] = Gate{"result", "fail", "challenge_mismatch"}
		if success.resultCode == e.Challenge {
			r.Gates[1] = Gate{"result", "pass", "child_challenge_matched"}
		}
	}
	if cancel != nil && cancel.terminal {
		r.Gates[2] = Gate{"cancel", "fail", "terminal_not_cancelled"}
		if cancel.status == "cancelled" {
			r.Gates[2] = Gate{"cancel", "pass", "native_cancel_terminal_observed"}
		}
	}
	if inventory && residual > 0 {
		r.Gates[3] = Gate{"cleanup", "fail", "owned_children_remain"}
	} else if inventory && closed && success != nil && success.result && cancel != nil && cancel.terminal {
		r.Gates[3] = Gate{"cleanup", "pass", "final_inventory_empty_and_session_closed"}
	}
	r.Overall = "pass"
	for _, gate := range r.Gates {
		if gate.Status == "fail" {
			r.Overall = "fail"
			break
		}
		if gate.Status != "pass" {
			r.Overall = "unknown"
		}
	}
	return r, nil
}
