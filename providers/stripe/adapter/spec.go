// Package adapter implements the stripe provider for the stripe
// integration. v2.0.0 aligned with the Yggdrasil universal capability
// naming convention (ensure_/observe_/destroy_ + discover_/on_).
// See docs/superpowers/specs/2026-05-27-yggdrasil-integration-capability-convention.md.
//
// 6 managed resource types — payment_intent, customer, subscription,
// charge, webhook_endpoint, balance — each exposed via the canonical
// triple where applicable. Plus the kept allowlisted action helpers
// (create_refund — money-movement; verify_webhook_signature — pure
// helper) and the kept setup_intent / payout / connect_account ops
// (action-shaped state transitions not yet collapsed).
package adapter

import (
	"strings"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

const (
	Provider        = "stripe"
	IntegrationType = "stripe"
	AdapterVersion  = "2.4.0"
	// StripeAPIVersion pins the Stripe API version. Bumping requires a
	// full integration test cycle + adapter version bump. Documented in
	// README.md and integration_type manifest spec.adapter.version notes.
	StripeAPIVersion = "2024-12-18.acacia"

	// Canonical (v2.0.0) capability names — ensure_/observe_/destroy_
	// per the universal naming convention.
	OperationEnsurePaymentIntent   = "ensure_payment_intent"
	OperationObservePaymentIntents = "observe_payment_intents"
	OperationDestroyPaymentIntent  = "destroy_payment_intent"

	OperationEnsureCustomer   = "ensure_customer"
	OperationObserveCustomers = "observe_customers"
	OperationDestroyCustomer  = "destroy_customer"

	OperationEnsureSubscription   = "ensure_subscription"
	OperationObserveSubscriptions = "observe_subscriptions"
	OperationDestroySubscription  = "destroy_subscription"

	OperationObserveCharges = "observe_charges"
	OperationObserveBalance = "observe_balance"

	OperationEnsureWebhookEndpoint   = "ensure_webhook_endpoint"
	OperationObserveWebhookEndpoints = "observe_webhook_endpoints"
	OperationDestroyWebhookEndpoint  = "destroy_webhook_endpoint"

	// Kept ops — allowlisted action-shaped helpers + state transitions
	// that don't collapse cleanly into the ensure/observe/destroy triple.
	OperationCreateRefund         = "create_refund"
	OperationCreateSetupIntent    = "create_setup_intent"
	OperationCreatePayout         = "create_payout"
	OperationManageConnectAccount = "manage_connect_account"
	OperationVerifyWebhookSig     = "verify_webhook_signature"

	// OperationOnSurfaceQuery is the read-only aggregator invoked by core's
	// /api/v1/integrations/{instance_id}/surface-query proxy on behalf of the
	// payments-ops console surface. The surface passes { query_name, params }
	// as Input; the adapter routes by query_name to a read aggregator (see
	// surface_query.go). Its "on_" prefix classifies it as a "reactor"
	// (framework-invoked, hidden from grant pickers) via actionCategory — same
	// convention as the kubernetes/aws/grafana/prometheus adapters. It is a
	// pure GET wrapper over the existing observe_* handlers; it mutates nothing.
	OperationOnSurfaceQuery = "on_surface_query"

	// 1 reactor (NOT executable via execute; framework-invoked via webhook).
	ReactorStripeWebhookReceived = "stripe_webhook_received"

	QueueDescribe = "yggdrasil.adapter.stripe.describe"
	QueueExecute  = "yggdrasil.adapter.stripe.execute"

	// Resource types declared in the integration_type manifest.
	resourcePaymentIntent   = "payment_intent"
	resourceCustomer        = "customer"
	resourceSubscription    = "subscription"
	resourceCharge          = "charge"
	resourceBalance         = "balance"
	resourceWebhookEndpoint = "webhook_endpoint"
	resourceRefund          = "refund"
	resourceSetupIntent     = "setup_intent"
	resourcePayout          = "payout"
	resourceConnect         = "connect_account"
)

var SupportedExecuteOperations = []string{
	OperationEnsurePaymentIntent,
	OperationObservePaymentIntents,
	OperationDestroyPaymentIntent,
	OperationEnsureCustomer,
	OperationObserveCustomers,
	OperationDestroyCustomer,
	OperationEnsureSubscription,
	OperationObserveSubscriptions,
	OperationDestroySubscription,
	OperationObserveCharges,
	OperationObserveBalance,
	OperationEnsureWebhookEndpoint,
	OperationObserveWebhookEndpoints,
	OperationDestroyWebhookEndpoint,
	OperationCreateRefund,
	OperationCreateSetupIntent,
	OperationCreatePayout,
	OperationManageConnectAccount,
	OperationVerifyWebhookSig,
	OperationOnSurfaceQuery,
}

// Describe returns the adapter contract for yggdrasil-core handshake.
func Describe() contract.AdapterDescribeResponse {
	return contract.AdapterDescribeResponse{
		Provider: Provider,
		Adapter: contract.IntegrationAdapterSpec{
			Version:        AdapterVersion,
			TimeoutSeconds: 30,
			Transport:      "http_json",
			Endpoints: contract.IntegrationAdapterRoute{
				Describe: "/rpc/describe",
				Execute:  "/rpc/execute",
			},
		},
		Capabilities: []string{"describe", "execute"},
		CredentialSchema: contract.IntegrationSchemaSpec{
			Mode: "inline",
			// Either field is accepted. Adapter reads stripe_api_key first
			// then falls back to stripe_secret_key — operators using
			// secret-store conventions that name the value
			// "stripe_secret_key" don't need to rename their secret.
			// At least one must be present.
			Required: []string{},
			Properties: map[string]contract.IntegrationSchemaProperty{
				"stripe_api_key": {
					Type:        "string",
					Description: "Stripe API key (sk_live_* or rk_live_*). Canonical field name.",
					Secret:      true,
					Label:       "Stripe API key",
					LabelLocale: map[string]string{"pt-BR": "Chave da API Stripe", "en-US": "Stripe API key"},
					Placeholder: "sk_live_... ou rk_live_...",
					PlaceholderLocale: map[string]string{
						"pt-BR": "sk_live_... ou rk_live_...",
						"en-US": "sk_live_... or rk_live_...",
					},
					DescriptionLocale: map[string]string{
						"pt-BR": "API key da Stripe. Encontre em Stripe Dashboard → Developers → API Keys. Prefira restricted keys (rk_).",
						"en-US": "Stripe API key from Stripe Dashboard → Developers → API Keys. Prefer restricted keys (rk_).",
					},
					Group:       "Stripe API credentials",
					GroupLocale: map[string]string{"pt-BR": "Credenciais da API Stripe", "en-US": "Stripe API credentials"},
					Order:       1,
					Sensitive:   true,
				},
				"stripe_secret_key": {
					Type:        "string",
					Description: "Stripe API key (sk_live_* or rk_live_*). Alias accepted when secret store entries follow that naming convention. Read only if stripe_api_key is absent.",
					Secret:      true,
					Label:       "Stripe API key (alias)",
					LabelLocale: map[string]string{"pt-BR": "Chave da API Stripe (alias)", "en-US": "Stripe API key (alias)"},
					Placeholder: "sk_live_... ou rk_live_...",
					DescriptionLocale: map[string]string{
						"pt-BR": "Alias do campo stripe_api_key, aceito para compatibilidade com cofres que usam o nome stripe_secret_key.",
						"en-US": "Alias for stripe_api_key, accepted for compatibility with secret stores that use the stripe_secret_key naming.",
					},
					Group:       "Stripe API credentials",
					GroupLocale: map[string]string{"pt-BR": "Credenciais da API Stripe", "en-US": "Stripe API credentials"},
					Order:       2,
					Sensitive:   true,
				},
			},
		},
		InstanceSchema: contract.IntegrationSchemaSpec{
			Mode: "inline",
			Properties: map[string]contract.IntegrationSchemaProperty{
				"stripe_webhook_secret": {
					Type:        "string",
					Description: "Stripe webhook endpoint signing secret (whsec_*).",
					Secret:      true,
					Label:       "Webhook signing secret",
					LabelLocale: map[string]string{"pt-BR": "Segredo de assinatura do webhook", "en-US": "Webhook signing secret"},
					Placeholder: "whsec_...",
					PlaceholderLocale: map[string]string{
						"pt-BR": "whsec_...",
						"en-US": "whsec_...",
					},
					DescriptionLocale: map[string]string{
						"pt-BR": "Secret de assinatura do webhook Stripe (whsec_). Encontre em Webhooks → endpoint → Signing secret.",
						"en-US": "Stripe webhook signing secret (whsec_). Found at Webhooks → endpoint → Signing secret.",
					},
					Group:       "Webhook",
					GroupLocale: map[string]string{"pt-BR": "Webhook", "en-US": "Webhook"},
					Order:       1,
					Sensitive:   true,
				},
				"stripe_account_id": {
					Type:        "string",
					Description: "Optional Stripe Connect account ID; sets Stripe-Account header.",
					Label:       "Connect account ID",
					LabelLocale: map[string]string{"pt-BR": "ID da conta Connect", "en-US": "Connect account ID"},
					Placeholder: "acct_...",
					DescriptionLocale: map[string]string{
						"pt-BR": "ID opcional da conta Stripe Connect. Define o header Stripe-Account para chamadas Connect.",
						"en-US": "Optional Stripe Connect account ID. Sets the Stripe-Account header for Connect calls.",
					},
					Group:       "Stripe Connect",
					GroupLocale: map[string]string{"pt-BR": "Stripe Connect", "en-US": "Stripe Connect"},
					Order:       2,
				},
				"stripe_api_version": {
					Type:        "string",
					Default:     StripeAPIVersion,
					Label:       "Stripe API version",
					LabelLocale: map[string]string{"pt-BR": "Versão da API Stripe", "en-US": "Stripe API version"},
					Placeholder: StripeAPIVersion,
					DescriptionLocale: map[string]string{
						"pt-BR": "Versão da API Stripe usada nas chamadas (header Stripe-Version).",
						"en-US": "Stripe API version sent via the Stripe-Version header.",
					},
					Group:       "Stripe settings",
					GroupLocale: map[string]string{"pt-BR": "Configuração Stripe", "en-US": "Stripe settings"},
					Order:       3,
				},
				"webhook_tolerance_seconds": {
					Type:        "integer",
					Default:     300,
					Label:       "Webhook tolerance (s)",
					LabelLocale: map[string]string{"pt-BR": "Tolerância do webhook (s)", "en-US": "Webhook tolerance (s)"},
					DescriptionLocale: map[string]string{
						"pt-BR": "Janela máxima (em segundos) entre o timestamp do webhook e o instante de verificação.",
						"en-US": "Maximum allowed clock skew (seconds) between webhook timestamp and verification.",
					},
					Group:       "Webhook",
					GroupLocale: map[string]string{"pt-BR": "Webhook", "en-US": "Webhook"},
					Order:       4,
				},
				// Operator-injected non-secret metadata accepted by every
				// stripe instance. Declared so yggdrasil-core does not
				// reject the config at validation time. The adapter
				// ignores fields it does not consume; the canonical
				// adapter-side override hook is `stripe_api_base_url`
				// (see clientForExecuteConfig). These three exist for
				// operator ergonomics (e.g. "sandbox" vs "production"
				// filtering, base URL override on the validation harness).
				"base_url": {
					Type:        "string",
					Description: "Override RPC base URL for the adapter pod (used by the validation harness).",
					Label:       "Adapter RPC base URL",
					LabelLocale: map[string]string{"pt-BR": "URL base do RPC do adapter", "en-US": "Adapter RPC base URL"},
					DescriptionLocale: map[string]string{
						"pt-BR": "Override da URL base do RPC do pod do adapter (usado por harness de validação).",
						"en-US": "Override RPC base URL for the adapter pod (used by the validation harness).",
					},
					Group:       "Advanced",
					GroupLocale: map[string]string{"pt-BR": "Avançado", "en-US": "Advanced"},
					Order:       5,
					Format:      "uri",
				},
				"environment": {
					Type:        "string",
					Description: "Free-form environment tag (sandbox/production) for operator filtering.",
					Label:       "Environment tag",
					LabelLocale: map[string]string{"pt-BR": "Tag de ambiente", "en-US": "Environment tag"},
					Placeholder: "production",
					DescriptionLocale: map[string]string{
						"pt-BR": "Tag livre (sandbox/production) usada para filtragem operacional.",
						"en-US": "Free-form environment tag (sandbox/production) for operator filtering.",
					},
					Group:       "Advanced",
					GroupLocale: map[string]string{"pt-BR": "Avançado", "en-US": "Advanced"},
					Order:       6,
				},
				"provider": {
					Type:        "string",
					Description: "Provider-name echo (always \"stripe\"). Kept for instance ergonomics.",
					Label:       "Provider name",
					LabelLocale: map[string]string{"pt-BR": "Nome do provider", "en-US": "Provider name"},
					Default:     "stripe",
					DescriptionLocale: map[string]string{
						"pt-BR": "Eco do nome do provider (sempre \"stripe\"). Mantido por ergonomia.",
						"en-US": "Provider name echo (always \"stripe\"). Kept for instance ergonomics.",
					},
					Group:       "Advanced",
					GroupLocale: map[string]string{"pt-BR": "Avançado", "en-US": "Advanced"},
					Order:       7,
				},
			},
		},
		ResourceTypes: []contract.IntegrationResourceType{
			{
				Name:             resourcePaymentIntent,
				CanonicalPrefix:  "thirdparty.stripe.payment_intent",
				IdentityTemplate: "payment_intent.{id}",
				DefaultActions: []string{
					OperationEnsurePaymentIntent,
					OperationObservePaymentIntents,
					OperationDestroyPaymentIntent,
				},
			},
			{
				Name:             resourceCustomer,
				CanonicalPrefix:  "thirdparty.stripe.customer",
				IdentityTemplate: "customer.{id}",
				DefaultActions: []string{
					OperationEnsureCustomer,
					OperationObserveCustomers,
					OperationDestroyCustomer,
				},
			},
			{
				Name:             resourceSubscription,
				CanonicalPrefix:  "thirdparty.stripe.subscription",
				IdentityTemplate: "subscription.{id}",
				DefaultActions: []string{
					OperationEnsureSubscription,
					OperationObserveSubscriptions,
					OperationDestroySubscription,
				},
			},
			{
				Name:             resourceCharge,
				CanonicalPrefix:  "thirdparty.stripe.charge",
				IdentityTemplate: "charge.{id}",
				DefaultActions:   []string{OperationObserveCharges, OperationCreateRefund},
			},
			{
				Name:             resourceBalance,
				CanonicalPrefix:  "thirdparty.stripe.balance",
				IdentityTemplate: "balance.{account}",
				DefaultActions:   []string{OperationObserveBalance},
			},
			{
				Name:             resourceWebhookEndpoint,
				CanonicalPrefix:  "thirdparty.stripe.webhook_endpoint",
				IdentityTemplate: "webhook_endpoint.{id}",
				DefaultActions: []string{
					OperationEnsureWebhookEndpoint,
					OperationObserveWebhookEndpoints,
					OperationDestroyWebhookEndpoint,
					OperationVerifyWebhookSig,
				},
			},
			{Name: resourceRefund, CanonicalPrefix: "thirdparty.stripe.refund", IdentityTemplate: "refund.{id}", DefaultActions: []string{OperationCreateRefund}},
			{Name: resourceSetupIntent, CanonicalPrefix: "thirdparty.stripe.setup_intent", IdentityTemplate: "setup_intent.{id}", DefaultActions: []string{OperationCreateSetupIntent}},
			{Name: resourcePayout, CanonicalPrefix: "thirdparty.stripe.payout", IdentityTemplate: "payout.{id}", DefaultActions: []string{OperationCreatePayout}},
			{Name: resourceConnect, CanonicalPrefix: "thirdparty.stripe.connect_account", IdentityTemplate: "connect_account.{id}", DefaultActions: []string{OperationManageConnectAccount}},
		},
		ActionCatalog: describeActionCatalog(),
		Discovery: contract.IntegrationDiscoverySpec{
			Mode:             "push",
			Cursor:           "none",
			SupportsWebhooks: true,
		},
		Normalization: contract.IntegrationNormalizationSpec{
			ExternalIDPath:         "data.object.id",
			NamePath:               "data.object.id",
			FallbackResourcePrefix: "thirdparty.stripe.custom",
		},
		Execution: contract.IntegrationExecutionSpec{
			SupportsDryRun:    false,
			IdempotentActions: idempotentExecuteOperations(),
		},
		Extensions: contract.IntegrationExtensionsSpec{
			AllowCustomResourceTypes: false,
			AllowCustomActions:       false,
			PreserveRawPayload:       true,
		},
	}
}

func describeActionCatalog() []contract.IntegrationActionDefinition {
	catalog := []contract.IntegrationActionDefinition{
		// payment_intent triple — Stripe treats POST as create when no
		// idempotency-key match, update when same key reused. observe
		// dispatches GET /v1/payment_intents/{id} when filter.id is set,
		// list otherwise. destroy maps to POST /cancel (404-tolerant).
		{Name: OperationEnsurePaymentIntent, Description: "Ensure a Stripe PaymentIntent exists. Set input.confirm=true to also confirm (state-transition collapsed into ensure). Idempotent via Idempotency-Key header.", ResourceTypes: []string{resourcePaymentIntent}, Idempotent: true},
		{Name: OperationObservePaymentIntents, Description: "Observe Stripe PaymentIntents. Filter {id} → single record; otherwise paginated list (limit, starting_after).", ResourceTypes: []string{resourcePaymentIntent}, Idempotent: true},
		{Name: OperationDestroyPaymentIntent, Description: "Destroy (cancel) a Stripe PaymentIntent. POST /v1/payment_intents/{id}/cancel. 404 → already-absent success.", ResourceTypes: []string{resourcePaymentIntent}, Idempotent: true},

		// customer triple — ensure GET-by-email-or-id first, POST/PATCH.
		{Name: OperationEnsureCustomer, Description: "Ensure a Stripe Customer exists for the given email or id. POST new when absent, PATCH deltas when present.", ResourceTypes: []string{resourceCustomer}, Idempotent: true},
		{Name: OperationObserveCustomers, Description: "Observe Stripe Customers. Filter {id} or {email} → single/by-email lookup; otherwise paginated list.", ResourceTypes: []string{resourceCustomer}, Idempotent: true},
		{Name: OperationDestroyCustomer, Description: "Destroy a Stripe Customer. DELETE /v1/customers/{id}. 404 → already-absent success.", ResourceTypes: []string{resourceCustomer}, Idempotent: true},

		// subscription triple.
		{Name: OperationEnsureSubscription, Description: "Ensure a Stripe Subscription exists for the customer + items. POST when absent, PATCH deltas (cancel_at_period_end etc.) when present.", ResourceTypes: []string{resourceSubscription}, Idempotent: true},
		{Name: OperationObserveSubscriptions, Description: "Observe Stripe Subscriptions. Filter {id} → single, {customer} → list-by-customer, else paginated list.", ResourceTypes: []string{resourceSubscription}, Idempotent: true},
		{Name: OperationDestroySubscription, Description: "Destroy a Stripe Subscription. DELETE /v1/subscriptions/{id} immediate; pass {cancel_at_period_end=true} for graceful update path. 404 → already-absent success.", ResourceTypes: []string{resourceSubscription}, Idempotent: true},

		// charges (read-only) + refunds (money-movement allowlist).
		{Name: OperationObserveCharges, Description: "Observe Stripe Charges filtered by {customer}, {payment_intent}, or list with cursor pagination.", ResourceTypes: []string{resourceCharge}, Idempotent: true},
		{Name: OperationCreateRefund, Description: "Refund a charge partially or fully. Money-movement action — allowlisted (not collapsed into ensure_refund).", ResourceTypes: []string{resourceRefund}, Idempotent: true},

		// balance.
		{Name: OperationObserveBalance, Description: "Observe the Stripe account balance. GET /v1/balance — returns available + pending arrays per currency.", ResourceTypes: []string{resourceBalance}, Idempotent: true},

		// webhook_endpoint triple.
		{Name: OperationEnsureWebhookEndpoint, Description: "Ensure a Stripe WebhookEndpoint exists for the URL + events. POST when absent, PATCH (URL/enabled_events) when present.", ResourceTypes: []string{resourceWebhookEndpoint}, Idempotent: true},
		{Name: OperationObserveWebhookEndpoints, Description: "Observe Stripe WebhookEndpoints. Filter {id} → single; otherwise paginated list.", ResourceTypes: []string{resourceWebhookEndpoint}, Idempotent: true},
		{Name: OperationDestroyWebhookEndpoint, Description: "Destroy a Stripe WebhookEndpoint. DELETE /v1/webhook_endpoints/{id}. 404 → already-absent success.", ResourceTypes: []string{resourceWebhookEndpoint}, Idempotent: true},

		// Kept action helpers (allowlisted via core's capability_naming_allowlist.yaml).
		{Name: OperationCreateSetupIntent, Description: "Create a SetupIntent to save a payment method.", ResourceTypes: []string{resourceSetupIntent}, Idempotent: true},
		{Name: OperationCreatePayout, Description: "Create a payout to a bank account. Money-movement action — allowlisted.", ResourceTypes: []string{resourcePayout}, Idempotent: true},
		{Name: OperationManageConnectAccount, Description: "Create / get / update a Stripe Connect Express/Custom account.", ResourceTypes: []string{resourceConnect}, Idempotent: true},
		{Name: OperationVerifyWebhookSig, Description: "Standalone HMAC-SHA256 webhook signature verification. Pure helper — allowlisted.", ResourceTypes: []string{resourceWebhookEndpoint}, Idempotent: true},

		// Surface-driven read aggregator (category=reactor via on_ prefix:
		// framework-invoked, hidden from grant pickers). Read-only.
		{Name: OperationOnSurfaceQuery, Description: "Surface-driven read aggregator invoked by core's /api/v1/integrations/{instance_id}/surface-query proxy. Accepts { query_name, params }; routes by query_name to a read aggregator. Supported: list-webhook-endpoints (webhook-health pillar — {id,url,status,enabled_events,api_version}), get-balance (available + pending arrays per currency, smallest unit), list-charges (recent charges for reconciliation — {id,amount,currency,status,created,refunded,payment_intent}; customer PII intentionally not projected). Read-only; never mutates.", ResourceTypes: []string{resourceWebhookEndpoint}, Idempotent: true},

		// Reactor.
		{Name: ReactorStripeWebhookReceived, Description: "Inbound Stripe webhook delivery (framework-invoked).", ResourceTypes: []string{resourceWebhookEndpoint}, Idempotent: false},
	}
	for i := range catalog {
		catalog[i].Category = actionCategory(catalog[i].Name)
	}
	return catalog
}

// actionCategory classifies an action by its prefix convention.
// "stripe_webhook_*" (the inbound webhook reactor) and "on_*" lifecycle hooks
// (e.g. on_surface_query) → "reactor": framework-invoked, hidden from grant
// pickers. Everything else → "capability" (user-grantable). Mirrors the
// prometheus/grafana sibling adapters.
func actionCategory(name string) string {
	if strings.HasPrefix(name, "stripe_webhook_") || strings.HasPrefix(name, "on_") {
		return "reactor"
	}
	return "capability"
}

func idempotentExecuteOperations() []string {
	out := make([]string, 0, len(SupportedExecuteOperations))
	out = append(out, SupportedExecuteOperations...)
	return out
}

// SupportsExecuteCapability returns true when value matches one of the
// known execute operations, or is empty (the SDK passes "" for the
// default capability dispatch).
func SupportsExecuteCapability(value string) bool {
	v := strings.TrimSpace(value)
	for _, supported := range SupportedExecuteOperations {
		if v == supported {
			return true
		}
	}
	return v == ""
}

// NormalizeExecuteOperation prefers the explicit operation field, falling
// back to the capability field for clients that only send the latter.
func NormalizeExecuteOperation(operation, capability string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = strings.TrimSpace(capability)
	}
	return operation
}

