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

func TestExecute_ConfirmPaymentIntent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payment_intents/pi_abc/confirm", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"pi_abc","status":"requires_action","next_action":{"redirect_to_url":{"url":"https://stripe.com/3ds"}}}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationConfirmPaymentIntent,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{"payment_intent_id": "pi_abc"},
	})
	require.NoError(t, err)
	require.Equal(t, "pi_abc", resp.Output["payment_intent_id"])
	require.Equal(t, "requires_action", resp.Output["status"])
	require.NotNil(t, resp.Output["next_action"])
}

func TestExecute_CancelPaymentIntent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payment_intents/pi_abc/cancel", r.URL.Path)
		require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"pi_abc","status":"canceled","cancellation_reason":"requested_by_customer"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCancelPaymentIntent,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"payment_intent_id":   "pi_abc",
			"cancellation_reason": "requested_by_customer",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "canceled", resp.Output["status"])
	require.Equal(t, "requested_by_customer", resp.Output["cancellation_reason"])
}

func TestExecute_CreateCustomer(t *testing.T) {
	var seenIdempotencyKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/customers", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		seenIdempotencyKey = r.Header.Get("Idempotency-Key")
		_, _ = w.Write([]byte(`{"id":"cus_abc","object":"customer","email":"host@dakasa.io","created":1700000000}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCreateCustomer,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"email": "host@dakasa.io",
			"name":  "Dakasa Host",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "cus_abc", resp.Output["customer_id"])
	require.Equal(t, "create_customer_host@dakasa.io", seenIdempotencyKey)
}

// silence unused json import until verify_webhook_signature lands in Task 32.
var _ = json.Marshal
var _ = time.Now
