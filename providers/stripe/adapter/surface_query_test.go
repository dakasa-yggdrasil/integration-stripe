package adapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestOnSurfaceQuery_ListSubscriptions projects observe_subscriptions list rows
// into the deeper payments-ops shape: {id, status, plan{nickname,price_id},
// amount, currency, current_period_end, cancel_at_period_end}. The customer is
// projected ONLY as an opaque `customer` id ref — never name / email (rule #0).
func TestOnSurfaceQuery_ListSubscriptions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/subscriptions", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "2", r.URL.Query().Get("limit"))
		// Two subs: first carries a Price (nickname/unit_amount), second a
		// legacy Plan, and the customer object is expanded with PII to prove
		// the projection drops it.
		_, _ = w.Write([]byte(`{"object":"list","has_more":false,"data":[
			{"id":"sub_1","status":"active","cancel_at_period_end":false,"currency":"brl",
			 "customer":{"id":"cus_1","email":"host@dakasa.io","name":"Dakasa Host"},
			 "items":{"object":"list","data":[
			   {"id":"si_1","current_period_end":1700001234,
			    "price":{"id":"price_1","nickname":"Pro Monthly","unit_amount":4990,"currency":"brl"}}
			 ]}},
			{"id":"sub_2","status":"past_due","cancel_at_period_end":true,"currency":"usd",
			 "customer":"cus_2",
			 "items":{"object":"list","data":[
			   {"id":"si_2","current_period_end":1700009999,
			    "plan":{"id":"plan_2","nickname":"Legacy Gold","amount":1000,"currency":"usd"}}
			 ]}}
		]}`))
	}))
	defer ts.Close()

	out := runSurfaceQuery(t, ts, "list-subscriptions", map[string]any{"limit": 2})
	items, ok := out["items"].([]map[string]any)
	require.True(t, ok, "items must be []map[string]any, got %T", out["items"])
	require.Len(t, items, 2)

	first := items[0]
	require.Equal(t, "sub_1", first["id"])
	require.Equal(t, "active", first["status"])
	require.Equal(t, int64(4990), first["amount"])
	require.Equal(t, "brl", first["currency"])
	require.Equal(t, int64(1700001234), first["current_period_end"])
	require.Equal(t, false, first["cancel_at_period_end"])
	// customer is an opaque id ref only.
	require.Equal(t, "cus_1", first["customer"])
	plan, ok := first["plan"].(map[string]any)
	require.True(t, ok, "plan must be map[string]any, got %T", first["plan"])
	require.Equal(t, "Pro Monthly", plan["nickname"])
	require.Equal(t, "price_1", plan["price_id"])
	// rule #0: no customer-identifying projection anywhere on the row.
	_, hasEmail := first["email"]
	require.False(t, hasEmail, "subscription row must not project customer email (rule #0)")
	_, hasName := first["name"]
	require.False(t, hasName, "subscription row must not project customer name (rule #0)")

	second := items[1]
	require.Equal(t, "sub_2", second["id"])
	require.Equal(t, "past_due", second["status"])
	require.Equal(t, true, second["cancel_at_period_end"])
	require.Equal(t, "cus_2", second["customer"])
	require.Equal(t, int64(1700009999), second["current_period_end"])
	// legacy Plan path: amount from plan, nickname from plan, price_id from plan id.
	require.Equal(t, int64(1000), second["amount"])
	require.Equal(t, "usd", second["currency"])
	plan2, ok := second["plan"].(map[string]any)
	require.True(t, ok, "plan must be map[string]any, got %T", second["plan"])
	require.Equal(t, "Legacy Gold", plan2["nickname"])
	require.Equal(t, "plan_2", plan2["price_id"])
}

// TestOnSurfaceQuery_ListPaymentIntents projects observe_payment_intents list
// rows into {id, status, amount, currency, created, capture_method}. No customer
// PII is projected — only the opaque intent id.
func TestOnSurfaceQuery_ListPaymentIntents(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/payment_intents", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "5", r.URL.Query().Get("limit"))
		_, _ = w.Write([]byte(`{"object":"list","has_more":false,"data":[
			{"id":"pi_1","status":"succeeded","amount":1990,"currency":"brl","created":1700000000,"capture_method":"automatic","receipt_email":"host@dakasa.io"},
			{"id":"pi_2","status":"requires_capture","amount":500,"currency":"brl","created":1700000100,"capture_method":"manual"}
		]}`))
	}))
	defer ts.Close()

	out := runSurfaceQuery(t, ts, "list-payment-intents", map[string]any{"limit": 5})
	items, ok := out["items"].([]map[string]any)
	require.True(t, ok, "items must be []map[string]any, got %T", out["items"])
	require.Len(t, items, 2)

	first := items[0]
	require.Equal(t, "pi_1", first["id"])
	require.Equal(t, "succeeded", first["status"])
	require.Equal(t, int64(1990), first["amount"])
	require.Equal(t, "brl", first["currency"])
	require.Equal(t, int64(1700000000), first["created"])
	require.Equal(t, "automatic", first["capture_method"])
	// rule #0: no customer-identifying projection.
	_, hasEmail := first["receipt_email"]
	require.False(t, hasEmail, "payment-intent row must not project receipt_email (rule #0)")
	_, hasEmail2 := first["email"]
	require.False(t, hasEmail2, "payment-intent row must not project email (rule #0)")

	second := items[1]
	require.Equal(t, "pi_2", second["id"])
	require.Equal(t, "requires_capture", second["status"])
	require.Equal(t, "manual", second["capture_method"])
}

