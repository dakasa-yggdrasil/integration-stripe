package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v83"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// Execute dispatches one adapter capability call. v2.0.0 routes canonical
// ensure_/observe_/destroy_ ops plus the kept allowlisted action helpers.
// Pre-convention names are translated through ResolveOperation and logged
// once per invocation as WARN during the compat window (SDK v0.5.x).
func Execute(req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	rawOp := NormalizeExecuteOperation(req.Operation, req.Capability)
	if rawOp == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("operation is required")
	}
	op, legacy := ResolveOperation(rawOp)
	if legacy {
		log.Printf("WARN stripe: deprecated capability name %q invoked; use %q (compat shim, removed in v0.6.0)", rawOp, op)
	}
	instance := req.Integration.InstanceID
	if instance == "" {
		instance = "unknown"
	}
	start := time.Now()
	defer func() {
		StripeExecuteDuration.WithLabelValues(op, instance).Observe(time.Since(start).Seconds())
	}()
	StripeExecuteRequests.WithLabelValues(op, instance).Inc()
	ctx := context.Background()

	// verify_webhook_signature does not need a Stripe HTTP client.
	if op == OperationVerifyWebhookSig {
		return verifyWebhookSig(req)
	}

	// Read per-instance credentials + config rehydrated by the bridge
	// (providers/stripe/message/execute.go::buildSDKDelivery →
	// reconcile.go::buildExecuteRequest). Pre-2.2.2 the call was
	// clientForInstance(InstanceID, "", "", ...) with a hardcoded empty
	// apiKey — every write capability failed at NewStripeClient with
	// "stripe api key is required" regardless of bridge state. Reading
	// from Spec.Credentials closes the secondary bug noted in v2.2.1
	// CHANGELOG.
	//
	// Accept "stripe_api_key" (the canonical credential_schema field) or
	// "stripe_secret_key" (the field name commonly used in operator AWS
	// Secrets Manager / GCP Secret Manager / Vault entries — many
	// secret-store conventions name the value "stripe_secret_key"). The
	// alias is intentional: forcing every operator secret to rename to
	// match the schema would create deployment friction that the Lego
	// principle (§2) discourages. Both reach the same Stripe SDK
	// boundary; the schema field name is purely a label.
	apiKey := stringOr(req.Integration.Spec.Credentials, "stripe_api_key")
	if apiKey == "" {
		apiKey = stringOr(req.Integration.Spec.Credentials, "stripe_secret_key")
	}
	baseURL := stringOr(req.Integration.Spec.Config, "stripe_api_base_url")
	apiVersion := stringOr(req.Integration.Spec.Config, "stripe_api_version")
	if apiVersion == "" {
		apiVersion = StripeAPIVersion
	}

	client, err := clientForInstance(req.Integration.InstanceID, apiKey, baseURL, apiVersion)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}

	switch op {
	// payment_intent triple.
	case OperationEnsurePaymentIntent:
		return ensurePaymentIntent(ctx, client, req)
	case OperationObservePaymentIntents:
		return observePaymentIntents(ctx, client, req)
	case OperationDestroyPaymentIntent:
		return destroyPaymentIntent(ctx, client, req)
	// customer triple.
	case OperationEnsureCustomer:
		return ensureCustomer(ctx, client, req)
	case OperationObserveCustomers:
		return observeCustomers(ctx, client, req)
	case OperationDestroyCustomer:
		return destroyCustomer(ctx, client, req)
	// subscription triple.
	case OperationEnsureSubscription:
		return ensureSubscription(ctx, client, req)
	case OperationObserveSubscriptions:
		return observeSubscriptions(ctx, client, req)
	case OperationDestroySubscription:
		return destroySubscription(ctx, client, req)
	// charge read.
	case OperationObserveCharges:
		return observeCharges(ctx, client, req)
	// balance read.
	case OperationObserveBalance:
		return observeBalance(ctx, client, req)
	// webhook_endpoint triple.
	case OperationEnsureWebhookEndpoint:
		return ensureWebhookEndpoint(ctx, client, req)
	case OperationObserveWebhookEndpoints:
		return observeWebhookEndpoints(ctx, client, req)
	case OperationDestroyWebhookEndpoint:
		return destroyWebhookEndpoint(ctx, client, req)
	// Allowlisted action helpers.
	case OperationCreateRefund:
		return createRefund(ctx, client, req)
	case OperationCreateSetupIntent:
		return createSetupIntent(ctx, client, req)
	case OperationCreatePayout:
		return createPayout(ctx, client, req)
	case OperationManageConnectAccount:
		return manageConnectAccount(ctx, client, req)
	default:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("unsupported operation %q", op)
	}
}

