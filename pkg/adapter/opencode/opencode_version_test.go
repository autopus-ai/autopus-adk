package opencode

import (
	"context"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeRuntimeMajor(t *testing.T) {
	for _, tc := range []struct {
		version string
		v2      bool
		invalid bool
	}{
		{"1.18.31", false, false}, {"opencode v2.0.10\n", true, false},
		{"2.0.10", true, false}, {"", false, false}, {"unknown", false, false},
		{"opencode v3.0.0", false, true},
	} {
		t.Run(tc.version, func(t *testing.T) {
			a := NewWithRoot(t.TempDir(), WithCLIVersion(tc.version))
			require.Equal(t, tc.v2, a.isV2())
			_, err := a.prepareFiles(context.Background(), config.DefaultFullConfig("test"))
			if tc.invalid {
				require.ErrorContains(t, err, "unsupported OpenCode major")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
