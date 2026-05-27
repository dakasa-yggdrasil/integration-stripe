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

// Reserved payload keys used by the providers/stripe/message bridge
// to forward integration context (instance_spec.config /
// instance_spec.credentials) and per-request auth through the SDK
// reconcile envelope into the per-resource dispatch helpers. The SDK
// only knows about input JSON — so the bridge stashes the integration
// context here, and dispatch lifts these back onto a full
// AdapterExecuteIntegrationRequest before invoking Execute().
//
// The "__" prefix prevents collision with operator-supplied input
// fields (no convention allows leading-underscore field names). The
// dispatch helpers strip these keys from the forwarded payload before
// handing it to Execute() so handlers never see the reserved data
// inside req.Input.
//
// Fixes the cycle-#243 bridge-side regression where the bridge dropped
// instance_spec / req.Auth on the SDK-reconciler-routed path. Mirrors
// the canonical fix in integration-github v2.4.1.
//
// NOTE (DONE_WITH_CONCERNS, 2026-05-27): integration-stripe ALSO
// carries a pre-existing structural bug independent of this fix —
// adapter.Execute calls clientForInstance(InstanceID, "", "", ...)
// with an empty apiKey unconditionally and never reads
// req.Integration.InstanceSpec.Credentials["stripe_api_key"]. The
// instances map loaded by cmd/adapter/main.go::config.LoadInstances
// is captured by message.ExecuteHandler but never threaded into
// clientForInstance, so in production NewStripeClient("") returns
// "stripe api key is required" and writes fail regardless of whether
// the bridge forwards credentials. The bridge fix here is necessary
// but not sufficient; the secondary Execute()/clientForInstance bug
// needs a separate cycle.
const (
	InstanceConfigKey = "__instance_config"
	InstanceCredsKey  = "__instance_credentials"
	InstanceAuthKey   = "__request_auth"
)

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
	resp, err := Execute(buildExecuteRequest(op, in, r.instanceID))
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
	resp, err := Execute(buildExecuteRequest(op, in, r.instanceID))
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
	resp, err := Execute(buildExecuteRequest(op, in, r.instanceID))
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
	resp, err := Execute(buildExecuteRequest(op, in, r.instanceID))
	if err != nil {
		return nil, err
	}
	return reconcilePayload(resp.Output), nil
}

// buildExecuteRequest rebuilds a full AdapterExecuteIntegrationRequest from
// the reconcile-layer payload, restoring the instance_spec and request auth
// that the bridge stashed under reserved keys (InstanceConfigKey /
// InstanceCredsKey / InstanceAuthKey). Without this rehydration Execute()
// receives empty InstanceSpec + Auth, which (combined with the secondary
// pre-existing bug in adapter.Execute / clientForInstance — see package
// docstring) surfaces as "stripe api key is required". Shared across all
// per-resource reconcilers so the wire shape is uniform.
func buildExecuteRequest(op string, in reconcilePayload, fallbackInstanceID string) contract.AdapterExecuteIntegrationRequest {
	instanceConfig := extractInstanceMap(in, InstanceConfigKey)
	instanceCredentials := extractInstanceMap(in, InstanceCredsKey)
	requestAuth := extractInstanceMap(in, InstanceAuthKey)
	forwardedInput := stripReservedKeys(in)
	return contract.AdapterExecuteIntegrationRequest{
		Operation: op,
		Input:     forwardedInput,
		Auth:      requestAuth,
		Integration: contract.IntegrationContext{
			InstanceID: instanceFromPayload(in, fallbackInstanceID),
			Spec: contract.IntegrationInstanceManifestSpec{
				Config:      instanceConfig,
				Credentials: instanceCredentials,
			},
		},
	}
}

// extractInstanceMap pulls one of the reserved bridge-forwarded fields off
// the reconcile payload. Returns nil when absent — Execute() handles nil
// maps the same as empty.
func extractInstanceMap(in reconcilePayload, key string) map[string]any {
	if in == nil {
		return nil
	}
	if v, ok := in[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}

// stripReservedKeys returns a copy of in with the bridge-reserved keys
// removed so handlers only see operator-supplied fields. Returns nil on
// nil input to preserve the existing Execute() nil-input contract.
func stripReservedKeys(in reconcilePayload) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch k {
		case InstanceConfigKey, InstanceCredsKey, InstanceAuthKey:
			continue
		}
		out[k] = v
	}
	return out
}

// instanceFromPayload pulls the instance_id off a reconcilePayload (the
// SDK forwards env.InstanceID into the input map via the bridge in
// controllers/message/execute.go) so production routing keeps the
// per-request instance context. Falls back to fallback when the
// payload carries no override — preserving test/single-instance flows.
func instanceFromPayload(in reconcilePayload, fallback string) string {
	if in == nil {
		return fallback
	}
	if v, ok := in["instance_id"].(string); ok && v != "" {
		return v
	}
	return fallback
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
// WARN log entry. The shim removal target moved to SDK v0.7.0.
//
// Production wiring (v0.7.0+): main() calls WireReconcilers BEFORE
// registering describe/execute, and the controllers/message
// ExecuteHandler routes inbound traffic through reconcile.Dispatch —
// activating §6.5 mutation event auto-emission for every operator
// request. instanceID is the FALLBACK passed when the inbound
// envelope carries no instance_id; the reconciler dispatch helpers
// prefer the payload-bound value (instanceFromPayload).
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
