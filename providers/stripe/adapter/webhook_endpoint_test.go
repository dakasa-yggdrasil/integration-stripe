package adapter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

func TestEnsureWebhookEndpointAdoptsExactURLAndReenables(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe"
	var mu sync.Mutex
	var requests []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/webhook_endpoints":
			require.Equal(t, "100", r.URL.Query().Get("limit"))
			_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[{"id":"we_existing","url":"`+endpointURL+`","status":"disabled","livemode":true,"created":1776177009,"enabled_events":["invoice.paid"],"description":"production payments","metadata":{"yggdrasil_scope":"account","yggdrasil_instance_id":"stripe-webhook-adopt"}}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/webhook_endpoints/we_existing":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "false", r.Form.Get("disabled"))
			require.ElementsMatch(t, []string{"invoice.paid", "invoice.payment_failed", "payout.paid"}, stripeFormArray(r, "enabled_events"))
			_, _ = io.WriteString(w, `{"id":"we_existing","url":"`+endpointURL+`","status":"enabled","livemode":true,"created":1776177009,"enabled_events":["invoice.paid","invoice.payment_failed","payout.paid"],"description":"production payments"}`)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-adopt", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationEnsureWebhookEndpoint,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-webhook-adopt",
		},
		Input: map[string]any{
			"url":            endpointURL,
			"enabled_events": []any{"invoice.paid", "invoice.payment_failed", "payout.paid"},
			"disabled":       false,
			"connect":        false,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "we_existing", resp.Output["id"])
	require.Equal(t, "enabled", resp.Output["status"])
	require.Equal(t, true, resp.Output["adopted"])
	require.Equal(t, false, resp.Output["created"])
	require.Equal(t, true, resp.Output["updated"])
	require.NotContains(t, resp.Output, "secret")

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{
		"GET /v1/webhook_endpoints",
		"POST /v1/webhook_endpoints/we_existing",
	}, requests)
}

func TestEnsureWebhookEndpointRefusesToCreateWhenURLIsAbsent(t *testing.T) {
	t.Parallel()

	var postSeen bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postSeen = true
		}
		_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[]}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-absent", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationEnsureWebhookEndpoint,
		Integration: contract.IntegrationContext{InstanceID: "stripe-webhook-absent"},
		Input: map[string]any{
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe",
			"enabled_events": []any{"invoice.paid"},
			"disabled":       false,
		},
	})
	require.ErrorContains(t, err, "provision_webhook_endpoint")
	require.False(t, postSeen, "ensure must never create a signing secret outside the transient sink contract")
}

func TestEnsureWebhookEndpointRejectsAmbiguousURLAdoption(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[{"id":"we_a","url":"`+endpointURL+`","status":"enabled","enabled_events":["invoice.paid"]},{"id":"we_b","url":"`+endpointURL+`","status":"disabled","enabled_events":["invoice.paid"]}]}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-ambiguous", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationEnsureWebhookEndpoint,
		Integration: contract.IntegrationContext{InstanceID: "stripe-webhook-ambiguous"},
		Input: map[string]any{
			"url":            endpointURL,
			"enabled_events": []any{"invoice.paid"},
		},
	})
	require.ErrorContains(t, err, "multiple Stripe webhook endpoints")
}

func TestProvisionWebhookEndpointRequiresTransientSecretSinkHandshake(t *testing.T) {
	t.Parallel()

	var requestSeen bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestSeen = true
		http.Error(w, "must not reach Stripe", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-no-sink", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationProvisionWebhookEndpoint,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-webhook-no-sink",
			Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
				"allow_sensitive_webhook_endpoint_creation": true,
				"webhook_endpoint_provisioning_generation":  "test-no-sink-v1",
			}},
		},
		Input: map[string]any{
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe",
			"enabled_events": []any{"invoice.paid"},
		},
	})
	require.ErrorContains(t, err, "transient secret sink")
	require.False(t, requestSeen)
}

