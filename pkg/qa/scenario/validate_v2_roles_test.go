package scenario

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateV2_RejectsNamedLiveRegionRoles(t *testing.T) {
	base := func(step Step) Scenario {
		return Scenario{
			SchemaVersion: SchemaVersionV2, ID: "login", Title: "Login", Journey: "web",
			IntentSource: IntentAcceptance, Spec: "SPEC-LOGIN-001", AcceptanceRef: []string{"AC-LOGIN-003"},
			Origin: "http://127.0.0.1:4173", Path: "login.yaml",
			Screens: []Screen{{ID: "login", Path: "/", Steps: []Step{step}}},
		}
	}
	for name, step := range map[string]Step{
		"expect_role alert":   {ExpectRole: &RoleTarget{Role: "alert", Name: "Wrong password"}, Ac: "AC-LOGIN-003"},
		"expect_count status": {ExpectCount: &CountTarget{Role: "Status", Name: "Saved", Count: 1}, Ac: "AC-LOGIN-003"},
		"click alert":         {Click: &Target{Role: "alert", Name: "Dismiss"}},
	} {
		err := Validate(base(step))
		var verr *ValidationError
		require.True(t, errors.As(err, &verr), name)
		assert.Equal(t, "qa_scenario_role_name_not_from_content", verr.Code, name)
	}
	// An unnamed alert, and a named heading, stay valid.
	require.NoError(t, Validate(base(Step{ExpectRole: &RoleTarget{Role: "alert"}, Ac: "AC-LOGIN-003"})))
	require.NoError(t, Validate(base(Step{ExpectRole: &RoleTarget{Role: "heading", Name: "로그인"}, Ac: "AC-LOGIN-003"})))
}

func TestValidateV1_NamedAlertUnchanged(t *testing.T) {
	s := Scenario{
		SchemaVersion: SchemaVersion, ID: "home", Title: "Home", Journey: "web",
		Origin: "http://127.0.0.1:4173", Path: "home.yaml",
		Screens: []Screen{{ID: "home", Path: "/", Steps: []Step{{ExpectRole: &RoleTarget{Role: "alert", Name: "Hi"}}}}},
	}
	require.NoError(t, Validate(s))
}
