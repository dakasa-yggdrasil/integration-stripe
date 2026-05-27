package adapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
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

func TestExecute_CreatePaymentIntent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payment_intents", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"pi_test","client_secret":"pi_test_secret","status":"requires_payment_method","amount":1990,"currency":"brl"}`))
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCreatePaymentIntent,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"amount":   1990,
			"currency": "brl",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "pi_test", resp.Output["payment_intent_id"])
	require.Equal(t, "requires_payment_method", resp.Output["status"])
}

// silence unused json import until verify_webhook_signature lands in Task 32.
var _ = json.Marshal
var _ = time.Now
