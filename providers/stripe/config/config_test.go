package config

import (
	"os"
	"path/filepath"
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

func TestLoadInstances_MultipleStripeAccounts_IsolatedSecrets(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "instances.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"instances": [
			{"instance_id": "dakasa", "stripe_api_key": "sk_dakasa", "stripe_webhook_secret": "whsec_dakasa", "stripe_account_id": "acct_dak"},
			{"instance_id": "acme",   "stripe_api_key": "sk_acme",   "stripe_webhook_secret": "whsec_acme",   "stripe_account_id": "acct_acme"}
		]
	}`), 0o644))

	got, err := LoadInstances(path)
	require.NoError(t, err)
	require.Len(t, got, 2)

	dak, ok := got["dakasa"]
	require.True(t, ok)
	acme, ok := got["acme"]
	require.True(t, ok)

	// Secrets are loaded independently; APIKey/WebhookSecret for one
	// MUST NOT equal the other after WithDefaults.
	require.Equal(t, "sk_dakasa", dak.APIKey)
	require.Equal(t, "sk_acme", acme.APIKey)
	require.NotEqual(t, dak.APIKey, acme.APIKey, "secrets must be per-instance")
	require.NotEqual(t, dak.WebhookSecret, acme.WebhookSecret, "webhook secrets must be per-instance")
	require.Equal(t, "acct_dak", dak.AccountID)
	require.Equal(t, "acct_acme", acme.AccountID)
}
