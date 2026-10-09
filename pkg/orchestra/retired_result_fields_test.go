package orchestra

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// SPEC-PANERM-001 follow-up: the pane backend was the only producer of the
// reliability store summary, the yield passthrough, and the startup timeout.
// With it retired these fields stayed empty on every path, so they are gone
// from the public result and config types and must not return as dead
// always-empty members.
func TestRetiredPaneOnlyFieldsStayRemoved(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		typ   reflect.Type
		field string
	}{
		{reflect.TypeOf(OrchestraResult{}), "Reliability"},
		{reflect.TypeOf(OrchestraResult{}), "Yield"},
		{reflect.TypeOf(OrchestraConfig{}), "ReliabilityStore"},
		{reflect.TypeOf(ProviderConfig{}), "StartupTimeout"},
	} {
		_, found := tc.typ.FieldByName(tc.field)
		assert.Falsef(t, found, "%s.%s was retired with the orchestra pane backend", tc.typ.Name(), tc.field)
	}
}
