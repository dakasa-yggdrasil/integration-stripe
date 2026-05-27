// Package adapter implements the stripe provider for the stripe
// integration. It exposes 13 mutating capabilities (PaymentIntents,
// Customers, Subscriptions, Refunds, SetupIntents, charges, payouts,
// Connect, webhook-signature verify) plus 1 reactor capability that
// receives Stripe webhook deliveries.
package adapter

import (
	"strings"

	"github.com/dakasa-yggdrasil/integration-stripe/family/contract"
)

const (
	Provider        = "stripe"
	IntegrationType = "stripe"
	AdapterVersion  = "2.0.0"
	// StripeAPIVersion pins the Stripe API version. Bumping requires a
	// full integration test cycle + adapter version bump. Documented in
	// README.md and integration_type manifest spec.adapter.version notes.
	StripeAPIVersion = "2024-12-18.acacia"

	// 13 execute capabilities
	OperationCreatePaymentIntent  = "create_payment_intent"
	OperationConfirmPaymentIntent = "confirm_payment_intent"
	OperationCancelPaymentIntent  = "cancel_payment_intent"
	OperationCreateCustomer       = "create_customer"
	OperationUpdateCustomer       = "update_customer"
	OperationCreateSubscription   = "create_subscription"
	OperationCancelSubscription   = "cancel_subscription"
	OperationCreateRefund         = "create_refund"
	OperationCreateSetupIntent    = "create_setup_intent"
	OperationListCharges          = "list_charges"
	OperationCreatePayout         = "create_payout"
	OperationManageConnectAccount = "manage_connect_account"
	OperationVerifyWebhookSig     = "verify_webhook_signature"

	// 1 reactor (NOT executable via execute; framework-invoked via webhook).
	ReactorStripeWebhookReceived = "stripe_webhook_received"

	QueueDescribe = "yggdrasil.adapter.stripe.describe"
	QueueExecute  = "yggdrasil.adapter.stripe.execute"

	// Resource types declared in the integration_type manifest. Used by
	// describeActionCatalog to wire each capability to the resource it
	// produces / mutates so contractcheck passes.
	resourcePayment      = "payment"
	resourceCustomer     = "customer"
	resourceSubscription = "subscription"
	resourceRefund       = "refund"
	resourceSetupIntent  = "setup_intent"
	resourceCharge       = "charge"
	resourcePayout       = "payout"
	resourceConnect      = "connect_account"
	resourceWebhook      = "webhook"
)

var SupportedExecuteOperations = []string{
	OperationCreatePaymentIntent,
	OperationConfirmPaymentIntent,
	OperationCancelPaymentIntent,
	OperationCreateCustomer,
	OperationUpdateCustomer,
	OperationCreateSubscription,
	OperationCancelSubscription,
	OperationCreateRefund,
	OperationCreateSetupIntent,
	OperationListCharges,
	OperationCreatePayout,
	OperationManageConnectAccount,
	OperationVerifyWebhookSig,
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
			Mode:     "inline",
			Required: []string{"stripe_api_key"},
			Properties: map[string]contract.IntegrationSchemaProperty{
				"stripe_api_key": {
					Type:        "string",
					Description: "Stripe API key (sk_live_* or rk_live_*).",
					Secret:      true,
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
				},
				"stripe_account_id": {
					Type:        "string",
					Description: "Optional Stripe Connect account ID; sets Stripe-Account header.",
				},
				"stripe_api_version": {
					Type:    "string",
					Default: StripeAPIVersion,
				},
				"webhook_tolerance_seconds": {
					Type:    "integer",
					Default: 300,
				},
			},
		},
		ResourceTypes: []contract.IntegrationResourceType{
			{Name: resourcePayment, CanonicalPrefix: "thirdparty.stripe.payment", IdentityTemplate: "payment.{id}"},
			{Name: resourceCustomer, CanonicalPrefix: "thirdparty.stripe.customer", IdentityTemplate: "customer.{id}"},
			{Name: resourceSubscription, CanonicalPrefix: "thirdparty.stripe.subscription", IdentityTemplate: "subscription.{id}"},
			{Name: resourceRefund, CanonicalPrefix: "thirdparty.stripe.refund", IdentityTemplate: "refund.{id}"},
			{Name: resourceSetupIntent, CanonicalPrefix: "thirdparty.stripe.setup_intent", IdentityTemplate: "setup_intent.{id}"},
			{Name: resourceCharge, CanonicalPrefix: "thirdparty.stripe.charge", IdentityTemplate: "charge.{id}"},
			{Name: resourcePayout, CanonicalPrefix: "thirdparty.stripe.payout", IdentityTemplate: "payout.{id}"},
			{Name: resourceConnect, CanonicalPrefix: "thirdparty.stripe.connect_account", IdentityTemplate: "connect_account.{id}"},
			{Name: resourceWebhook, CanonicalPrefix: "thirdparty.stripe.webhook", IdentityTemplate: "webhook.{id}"},
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
		{Name: OperationCreatePaymentIntent, Description: "Create a Stripe PaymentIntent for one-time payments.", ResourceTypes: []string{resourcePayment}, Idempotent: true},
		{Name: OperationConfirmPaymentIntent, Description: "Confirm an existing PaymentIntent, triggering collection.", ResourceTypes: []string{resourcePayment}, Idempotent: true},
		{Name: OperationCancelPaymentIntent, Description: "Cancel a PaymentIntent. Safe to call on already-canceled.", ResourceTypes: []string{resourcePayment}, Idempotent: true},
		{Name: OperationCreateCustomer, Description: "Create a Stripe Customer object.", ResourceTypes: []string{resourceCustomer}, Idempotent: true},
		{Name: OperationUpdateCustomer, Description: "Update mutable fields of an existing Customer.", ResourceTypes: []string{resourceCustomer}, Idempotent: true},
		{Name: OperationCreateSubscription, Description: "Create a Stripe Subscription.", ResourceTypes: []string{resourceSubscription}, Idempotent: true},
		{Name: OperationCancelSubscription, Description: "Cancel a Subscription immediately or at period end.", ResourceTypes: []string{resourceSubscription}, Idempotent: true},
		{Name: OperationCreateRefund, Description: "Refund a charge partially or fully.", ResourceTypes: []string{resourceRefund}, Idempotent: true},
		{Name: OperationCreateSetupIntent, Description: "Create a SetupIntent to save a payment method.", ResourceTypes: []string{resourceSetupIntent}, Idempotent: true},
		{Name: OperationListCharges, Description: "List charges filtered by customer or payment intent.", ResourceTypes: []string{resourceCharge}, Idempotent: true},
		{Name: OperationCreatePayout, Description: "Create a payout to a bank account.", ResourceTypes: []string{resourcePayout}, Idempotent: true},
		{Name: OperationManageConnectAccount, Description: "Create / get / update a Stripe Connect Express/Custom account.", ResourceTypes: []string{resourceConnect}, Idempotent: true},
		{Name: OperationVerifyWebhookSig, Description: "Standalone HMAC-SHA256 webhook signature verification.", ResourceTypes: []string{resourceWebhook}, Idempotent: true},
		{Name: ReactorStripeWebhookReceived, Description: "Inbound Stripe webhook delivery (framework-invoked).", ResourceTypes: []string{resourceWebhook}, Idempotent: false},
	}
	for i := range catalog {
		catalog[i].Category = actionCategory(catalog[i].Name)
	}
	return catalog
}

func actionCategory(name string) string {
	if strings.HasPrefix(name, "stripe_webhook_") {
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
