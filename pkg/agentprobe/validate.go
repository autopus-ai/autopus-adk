package agentprobe

import "fmt"

func token(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == ':' || r == '/' || r == '+') {
			return false
		}
	}
	return true
}

func validateEvidence(e Evidence) error {
	if e.Version != 1 || len(e.Events) > MaxEvents {
		return fmt.Errorf("invalid lifecycle schema or event count")
	}
	for _, value := range []string{e.Platform, e.RuntimeVersion, e.RunID, e.SupervisorID, e.Challenge} {
		if !token(value) {
			return fmt.Errorf("invalid lifecycle identity")
		}
	}
	for _, event := range e.Events {
		if event.Source != "native_protocol" || event.RunID != e.RunID || event.ParentID != e.SupervisorID || event.Sequence <= 0 {
			return fmt.Errorf("invalid native event identity")
		}
		global := event.Kind == "inventory" || event.Kind == "session_closed"
		if global {
			if event.ChildID != "" || event.Case != "" || event.ResultCode != "" || event.Status != "" {
				return fmt.Errorf("invalid supervisor event")
			}
		} else if !token(event.ChildID) || (event.Case != "success" && event.Case != "cancel") {
			return fmt.Errorf("invalid child identity")
		}
		if event.Kind != "result" && event.ResultCode != "" || event.Kind != "terminal" && event.Status != "" || event.Kind != "inventory" && event.OwnedIDs != nil {
			return fmt.Errorf("unexpected event fields")
		}
		switch event.Kind {
		case "spawn", "cancel_requested", "cancel_ack", "session_closed":
		case "result":
			if !token(event.ResultCode) {
				return fmt.Errorf("invalid result code")
			}
		case "terminal":
			if event.Status != "cancelled" && event.Status != "completed" && event.Status != "failed" {
				return fmt.Errorf("invalid terminal status")
			}
		case "inventory":
			if event.OwnedIDs == nil || len(event.OwnedIDs) > MaxEvents {
				return fmt.Errorf("inventory must declare bounded child IDs")
			}
			seen := map[string]bool{}
			for _, id := range event.OwnedIDs {
				if !token(id) || seen[id] {
					return fmt.Errorf("invalid inventory child")
				}
				seen[id] = true
			}
		default:
			return fmt.Errorf("unknown native lifecycle event")
		}
	}
	return nil
}
