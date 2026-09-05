package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
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
	return ExecuteContext(context.Background(), req)
}

// ExecuteContext dispatches one adapter capability call with the caller's
// cancellation and deadline. The transport handler must use this entry point
// for provision_webhook_endpoint so a timed-out workflow cannot leave a
// background provider POST running after the Core has abandoned the response.
func ExecuteContext(ctx context.Context, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
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
	configuredAccount := strings.TrimSpace(stringOr(req.Integration.Spec.Config, "stripe_account_id"))
	requestedAccount := strings.TrimSpace(stringOr(req.Input, "stripe_account"))
	if configuredAccount != "" {
		if !strings.HasPrefix(configuredAccount, "acct_") {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("configured stripe_account_id must start with acct_")
		}
		if requestedAccount != "" && requestedAccount != configuredAccount {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("stripe_account conflicts with the integration instance account")
		}
		if requestedAccount == "" {
			req.Input = cloneStringAnyMap(req.Input)
			req.Input["stripe_account"] = configuredAccount
		}
	} else if requestedAccount != "" && !strings.HasPrefix(requestedAccount, "acct_") {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("stripe_account must start with acct_")
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
	case OperationProvisionWebhookEndpoint:
		return provisionWebhookEndpoint(ctx, client, req)
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
	// Surface-driven read aggregator (read-only; routes by query_name).
	case OperationOnSurfaceQuery:
		return onSurfaceQuery(ctx, client, req)
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
		row := map[string]any{
			"id":       charge.ID,
			"amount":   charge.Amount,
			"currency": string(charge.Currency),
			"status":   string(charge.Status),
			"created":  charge.Created,
			"refunded": charge.Refunded,
		}
		// PaymentIntent is an opaque ref (rule #0): expose only the id,
		// never customer name / email from billing_details.
		if charge.PaymentIntent != nil {
			row["payment_intent"] = charge.PaymentIntent.ID
		} else {
			row["payment_intent"] = ""
		}
		out = append(out, row)
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

// ensureWebhookEndpoint adopts an existing endpoint by exact URL or ID, then
// reconciles mutable state. It deliberately never creates: Stripe returns the
// signing secret only on creation, and the normal reconcile path may persist
// or emit adapter output. Creation therefore lives in the separately gated
// provision_webhook_endpoint action.
func ensureWebhookEndpoint(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	if id := stringOr(in, "id"); id != "" {
		params := &stripe.WebhookEndpointRetrieveParams{}
		if acc := stringOr(in, "stripe_account"); acc != "" {
			params.SetStripeAccount(acc)
		}
		current, err := c.V1WebhookEndpoints.Retrieve(ctx, id, params)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		return reconcileWebhookEndpoint(ctx, c, req, current, false)
	}
	url := stringOr(in, "url")
	if url == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("url required for webhook_endpoint")
	}
	if events, ok := strictStringSliceInput(in, "enabled_events"); !ok || len(events) == 0 {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("enabled_events required for webhook_endpoint adoption")
	}

	matches, err := findWebhookEndpointsByURL(ctx, c, url, stringOr(in, "stripe_account"))
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	switch len(matches) {
	case 0:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("Stripe webhook endpoint %q is absent; call %s only from a workflow with a transient secret sink", url, OperationProvisionWebhookEndpoint)
	case 1:
		return reconcileWebhookEndpoint(ctx, c, req, matches[0], true)
	default:
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("multiple Stripe webhook endpoints match exact URL %q; supply id to adopt one safely", url)
	}
}

// provisionWebhookEndpoint is the only create path because the provider returns
// the signing secret once. Two independent gates are required: an instance
// operator opt-in and Core's transient-next-step sink handshake. The operation
// is intentionally not registered with the SDK reconciler, preventing its
// automatic mutation event from serializing the create-only secret.
func provisionWebhookEndpoint(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest) (contract.AdapterExecuteIntegrationResponse, error) {
	if !boolFromInput(req.Integration.Spec.Config, "allow_sensitive_webhook_endpoint_creation") {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("sensitive Stripe webhook endpoint creation is disabled for this integration instance")
	}
	if !hasTransientSecretSinkHandshake(req.Metadata) {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("provision_webhook_endpoint requires a Core-authorized transient secret sink in the immediately following workflow step")
	}

	in := req.Input
	url := stringOr(in, "url")
	if url == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("url required for webhook_endpoint")
	}
	events, ok := strictStringSliceInput(in, "enabled_events")
	if !ok || len(events) == 0 {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("enabled_events required for webhook_endpoint creation")
	}
	if disabled, present := boolInputWithPresence(in, "disabled"); present && disabled {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("Stripe webhook endpoints cannot be provisioned disabled; create and then reconcile status")
	}

	connect, connectPresent := boolInputWithPresence(in, "connect")
	if !connectPresent {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("connect must be an explicit boolean for webhook_endpoint creation")
	}
	stripeAccount := stringOr(in, "stripe_account")
	if connect && stripeAccount != "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("connect=true cannot be combined with a Stripe-Account scope")
	}
	apiVersion := stringOr(in, "api_version")
	if apiVersion != StripeAPIVersion {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("api_version must equal the adapter SDK version %s", StripeAPIVersion)
	}
	if strings.TrimSpace(req.Integration.InstanceID) == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("integration instance_id is required for webhook_endpoint creation")
	}
	generation, err := webhookProvisioningGeneration(req.Integration.Spec.Config)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	metadata, err := strictMetadataFromInput(in)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	scope := "account"
	if connect {
		scope = "connect"
	}
	for _, reserved := range []string{"yggdrasil_scope", "yggdrasil_instance_id", "yggdrasil_stripe_account", "yggdrasil_provisioning_generation"} {
		if _, exists := metadata[reserved]; exists {
			return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("metadata key %q is reserved", reserved)
		}
	}
	metadata["yggdrasil_scope"] = scope
	metadata["yggdrasil_instance_id"] = req.Integration.InstanceID
	metadata["yggdrasil_provisioning_generation"] = generation
	if stripeAccount != "" {
		metadata["yggdrasil_stripe_account"] = stripeAccount
	}
	if _, present := in["idempotency_key"]; present {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("idempotency_key is adapter-owned for webhook_endpoint creation")
	}
	matches, err := findWebhookEndpointsByURL(ctx, c, url, stripeAccount)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	if len(matches) > 0 {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("Stripe webhook endpoint %q already exists as %s; use ensure_webhook_endpoint to adopt it", url, matches[0].ID)
	}

	params := &stripe.WebhookEndpointCreateParams{
		URL:           stripe.String(url),
		EnabledEvents: stripe.StringSlice(events),
		Connect:       stripe.Bool(connect),
		APIVersion:    stripe.String(apiVersion),
		Metadata:      metadata,
	}
	if desc := stringOr(in, "description"); desc != "" {
		params.Description = stripe.String(desc)
	}
	if stripeAccount != "" {
		params.SetStripeAccount(stripeAccount)
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived(
		"",
		"provision_we",
		webhookProvisionAttempt(url, connect, stripeAccount, generation),
	))

	we, err := c.V1WebhookEndpoints.Create(ctx, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	if strings.TrimSpace(we.Secret) == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("Stripe created webhook endpoint %s without returning its create-only signing secret", we.ID)
	}
	out := webhookEndpointOutput(we)
	out["secret"] = we.Secret
	out["created"] = true
	out["adopted"] = false
	out["updated"] = false
	out["scope"] = scope
	out["stripe_account"] = stripeAccount
	return contract.AdapterExecuteIntegrationResponse{
		Output: out,
		Metadata: map[string]any{
			"secret_returned":             true,
			"secret_persistence_required": true,
			"sensitive_output_paths":      []string{"secret"},
		},
	}, nil
}

