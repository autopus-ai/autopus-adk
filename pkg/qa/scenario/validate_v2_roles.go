package scenario

import "strings"

// authorNamedRoles are live-region roles whose accessible name comes only from
// aria-label or aria-labelledby, never from their text. `getByRole("alert",
// { name: "Wrong password" })` therefore finds nothing on a page that plainly
// shows that alert, and the failure lands on an expect step, where triage
// reads it as a product defect. The loop would then "fix" the product by
// adding a label to satisfy the test. Rejecting the combination at validation
// sends the author to expect_text, which asserts what the user actually sees.
var authorNamedRoles = map[string]bool{
	"alert":   true,
	"status":  true,
	"log":     true,
	"marquee": true,
	"timer":   true,
}

// validateRoleNames applies to v2 only; v1 scenarios keep their behaviour.
func validateRoleNames(path, where string, step Step) error {
	check := func(role, name string) error {
		if strings.TrimSpace(name) == "" || !authorNamedRoles[strings.ToLower(strings.TrimSpace(role))] {
			return nil
		}
		return invalid(path, "qa_scenario_role_name_not_from_content",
			"%s names role %q by %q, but that role does not take its name from its text; assert the text with expect_text instead",
			where, role, name)
	}
	if step.ExpectRole != nil {
		if err := check(step.ExpectRole.Role, step.ExpectRole.Name); err != nil {
			return err
		}
	}
	if step.ExpectCount != nil {
		if err := check(step.ExpectCount.Role, step.ExpectCount.Name); err != nil {
			return err
		}
	}
	for _, target := range []*Target{step.Click, step.Check, fillTarget(step), pressTarget(step), selectTarget(step)} {
		if target != nil {
			if err := check(target.Role, target.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func fillTarget(step Step) *Target {
	if step.Fill == nil {
		return nil
	}
	return &step.Fill.Target
}

func pressTarget(step Step) *Target {
	if step.Press == nil {
		return nil
	}
	return &step.Press.Target
}

func selectTarget(step Step) *Target {
	if step.Select == nil {
		return nil
	}
	return &step.Select.Target
}
