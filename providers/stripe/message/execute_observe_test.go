package message

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
)

// Exercise the public RPC bridge: testing Execute alone misses the reconcile
// conversion that previously turned all four by-ID responses into items:null.
func TestExecuteHandlerObserveByIDPreservesItems(t *testing.T) {
	for _, tc := range []struct {
		operation, id, path, idKey, response string
	}{
		{ad.OperationObserveWebhookEndpoints, "we_legacy", "/v1/webhook_endpoints/we_legacy", "id", `{"id":"we_legacy","url":"https://example.test/legacy","status":"disabled","secret":"must-not-escape"}`},
		{ad.OperationObservePaymentIntents, "pi_one", "/v1/payment_intents/pi_one", "payment_intent_id", `{"id":"pi_one","status":"succeeded","amount":123,"currency":"brl"}`},
		{ad.OperationObserveCustomers, "cus_one", "/v1/customers/cus_one", "customer_id", `{"id":"cus_one","email":"test@example.test"}`},
		{ad.OperationObserveSubscriptions, "sub_one", "/v1/subscriptions/sub_one", "subscription_id", `{"id":"sub_one","status":"active"}`},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			var calls atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, tc.path, r.URL.Path, "by-ID observation must never list")
				require.Equal(t, "acct_observe", r.Header.Get("Stripe-Account"))
				_, _ = io.WriteString(w, tc.response)
			}))
			defer ts.Close()
			body := executeObserveRPC(t, ts.URL, tc.operation, tc.id)
			require.Equal(t, int32(1), calls.Load())
			require.NotContains(t, string(body), "must-not-escape")
			var result struct {
				OK   bool `json:"ok"`
				Data struct {
					Output struct {
						Items []map[string]any `json:"items"`
					} `json:"output"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(body, &result))
			require.True(t, result.OK, string(body))
			require.Len(t, result.Data.Output.Items, 1, string(body))
			require.Equal(t, tc.id, result.Data.Output.Items[0][tc.idKey])
			if tc.id == "we_legacy" {
				require.Equal(t, "disabled", result.Data.Output.Items[0]["status"])
				require.Equal(t, "https://example.test/legacy", result.Data.Output.Items[0]["url"])
			}
		})
	}
}

func TestExecuteHandlerObserveWebhookAbsenceAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
		absent bool
	}{
		{"missing", 404, "resource_missing", true},
		{"forbidden", 403, "permission_error", false},
		{"misleading_missing", 403, "resource_missing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/webhook_endpoints/we_absent", r.URL.Path)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"`+tc.code+`","message":"provider error"}}`)
			}))
			defer ts.Close()
			body := executeObserveRPC(t, ts.URL, ad.OperationObserveWebhookEndpoints, "we_absent")
			var result map[string]any
			require.NoError(t, json.Unmarshal(body, &result))
			require.Equal(t, tc.absent, result["ok"], string(body))
			if tc.absent {
				output := result["data"].(map[string]any)["output"].(map[string]any)
				items, ok := output["items"].([]any)
				require.True(t, ok, "absence must be [] rather than null: %s", body)
				require.Empty(t, items)
			} else {
				require.NotContains(t, result, "data")
				require.Equal(t, "execute_failed", result["error"].(map[string]any)["code"])
			}
		})
	}
}

func executeObserveRPC(t *testing.T, baseURL, operation, id string) []byte {
	t.Helper()
	instanceID := "observe-rpc-" + t.Name()
	client, err := ad.NewStripeClient("sk_test", baseURL, ad.StripeAPIVersion)
	require.NoError(t, err)
	restore := ad.SetStripeClientForTest(instanceID, client)
	t.Cleanup(restore)
	sdk := sdkadapter.New(sdkadapter.Config{Provider: ad.Provider, IntegrationType: ad.IntegrationType, Version: ad.AdapterVersion})
	ad.WireReconcilers(sdk, instanceID)
	req, err := json.Marshal(contract.AdapterExecuteIntegrationRequest{
		Operation: operation, Capability: operation,
		Integration: contract.IntegrationContext{InstanceID: instanceID},
		Input:       map[string]any{"id": id, "stripe_account": "acct_observe"},
	})
	require.NoError(t, err)
	body, contentType, err := ExecuteHandler(zap.NewNop(), sdk, nil)(context.Background(), rpc.Delivery{Body: req})
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	return body
}