// ensurePaymentIntent creates or confirms a Stripe PaymentIntent.
// Collapses the pre-convention create_/confirm_ pair: when input.confirm
// is true (or a payment_intent_id is present), the handler treats the
// call as "ensure this intent exists in confirmed state."
func ensurePaymentIntent(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	id := stringOr(in, "payment_intent_id")
	confirm := boolFromInput(in, "confirm")

	// When a payment_intent_id is provided, treat as confirm path (the
	// historical confirm_payment_intent semantic). If no id is provided
	// but confirm=true, callers MUST pass amount+currency so we can
	// create-then-confirm. Otherwise act as plain create.
	if id != "" {
		return confirmPaymentIntent(ctx, c, req)
	}

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
	if confirm {
		params.Confirm = stripe.Bool(true)
	}
	idk := stringOr(in, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "ensure_pi",
		fmt.Sprintf("%d", amount), currency,
		stringOr(in, "customer"),
	))

	pi, err := c.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	out := map[string]any{
		"payment_intent_id": pi.ID,
		"client_secret":     pi.ClientSecret,
		"status":            string(pi.Status),
		"amount":            pi.Amount,
		"currency":          pi.Currency,
	}
	if pi.NextAction != nil {
		out["next_action"] = pi.NextAction
	}
	return contract.AdapterExecuteIntegrationResponse{Output: out}, nil
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

// observePaymentIntents lists PIs or retrieves one when filter.id is set.
func observePaymentIntents(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "id"); id != "" {
		params := &stripe.PaymentIntentRetrieveParams{}
		if acc := stringOr(in, "stripe_account"); acc != "" {
			params.SetStripeAccount(acc)
		}
		pi, err := c.V1PaymentIntents.Retrieve(ctx, id, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
			"payment_intent_id": pi.ID,
			"status":            string(pi.Status),
			"amount":            pi.Amount,
			"currency":          pi.Currency,
		}}, nil
	}
	limit := intFromInput(in, "limit")
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	params := &stripe.PaymentIntentListParams{}
	params.Limit = stripe.Int64(limit)
	if cust := stringOr(in, "customer"); cust != "" {
		params.Customer = stripe.String(cust)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	out := make([]map[string]any, 0, limit)
	iter := c.V1PaymentIntents.List(ctx, params)
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
			"id":       pi.ID,
			"amount":   pi.Amount,
			"currency": pi.Currency,
			"status":   string(pi.Status),
		})
		count++
		return true
	})
	if seqErr != nil {
		return contract.AdapterExecuteIntegrationResponse{}, seqErr
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"items":    out,
		"has_more": stoppedEarly,
	}}, nil
}

func destroyPaymentIntent(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "payment_intent_id")
	if id == "" {
		// Accept "ref" for canonical Destroy(ctx, ref) shape via SDK.
		id = stringOr(req.Input, "ref")
	}
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("payment_intent_id (or ref) required")
	}
	params := &stripe.PaymentIntentCancelParams{}
	if reason := stringOr(req.Input, "cancellation_reason"); reason != "" {
		params.CancellationReason = stripe.String(reason)
	}
	if acc := stringOr(req.Input, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "destroy_pi", id))

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

// ensureCustomer creates a Customer when email is supplied and no
// customer_id is present, updates by id when customer_id is supplied.
// Collapses create_customer + update_customer behind one canonical name.
func ensureCustomer(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	id := stringOr(in, "customer_id")
	if id != "" {
		return updateCustomer(ctx, c, req)
	}
	return createCustomer(ctx, c, req)
}

func createCustomer(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	email := stringOr(in, "email")
	if email == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("email required")
	}
	params := &stripe.CustomerCreateParams{
		Email: stripe.String(email),
	}
	if name := stringOr(in, "name"); name != "" {
		params.Name = stripe.String(name)
	}
	if phone := stringOr(in, "phone"); phone != "" {
		params.Phone = stripe.String(phone)
	}
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	idk := stringOr(in, "idempotency_key")
	if idk == "" {
		idk = "create_customer_" + email // matches enterprise-payments-api convention
	}
	params.SetIdempotencyKey(idk)

	cust, err := c.V1Customers.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"customer_id": cust.ID,
		"email":       cust.Email,
		"created":     cust.Created,
	}}, nil
}

