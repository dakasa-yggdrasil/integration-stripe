package adapter

import (
	"context"
	"encoding/json"
	"os"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/events"
	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/reconcile"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

// reconcilePayload is a thin envelope used by reconciler implementations
// to forward typed desired/observed payloads through to the existing
// Execute() switch — the production runtime path the integration-stripe
// adapter has shipped since v1.0.0. The reconcile.RegisterReconciler
// wiring is provided as the canonical Go-level expression of the
// convention; both paths reach the same handlers.
type reconcilePayload map[string]any

// paymentIntentReconciler wraps Execute() to implement
// reconcile.Reconciler[D, O] for Stripe PaymentIntents.
type paymentIntentReconciler struct {
	instanceID string
}

func newPaymentIntentReconciler(instanceID string) *paymentIntentReconciler {
	return &paymentIntentReconciler{instanceID: instanceID}
}

func (r *paymentIntentReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsurePaymentIntent, d)
}

func (r *paymentIntentReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	out, err := r.dispatch(OperationObservePaymentIntents, filter)
	if err != nil {
		return nil, "", err
	}
	return extractItems(out), "", nil
}

func (r *paymentIntentReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroyPaymentIntent, reconcilePayload{"payment_intent_id": ref})
	return err
}

func (r *paymentIntentReconciler) dispatch(op string, in reconcilePayload) (reconcilePayload, error) {
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   op,
		Integration: contract.IntegrationContext{InstanceID: r.instanceID},
		Input:       in,
	})
	if err != nil {
		return nil, err
	}
	return reconcilePayload(resp.Output), nil
}

// customerReconciler wraps Execute() for Stripe Customers.
type customerReconciler struct{ instanceID string }

func newCustomerReconciler(instanceID string) *customerReconciler {
	return &customerReconciler{instanceID: instanceID}
}

func (r *customerReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsureCustomer, d)
}

func (r *customerReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	out, err := r.dispatch(OperationObserveCustomers, filter)
	if err != nil {
		return nil, "", err
	}
	return extractItems(out), "", nil
}

func (r *customerReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroyCustomer, reconcilePayload{"customer_id": ref})
	return err
}

func (r *customerReconciler) dispatch(op string, in reconcilePayload) (reconcilePayload, error) {
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   op,
		Integration: contract.IntegrationContext{InstanceID: r.instanceID},
		Input:       in,
	})
	if err != nil {
		return nil, err
	}
	return reconcilePayload(resp.Output), nil
}

// subscriptionReconciler wraps Execute() for Stripe Subscriptions.
type subscriptionReconciler struct{ instanceID string }

func newSubscriptionReconciler(instanceID string) *subscriptionReconciler {
	return &subscriptionReconciler{instanceID: instanceID}
}

func (r *subscriptionReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsureSubscription, d)
}

func (r *subscriptionReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	out, err := r.dispatch(OperationObserveSubscriptions, filter)
	if err != nil {
		return nil, "", err
	}
	return extractItems(out), "", nil
}

func (r *subscriptionReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroySubscription, reconcilePayload{"subscription_id": ref})
	return err
}

func (r *subscriptionReconciler) dispatch(op string, in reconcilePayload) (reconcilePayload, error) {
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   op,
		Integration: contract.IntegrationContext{InstanceID: r.instanceID},
		Input:       in,
	})
	if err != nil {
		return nil, err
	}
	return reconcilePayload(resp.Output), nil
}

// webhookEndpointReconciler wraps Execute() for Stripe WebhookEndpoints.
type webhookEndpointReconciler struct{ instanceID string }

func newWebhookEndpointReconciler(instanceID string) *webhookEndpointReconciler {
	return &webhookEndpointReconciler{instanceID: instanceID}
}

func (r *webhookEndpointReconciler) Ensure(ctx context.Context, d reconcilePayload) (reconcilePayload, error) {
	return r.dispatch(OperationEnsureWebhookEndpoint, d)
}

