package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpec_ProviderAndVersion(t *testing.T) {
	require.Equal(t, "stripe", Provider)
	require.Equal(t, "stripe", IntegrationType)
	require.Equal(t, "1.0.0", AdapterVersion)
	require.Equal(t, "2024-12-18.acacia", StripeAPIVersion)
}

func TestSpec_Describe_HasFourteenCapabilities(t *testing.T) {
	resp := Describe()
	require.Equal(t, "stripe", resp.Provider)
	// 13 execute + 1 reactor = 14 total in ActionCatalog.
	require.Len(t, resp.ActionCatalog, 14, "expected 14 actions in catalog")
	// SupportedExecuteOperations excludes the reactor.
	require.Len(t, SupportedExecuteOperations, 13, "expected 13 executable ops")
}
