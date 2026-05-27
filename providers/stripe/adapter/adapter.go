package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/stripe/stripe-go/v83"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// Execute dispatches one adapter capability call. Both the SDK execute
// handler and the verify_webhook_signature path delegate here. Returns
// a Response with Output populated; errors are surfaced as Go errors
// (the message layer translates them into rpc failure envelopes).
func Execute(req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	op := NormalizeExecuteOperation(req.Operation, req.Capability)
	if op == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("operation is required")
	}
	ctx := context.Background()

	// verify_webhook_signature does not need a Stripe HTTP client.
	if op == OperationVerifyWebhookSig {
		return verifyWebhookSig(req)
	}

	client, err := clientForInstance(req.Integration.InstanceID, "", "", StripeAPIVersion)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}

	switch op {
	case OperationCreatePaymentIntent:
		return createPaymentIntent(ctx, client, req)
	case OperationConfirmPaymentIntent:
		return confirmPaymentIntent(ctx, client, req)
	case OperationCancelPaymentIntent:
		return cancelPaymentIntent(ctx, client, req)
	default:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported operation %q", op)
	}
}

func confirmPaymentIntent(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "payment_intent_id")
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("payment_intent_id required")
	}
	params := &stripe.PaymentIntentConfirmParams{}
	if pm := stringOr(req.Input, "payment_method"); pm != "" {
		params.PaymentMethod = stripe.String(pm)
	}
	if ru := stringOr(req.Input, "return_url"); ru != "" {
		params.ReturnURL = stripe.String(ru)
	}
	if acc := stringOr(req.Input, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	idk := stringOr(req.Input, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "confirm_pi", id))

	pi, err := c.V1PaymentIntents.Confirm(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	out := map[string]any{
		"payment_intent_id": pi.ID,
		"status":            string(pi.Status),
	}
	if pi.NextAction != nil {
		out["next_action"] = pi.NextAction
	}
	return contract.AdapterExecuteIntegrationResponse{Output: out}, nil
}

func createPaymentIntent(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	amount := intFromInput(in, "amount")
	currency := stringOr(in, "currency")
	if amount <= 0 || strings.TrimSpace(currency) == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("amount and currency are required")
	}

	params := &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(currency),
	}
	if cust := stringOr(in, "customer"); cust != "" {
		params.Customer = stripe.String(cust)
	}
	if pm := stringOr(in, "payment_method"); pm != "" {
		params.PaymentMethod = stripe.String(pm)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	idk := stringOr(in, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "create_pi",
		fmt.Sprintf("%d", amount), currency,
		stringOr(in, "customer"),
	))

	pi, err := c.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{
		Output: map[string]any{
			"payment_intent_id": pi.ID,
			"client_secret":     pi.ClientSecret,
			"status":            string(pi.Status),
			"amount":            pi.Amount,
			"currency":          pi.Currency,
		},
	}, nil
}

func cancelPaymentIntent(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "payment_intent_id")
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("payment_intent_id required")
	}
	params := &stripe.PaymentIntentCancelParams{}
	if reason := stringOr(req.Input, "cancellation_reason"); reason != "" {
		params.CancellationReason = stripe.String(reason)
	}
	if acc := stringOr(req.Input, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	// Always derive the same key for a given PI so a duplicate cancel
	// is no-op even if no idempotency_key is passed.
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "cancel_pi", id))

	pi, err := c.V1PaymentIntents.Cancel(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"payment_intent_id":   pi.ID,
		"status":              string(pi.Status),
		"cancellation_reason": string(pi.CancellationReason),
	}}, nil
}

// verifyWebhookSig is implemented in Task 32 (verify_webhook_signature
// capability). Stub here so Execute can dispatch to it ahead of the
// implementation landing.
func verifyWebhookSig(req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("verify_webhook_signature not yet implemented")
}

// stringOr returns m[key] as a string (or "" when absent or not a
// string).
func stringOr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// intFromInput coerces m[key] to int64 — JSON numbers decode as
// float64 via encoding/json, but YAML-derived inputs may arrive as
// int / int64 directly. Returns 0 when missing or unconvertible.
func intFromInput(m map[string]any, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case int32:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}
