// Deep-link helpers to the NATIVE Stripe Dashboard. RULE #0: this console is a
// payments-OPS view — it shows config / health / reconciliation refs, never a
// per-customer billing UI and never money-movement controls. Anything that means
// "act on this in Stripe" (open a dispute, inspect a payout, refund a charge) is
// a deep-link ("↗") OUT to the real Stripe Dashboard, never an in-console action.
//
// HONESTY about the base URL: the surface-query responses do NOT carry the
// instance's Stripe Dashboard host (the adapter shapes only row fields). Under
// `?mock` we supply the real dashboard host (MOCK_STRIPE_BASE). In the live path
// the host is not yet wired through a surface read, so {@link useStripeBase}
// returns "" and the UI degrades honestly: a deep-link with an unknown base is
// rendered DISABLED with a tooltip — we never point "↗" at a guessed URL.

/** Normalize a Stripe Dashboard base into a host root with no trailing slash. */
export function normalizeStripeBase(raw: string | undefined): string {
  let base = (raw ?? "").trim();
  if (base === "") return "";
  base = base.replace(/\/+$/, "");
  return base;
}

/** The native Stripe webhooks settings page, or "" when the host is unknown. */
export function webhooksHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/webhooks`;
}

/** A single webhook endpoint's detail page (`/webhooks/{id}`), or "". */
export function webhookEndpointHref(base: string, id: string): string {
  const host = normalizeStripeBase(base);
  if (host === "" || id.trim() === "") return "";
  return `${host}/webhooks/${encodeURIComponent(id)}`;
}

/** The native Stripe balance / payouts page, or "" when the host is unknown. */
export function balanceHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/balance/overview`;
}

/** The native Stripe payouts page, or "" when the host is unknown. */
export function payoutsHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/payouts`;
}

/** The native Stripe disputes page, or "" when the host is unknown. */
export function disputesHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/disputes`;
}

/** The native Stripe payments page, or "" when the host is unknown. */
export function paymentsHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/payments`;
}

/** A single payment_intent's detail page (`/payments/{pi}`), or "". */
export function paymentIntentHref(base: string, pi: string): string {
  const host = normalizeStripeBase(base);
  if (host === "" || pi.trim() === "") return "";
  return `${host}/payments/${encodeURIComponent(pi)}`;
}

/** The native Stripe subscriptions list page, or "" when the host is unknown. */
export function subscriptionsHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/subscriptions`;
}

/** A single subscription's detail page (`/subscriptions/{id}`), or "". */
export function subscriptionHref(base: string, id: string): string {
  const host = normalizeStripeBase(base);
  if (host === "" || id.trim() === "") return "";
  return `${host}/subscriptions/${encodeURIComponent(id)}`;
}

/** The native Stripe payment-intents list page, or "" when the host is unknown. */
export function paymentIntentsHref(base: string): string {
  const host = normalizeStripeBase(base);
  if (host === "") return "";
  return `${host}/payments`;
}

/** A single charge's payment detail page (`/payments/{ch}`), or "". */
export function chargeHref(base: string, charge: string): string {
  const host = normalizeStripeBase(base);
  if (host === "" || charge.trim() === "") return "";
  return `${host}/payments/${encodeURIComponent(charge)}`;
}