// legacyOperationAliases maps pre-v2.0.0 capability names to their v2
// canonical replacements. Used by the in-adapter compat shim attached
// by Execute when an unknown-but-known-legacy operation arrives. The
// SDK-level reconcile.WithLegacyNames provides the same behavior for
// callers wired through reconcile.RegisterReconciler; this map covers
// the existing Execute() switch path during the v0.5.x compat window.
var legacyOperationAliases = map[string]string{
	"create_payment_intent":   OperationEnsurePaymentIntent,
	"confirm_payment_intent":  OperationEnsurePaymentIntent, // collapsed via {confirm:true}
	"cancel_payment_intent":   OperationDestroyPaymentIntent,
	"retrieve_payment_intent": OperationObservePaymentIntents,
	"create_customer":         OperationEnsureCustomer,
	"update_customer":         OperationEnsureCustomer,
	"list_customers":          OperationObserveCustomers,
	"create_subscription":     OperationEnsureSubscription,
	"update_subscription":     OperationEnsureSubscription,
	"cancel_subscription":     OperationDestroySubscription,
	"list_subscriptions":      OperationObserveSubscriptions,
	"list_charges":            OperationObserveCharges,
	"create_webhook_endpoint": OperationEnsureWebhookEndpoint,
	"update_webhook_endpoint": OperationEnsureWebhookEndpoint,
	"delete_webhook_endpoint": OperationDestroyWebhookEndpoint,
	"list_webhook_endpoints":  OperationObserveWebhookEndpoints,
	"retrieve_balance":        OperationObserveBalance,
}

// ResolveOperation returns the canonical operation name for op, mapping
// pre-v2.0.0 names through legacyOperationAliases. The bool return is
// true when a legacy alias was applied — callers can log the WARN.
func ResolveOperation(op string) (string, bool) {
	op = strings.TrimSpace(op)
	if canonical, ok := legacyOperationAliases[op]; ok {
		return canonical, true
	}
	return op, false
}
