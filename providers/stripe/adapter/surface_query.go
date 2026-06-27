package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/stripe/stripe-go/v83"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// onSurfaceQuery is the read-only dispatcher behind the on_surface_query
// execute op. Core's /api/v1/integrations/{instance_id}/surface-query proxy
// hands it { query_name, params } as Input; it routes by query_name to a
// provider-specific read aggregator that returns a JSON shape the payments-ops
// console surface renders directly. New surface tabs add new branches here; the
// surface read contract is the union of query_name strings accepted.
//
// All three wired reads are pure GET wrappers over the existing observe_*
// handlers (same Stripe client, same httptest seam) re-projected for the
// surface. Read-only — they never mutate Stripe state.
//
// rule #0 (customer-data minimization): customer-identifying fields stay as
// opaque refs. list-charges projects only id / payment_intent — never the
// customer name or email Stripe may carry on a charge's billing_details. A
// payments-ops operator reconciles by reference, not by PII.
//
// needs-work: disputes — there is no observe_disputes op on this adapter, and
// Stripe disputes are reconstructed downstream from the RTA event log /
// Prometheus, not read here. The surface ships a disputes tab as an honest
// empty-state + deep-link rather than faking rows. Wiring it means a real
// /v1/disputes client + parser + tests; do not stub it.
//
// needs-work: payout history — same shape as disputes. create_payout exists
// (money-movement, write-only, out of scope for reads) but there is no
// observe_payouts read op to list historical payouts. Left unwired, not faked.
//
// needs-work: /metrics-derived signals — signature-failure rate and
// rta-emit-errors live on the adapter's health-port /metrics (see metrics.go:
// StripeSigFailures / StripeRTAEmitErrors), NOT as a surface-query. Surfacing
// them on the surface needs a core passthrough to the adapter health port;
// it is not a Stripe-API read and is out of scope here.
func onSurfaceQuery(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	queryName := strings.TrimSpace(stringOr(req.Input, "query_name"))
	if queryName == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("query_name is required")
	}
	params := asAnyMap(req.Input["params"])

	switch queryName {
	case "list-webhook-endpoints":
		out, err := surfaceWebhookEndpoints(ctx, c, req)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("list-webhook-endpoints: %w", err)
		}
		return contract.AdapterExecuteIntegrationResponse{Output: out}, nil

	case "get-balance":
		out, err := surfaceBalance(ctx, c, req)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("get-balance: %w", err)
		}
		return contract.AdapterExecuteIntegrationResponse{Output: out}, nil

	case "list-charges":
		out, err := surfaceCharges(ctx, c, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("list-charges: %w", err)
		}
		return contract.AdapterExecuteIntegrationResponse{Output: out}, nil

	default:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unknown query: %q", queryName)
	}
}

// surfaceWebhookEndpoints is the webhook-health pillar — the contract's
// canonical signal. It delegates to the same GET /v1/webhook_endpoints list as
// observe_webhook_endpoints, projecting {id, url, status, enabled_events,
// api_version} per endpoint.
func surfaceWebhookEndpoints(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (map[string]any, error) {
	resp, err := observeWebhookEndpoints(ctx, c, req)
	if err != nil {
		return nil, err
	}
	return resp.Output, nil
}

// surfaceBalance returns the available + pending arrays straight from
// observe_balance. Amounts stay in the smallest currency unit exactly as
// Stripe returns them — the UI formats per-currency.
func surfaceBalance(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (map[string]any, error) {
	resp, err := observeBalance(ctx, c, req)
	if err != nil {
		return nil, err
	}
	return resp.Output, nil
}

// surfaceCharges lists recent charges for reconciliation context. It delegates
// to the same GET /v1/charges list as observe_charges, projecting
// {id, amount, currency, status, created, refunded, payment_intent}. The
// optional `limit` param is threaded through.
//
// rule #0: only opaque refs (id, payment_intent) leave this projection —
// never the customer name / email Stripe may carry on the charge.
func surfaceCharges(ctx context.Context, c *stripe.Client, params map[string]any) (map[string]any, error) {
	in := map[string]any{}
	if limit := intFromInput(params, "limit"); limit > 0 {
		in["limit"] = limit
	}
	resp, err := observeCharges(ctx, c, contract.AdapterExecuteIntegrationRequest{Input: in})
	if err != nil {
		return nil, err
	}
	return resp.Output, nil
}

// asAnyMap coerces an input value into a map[string]any (params bag).
func asAnyMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