func TestProvisionWebhookEndpointRejectsMalformedTransientSecretSinkHandshake(t *testing.T) {
	t.Parallel()

	tests := map[string]func(map[string]any){
		"producer does not match current step": func(metadata map[string]any) {
			metadata["step_id"] = "another-producer"
		},
		"extra source path": func(metadata map[string]any) {
			metadata["sensitive_output_sink"].(map[string]any)["source_output_paths"] = []any{"secret", "id"}
		},
		"non-string source path": func(metadata map[string]any) {
			metadata["sensitive_output_sink"].(map[string]any)["source_output_paths"] = []any{"secret", 1}
		},
		"scalar source path": func(metadata map[string]any) {
			metadata["sensitive_output_sink"].(map[string]any)["source_output_paths"] = "secret"
		},
	}

	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requestSeen bool
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestSeen = true
				http.Error(w, "must not reach Stripe", http.StatusInternalServerError)
			}))
			defer ts.Close()

			client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
			require.NoError(t, err)
			restore := SetStripeClientForTest("stripe-webhook-malformed-"+strings.ReplaceAll(name, " ", "-"), client)
			defer restore()

			metadata := transientSecretSinkMetadataForTest()
			mutate(metadata)
			_, err = Execute(contract.AdapterExecuteIntegrationRequest{
				Operation: OperationProvisionWebhookEndpoint,
				Integration: contract.IntegrationContext{
					InstanceID: "stripe-webhook-malformed-" + strings.ReplaceAll(name, " ", "-"),
					Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
						"allow_sensitive_webhook_endpoint_creation": true,
						"webhook_endpoint_provisioning_generation":  "test-malformed-v1",
					}},
				},
				Input: map[string]any{
					"url":            "https://enterprise.dakasa.me/payment/webhook/stripe",
					"enabled_events": []any{"invoice.paid"},
				},
				Metadata: metadata,
			})
			require.ErrorContains(t, err, "transient secret sink")
			require.False(t, requestSeen)
		})
	}
}

func TestProvisionWebhookEndpointCreatesConnectEndpointAndMarksSecretSensitive(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe/connect"
	var requests []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[]}`)
		case http.MethodPost:
			require.NoError(t, r.ParseForm())
			require.Empty(t, r.Header.Get("Stripe-Account"))
			require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
			require.Equal(t, "true", r.Form.Get("connect"))
			require.Equal(t, "2025-10-29.clover", r.Form.Get("api_version"))
			require.Equal(t, endpointURL, r.Form.Get("url"))
			require.ElementsMatch(t, []string{"invoice.paid", "payout.paid"}, stripeFormArray(r, "enabled_events"))
			require.Equal(t, "connect", r.Form.Get("metadata[yggdrasil_scope]"))
			require.Equal(t, "stripe-webhook-provision", r.Form.Get("metadata[yggdrasil_instance_id]"))
			require.Equal(t, "test-provision-v1", r.Form.Get("metadata[yggdrasil_provisioning_generation]"))
			_, _ = io.WriteString(w, `{"id":"we_created","url":"`+endpointURL+`","status":"enabled","livemode":true,"enabled_events":["invoice.paid","payout.paid"],"secret":"whsec_create_only"}`)
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-provision", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation: OperationProvisionWebhookEndpoint,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-webhook-provision",
			Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
				"allow_sensitive_webhook_endpoint_creation": true,
				"webhook_endpoint_provisioning_generation":  "test-provision-v1",
			}},
		},
		Input: map[string]any{
			"url":            endpointURL,
			"enabled_events": []any{"invoice.paid", "payout.paid"},
			"connect":        true,
			"api_version":    "2025-10-29.clover",
		},
		Metadata: transientSecretSinkMetadataForTest(),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"GET /v1/webhook_endpoints", "POST /v1/webhook_endpoints"}, requests)
	require.Equal(t, "we_created", resp.Output["id"])
	require.Equal(t, "whsec_create_only", resp.Output["secret"])
	require.Equal(t, true, resp.Output["created"])
	require.Equal(t, "connect", resp.Output["scope"])
	require.Equal(t, []string{"secret"}, resp.Metadata["sensitive_output_paths"])
	require.Equal(t, true, resp.Metadata["secret_persistence_required"])
}

func TestProvisionWebhookEndpointHonorsCallerCancellation(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{"id":"we_unexpected","secret":"test-only-placeholder"}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-cancelled", client)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ExecuteContext(ctx, contract.AdapterExecuteIntegrationRequest{
		Operation: OperationProvisionWebhookEndpoint,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-webhook-cancelled",
			Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
				"allow_sensitive_webhook_endpoint_creation": true,
				"webhook_endpoint_provisioning_generation":  "test-cancelled-v1",
			}},
		},
		Input: map[string]any{
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
			"api_version":    StripeAPIVersion,
		},
		Metadata: transientSecretSinkMetadataForTest(),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, requests.Load(), "a cancelled workflow must not start the provider POST")
}

func TestProvisionWebhookEndpointRejectsAmbiguousScopeAndVersionBeforeStripe(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing connect": {
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events": []any{"invoice.paid"},
			"api_version":    StripeAPIVersion,
		},
		"wrong api version": {
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
			"api_version":    "2024-12-18.acacia",
		},
		"connect plus account header": {
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
			"api_version":    StripeAPIVersion,
			"stripe_account": "acct_rewards",
		},
		"non-string event": {
			"url":            "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events": []any{"invoice.paid", 1},
			"connect":        true,
			"api_version":    StripeAPIVersion,
		},
		"caller idempotency override": {
			"url":             "https://enterprise.dakasa.me/payment/webhook/stripe/connect",
			"enabled_events":  []any{"invoice.paid"},
			"connect":         true,
			"api_version":     StripeAPIVersion,
			"idempotency_key": "caller-selected",
		},
	}

	for name, input := range tests {
		name, input := name, input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var requestSeen bool
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestSeen = true
				http.Error(w, "must not reach Stripe", http.StatusInternalServerError)
			}))
			defer ts.Close()

			instanceID := "stripe-webhook-reject-" + strings.ReplaceAll(name, " ", "-")
			client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
			require.NoError(t, err)
			restore := SetStripeClientForTest(instanceID, client)
			defer restore()

			_, err = Execute(contract.AdapterExecuteIntegrationRequest{
				Operation: OperationProvisionWebhookEndpoint,
				Integration: contract.IntegrationContext{
					InstanceID: instanceID,
					Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
						"allow_sensitive_webhook_endpoint_creation": true,
						"webhook_endpoint_provisioning_generation":  "test-rejection-v1",
					}},
				},
				Input:    input,
				Metadata: transientSecretSinkMetadataForTest(),
			})
			require.Error(t, err)
			require.False(t, requestSeen)
		})
	}
}

func TestExecuteBindsInstanceStripeAccountAndRejectsOverride(t *testing.T) {
	t.Parallel()

	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		require.Equal(t, "acct_bound", r.Header.Get("Stripe-Account"))
		_, _ = io.WriteString(w, `{"object":"balance","available":[],"pending":[]}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-account-binding", client)
	defer restore()

	base := contract.AdapterExecuteIntegrationRequest{
		Operation: OperationObserveBalance,
		Integration: contract.IntegrationContext{
			InstanceID: "stripe-account-binding",
			Spec: contract.IntegrationInstanceManifestSpec{Config: map[string]any{
				"stripe_account_id": "acct_bound",
			}},
		},
		Input: map[string]any{},
	}
	_, err = Execute(base)
	require.NoError(t, err)
	require.Equal(t, 1, requestCount)

	base.Input = map[string]any{"stripe_account": "acct_other"}
	_, err = Execute(base)
	require.ErrorContains(t, err, "conflicts")
	require.Equal(t, 1, requestCount)
}

