package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstanceConfig_Defaults(t *testing.T) {
	c := InstanceConfig{}.WithDefaults()
	require.Equal(t, "2024-12-18.acacia", c.APIVersion)
	require.Equal(t, int64(300), c.ToleranceSecs)
}

func TestInstanceConfig_RespectsCustom(t *testing.T) {
	c := InstanceConfig{APIVersion: "2025-01-01", ToleranceSecs: 120}.WithDefaults()
	require.Equal(t, "2025-01-01", c.APIVersion)
	require.Equal(t, int64(120), c.ToleranceSecs)
}
