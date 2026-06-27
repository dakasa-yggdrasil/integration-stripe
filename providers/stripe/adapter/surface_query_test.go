package adapter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// runSurfaceQuery is a small harness mirroring the grafana sibling's
// surface_query_reads_test.go: it points a fresh Stripe client at the given
// httptest.Server, registers it for the "dakasa" instance, then invokes
// Execute(on_surface_query, {query_name, params}) and returns the Output bag.
// It asserts the call succeeded; per-query shape assertions live in the
// individual tests.
func runSurfaceQuery(t *testing.T, ts *httptest.Server, queryName string, params map[string]any) map[string]any {
	t.Helper()
	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("dakasa", client)
	t.Cleanup(restore)

	input := map[string]any{"query_name": queryName}
	if params != nil {
		input["params"] = params
	}
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       input,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Output)
	return resp.Output
}

// TestOnSurfaceQuery_ListWebhookEndpoints is the webhook-health pillar — the
// contract's canonical signal. It projects observe_webhook_endpoints rows into
// {id, url, status, enabled_events, api_version}.
func TestOnSurfaceQuery_ListWebhookEndpoints(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/webhook_endpoints", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		_, _ = w.Write([]byte(`{"object":"list","has_more":false,"data":[
			{"id":"we_1","url":"https://a.dakasa.me/wh","status":"enabled","enabled_events":["payment_intent.succeeded","charge.refunded"],"api_version":"2024-12-18.acacia"},
			{"id":"we_2","url":"https://b.dakasa.me/wh","status":"disabled","enabled_events":["*"],"api_version":""}
		]}`))
	}))
	defer ts.Close()

	out := runSurfaceQuery(t, ts, "list-webhook-endpoints", nil)
	items, ok := out["items"].([]map[string]any)
	require.True(t, ok, "items must be []map[string]any, got %T", out["items"])
	require.Len(t, items, 2)

	first := items[0]
	require.Equal(t, "we_1", first["id"])
	require.Equal(t, "https://a.dakasa.me/wh", first["url"])
	require.Equal(t, "enabled", first["status"])
	require.Equal(t, "2024-12-18.acacia", first["api_version"])
	events, ok := first["enabled_events"].([]string)
	require.True(t, ok, "enabled_events must be []string, got %T", first["enabled_events"])
	require.Equal(t, []string{"payment_intent.succeeded", "charge.refunded"}, events)
}

// TestOnSurfaceQuery_GetBalance returns the available + pending arrays straight
// from observe_balance, amounts kept in the smallest unit (the UI formats).
func TestOnSurfaceQuery_GetBalance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/balance", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		_, _ = w.Write([]byte(`{"object":"balance","available":[{"amount":150000,"currency":"brl"}],"pending":[{"amount":2500,"currency":"brl"}]}`))
	}))
	defer ts.Close()

	out := runSurfaceQuery(t, ts, "get-balance", nil)

	available, ok := out["available"].([]map[string]any)
	require.True(t, ok, "available must be []map[string]any, got %T", out["available"])
	require.Len(t, available, 1)
	require.Equal(t, int64(150000), available[0]["amount"])
	require.Equal(t, "brl", available[0]["currency"])

	pending, ok := out["pending"].([]map[string]any)
	require.True(t, ok, "pending must be []map[string]any, got %T", out["pending"])
	require.Len(t, pending, 1)
	require.Equal(t, int64(2500), pending[0]["amount"])
}

// TestOnSurfaceQuery_ListCharges projects observe_charges rows into
// {id, amount, currency, status, created, refunded, payment_intent}. The
// param `limit` is threaded through. Customer-identifying fields (name/email)
// are intentionally NOT projected — only opaque refs (id / payment_intent).
func TestOnSurfaceQuery_ListCharges(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/charges", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "3", r.URL.Query().Get("limit"))
		_, _ = w.Write([]byte(`{"object":"list","has_more":false,"data":[
			{"id":"ch_1","amount":1990,"currency":"brl","status":"succeeded","created":1700000000,"refunded":false,"payment_intent":"pi_1","billing_details":{"email":"host@dakasa.io","name":"Dakasa Host"}},
			{"id":"ch_2","amount":500,"currency":"brl","status":"succeeded","created":1700000100,"refunded":true,"payment_intent":"pi_2"}
		]}`))
	}))
	defer ts.Close()

	out := runSurfaceQuery(t, ts, "list-charges", map[string]any{"limit": 3})
	items, ok := out["items"].([]map[string]any)
	require.True(t, ok, "items must be []map[string]any, got %T", out["items"])
	require.Len(t, items, 2)

	first := items[0]
	require.Equal(t, "ch_1", first["id"])
	require.Equal(t, int64(1990), first["amount"])
	require.Equal(t, "brl", first["currency"])
	require.Equal(t, "succeeded", first["status"])
	require.Equal(t, int64(1700000000), first["created"])
	require.Equal(t, false, first["refunded"])
	require.Equal(t, "pi_1", first["payment_intent"])
	// rule #0: no customer-identifying projection, even when present upstream.
	_, hasEmail := first["email"]
	require.False(t, hasEmail, "charge row must not project customer email (rule #0)")
	_, hasName := first["name"]
	require.False(t, hasName, "charge row must not project customer name (rule #0)")

	second := items[1]
	require.Equal(t, true, second["refunded"])
	require.Equal(t, "pi_2", second["payment_intent"])
}

// TestOnSurfaceQuery_UnknownQuery returns an error for an unrouted query_name
// so the surface gets an honest failure instead of a silent empty result.
func TestOnSurfaceQuery_UnknownQuery(t *testing.T) {
	client, err := NewStripeClient("sk_test", "http://nowhere.invalid", StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{"query_name": "disputes"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown query")
	require.Contains(t, err.Error(), "disputes")
}

// TestOnSurfaceQuery_MissingQueryName errors when query_name is absent.
func TestOnSurfaceQuery_MissingQueryName(t *testing.T) {
	client, err := NewStripeClient("sk_test", "http://nowhere.invalid", StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "query_name")
}

// TestSpec_OnSurfaceQuery_InCatalogAsReactor pins the spec wiring: the op is in
// SupportedExecuteOperations, present in the ActionCatalog, and categorized as
// a reactor (on_ prefix → hidden from grant pickers).
func TestSpec_OnSurfaceQuery_InCatalogAsReactor(t *testing.T) {
	desc := Describe()
	var found *contract.IntegrationActionDefinition
	for i := range desc.ActionCatalog {
		if desc.ActionCatalog[i].Name == OperationOnSurfaceQuery {
			found = &desc.ActionCatalog[i]
			break
		}
	}
	require.NotNil(t, found, "on_surface_query must be in ActionCatalog")
	require.Equal(t, "reactor", found.Category, "on_ prefix classifies as reactor")

	inSupported := false
	for _, op := range SupportedExecuteOperations {
		if op == OperationOnSurfaceQuery {
			inSupported = true
			break
		}
	}
	require.True(t, inSupported, "on_surface_query must be in SupportedExecuteOperations")
}
