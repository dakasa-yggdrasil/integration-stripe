package message

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkadapter "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
	ad "github.com/dakasa-yggdrasil/integration-stripe/providers/stripe/adapter"
)

func TestExecuteHandlerSanitizesProvisionProviderError(t *testing.T) {
	t.Parallel()

	const canary = "provider-reflected-sensitive-canary"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"`+canary+`"}}`)
	}))
	defer ts.Close()

	client, err := ad.NewStripeClient("sk_test", ts.URL, ad.StripeAPIVersion)
	require.NoError(t, err)
	restore := ad.SetStripeClientForTest("stripe-sensitive-error", client)
	defer restore()

	sdk := sdkadapter.New(sdkadapter.Config{
		Provider:        ad.Provider,
		IntegrationType: ad.IntegrationType,
		Version:         ad.AdapterVersion,
	})
	ad.WireReconcilers(sdk, "stripe-sensitive-error")
	observed, logs := observer.New(zap.ErrorLevel)
	handler := ExecuteHandler(zap.New(observed), sdk, nil)
	req := contract.AdapterExecuteIntegrationRequest{
		Operation: ad.OperationProvisionWebhookEndpoint,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-sensitive-error",
			Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
				"allow_sensitive_webhook_endpoint_creation": true,
				"webhook_endpoint_provisioning_generation":  "test-sensitive-error-v1",
			}},
		},
		Input: map[string]any{
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
			"api_version":    ad.StripeAPIVersion,
		},
		Metadata: map[string]any{
			"supports_sensitive_output_paths": true,
			"step_id":                         "provision-stripe-webhook",
			"sensitive_output_sink": map[string]any{
				"version":             "v1",
				"mode":                "transient_next_step",
				"producer_step_id":    "provision-stripe-webhook",
				"step_id":             "persist-stripe-webhook-secret",
				"family":              "secrets-management",
				"operation":           "ensure_secret",
				"input_path":          "secret.generation.manual.value",
				"source_output_paths": []any{"secret"},
			},
		},
	}
	body, err := json.Marshal(req)
	require.NoError(t, err)

	response, contentType, err := handler(context.Background(), rpc.Delivery{Body: body})
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.NotContains(t, string(response), canary)
	require.Contains(t, string(response), sensitiveOperationFailureMessage)

	encodedLogs, err := json.Marshal(logs.All())
	require.NoError(t, err)
	require.NotContains(t, string(encodedLogs), canary)
	require.Len(t, logs.All(), 1)
}
