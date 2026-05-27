package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/rpc"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"
)

// TestE2E_RegisterReconciler_EnsurePaymentIntent drives the adapter
// through the SDK-level reconcile dispatch path (mirrors Task 39 step 2
// from the rollout plan).
func TestE2E_RegisterReconciler_EnsurePaymentIntent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment_intents" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"pi_e2e","client_secret":"pi_e2e_secret","status":"requires_payment_method","amount":2500,"currency":"brl"}`))
	}))
	defer ts.Close()
	client, err := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	if err != nil {
		t.Fatalf("NewStripeClient: %v", err)
	}
	restore := SetStripeClientForTest("dakasa", client)
	defer restore()

	a := adapter.New(adapter.Config{Provider: Provider, IntegrationType: IntegrationType, Version: AdapterVersion})
	WireReconcilers(a, "dakasa")

	body := []byte(`{"operation":"ensure_payment_intent","input":{"amount":2500,"currency":"brl"}}`)
	resp, _, err := reconcile.ExecuteForTest(context.Background(), a, rpc.Delivery{Body: body})
	if err != nil {
		t.Fatalf("ensure_payment_intent dispatch failed: %v", err)
	}
	if !strings.Contains(string(resp), "pi_e2e") {
		t.Fatalf("expected pi_e2e in response, got %s", resp)
	}
}

// TestE2E_StripeLegacyCreatePaymentIntentShim confirms the SDK-level
// WithLegacyNames shim routes a pre-v2.0.0 capability name through
// the canonical handler, emitting one WARN log entry.
func TestE2E_StripeLegacyCreatePaymentIntentShim(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"pi_legacy","client_secret":"pi_legacy_secret","status":"requires_payment_method","amount":1000,"currency":"brl"}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa-legacy", client)
	defer restore()

	var warns int
	a := adapter.New(adapter.Config{Provider: Provider, IntegrationType: IntegrationType, Version: AdapterVersion})
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "payment_intent", "payment_intents",
		newPaymentIntentReconciler("dakasa-legacy"),
		reconcile.WithLegacyNames("create_payment_intent"),
		reconcile.WithWarnLogger(func(string, ...any) { warns++ }),
	)
	body := []byte(`{"operation":"create_payment_intent","input":{"amount":1000,"currency":"brl"}}`)
	resp, _, err := reconcile.ExecuteForTest(context.Background(), a, rpc.Delivery{Body: body})
	if err != nil {
		t.Fatalf("legacy create_payment_intent dispatch failed: %v", err)
	}
	if !strings.Contains(string(resp), "pi_legacy") {
		t.Fatalf("expected pi_legacy in legacy-shim response, got %s", resp)
	}
	if warns != 1 {
		t.Fatalf("expected 1 WARN entry, got %d", warns)
	}
}

// TestE2E_RegisterReconciler_DestroyCustomer drives the destroy_customer
// canonical handler through the SDK reconcile dispatch.
func TestE2E_RegisterReconciler_DestroyCustomer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/customers/cus_xyz" || r.Method != "DELETE" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"cus_xyz","object":"customer","deleted":true}`))
	}))
	defer ts.Close()
	client, _ := NewStripeClient("sk_test", ts.URL, StripeAPIVersion)
	restore := SetStripeClientForTest("dakasa-destroy", client)
	defer restore()

	a := adapter.New(adapter.Config{Provider: Provider, IntegrationType: IntegrationType, Version: AdapterVersion})
	WireReconcilers(a, "dakasa-destroy")

	body := []byte(`{"operation":"destroy_customer","input":{"ref":"cus_xyz"}}`)
	resp, _, err := reconcile.ExecuteForTest(context.Background(), a, rpc.Delivery{Body: body})
	if err != nil {
		t.Fatalf("destroy_customer dispatch failed: %v", err)
	}
	if !strings.Contains(string(resp), `"deleted":true`) {
		t.Fatalf("expected deleted:true in response, got %s", resp)
	}
}
