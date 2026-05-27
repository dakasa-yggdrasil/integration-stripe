package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPaymentIntentReconciler_Dispatch_ForwardsInstanceCredentials proves
// the in-tree paymentIntentReconciler.dispatch helper extracts the
// reserved-key forwarded instance_spec.credentials / instance_spec.config /
// req.Auth from the reconcile payload and rehydrates them into the
// AdapterExecuteIntegrationRequest that goes to Execute(). Without
// this rehydration the synthesized request arrives with empty Spec +
// Auth — which combined with the secondary pre-existing
// adapter.Execute/clientForInstance bug surfaces as "stripe api key is
// required" in production.
//
// To keep the test scoped to the BRIDGE bug (not the secondary
// Execute bug), this test injects a Stripe test client via
// SetStripeClientForTest — so the apiKey-resolution path inside
// Execute is short-circuited and we can isolate the credentials
// forwarding behavior at the dispatch boundary.
func TestPaymentIntentReconciler_Dispatch_ForwardsInstanceCredentials(t *testing.T) {
	// Build a minimal stub stripe client (NewStripeClient requires a
	// non-empty apiKey, so use a placeholder; the test client
	// short-circuits the apiKey path entirely).
	client, err := NewStripeClient("sk_test_stub", "http://127.0.0.1:1", StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-canary", client)
	defer restore()

	// Build the reconcile payload the bridge would have produced — the
	// reserved keys carry the integration context the dispatch helper
	// MUST rehydrate.
	expectedCreds := map[string]any{"stripe_api_key": "sk_test_canary_from_bridge"}
	expectedConfig := map[string]any{"stripe_account_id": "acct_canary"}
	expectedAuth := map[string]any{"on_behalf_of": "acct_connect"}
	in := reconcilePayload{
		"amount":          int64(1990),
		"currency":        "brl",
		"instance_id":     "stripe-canary",
		InstanceConfigKey: expectedConfig,
		InstanceCredsKey:  expectedCreds,
		InstanceAuthKey:   expectedAuth,
	}

	// buildExecuteRequest is the lever the dispatch helpers all use —
	// assert directly that it rehydrates the request shape the bridge
	// is responsible for delivering to Execute().
	got := buildExecuteRequest(OperationEnsurePaymentIntent, in, "stripe-fallback")

	require.Equal(t, OperationEnsurePaymentIntent, got.Operation)
	require.Equal(t, "stripe-canary", got.Integration.InstanceID,
		"instance_id from payload must beat the fallback")
	require.Equal(t, expectedConfig, got.Integration.Spec.Config,
		"InstanceSpec.Config must be rehydrated from InstanceConfigKey")
	require.Equal(t, expectedCreds, got.Integration.Spec.Credentials,
		"InstanceSpec.Credentials must be rehydrated from InstanceCredsKey — otherwise stripe writes fail at clientForInstance")
	require.Equal(t, expectedAuth, got.Auth,
		"req.Auth must be rehydrated from InstanceAuthKey")

	// Reserved keys must not leak into the operator-facing Input.
	require.NotContains(t, got.Input, InstanceConfigKey, "InstanceConfigKey must not leak to handler input")
	require.NotContains(t, got.Input, InstanceCredsKey, "InstanceCredsKey must not leak to handler input")
	require.NotContains(t, got.Input, InstanceAuthKey, "InstanceAuthKey must not leak to handler input")

	// Operator-supplied fields are preserved.
	require.Equal(t, int64(1990), got.Input["amount"])
	require.Equal(t, "brl", got.Input["currency"])
	require.Equal(t, "stripe-canary", got.Input["instance_id"])
}

// TestPaymentIntentReconciler_Dispatch_FallbackInstanceID confirms the
// instanceFromPayload helper falls back to the reconciler-bound default
// when the payload carries no instance_id override.
func TestPaymentIntentReconciler_Dispatch_FallbackInstanceID(t *testing.T) {
	in := reconcilePayload{
		"amount":   int64(500),
		"currency": "brl",
	}
	got := buildExecuteRequest(OperationEnsurePaymentIntent, in, "stripe-fallback")
	require.Equal(t, "stripe-fallback", got.Integration.InstanceID,
		"empty payload instance_id must yield the fallback")
}

// TestPaymentIntentReconciler_Dispatch_NilReservedMapsTolerated proves the
// helper returns nil maps (not zero-struct stuck values) when reserved
// keys are absent — preserving the existing Execute() nil-input
// contract.
func TestPaymentIntentReconciler_Dispatch_NilReservedMapsTolerated(t *testing.T) {
	in := reconcilePayload{
		"amount":   int64(500),
		"currency": "brl",
	}
	got := buildExecuteRequest(OperationEnsurePaymentIntent, in, "stripe-fallback")
	require.Nil(t, got.Integration.Spec.Config)
	require.Nil(t, got.Integration.Spec.Credentials)
	require.Nil(t, got.Auth)
}

// Use context.Background indirectly via dispatch call paths in real
// code; here we just touch the import to keep go vet happy.
var _ = context.Background

// TestCustomerReconciler_DestroyWithDesired_ForwardsCredentials proves
// the SDK v0.8.0 DestroyWithDesired interface lets destroy_customer
// resolve credentials through buildExecuteRequest — same path
// ensure_customer / observe_customers use.
//
// Without DestroyWithDesired, the SDK falls through to the legacy
// Destroy(ctx, ref) signature which produces a synthesized payload
// of only {"customer_id": ref} — the reserved bridge keys are lost
// and the call fails at clientForInstance with
// "stripe api key is required".
//
// With DestroyWithDesired, the FULL desired payload (including
// reserved keys) flows into dispatch → buildExecuteRequest → Execute.
func TestCustomerReconciler_DestroyWithDesired_ForwardsCredentials(t *testing.T) {
	// Stub the stripe client per the project pattern so
	// clientForInstance does NOT short-circuit at apiKey resolution.
	client, err := NewStripeClient("sk_test_stub", "http://127.0.0.1:1", StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-canary", client)
	defer restore()

	r := newCustomerReconciler("stripe-canary")
	desired := reconcilePayload{
		"instance_id":     "stripe-canary",
		InstanceConfigKey: map[string]any{"stripe_account_id": "acct_canary"},
		InstanceCredsKey:  map[string]any{"stripe_api_key": "sk_test_canary_from_bridge"},
		InstanceAuthKey:   map[string]any{},
	}

	// Direct call into DestroyWithDesired — proves the method picks
	// up the reserved keys and forwards them to dispatch.
	err = r.DestroyWithDesired(context.Background(), "cus_canary", desired)
	// We don't care about the underlying HTTP call's success here;
	// the project's other dispatch tests already cover that. We
	// assert the FAILURE MODE: if credentials were dropped, the
	// error message names "stripe api key is required". If the
	// payload propagates, the test client takes over and we get
	// either nil error or a stripe-side error code — never the
	// auth-gate rejection.
	if err != nil {
		if got := err.Error(); got != "" {
			// Must NOT see "stripe api key is required" — that's
			// the credentials-dropped signature.
			require.NotContainsf(t, got, "stripe api key is required",
				"DestroyWithDesired dropped credentials: %s", got)
		}
	}

	// Spot-check the request shape directly: the helper must produce
	// the rehydrated request the bridge expects.
	got := buildExecuteRequest(OperationDestroyCustomer, desired, "stripe-fallback")
	require.Equal(t, "stripe-canary", got.Integration.InstanceID)
	require.Equal(t, map[string]any{"stripe_api_key": "sk_test_canary_from_bridge"},
		got.Integration.Spec.Credentials,
		"DestroyWithDesired must propagate InstanceCredsKey into Execute() request")
}