func reconcileWebhookEndpoint(ctx context.Context, c *stripe.Client, req contract.AdapterExecuteIntegrationRequest, current *stripe.WebhookEndpoint, adoptedByURL bool) (contract.AdapterExecuteIntegrationResponse, error) {
	in := req.Input
	connect, present := boolInputWithPresence(in, "connect")
	if !present {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("connect must be an explicit boolean for webhook_endpoint adoption")
	}
	stripeAccount := stringOr(in, "stripe_account")
	if connect && stripeAccount != "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("connect=true cannot be combined with a Stripe-Account scope")
	}
	expectedScope := "account"
	if connect {
		expectedScope = "connect"
	}
	instanceID := strings.TrimSpace(req.Integration.InstanceID)
	if instanceID == "" {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("integration instance_id is required for webhook_endpoint adoption")
	}
	if current.Metadata == nil || current.Metadata["yggdrasil_scope"] != expectedScope {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("existing webhook endpoint scope cannot be proven as %s", expectedScope)
	}
	if current.Metadata["yggdrasil_instance_id"] != instanceID {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("existing webhook endpoint ownership cannot be proven for this integration instance")
	}
	if current.Metadata["yggdrasil_stripe_account"] != stripeAccount {
		return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("existing webhook endpoint Stripe-Account scope cannot be proven")
	}
	params := &stripe.WebhookEndpointUpdateParams{}
	changed := false
	if url := stringOr(in, "url"); url != "" && url != current.URL {
		params.URL = stripe.String(url)
		changed = true
	}
	if events := stringSliceFromInput(in, "enabled_events"); len(events) > 0 && !equalStringSet(events, current.EnabledEvents) {
		params.EnabledEvents = stripe.StringSlice(events)
		changed = true
	}
	if desc, present := stringInputWithPresence(in, "description"); present && desc != current.Description {
		params.Description = stripe.String(desc)
		changed = true
	}
	if disabled, present := boolInputWithPresence(in, "disabled"); present && disabled != (string(current.Status) == "disabled") {
		params.Disabled = stripe.Bool(disabled)
		changed = true
	}
	if _, present := in["metadata"]; present {
		metadata, err := strictMetadataFromInput(in)
		if err != nil {
			return contract.AdapterExecuteIntegrationResponse{}, err
		}
		for _, reserved := range []string{"yggdrasil_scope", "yggdrasil_instance_id", "yggdrasil_stripe_account", "yggdrasil_provisioning_generation"} {
			currentValue, currentHas := current.Metadata[reserved]
			requestedValue, requestedHas := metadata[reserved]
			if requestedHas && (!currentHas || requestedValue != currentValue) {
				return contract.AdapterExecuteIntegrationResponse{}, fmt.Errorf("metadata key %q is provider-owned and cannot be changed", reserved)
			}
			if currentHas {
				metadata[reserved] = currentValue
			}
		}
		if !equalStringMap(metadata, current.Metadata) {
			params.Metadata = make(map[string]string, len(metadata)+len(current.Metadata))
			for key := range current.Metadata {
				if _, keep := metadata[key]; !keep {
					params.Metadata[key] = ""
				}
			}
			for key, value := range metadata {
				params.Metadata[key] = value
			}
			changed = true
		}
	}
	if acc := stringOr(in, "stripe_account"); acc != "" {
		params.SetStripeAccount(acc)
	}
	if !changed {
		out := webhookEndpointOutput(current)
		out["created"] = false
		out["adopted"] = adoptedByURL
		out["updated"] = false
		return contract.AdapterExecuteIntegrationResponse{Output: out}, nil
	}
	params.SetIdempotencyKey(idempotencyKeyOrDerived(stringOr(in, "idempotency_key"), "update_we", current.ID))
	we, err := c.V1WebhookEndpoints.Update(ctx, current.ID, params)
	if err != nil {
		return contract.AdapterExecuteIntegrationResponse{}, err
	}
	out := webhookEndpointOutput(we)
	out["created"] = false
	out["adopted"] = adoptedByURL
	out["updated"] = true
	return contract.AdapterExecuteIntegrationResponse{Output: out}, nil
}

