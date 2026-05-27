package adapter

import (
	"encoding/json"
	"io"
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

func TestExecute_UpdateCustomer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/customers/cus_abc", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"cus_abc","object":"customer","email":"new@dakasa.io"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationUpdateCustomer,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"customer_id": "cus_abc",
			"email":       "new@dakasa.io",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "cus_abc", resp.Output["customer_id"])
	require.Equal(t, true, resp.Output["updated"])
}

func TestExecute_CreateSubscription(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/subscriptions", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), "items[0][price]=price_test")
		require.Contains(t, string(body), "payment_behavior=default_incomplete")
		_, _ = w.Write([]byte(`{"id":"sub_test","status":"incomplete","latest_invoice":"in_test"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCreateSubscription,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"customer": "cus_abc",
			"items": []any{
				map[string]any{"price": "price_test", "quantity": 1},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "sub_test", resp.Output["subscription_id"])
	require.Equal(t, "incomplete", resp.Output["status"])
}

func TestExecute_CancelSubscription_Immediate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/subscriptions/sub_abc", r.URL.Path)
		require.Equal(t, "DELETE", r.Method)
		_, _ = w.Write([]byte(`{"id":"sub_abc","status":"canceled","cancel_at_period_end":false,"canceled_at":1700000000}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCancelSubscription,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{"subscription_id": "sub_abc"},
	})
	require.NoError(t, err)
	require.Equal(t, "canceled", resp.Output["status"])
	require.Equal(t, false, resp.Output["cancel_at_period_end"])
}

func TestExecute_CancelSubscription_AtPeriodEnd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/subscriptions/sub_abc", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), "cancel_at_period_end=true")
		_, _ = w.Write([]byte(`{"id":"sub_abc","status":"active","cancel_at_period_end":true}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCancelSubscription,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"subscription_id":      "sub_abc",
			"cancel_at_period_end": true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, true, resp.Output["cancel_at_period_end"])
}

func TestExecute_CreateRefund(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/refunds", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"re_test","status":"succeeded","amount":500,"charge":"ch_abc"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCreateRefund,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"charge": "ch_abc",
			"amount": 500,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "re_test", resp.Output["refund_id"])
	require.Equal(t, "succeeded", resp.Output["status"])
}

func TestExecute_CreateSetupIntent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/setup_intents", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		body, _ := io.ReadAll(r.Body)
		require.Contains(t, string(body), "usage=off_session")
		_, _ = w.Write([]byte(`{"id":"seti_test","client_secret":"seti_secret","status":"requires_payment_method"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCreateSetupIntent,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"customer": "cus_xyz",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "seti_test", resp.Output["setup_intent_id"])
	require.Equal(t, "seti_secret", resp.Output["client_secret"])
}

func TestExecute_ListCharges(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/charges", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "cus_abc", r.URL.Query().Get("customer"))
		require.Equal(t, "2", r.URL.Query().Get("limit"))
		_, _ = w.Write([]byte(`{"object":"list","has_more":false,"data":[
			{"id":"ch_1","amount":1000,"currency":"brl","status":"succeeded"},
			{"id":"ch_2","amount":2000,"currency":"brl","status":"succeeded"}
		]}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationListCharges,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"customer": "cus_abc",
			"limit":    2,
		},
	})
	require.NoError(t, err)
	charges, ok := resp.Output["charges"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, charges, 2)
	require.Equal(t, false, resp.Output["has_more"])
}

func TestExecute_CreatePayout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payouts", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "acct_xyz", r.Header.Get("Stripe-Account"))
		_, _ = w.Write([]byte(`{"id":"po_test","status":"pending","arrival_date":1700000000,"method":"standard"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationCreatePayout,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"amount":         1000,
			"currency":       "brl",
			"stripe_account": "acct_xyz",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "po_test", resp.Output["payout_id"])
	require.Equal(t, "pending", resp.Output["status"])
	require.Equal(t, "standard", resp.Output["method"])
}

func TestExecute_ManageConnectAccount_Create(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/accounts", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		_, _ = w.Write([]byte(`{"id":"acct_abc","type":"express","country":"BR","charges_enabled":false,"payouts_enabled":false,"details_submitted":false}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationManageConnectAccount,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"operation": "create",
			"type":      "express",
			"country":   "BR",
			"email":     "host@dakasa.io",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "acct_abc", resp.Output["account_id"])
}

func TestExecute_ManageConnectAccount_Get(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/accounts/acct_abc", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		_, _ = w.Write([]byte(`{"id":"acct_abc","type":"express","country":"BR","charges_enabled":true,"payouts_enabled":true,"details_submitted":true}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationManageConnectAccount,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{"operation": "get", "account_id": "acct_abc"},
	})
	require.NoError(t, err)
	require.Equal(t, true, resp.Output["charges_enabled"])
}

func TestExecute_ManageConnectAccount_UnsupportedOperation(t *testing.T) {
	client, _ := NewStripeClient("sk_test", "http://nowhere.invalid", StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	_, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationManageConnectAccount,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{"operation": "delete"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported_operation")
}

func TestExecute_VerifyWebhookSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_xyz","type":"payment_intent.succeeded","livemode":false}`)
	secret := "whsec_demo"
	tsUnix := time.Now().Unix()
	sigHeader := makeStripeSig(payload, []byte(secret), tsUnix)

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationVerifyWebhookSig,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input: map[string]any{
			"payload":           string(payload),
			"stripe_signature":  sigHeader,
			"endpoint_secret":   secret,
			"tolerance_seconds": int64(300),
		},
	})
	require.NoError(t, err)
	require.Equal(t, true, resp.Output["valid"])
	require.Equal(t, "evt_xyz", resp.Output["event_id"])
	require.Equal(t, "payment_intent.succeeded", resp.Output["event_type"])
}

// silence unused json import (used by verify_webhook_signature impl) when tests
// elsewhere don't reach json.Marshal.
var _ = json.Marshal