func TestEnsureWebhookEndpointPreservesProviderOwnedScopeMetadata(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe/connect"
	var postSeen bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[{"id":"we_connect","url":"`+endpointURL+`","status":"enabled","enabled_events":["invoice.paid"],"metadata":{"yggdrasil_scope":"connect","yggdrasil_instance_id":"stripe-webhook-preserve","old":"remove"}}]}`)
		case http.MethodPost:
			postSeen = true
			require.NoError(t, r.ParseForm())
			require.Equal(t, "connect", r.Form.Get("metadata[yggdrasil_scope]"))
			require.Equal(t, "stripe-webhook-preserve", r.Form.Get("metadata[yggdrasil_instance_id]"))
			require.Equal(t, "payments", r.Form.Get("metadata[purpose]"))
			require.Equal(t, "", r.Form.Get("metadata[old]"))
			_, _ = io.WriteString(w, `{"id":"we_connect","url":"`+endpointURL+`","status":"enabled","enabled_events":["invoice.paid"],"metadata":{"yggdrasil_scope":"connect","yggdrasil_instance_id":"stripe-webhook-preserve","purpose":"payments"}}`)
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-preserve", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationEnsureWebhookEndpoint,
		Integration: contract.IntegrationContext{InstanceID: "stripe-webhook-preserve"},
		Input: map[string]any{
			"url":            endpointURL,
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
			"metadata":       map[string]any{"purpose": "payments"},
		},
	})
	require.NoError(t, err)
	require.True(t, postSeen)
	require.Equal(t, "connect", resp.Output["scope"])
	require.Equal(t, "stripe-webhook-preserve", resp.Output["metadata"].(map[string]string)["yggdrasil_instance_id"])
}

func TestEnsureWebhookEndpointRejectsProviderOwnedScopeMetadataOverride(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe/connect"
	var postSeen bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postSeen = true
		}
		_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[{"id":"we_connect","url":"`+endpointURL+`","status":"enabled","enabled_events":["invoice.paid"],"metadata":{"yggdrasil_scope":"connect","yggdrasil_instance_id":"stripe-webhook-no-override"}}]}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-no-override", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationEnsureWebhookEndpoint,
		Integration: contract.IntegrationContext{InstanceID: "stripe-webhook-no-override"},
		Input: map[string]any{
			"url":            endpointURL,
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
			"metadata":       map[string]any{"yggdrasil_scope": "account"},
		},
	})
	require.ErrorContains(t, err, "provider-owned")
	require.False(t, postSeen)
}

func TestEnsureWebhookEndpointRejectsDifferentInstanceOwner(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe/connect"
	var postSeen bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postSeen = true
		}
		_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[{"id":"we_connect","url":"`+endpointURL+`","status":"enabled","enabled_events":["invoice.paid"],"metadata":{"yggdrasil_scope":"connect","yggdrasil_instance_id":"different-instance"}}]}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-owner", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationEnsureWebhookEndpoint,
		Integration: contract.IntegrationContext{InstanceID: "stripe-webhook-owner"},
		Input: map[string]any{
			"url":            endpointURL,
			"enabled_events": []any{"invoice.paid"},
			"connect":        true,
		},
	})
	require.ErrorContains(t, err, "ownership cannot be proven")
	require.False(t, postSeen)
}

func TestObserveWebhookEndpointsReturnsProviderBackedScope(t *testing.T) {
	t.Parallel()

	const endpointURL = "https://enterprise.dakasa.me/payment/webhook/stripe/connect"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"object":"list","has_more":false,"data":[{"id":"we_connect","url":"`+endpointURL+`","status":"enabled","livemode":true,"created":1776177009,"api_version":"2025-10-29.clover","enabled_events":["invoice.paid"],"metadata":{"yggdrasil_scope":"connect","yggdrasil_instance_id":"stripe-webhook-observe"}}]}`)
	}))
	defer ts.Close()

	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("stripe-webhook-observe", client)
	defer restore()

	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationObserveWebhookEndpoints,
		Integration: contract.IntegrationContext{InstanceID: "stripe-webhook-observe"},
		Input:       map[string]any{"limit": 10},
	})
	require.NoError(t, err)
	items := resp.Output["items"].([]map[string]any)
	require.Len(t, items, 1)
	require.Equal(t, "connect", items[0]["scope"])
	require.Equal(t, true, items[0]["livemode"])
	require.Equal(t, int64(1776177009), items[0]["created_at"])
	require.Equal(t, "2025-10-29.clover", items[0]["api_version"])
}