func updateCustomer(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	id := stringOr(in, "customer_id")
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("customer_id required")
	}
	params := &stripe.CustomerUpdateParams{}
	if email := stringOr(in, "email"); email != "" {
		params.Email = stripe.String(email)
	}
	if name := stringOr(in, "name"); name != "" {
		params.Name = stripe.String(name)
	}
	if phone := stringOr(in, "phone"); phone != "" {
		params.Phone = stripe.String(phone)
	}
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	// Derive key from customer_id + sha256(email|name|phone) so a
	// duplicate "update X to the same values" is idempotent.
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "update_customer", id,
		stringOr(in, "email"), stringOr(in, "name"), stringOr(in, "phone")))

	cust, err := c.V1Customers.Update(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"customer_id": cust.ID,
		"updated":     true,
	}}, nil
}

func observeCustomers(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "id"); id != "" {
		params := &stripe.CustomerRetrieveParams{}
		if acc := stringOr(in, "stripe_account"); acc != "" {
			params.SetStripeAccount(acc)
		}
		cust, err := c.V1Customers.Retrieve(ctx, id, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
			"customer_id": cust.ID,
			"email":       cust.Email,
		}}, nil
	}
	limit := intFromInput(in, "limit")
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	params := &stripe.CustomerListParams{}
	params.Limit = stripe.Int64(limit)
	if email := stringOr(in, "email"); email != "" {
		params.Email = stripe.String(email)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	out := make([]map[string]any, 0, limit)
	iter := c.V1Customers.List(ctx, params)
	var seqErr error
	var count int64
	stoppedEarly := false
	iter(func(cu *stripe.Customer, err error) bool {
		if err != nil {
			seqErr = err
			return false
		}
		if cu == nil {
			return true
		}
		if count >= limit {
			stoppedEarly = true
			return false
		}
		out = append(out, map[string]any{
			"id":    cu.ID,
			"email": cu.Email,
		})
		count++
		return true
	})
	if seqErr != nil {
		return contract.AdapterExecuteIntegrationResponse{}, seqErr
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"items":    out,
		"has_more": stoppedEarly,
	}}, nil
}

func destroyCustomer(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "customer_id")
	if id == "" {
		id = stringOr(req.Input, "ref")
	}
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("customer_id (or ref) required")
	}
	params := &stripe.CustomerDeleteParams{}
	if acc := stringOr(req.Input, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	cust, err := c.V1Customers.Delete(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"customer_id": cust.ID,
		"deleted":     true,
	}}, nil
}

// ensureSubscription: PATCH when subscription_id supplied, POST otherwise.
func ensureSubscription(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "subscription_id"); id != "" {
		return updateSubscription(ctx, c, req, id)
	}
	return createSubscription(ctx, c, req)
}

func createSubscription(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	customer := stringOr(in, "customer")
	if customer == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("customer required")
	}
	rawItems, _ := in["items"].([]any)
	if len(rawItems) == 0 {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("at least one item is required")
	}
	items := make([]*stripe.SubscriptionCreateItemParams, 0, len(rawItems))
	for _, raw := range rawItems {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		item := &stripe.SubscriptionCreateItemParams{}
		if price := stringOr(entry, "price"); price != "" {
			item.Price = stripe.String(price)
		}
		if qty := intFromInput(entry, "quantity"); qty > 0 {
			item.Quantity = stripe.Int64(qty)
		}
		items = append(items, item)
	}
	behavior := stringOr(in, "payment_behavior")
	if behavior == "" {
		behavior = "default_incomplete"
	}
	params := &stripe.SubscriptionCreateParams{
		Customer:        stripe.String(customer),
		Items:           items,
		PaymentBehavior: stripe.String(behavior),
	}
	if trial := intFromInput(in, "trial_end"); trial > 0 {
		params.TrialEnd = stripe.Int64(trial)
	}
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	idk := stringOr(in, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "create_sub", customer))

	sub, err := c.V1Subscriptions.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	out := map[string]any{
		"subscription_id": sub.ID,
		"status":          string(sub.Status),
	}
	if sub.LatestInvoice != nil {
		out["latest_invoice"] = sub.LatestInvoice.ID
	}
	return contract.AdapterExecuteIntegrationResponse{Output: out}, nil
}