func (r *webhookEndpointReconciler) Observe(ctx context.Context, filter map[string]any) ([]reconcilePayload, string, error) {
	out, err := r.dispatch(OperationObserveWebhookEndpoints, filter)
	if err != nil {
		return nil, "", err
	}
	return extractItems(out), "", nil
}

func (r *webhookEndpointReconciler) Destroy(ctx context.Context, ref string) error {
	_, err := r.dispatch(OperationDestroyWebhookEndpoint, reconcilePayload{"id": ref})
	return err
}

func (r *webhookEndpointReconciler) dispatch(op string, in reconcilePayload) (reconcilePayload, error) {
	resp, err := Execute(contract.AdapterExecuteIntegrationRequest{
		Operation:   op,
		Integration: contract.IntegrationContext{InstanceID: r.instanceID},
		Input:       in,
	})
	if err != nil {
		return nil, err
	}
	return reconcilePayload(resp.Output), nil
}

// extractItems pulls the items array from a paged observe response,
// converting each map[string]any element into a reconcilePayload. The
// observe handlers always wrap results in {"items": [...], "has_more": ...}.
func extractItems(resp reconcilePayload) []reconcilePayload {
	if resp == nil {
		return nil
	}
	raw, ok := resp["items"].([]map[string]any)
	if !ok {
		return nil
	}
	out := make([]reconcilePayload, 0, len(raw))
	for _, item := range raw {
		out = append(out, reconcilePayload(item))
	}
	return out
}

// WireReconcilers installs reconcile.RegisterReconciler handlers for
// each managed resource type (payment_intent, customer, subscription,
// webhook_endpoint). The pre-v2.0.0 legacy capability names are kept
// alive through reconcile.WithLegacyNames so callers that still send
// e.g. "create_payment_intent" route to ensure_payment_intent with a
// WARN log entry. The shim is removed in SDK v0.6.0; adapters MUST
// drop the WithLegacyNames lists before bumping past v0.5.x.
//
// Production main() does NOT wire this — the existing message.Execute
// dispatch path (Execute → ResolveOperation → switch) is the runtime.
// WireReconcilers is the Go-API expression of the convention used by
// tests and any callers that want to drive the adapter through the
// SDK's typed Reconciler[D,O] interface.
func WireReconcilers(a *adapter.Adapter, instanceID string) {
	emitter := newEmitterFromEnv()
	commonOpts := []reconcile.Option{
		reconcile.WithProvider(Provider),
		reconcile.WithEmitter(emitter),
		reconcile.WithInstanceID(instanceID),
	}

	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "payment_intent", "payment_intents",
		newPaymentIntentReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"create_payment_intent",
				"confirm_payment_intent",
				"cancel_payment_intent",
				"retrieve_payment_intent",
			),
		)...,
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "customer", "customers",
		newCustomerReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"create_customer",
				"update_customer",
				"list_customers",
			),
		)...,
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "subscription", "subscriptions",
		newSubscriptionReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"create_subscription",
				"update_subscription",
				"cancel_subscription",
				"list_subscriptions",
			),
		)...,
	)
	reconcile.RegisterReconciler[reconcilePayload, reconcilePayload](
		a, "webhook_endpoint", "webhook_endpoints",
		newWebhookEndpointReconciler(instanceID),
		append(commonOpts,
			reconcile.WithLegacyNames(
				"create_webhook_endpoint",
				"update_webhook_endpoint",
				"delete_webhook_endpoint",
				"list_webhook_endpoints",
			),
		)...,
	)
}

// newEmitterFromEnv returns an events.Emitter wired to yggdrasil-core
// when YGGDRASIL_CORE_URL is set, otherwise a NoopEmitter. Env-driven
// keeps the Lego principle: no broker / secret-store / cloud is
// hardcoded; callers point us at any core URL they want. Emission is
// best-effort (see sdk/reconcile.WithEmitter docstring).
func newEmitterFromEnv() events.Emitter {
	if os.Getenv(events.EnvCoreURL) == "" {
		return &events.NoopEmitter{}
	}
	return events.NewHTTPEmitter()
}

// Silence "json imported but unused" — kept for callers that marshal
// reconcilePayload manually.
var _ = json.Marshal
