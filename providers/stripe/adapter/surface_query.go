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
// Every wired read is a pure GET over the same Stripe endpoints the observe_*
// handlers use (same Stripe client, same httptest seam) re-projected for the
// surface. The list-* reads (charges, subscriptions, payment-intents) hit the
// list endpoints; charge-detail hits GET /v1/charges/{id} — the single-object
// drill-down. Read-only — they never mutate Stripe state.
//
// rule #0 (customer-data minimization): customer-identifying fields stay as
// opaque refs across ALL reads. Charges/charge-detail project only id /
// payment_intent; subscriptions project the customer as a bare `customer` id
// (never the expanded customer's name/email); payment-intents project only the
// intent id (never receipt_email). A payments-ops operator reconciles by
// reference, not by PII — name/email never enter any projection.
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

	case "list-subscriptions":
		out, err := surfaceSubscriptions(ctx, c, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("list-subscriptions: %w", err)
		}
		return contract.AdapterExecuteIntegrationResponse{Output: out}, nil

	case "list-payment-intents":
		out, err := surfacePaymentIntents(ctx, c, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("list-payment-intents: %w", err)
		}
		return contract.AdapterExecuteIntegrationResponse{Output: out}, nil

	case "charge-detail":
		out, err := surfaceChargeDetail(ctx, c, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("charge-detail: %w", err)
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

// surfaceSubscriptions lists subscriptions for the payments-ops surface,
// re-projecting each into a deeper shape than observe_subscriptions' bare
// {id, status}: {id, status, plan{nickname, price_id}, amount, currency,
// current_period_end, cancel_at_period_end}. It hits the same GET
// /v1/subscriptions list endpoint as observe_subscriptions, with the optional
// `limit` param threaded through and clamped to [1,100].
//
// rule #0: the customer is exposed ONLY as the opaque `customer` id ref —
// never the name / email Stripe carries on an expanded customer object. plan,
// amount, currency, current_period_end are read from the subscription's first
// item (the per-item price/plan + billing period, post-v83 schema).
func surfaceSubscriptions(ctx context.Context, c *stripe.Client, params map[string]any) (map[string]any, error) {
	limit := clampLimit(intFromInput(params, "limit"))
	listParams := &stripe.SubscriptionListParams{}
	listParams.Limit = stripe.Int64(limit)
	if cust := stringOr(params, "customer"); cust != "" {
		listParams.Customer = stripe.String(cust)
	}
	if acc := stringOr(params, "stripe_account"); acc != "" {
		listParams.SetStripeAccount(acc)
	}

	out := make([]map[string]any, 0, limit)
	iter := c.V1Subscriptions.List(ctx, listParams)
	var seqErr error
	var count int64
	stoppedEarly := false
	iter(func(sub *stripe.Subscription, err error) bool {
		if err != nil {
			seqErr = err
			return false
		}
		if sub == nil {
			return true
		}
		if count >= limit {
			stoppedEarly = true
			return false
		}
		out = append(out, subscriptionSurfaceRow(sub))
		count++
		return true
	})
	if seqErr != nil {
		return nil, seqErr
	}
	return map[string]any{"items": out, "has_more": stoppedEarly}, nil
}

// subscriptionSurfaceRow projects one Subscription into the surface shape.
// rule #0: customer is the opaque id ref only.
func subscriptionSurfaceRow(sub *stripe.Subscription) map[string]any {
	row := map[string]any{
		"id":                   sub.ID,
		"status":               string(sub.Status),
		"currency":             string(sub.Currency),
		"cancel_at_period_end": sub.CancelAtPeriodEnd,
		"current_period_end":   int64(0),
		"amount":               int64(0),
		"plan":                 map[string]any{"nickname": "", "price_id": ""},
		"customer":             "",
	}
	if sub.Customer != nil {
		// Opaque ref only — never sub.Customer.Email / .Name.
		row["customer"] = sub.Customer.ID
	}
	// Post-v83 the billing period + price live on the subscription item, not
	// the subscription. Read from the first item; fall back to the legacy Plan.
	if sub.Items != nil && len(sub.Items.Data) > 0 {
		item := sub.Items.Data[0]
		row["current_period_end"] = item.CurrentPeriodEnd
		switch {
		case item.Price != nil:
			row["amount"] = item.Price.UnitAmount
			if item.Price.Currency != "" {
				row["currency"] = string(item.Price.Currency)
			}
			row["plan"] = map[string]any{
				"nickname": item.Price.Nickname,
				"price_id": item.Price.ID,
			}
		case item.Plan != nil:
			row["amount"] = item.Plan.Amount
			if item.Plan.Currency != "" {
				row["currency"] = string(item.Plan.Currency)
			}
			row["plan"] = map[string]any{
				"nickname": item.Plan.Nickname,
				"price_id": item.Plan.ID,
			}
		}
	}
	return row
}

// surfacePaymentIntents lists payment intents for the surface, projecting
// {id, status, amount, currency, created, capture_method}. It hits the same GET
// /v1/payment_intents list as observe_payment_intents but adds the created /
// capture_method columns the surface needs (observe's list omits them). The
// `limit` param is threaded through and clamped to [1,100].
//
// rule #0: only the opaque intent id leaves the projection — never
// receipt_email or any customer-identifying field on the intent.
func surfacePaymentIntents(ctx context.Context, c *stripe.Client, params map[string]any) (map[string]any, error) {
	limit := clampLimit(intFromInput(params, "limit"))
	listParams := &stripe.PaymentIntentListParams{}
	listParams.Limit = stripe.Int64(limit)
	if cust := stringOr(params, "customer"); cust != "" {
		listParams.Customer = stripe.String(cust)
	}
	if acc := stringOr(params, "stripe_account"); acc != "" {
		listParams.SetStripeAccount(acc)
	}

	out := make([]map[string]any, 0, limit)
	iter := c.V1PaymentIntents.List(ctx, listParams)
	var seqErr error
	var count int64
	stoppedEarly := false
	iter(func(pi *stripe.PaymentIntent, err error) bool {
		if err != nil {
			seqErr = err
			return false
		}
		if pi == nil {
			return true
		}
		if count >= limit {
			stoppedEarly = true
			return false
		}
		out = append(out, map[string]any{
			"id":             pi.ID,
			"status":         string(pi.Status),
			"amount":         pi.Amount,
			"currency":       string(pi.Currency),
			"created":        pi.Created,
			"capture_method": string(pi.CaptureMethod),
		})
		count++
		return true
	})
	if seqErr != nil {
		return nil, seqErr
	}
	return map[string]any{"items": out, "has_more": stoppedEarly}, nil
}

// surfaceChargeDetail is the reconciliation drill-down: given a required
// `charge_id` param it retrieves a single charge (refunds expanded) and projects
// {id, amount, currency, status, created, refunded, refundedAmount,
// payment_intent, failureCode, failureMessage, refunds:[{id,amount,reason,created}]}.
// It hits GET /v1/charges/{id}, the single-object twin of observe_charges' list.
//
// rule #0: payment_intent is the opaque id ref only; no customer name / email
// from billing_details ever enters the projection.
func surfaceChargeDetail(ctx context.Context, c *stripe.Client, params map[string]any) (map[string]any, error) {
	id := stringOr(params, "charge_id")
	if id == "" {
		return nil, fmt.Errorf("charge_id is required")
	}
	p := &stripe.ChargeRetrieveParams{}
	// Expand refunds so the list is materialized (newer API versions return it
	// unexpanded by default).
	p.AddExpand("refunds")
	if acc := stringOr(params, "stripe_account"); acc != "" {
		p.SetStripeAccount(acc)
	}
	charge, err := c.V1Charges.Retrieve(ctx, id, p)
	if err != nil {
		return nil, err
	}

	paymentIntent := ""
	if charge.PaymentIntent != nil {
		paymentIntent = charge.PaymentIntent.ID
	}
	refunds := make([]map[string]any, 0)
	if charge.Refunds != nil {
		for _, rf := range charge.Refunds.Data {
			if rf == nil {
				continue
			}
			refunds = append(refunds, map[string]any{
				"id":      rf.ID,
				"amount":  rf.Amount,
				"reason":  string(rf.Reason),
				"created": rf.Created,
			})
		}
	}
	return map[string]any{
		"id":             charge.ID,
		"amount":         charge.Amount,
		"currency":       string(charge.Currency),
		"status":         string(charge.Status),
		"created":        charge.Created,
		"refunded":       charge.Refunded,
		"refundedAmount": charge.AmountRefunded,
		"payment_intent": paymentIntent,
		"failureCode":    charge.FailureCode,
		"failureMessage": charge.FailureMessage,
		"refunds":        refunds,
	}, nil
}

// clampLimit mirrors the observe_* default/clamp: missing/non-positive → 10,
// over 100 → 100.
func clampLimit(limit int64) int64 {
	if limit <= 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// asAnyMap coerces an input value into a map[string]any (params bag).
func asAnyMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