func updateSubscription(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest, id string) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	params := &stripe.SubscriptionUpdateParams{}
	if boolFromInput(in, "cancel_at_period_end") {
		params.CancelAtPeriodEnd = stripe.Bool(true)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "update_sub", id))
	sub, err := c.V1Subscriptions.Update(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: subOutput(sub)}, nil
}

func observeSubscriptions(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "id"); id != "" {
		params := &stripe.SubscriptionRetrieveParams{}
		if acc := stringOr(in, "stripe_account"); acc != "" {
			params.SetStripeAccount(acc)
		}
		sub, err := c.V1Subscriptions.Retrieve(ctx, id, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		return contract.AdapterExecuteIntegrationResponse{Output: subOutput(sub)}, nil
	}
	limit := intFromInput(in, "limit")
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	params := &stripe.SubscriptionListParams{}
	params.Limit = stripe.Int64(limit)
	if cust := stringOr(in, "customer"); cust != "" {
		params.Customer = stripe.String(cust)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	out := make([]map[string]any, 0, limit)
	iter := c.V1Subscriptions.List(ctx, params)
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
		out = append(out, map[string]any{
			"id":     sub.ID,
			"status": string(sub.Status),
		})
		count++
		return true
	})
	if seqErr != nil {
		return contract.AdapterExecuteIntegrationResponse{}, seqErr
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"items":    out,
		"has_more": stoppedEarly,
	}}, nil
}

func destroySubscription(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	id := stringOr(in, "subscription_id")
	if id == "" {
		id = stringOr(in, "ref")
	}
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("subscription_id (or ref) required")
	}
	// When cancel_at_period_end=true Stripe expects POST /v1/subscriptions/{id}
	// (update). The "immediate" path is DELETE /v1/subscriptions/{id}.
	atPeriodEnd := boolFromInput(in, "cancel_at_period_end")
	if atPeriodEnd {
		return updateSubscription(ctx, c, req, id)
	}

	params := &stripe.SubscriptionCancelParams{}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "destroy_sub_now", id))
	sub, err := c.V1Subscriptions.Cancel(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: subOutput(sub)}, nil
}

func subOutput(sub *stripe.Subscription) map[string]any {
	return map[string]any{
		"subscription_id":      sub.ID,
		"status":               string(sub.Status),
		"cancel_at_period_end": sub.CancelAtPeriodEnd,
		"canceled_at":          sub.CanceledAt,
	}
}

// observeCharges replaces list_charges. Same upstream semantic
// (GET /v1/charges with filter), under the canonical name.
func observeCharges(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	limit := intFromInput(in, "limit")
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	params := &stripe.ChargeListParams{}
	params.Limit = stripe.Int64(limit)
	if cust := stringOr(in, "customer"); cust != "" {
		params.Customer = stripe.String(cust)
	}
	if pi := stringOr(in, "payment_intent"); pi != "" {
		params.PaymentIntent = stripe.String(pi)
	}
	if cursor := stringOr(in, "starting_after"); cursor != "" {
		params.StartingAfter = stripe.String(cursor)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}

	out := make([]map[string]any, 0, limit)
	iter := c.V1Charges.List(ctx, params)
	count := int64(0)
	stoppedEarly := false
	var seqErr error
	iter(func(charge *stripe.Charge, err error) bool {
		if err != nil {
			seqErr = err
			return false
		}
		if charge == nil {
			return true
		}
		if count >= limit {
			stoppedEarly = true
			return false
		}
		out = append(out, map[string]any{
			"id":       charge.ID,
			"amount":   charge.Amount,
			"currency": charge.Currency,
			"status":   string(charge.Status),
		})
		count++
		return true
	})
	if seqErr != nil {
		return contract.AdapterExecuteIntegrationResponse{}, seqErr
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"items":    out,
		"has_more": stoppedEarly,
	}}, nil
}