func TestWebhookProvisioningGenerationControlsRecoveryIdempotency(t *testing.T) {
	t.Parallel()

	firstAttempt := idempotencyKeyOrDerived("", "provision_we",
		webhookProvisionAttempt("https://enterprise.dakasa.me/payment/webhook/stripe/connect", true, "", "release-v1"))
	repeatedAttempt := idempotencyKeyOrDerived("", "provision_we",
		webhookProvisionAttempt("https://enterprise.dakasa.me/payment/webhook/stripe/connect", true, "", "release-v1"))
	recoveryAttempt := idempotencyKeyOrDerived("", "provision_we",
		webhookProvisionAttempt("https://enterprise.dakasa.me/payment/webhook/stripe/connect", true, "", "release-v2"))

	require.Equal(t, firstAttempt, repeatedAttempt, "retries within one attempt must reuse the provider key")
	require.NotEqual(t, firstAttempt, recoveryAttempt, "recovery after destroy must select a fresh provider key")

	for name, config := range map[string]map[string]any{
		"missing":       {},
		"wrong type":    {"webhook_endpoint_provisioning_generation": true},
		"empty":         {"webhook_endpoint_provisioning_generation": "  "},
		"invalid slash": {"webhook_endpoint_provisioning_generation": "release/v1"},
	} {
		name, config := name, config
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := webhookProvisioningGeneration(config)
			require.Error(t, err)
		})
	}
}

func transientSecretSinkMetadataForTest() map[string]any {
	return map[string]any{
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
	}
}

func stripeFormArray(r *http.Request, field string) []string {
	values := make([]string, 0)
	for key, entries := range r.Form {
		if key == field || strings.HasPrefix(key, field+"[") {
			values = append(values, entries...)
		}
	}
	sort.Strings(values)
	return values
}