// TestOnSurfaceQuery_ChargeDetail is the reconciliation drill-down. Given a
// required charge_id param it retrieves a single charge (refunds expanded) and
// projects {id, amount, currency, status, created, refunded, refundedAmount,
// payment_intent, failureCode, failureMessage, refunds:[{id,amount,reason,created}]}.
// Only opaque refs leave the projection (rule #0).
func TestOnSurfaceQuery_ChargeDetail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/charges/ch_1", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		// refunds must be expanded so the list is materialized. stripe-go
		// serializes expand as an indexed key (expand[0]=refunds), so scan
		// every decoded query param for an "expand"-prefixed key carrying
		// "refunds" rather than asserting a fixed key.
		expandedRefunds := false
		for key, vals := range r.URL.Query() {
			if strings.HasPrefix(key, "expand") {
				for _, v := range vals {
					if v == "refunds" {
						expandedRefunds = true
					}
				}
			}
		}
		require.True(t, expandedRefunds, "charge-detail must expand refunds; got query %q", r.URL.RawQuery)
		_, _ = w.Write([]byte(`{
			"id":"ch_1","amount":1990,"currency":"brl","status":"succeeded","created":1700000000,
			"refunded":true,"amount_refunded":1990,
			"payment_intent":{"id":"pi_1","customer":"cus_1"},
			"failure_code":"","failure_message":"",
			"billing_details":{"email":"host@dakasa.io","name":"Dakasa Host"},
			"refunds":{"object":"list","has_more":false,"data":[
				{"id":"re_1","amount":1000,"reason":"requested_by_customer","created":1700000050},
				{"id":"re_2","amount":990,"reason":"duplicate","created":1700000060}
			]}
		}`))
	}))
	defer ts.Close()

	out := runSurfaceQuery(t, ts, "charge-detail", map[string]any{"charge_id": "ch_1"})

	require.Equal(t, "ch_1", out["id"])
	require.Equal(t, int64(1990), out["amount"])
	require.Equal(t, "brl", out["currency"])
	require.Equal(t, "succeeded", out["status"])
	require.Equal(t, int64(1700000000), out["created"])
	require.Equal(t, true, out["refunded"])
	require.Equal(t, int64(1990), out["refundedAmount"])
	require.Equal(t, "pi_1", out["payment_intent"]) // opaque ref only
	require.Equal(t, "", out["failureCode"])
	require.Equal(t, "", out["failureMessage"])

	refunds, ok := out["refunds"].([]map[string]any)
	require.True(t, ok, "refunds must be []map[string]any, got %T", out["refunds"])
	require.Len(t, refunds, 2)
	require.Equal(t, "re_1", refunds[0]["id"])
	require.Equal(t, int64(1000), refunds[0]["amount"])
	require.Equal(t, "requested_by_customer", refunds[0]["reason"])
	require.Equal(t, int64(1700000050), refunds[0]["created"])
	require.Equal(t, "re_2", refunds[1]["id"])
	require.Equal(t, "duplicate", refunds[1]["reason"])

	// rule #0: no customer-identifying projection anywhere on the object.
	_, hasEmail := out["email"]
	require.False(t, hasEmail, "charge-detail must not project customer email (rule #0)")
	_, hasName := out["name"]
	require.False(t, hasName, "charge-detail must not project customer name (rule #0)")
}

// TestOnSurfaceQuery_ChargeDetail_MissingChargeID errors when the required
// charge_id param is absent — an honest failure, not an empty object.
func TestOnSurfaceQuery_ChargeDetail_MissingChargeID(t *testing.T) {
	client, err := NewStripeClient("sk_test", "http://nowhere.invalid", StripeAPIVersion)
	require.NoError(t, err)
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	_, err = Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   OperationOnSurfaceQuery,
		Integration: contract.IntegrationContext{InstanceID: "dakasa"},
		Input:       map[string]any{"query_name": "charge-detail"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "charge-detail")
	require.Contains(t, err.Error(), "charge_id")
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