// observeBalance wraps GET /v1/balance. The Stripe Balance object is a
// singleton per account (no list endpoint) — observe_balance returns
// the current snapshot.
func observeBalance(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	params := &stripe.BalanceRetrieveParams{}
	if acc := stringOr(req.Input, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	bal, err := c.V1Balance.Retrieve(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	available := make([]map[string]any, 0, len(bal.Available))
	for _, a := range bal.Available {
		available = append(available, map[string]any{
			"amount":   a.Amount,
			"currency": string(a.Currency),
		})
	}
	pending := make([]map[string]any, 0, len(bal.Pending))
	for _, p := range bal.Pending {
		pending = append(pending, map[string]any{
			"amount":   p.Amount,
			"currency": string(p.Currency),
		})
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"available": available,
		"pending":   pending,
	}}, nil
}

// ensureWebhookEndpoint: POST when no id, PATCH when id present.
func ensureWebhookEndpoint(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "id"); id != "" {
		return updateWebhookEndpoint(ctx, c, req, id)
	}
	url := stringOr(in, "url")
	if url == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("url required for webhook_endpoint")
	}
	events := stringSliceFromInput(in, "enabled_events")
	if len(events) == 0 {
		events = []string{"*"}
	}
	params := &stripe.WebhookEndpointCreateParams{
		URL:           stripe.String(url),
		EnabledEvents: stripe.StringSlice(events),
	}
	if desc := stringOr(in, "description"); desc != "" {
		params.Description = stripe.String(desc)
	}
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived(stringOr(in, "idempotency_key"), "ensure_we", url))

	we, err := c.V1WebhookEndpoints.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"id":             we.ID,
		"url":            we.URL,
		"status":         string(we.Status),
		"enabled_events": we.EnabledEvents,
		"secret":         we.Secret,
	}}, nil
}

func updateWebhookEndpoint(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest, id string) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	params := &stripe.WebhookEndpointUpdateParams{}
	if url := stringOr(in, "url"); url != "" {
		params.URL = stripe.String(url)
	}
	if events := stringSliceFromInput(in, "enabled_events"); len(events) > 0 {
		params.EnabledEvents = stripe.StringSlice(events)
	}
	if desc := stringOr(in, "description"); desc != "" {
		params.Description = stripe.String(desc)
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived("", "update_we", id))
	we, err := c.V1WebhookEndpoints.Update(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"id":             we.ID,
		"url":            we.URL,
		"status":         string(we.Status),
		"enabled_events": we.EnabledEvents,
	}}, nil
}

func observeWebhookEndpoints(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "id"); id != "" {
		params := &stripe.WebhookEndpointRetrieveParams{}
		if acc := stringOr(in, "stripe_account"); acc != "" {
			params.SetStripeAccount(acc)
		}
		we, err := c.V1WebhookEndpoints.Retrieve(ctx, id, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
			"id":             we.ID,
			"url":            we.URL,
			"status":         string(we.Status),
			"enabled_events": we.EnabledEvents,
		}}, nil
	}
	limit := intFromInput(in, "limit")
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	params := &stripe.WebhookEndpointListParams{}
	params.Limit = stripe.Int64(limit)
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	out := make([]map[string]any, 0, limit)
	iter := c.V1WebhookEndpoints.List(ctx, params)
	var seqErr error
	var count int64
	stoppedEarly := false
	iter(func(we *stripe.WebhookEndpoint, err error) bool {
		if err != nil {
			seqErr = err
			return false
		}
		if we == nil {
			return true
		}
		if count >= limit {
			stoppedEarly = true
			return false
		}
		out = append(out, map[string]any{
			"id":             we.ID,
			"url":            we.URL,
			"status":         string(we.Status),
			"enabled_events": we.EnabledEvents,
		})
		count++
		return true
	})
	if seqErr != nil {
		return contract.AdapterExecuteIntegrationResponse{}, seqErr
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"items":    out,
		"has_more": stoppedEarly,
	}}, nil
}

func destroyWebhookEndpoint(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	id := stringOr(req.Input, "id")
	if id == "" {
		id = stringOr(req.Input, "ref")
	}
	if id == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("webhook_endpoint id (or ref) required")
	}
	params := &stripe.WebhookEndpointDeleteParams{}
	if acc := stringOr(req.Input, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	we, err := c.V1WebhookEndpoints.Delete(ctx, id, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"id":      we.ID,
		"deleted": true,
	}}, nil
}

