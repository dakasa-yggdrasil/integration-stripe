// Package adapter — tests for the apiKey + baseURL wiring through
// Execute -> clientForInstance.
//
// These tests cover the secondary bug noted in v2.2.1 CHANGELOG: even
// after the bridge fix rehydrates req.Integration.Spec.Credentials,
// Execute() was calling clientForInstance(InstanceID, "", "", ...) with
// a hardcoded empty apiKey. NewStripeClient("") returns "stripe api key
// is required" and every write capability fails before reaching the
// Stripe HTTP boundary regardless of bridge state. Closes the secondary
// half of the cycle-#243 bridge regression — once the bridge rehydrates
// credentials AND Execute reads them, writes work in production.
package adapter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// TestExecute_ReadsAPIKeyFromCredentials proves Execute() now reads
// stripe_api_key from req.Integration.Spec.Credentials and threads it
// into clientForInstance. Without this fix, the call goes through
// NewStripeClient("") which returns "stripe api key is required" and
// ALL stripe write capabilities fail — regardless of what the bridge
// forwards.
//
// The test deliberately does NOT register a test client via
// SetStripeClientForTest (which would short-circuit the apiKey path).
// Instead, it provides stripe_api_base_url via instance_spec.config so
// the freshly-built client points at an in-process httptest.Server,
// then asserts the Authorization: Bearer header carries the canary
// apiKey value that came from credentials.
func TestExecute_ReadsAPIKeyFromCredentials(t *testing.T) {
	const canary = "sk_test_canary_from_credentials"

	var mu sync.Mutex
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sawAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"cus_canary","object":"customer","email":"canary@example.com","created":1700000000}`))
	}))
	defer ts.Close()

	// Use a unique instance ID with NO test client registered — this
	// forces clientForInstance to fall through to NewStripeClient, which
	// is where the apiKey gate fires.
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationEnsureCustomer,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-credentials-canary",
			Spec: contract.IntegrationInstanceManifestSpec{
				Credentials: map[string]any{
					"stripe_api_key": canary,
				},
				Config: map[string]any{
					// Test hook — points the freshly-built stripe client at
					// the in-process server so we can capture the Authorization
					// header without crossing the network to api.stripe.com.
					"stripe_api_base_url": ts.URL,
				},
			},
		},
		Input: map[string]any{
			"email": "canary@example.com",
		},
	})

	// Without the fix this fails with: "stripe api key is required".
	require.NoError(t, err, "Execute must succeed when credentials carry stripe_api_key")
	require.NotNil(t, resp.Output)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "Bearer "+canary, sawAuth,
		"stripe_api_key from req.Integration.Spec.Credentials must reach the Stripe HTTP request as Bearer token")
}

// TestExecute_ReadsAPIKeyFromStripeSecretKeyAlias proves the alias
// "stripe_secret_key" is accepted when "stripe_api_key" is absent.
// Mirrors the operator convention where AWS Secrets Manager (and
// other secret stores) commonly name the secret value
// "stripe_secret_key". Without this alias the adapter would still
// fail with "stripe api key is required" for instances whose
// credentials_ref resolves to a secret using that naming.
func TestExecute_ReadsAPIKeyFromStripeSecretKeyAlias(t *testing.T) {
	const canary = "sk_test_canary_from_alias"

	var mu sync.Mutex
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sawAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"cus_alias","object":"customer","email":"alias@example.com","created":1700000000}`))
	}))
	defer ts.Close()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationEnsureCustomer,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-alias-canary",
			Spec: contract.IntegrationInstanceManifestSpec{
				Credentials: map[string]any{
					// NOTE: NOT "stripe_api_key" — the alias.
					"stripe_secret_key": canary,
				},
				Config: map[string]any{
					"stripe_api_base_url": ts.URL,
				},
			},
		},
		Input: map[string]any{
			"email": "alias@example.com",
		},
	})
	require.NoError(t, err, "Execute must succeed when credentials carry stripe_secret_key alias")
	require.NotNil(t, resp.Output)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "Bearer "+canary, sawAuth,
		"stripe_secret_key alias must reach Stripe HTTP as Bearer token")
}

// TestExecute_StripeAPIKeyPrefersCanonicalOverAlias proves the canonical
// field wins when both are present — preserves predictability so
// operators rotating to the canonical name don't see surprise routing.
func TestExecute_StripeAPIKeyPrefersCanonicalOverAlias(t *testing.T) {
	const canonicalKey = "sk_test_CANONICAL"
	const aliasKey = "sk_test_ALIAS"

	var mu sync.Mutex
	var sawAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sawAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"cus_pri","object":"customer","email":"pri@example.com","created":1700000000}`))
	}))
	defer ts.Close()

	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationEnsureCustomer,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-priority-canary",
			Spec: contract.IntegrationInstanceManifestSpec{
				Credentials: map[string]any{
					"stripe_api_key":    canonicalKey,
					"stripe_secret_key": aliasKey,
				},
				Config: map[string]any{
					"stripe_api_base_url": ts.URL,
				},
			},
		},
		Input: map[string]any{"email": "pri@example.com"},
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "Bearer "+canonicalKey, sawAuth,
		"canonical stripe_api_key must win when both fields are present")
}

// TestExecute_MissingAPIKeyRejected confirms the error contract is
// preserved: when credentials are absent, Execute() still surfaces a
// clear "api key is required" error rather than silently dispatching
// with empty credentials. Keeps regression-safety on the negative path.
func TestExecute_MissingAPIKeyRejected(t *testing.T) {
	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationEnsureCustomer,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-no-creds",
			Spec:       contract.IntegrationInstanceManifestSpec{},
		},
		Input: map[string]any{
			"email": "anyone@example.com",
		},
	})
	require.Error(t, err, "Execute must reject when stripe_api_key is missing from credentials")
	require.True(t, strings.Contains(err.Error(), "stripe api key"),
		"error must mention stripe api key, got: %v", err)
}
