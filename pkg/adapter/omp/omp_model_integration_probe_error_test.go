package omp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOMPCatalogUnavailableError_KeepsReasonAndAddsNextStep(t *testing.T) {
	t.Parallel()

	timeout := ompCatalogUnavailableError("catalog_timeout").Error()
	assert.Contains(t, timeout, "model_catalog_unavailable: catalog_timeout")
	assert.Contains(t, timeout, "omp models --json --no-extensions")
	assert.Contains(t, timeout, defaultOMPModelProbeTimeout.String())

	assert.Contains(t, ompCatalogUnavailableError("identity_unverified").Error(), "omp --version")
	assert.Equal(t, "model_catalog_unavailable: catalog_invalid", ompCatalogUnavailableError("catalog_invalid").Error())
}

// The probe bounds a hung process; a cold `omp models` on OMP 18.x routinely
// takes several seconds, so the ceiling must leave room for it.
func TestDefaultOMPModelProbeTimeout_LeavesRoomForColdCatalog(t *testing.T) {
	t.Parallel()
	assert.GreaterOrEqual(t, defaultOMPModelProbeTimeout, 15*time.Second)
}
