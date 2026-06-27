// Shapes returned by the integration-stripe adapter's surface queries
// (providers/stripe/adapter/surface_query.go → onSurfaceQuery). The adapter
// emits flat JSON values; the published toolkit gives us no inference for
// `surface-query` responses, so every field is typed here and read defensively
// at the call site. Only fields the surface consumes are declared.
//
// IMPORTANT (keep in sync with surface_query.go + adapter.go observe_* handlers):
// the field NAMES below are the adapter's output keys.
//
// RULE #0 (this is the contract's canonical PAYMENT-RAIL surface): a
// payments-OPS view for the platform team, NEVER a per-customer billing UI.
// Customer-identifying data appears ONLY as opaque refs (charge id,
// payment_intent). The adapter already omits the customer name / email from
// every projection; the UI keeps it that way — there is intentionally NO
// "customer" field anywhere below.

/**
 * A row from `list-webhook-endpoints` (one Stripe webhook endpoint).
 *
 * The adapter projects GET /v1/webhook_endpoints →
 * `{id, url, status, enabled_events([]string), api_version}`. `status` is
 * Stripe's endpoint status ("enabled" / "disabled"). `enabled_events` is the
 * list of subscribed event types (or `["*"]` for all).
 */
export interface WebhookEndpointItem {
  /** Stripe webhook endpoint id (`we_…`). */
  id: string;
  /** The endpoint URL Stripe POSTs deliveries to. */
  url: string;
  /** Endpoint status — "enabled" when Stripe is delivering, else "disabled". */
  status: string;
  /** Subscribed event types (`["*"]` = all events). */
  enabledEvents: string[];
  /** The Stripe API version this endpoint renders events at ("" when default). */
  apiVersion: string;
}

/**
 * One balance bucket (per currency) from `get-balance`.
 *
 * Stripe returns amounts in the SMALLEST currency unit (cents) — the UI formats
 * per-currency at render time, never here. `currency` is the lower-case ISO
 * code Stripe uses ("brl", "usd").
 */
export interface BalanceAmount {
  /** Amount in the smallest currency unit (e.g. 4820000 = R$ 48.200,00). */
  amount: number;
  /** Lower-case ISO currency code ("brl", "usd"). */
  currency: string;
}

/** The `get-balance` envelope: available + pending arrays, one entry/currency. */
export interface BalanceResult {
  available: BalanceAmount[];
  pending: BalanceAmount[];
}

/**
 * A row from `list-charges` (one recent charge, for reconciliation context).
 *
 * The adapter projects GET /v1/charges →
 * `{id, amount, currency, status, created(unix), refunded(bool), payment_intent}`.
 *
 * RULE #0: only opaque refs (`id`, `payment_intent`) are projected — never the
 * customer name / email Stripe may carry on the charge's billing_details. The
 * surface NEVER renders a customer column.
 */
export interface ChargeItem {
  /** Charge id (`ch_…`) — the opaque ref shown mono. */
  id: string;
  /** Amount in the smallest currency unit. */
  amount: number;
  /** Lower-case ISO currency code. */
  currency: string;
  /** Charge status ("succeeded" / "pending" / "failed"). */
  status: string;
  /** Creation time as a Unix epoch (seconds). */
  created: number;
  /** Whether the charge was (fully) refunded. */
  refunded: boolean;
  /** The opaque payment_intent ref (`pi_…`), "" when absent. */
  paymentIntent: string;
}

/** The envelope every list surface query returns: `{ items, has_more }`. */
export interface ItemsEnvelope<T> {
  items: T[];
  has_more?: boolean;
}