func findWebhookEndpointsByURL(ctx context.Context, c *stripe.Client, url, stripeAccount string) ([]*stripe.WebhookEndpoint, error) {
	params := &stripe.WebhookEndpointListParams{}
	params.Limit = stripe.Int64(100)
	if stripeAccount != "" {
		params.SetStripeAccount(stripeAccount)
	}
	matches := make([]*stripe.WebhookEndpoint, 0, 1)
	var sequenceErr error
	iter := c.V1WebhookEndpoints.List(ctx, params)
	iter(func(endpoint *stripe.WebhookEndpoint, err error) bool {
		if err != nil {
			sequenceErr = err
			return false
		}
		if endpoint != nil && endpoint.URL == url {
			matches = append(matches, endpoint)
		}
		return true
	})
	if sequenceErr != nil {
		return nil, sequenceErr
	}
	return matches, nil
}

func webhookEndpointOutput(endpoint *stripe.WebhookEndpoint) map[string]any {
	out := map[string]any{
		"id":             endpoint.ID,
		"url":            endpoint.URL,
		"status":         string(endpoint.Status),
		"enabled_events": endpoint.EnabledEvents,
		"api_version":    endpoint.APIVersion,
		"application":    endpoint.Application,
		"livemode":       endpoint.Livemode,
		"created_at":     endpoint.Created,
		"description":    endpoint.Description,
		"metadata":       endpoint.Metadata,
	}
	if endpoint.Metadata != nil {
		out["scope"] = endpoint.Metadata["yggdrasil_scope"]
		out["stripe_account"] = endpoint.Metadata["yggdrasil_stripe_account"]
	}
	return out
}

func webhookProvisionAttempt(url string, connect bool, stripeAccount, generation string) string {
	return strings.Join([]string{url, strconv.FormatBool(connect), stripeAccount, generation}, "|")
}

