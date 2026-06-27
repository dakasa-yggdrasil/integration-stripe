// DEV-only fixtures so Home + the four detail pages can be designed and
// verified populated, without a live Stripe instance. Gated by `mockEnabled()`
// (import.meta.env.DEV + a `?mock` URL param) at every call site, so this is
// dead code tree-shaken out of any production build. The data is realistic and
// internally consistent: 2 webhook endpoints (1 enabled with many event types,
// 1 disabled — the honest "precisa de você" signal), a balance of available
// R$ 48,2k + pending R$ 12,9k, and ~8 recent charges (1 refunded).
//
// There is intentionally NO dispute fixture and NO signature-failure fixture —
// the adapter has no observe op for disputes (they are reconstructed downstream
// from the RTA event log) and signature-failure / rta-emit-error rates live on
// the adapter's /metrics health port with no surface passthrough yet. The
// Disputas page is honestly deep-link-only and the Home never fabricates a
// dispute count or a signature-failure number.

import type { CollaboratorScope } from "@dakasa-yggdrasil/surface-toolkit";
import type { WebhookEndpointItem, BalanceAmount, ChargeItem } from "./types";

/**
 * DEV `?mock` switch, shared across the hooks so every read short-circuits the
 * network together. Never true in a production build (guarded on DEV).
 */
export function mockEnabled(): boolean {
  return (
    import.meta.env.DEV &&
    typeof location !== "undefined" &&
    new URLSearchParams(location.search).has("mock")
  );
}

/** Fake instance id used to satisfy the surface-query handle under `?mock`. */
export const MOCK_INSTANCE_ID = "mock-stripe-instance";

/** The Stripe account label shown in the headline under `?mock`. */
export const MOCK_INSTANCE_LABEL = "DaKasa · Stripe (live)";

/**
 * Fixture Stripe Dashboard host for `?mock` deep-links. In production this host
 * is not yet wired through a surface read, so the live deep-links degrade
 * honestly (disabled "↗"); under the mock we point at the real dashboard host so
 * the "↗" affordance can be reviewed as a working link.
 */
export const MOCK_STRIPE_BASE = "https://dashboard.stripe.com";

/**
 * Fully-offline collaborator + permission context for `?mock` review. Under the
 * mock gate this replaces the network-backed `useCollaboratorScope()` so the
 * surface renders standalone with zero requests to /me, provisioning-status, or
 * the manifests list. Tier is `admin` and the perms cover every Stripe
 * money-movement capability the surface gates on, so the (gated, disabled)
 * "Em breve" affordances are visible for review. Never reached in production
 * (gated on {@link mockEnabled}).
 */
export function mockCollaboratorScope(): CollaboratorScope {
  return {
    collaborator: {
      id: "giomaster",
      slug: "Giomaster",
      display_name: "Giovanni Rios Martins",
      primary_email: "giovanni.martins@dakasa.me",
      status: "active"
    },
    teams: [{ teamId: "mock-team", slug: "plataforma", githubSlug: "plataforma" }],
    tier: "admin",
    perms: [
      "manage-integrations",
      "stripe.refunds.create",
      "stripe.payouts.create",
      "stripe.webhooks.ensure"
    ],
    isLoading: false,
    isError: false
  };
}

// ---------------------------------------------------------------- webhooks

// 2 webhook endpoints. The first is the busy production endpoint (enabled, many
// subscribed event types — the payments-ops workhorse). The second is a legacy
// endpoint that is DISABLED — the one honest, real "precisa de você" signal the
// adapter can read today (Stripe stops delivering to a disabled endpoint, so
// events silently pile up). The Home leads with this.
//
// [id, url, status, apiVersion, enabledEvents]
const WEBHOOK_ROWS: Array<[string, string, string, string, string[]]> = [
  [
    "we_1PdK9aLiveProd0001",
    "https://yggdrasil.dakasa.me/webhooks/stripe/integration-stripe-dakasa",
    "enabled",
    "2024-12-18.acacia",
    [
      "payment_intent.succeeded",
      "payment_intent.payment_failed",
      "payment_intent.canceled",
      "charge.succeeded",
      "charge.refunded",
      "charge.dispute.created",
      "charge.dispute.closed",
      "customer.subscription.created",
      "customer.subscription.updated",
      "customer.subscription.deleted",
      "invoice.payment_succeeded",
      "invoice.payment_failed",
      "payout.paid",
      "payout.failed"
    ]
  ],
  [
    "we_1MzQ2bLegacy00002",
    "https://legacy.dakasa.me/hooks/stripe",
    "disabled",
    "",
    ["payment_intent.succeeded", "charge.refunded"]
  ]
];

export function mockWebhookEndpoints(): WebhookEndpointItem[] {
  return WEBHOOK_ROWS.map(([id, url, status, apiVersion, enabledEvents]) => ({
    id,
    url,
    status,
    apiVersion,
    enabledEvents
  }));
}

// ---------------------------------------------------------------- balance

// available R$ 48.200,00 (4_820_000 cents) + pending R$ 12.900,00 (1_290_000
// cents), plus a small USD bucket so the per-currency formatting is exercised.
export function mockBalanceAvailable(): BalanceAmount[] {
  return [
    { amount: 4_820_000, currency: "brl" },
    { amount: 132_540, currency: "usd" }
  ];
}

export function mockBalancePending(): BalanceAmount[] {
  return [
    { amount: 1_290_000, currency: "brl" },
    { amount: 18_900, currency: "usd" }
  ];
}

// ---------------------------------------------------------------- charges

// ~8 recent charges for the reconciliation roster. Exactly ONE is refunded.
// All amounts in cents; mostly BRL with one USD row. Created times are recent
// Unix epochs (seconds), descending. payment_intent is the opaque ref — NO
// customer data anywhere (rule #0).
//
// [idSuffix, amountCents, currency, status, minutesAgo, refunded, piSuffix]
const CHARGE_ROWS: Array<[string, number, string, string, number, boolean, string]> = [
  ["3PqA1succeeded01", 48_200, "brl", "succeeded", 7, false, "3PqA1pi0001"],
  ["3PqA1succeeded02", 12_900, "brl", "succeeded", 22, false, "3PqA1pi0002"],
  ["3PqA1refunded003", 89_900, "brl", "succeeded", 64, true, "3PqA1pi0003"],
  ["3PqA1succeeded04", 4_990, "brl", "succeeded", 95, false, "3PqA1pi0004"],
  ["3PqA1usd00000005", 14_900, "usd", "succeeded", 130, false, "3PqA1pi0005"],
  ["3PqA1failed00006", 23_400, "brl", "failed", 168, false, "3PqA1pi0006"],
  ["3PqA1succeeded07", 159_000, "brl", "succeeded", 240, false, "3PqA1pi0007"],
  ["3PqA1pending0008", 7_500, "brl", "pending", 305, false, "3PqA1pi0008"]
];

export function mockCharges(): ChargeItem[] {
  const now = Math.floor(Date.now() / 1000);
  return CHARGE_ROWS.map(([idSuffix, amount, currency, status, minutesAgo, refunded, piSuffix]) => ({
    id: "ch_" + idSuffix,
    amount,
    currency,
    status,
    created: now - minutesAgo * 60,
    refunded,
    paymentIntent: "pi_" + piSuffix
  }));
}
