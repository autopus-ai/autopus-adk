package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func decodeOMPAgentCatalogStatus(t *testing.T, encoded string) ompPlatformProjection {
	t.Helper()
	var envelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(encoded), &envelope))
	var projection ompPlatformProjection
	require.NoError(t, json.Unmarshal(envelope.Data, &projection))
	return projection
}

type ompAgentCatalogExplainPayload struct {
	Status   string                     `json:"status"`
	Models   ompModelOperatorProjection `json:"models"`
	Blockers []string                   `json:"blockers"`
}

func decodeOMPAgentCatalogExplain(t *testing.T, encoded string) ompAgentCatalogExplainPayload {
	t.Helper()
	var envelope ompCLIJSONEnvelope
	require.NoError(t, json.Unmarshal([]byte(encoded), &envelope))
	var projection ompAgentCatalogExplainPayload
	require.NoError(t, json.Unmarshal(envelope.Data, &projection))
	return projection
}

func configTimeForOMPAgentCatalog() time.Time {
	return time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
}