func webhookProvisioningGeneration(config map[string]any) (string, error) {
	raw, present := config["webhook_endpoint_provisioning_generation"]
	if !present {
		return "", fmt.Errorf("webhook_endpoint_provisioning_generation is required for webhook_endpoint creation")
	}
	generation, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("webhook_endpoint_provisioning_generation must be a string")
	}
	generation = strings.TrimSpace(generation)
	if generation == "" || len(generation) > 64 {
		return "", fmt.Errorf("webhook_endpoint_provisioning_generation must contain 1 to 64 characters")
	}
	for _, character := range generation {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return "", fmt.Errorf("webhook_endpoint_provisioning_generation contains an invalid character")
	}
	return generation, nil
}

func hasTransientSecretSinkHandshake(metadata map[string]any) bool {
	if !boolFromInput(metadata, "supports_sensitive_output_paths") {
		return false
	}
	sink, ok := metadata["sensitive_output_sink"].(map[string]any)
	if !ok || stringOr(sink, "version") != "v1" || stringOr(sink, "mode") != "transient_next_step" {
		return false
	}
	producerStepID := stringOr(sink, "producer_step_id")
	if producerStepID == "" || producerStepID != stringOr(metadata, "step_id") || stringOr(sink, "step_id") == "" {
		return false
	}
	if stringOr(sink, "family") != "secrets-management" || stringOr(sink, "operation") != "ensure_secret" {
		return false
	}
	if stringOr(sink, "input_path") != "secret.generation.manual.value" {
		return false
	}
	outputPaths, ok := exactStringSlice(sink["source_output_paths"])
	if !ok || len(outputPaths) != 1 || outputPaths[0] != "secret" {
		return false
	}
	return true
}

func exactStringSlice(raw any) ([]string, bool) {
	switch values := raw.(type) {
	case []string:
		if values == nil {
			return nil, false
		}
		return values, true
	case []any:
		out := make([]string, len(values))
		for i, value := range values {
			stringValue, ok := value.(string)
			if !ok {
				return nil, false
			}
			out[i] = stringValue
		}
		return out, true
	default:
		return nil, false
	}
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
		return contract.AdapterExecuteIntegrationResponse{Output: webhookEndpointOutput(we)}, nil
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
		out = append(out, webhookEndpointOutput(we))
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

func boolInputWithPresence(m map[string]any, key string) (bool, bool) {
	value, present := m[key]
	if !present {
		return false, false
	}
	parsed, ok := value.(bool)
	return parsed, ok
}

func stringInputWithPresence(m map[string]any, key string) (string, bool) {
	value, present := m[key]
	if !present {
		return "", false
	}
	parsed, ok := value.(string)
	return parsed, ok
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input)+1)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func strictStringSliceInput(input map[string]any, key string) ([]string, bool) {
	var values []string
	switch raw := input[key].(type) {
	case []string:
		values = append([]string(nil), raw...)
	case []any:
		values = make([]string, len(raw))
		for index, item := range raw {
			value, ok := item.(string)
			if !ok {
				return nil, false
			}
			values[index] = value
		}
	default:
		return nil, false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return nil, false
		}
		if _, exists := seen[value]; exists {
			return nil, false
		}
		seen[value] = struct{}{}
	}
	return values, true
}

func strictMetadataFromInput(input map[string]any) (map[string]string, error) {
	raw, present := input["metadata"]
	if !present {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	switch values := raw.(type) {
	case map[string]string:
		for key, value := range values {
			if key == "" || key != strings.TrimSpace(key) {
				return nil, fmt.Errorf("metadata keys must be non-empty strings without surrounding whitespace")
			}
			out[key] = value
		}
	case map[string]any:
		for key, rawValue := range values {
			value, ok := rawValue.(string)
			if !ok || key == "" || key != strings.TrimSpace(key) {
				return nil, fmt.Errorf("metadata must contain only non-empty string keys and string values")
			}
			out[key] = value
		}
	default:
		return nil, fmt.Errorf("metadata must be an object of string values")
	}
	return out, nil
}

func metadataInputWithPresence(in map[string]any) (map[string]string, bool) {
	raw, present := in["metadata"]
	if !present {
		return nil, false
	}
	switch values := raw.(type) {
	case map[string]any:
		out := make(map[string]string, len(values))
		for key, value := range values {
			if text, ok := value.(string); ok {
				out[key] = text
			} else {
				out[key] = fmt.Sprint(value)
			}
		}
		return out, true
	case map[string]string:
		out := make(map[string]string, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out, true
	default:
		return nil, false
	}
}

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func equalStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
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
