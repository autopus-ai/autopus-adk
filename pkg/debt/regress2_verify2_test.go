package debt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDebtRegression2_MultiLineImportsLongNames(t *testing.T) {
	ts := "import {\n  useOrganizationMembershipQuery,\n  useOrganizationBillingSettingsQuery,\n  useOrganizationAuditTrailQuery,\n  useOrganizationWorkspaceListQuery,\n  useOrganizationInvitationQuery,\n  useOrganizationSeatAllocationQuery,\n} from '@/features/organization/hooks';\n\n"
	py := "from app.services.accounts import (\n    AccountProvisioningService,\n    AccountDeprovisioningService,\n    AccountEntitlementResolver,\n    AccountBillingReconciler,\n    AccountAuditTrailExporter,\n    AccountSessionRevocationJob,\n)\n\n"
	root := rvRepo(t, map[string]string{
		"x.ts": ts + "export const x = 1;\n",
		"y.ts": ts + "export const y = 2;\n",
		"p.py": py + "X = 1\n",
		"q.py": py + "Y = 2\n",
	})
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	for _, it := range items {
		t.Logf("%s", it)
	}
}