func createRefund(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	charge := stringOr(in, "charge")
	pi := stringOr(in, "payment_intent")
	if charge == "" && pi == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("charge or payment_intent required")
	}
	params := &stripe.RefundCreateParams{}
	if charge != "" {
		params.Charge = stripe.String(charge)
	}
	if pi != "" {
		params.PaymentIntent = stripe.String(pi)
	}
	amount := intFromInput(in, "amount")
	if amount > 0 {
		params.Amount = stripe.Int64(amount)
	}
	if reason := stringOr(in, "reason"); reason != "" {
		params.Reason = stripe.String(reason)
	}
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	idk := stringOr(in, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "refund",
		charge, fmt.Sprintf("%d", amount)))

	r, err := c.V1Refunds.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	out := map[string]any{
		"refund_id": r.ID,
		"status":    string(r.Status),
		"amount":    r.Amount,
	}
	if r.Charge != nil {
		out["charge"] = r.Charge.ID
	}
	return contract.AdapterExecuteIntegrationResponse{Output: out}, nil
}

func createSetupIntent(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	usage := stringOr(in, "usage")
	if usage == "" {
		usage = "off_session"
	}
	params := &stripe.SetupIntentCreateParams{
		Usage: stripe.String(usage),
	}
	if cust := stringOr(in, "customer"); cust != "" {
		params.Customer = stripe.String(cust)
	}
	if pm := stringOr(in, "payment_method"); pm != "" {
		params.PaymentMethod = stripe.String(pm)
	}
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	idk := stringOr(in, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "create_si",
		stringOr(in, "customer")))

	si, err := c.V1SetupIntents.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"setup_intent_id": si.ID,
		"client_secret":   si.ClientSecret,
		"status":          string(si.Status),
	}}, nil
}

func createPayout(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	amount := intFromInput(in, "amount")
	currency := stringOr(in, "currency")
	if amount <= 0 || currency == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("amount and currency required")
	}
	params := &stripe.PayoutCreateParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(currency),
	}
	method := stringOr(in, "method")
	if method == "" {
		method = "standard"
	}
	params.Method = stripe.String(method)
	if md := metadataFromInput(in); len(md) > 0 {
		params.Metadata = md
	}
	acct := stringOr(in, "stripe_account")
	if acct != "" {
		params.SetStripeAccount(acct)
	}
	idk := stringOr(in, "idempotency_key")
	params.SetIdempotencyKey(idempotencyKeyOrDerived(idk, "create_payout",
		acct, fmt.Sprintf("%d", amount), currency))

	po, err := c.V1Payouts.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	return contract.AdapterExecuteIntegrationResponse{Output: map[string]any{
		"payout_id":    po.ID,
		"status":       string(po.Status),
		"arrival_date": po.ArrivalDate,
		"method":       string(po.Method),
	}}, nil
}

// boolFromInput returns m[key] as a bool, defaulting to false.
func boolFromInput(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// metadataFromInput coerces input["metadata"] into a string-keyed
// string map. Stripe's API rejects non-string values, so anything
// that doesn't fit is stringified via fmt.Sprint.
func metadataFromInput(in map[string]any) map[string]string {
	raw, ok := in["metadata"].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch s := v.(type) {
		case string:
			out[k] = s
		default:
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

// stringSliceFromInput extracts m[key] as []string. Accepts []any with
// string elements or a single string fallback.
func stringSliceFromInput(m map[string]any, key string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, raw := range v {
			if s, ok := raw.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

// verifyWebhookSig implements the standalone verify_webhook_signature
// capability. Pure helper — allowlisted on the convention exemption list.
func verifyWebhookSig(req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	payload := []byte(stringOr(req.Input, "payload"))
	header := stringOr(req.Input, "stripe_signature")
	secret := []byte(stringOr(req.Input, "endpoint_secret"))
	tol := intFromInput(req.Input, "tolerance_seconds")
	if tol <= 0 {
		tol = 300
	}
	ts, err := VerifySignature(payload, header, secret, tol)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{
			Output: map[string]any{"valid": false, "error": err.Error()},
		}, nil
	}

	var ev struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	_ = json.Unmarshal(payload, &ev)

	return contract.AdapterExecuteIntegrationResponse{
		Output: map[string]any{
			"valid":      true,
			"event_id":   ev.ID,
			"event_type": ev.Type,
			"timestamp":  ts,
		},
	}, nil
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
