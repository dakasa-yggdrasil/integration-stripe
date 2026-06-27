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

/**
 * A row from `list-subscriptions` (one Stripe subscription).
 *
 * The adapter projects GET /v1/subscriptions →
 * `{id, status, plan{nickname,price_id}, amount(int smallest unit), currency,
 * current_period_end(unix), cancel_at_period_end(bool), customer}`.
 *
 * RULE #0: `customer` arrives on the wire as an OPAQUE Stripe ref (`cus_…`) —
 * never a name or email. The surface shows it (if at all) only as that opaque
 * id; the normalizer below never reads a name/email field, and the table has NO
 * customer-name column. We carry the ref so an operator can correlate, nothing
 * more.
 */
export interface SubscriptionItem {
  /** Subscription id (`sub_…`) — the opaque ref shown mono. */
  id: string;
  /** Subscription status ("active" / "past_due" / "canceled" / "trialing" / …). */
  status: string;
  /** The plan's human nickname ("" when Stripe carries none). */
  planNickname: string;
  /** The plan's price id (`price_…`), an opaque config ref. */
  planPriceId: string;
  /** Recurring amount in the smallest currency unit. */
  amount: number;
  /** Lower-case ISO currency code. */
  currency: string;
  /** Current period end as a Unix epoch (seconds) — the next renew/charge date. */
  currentPeriodEnd: number;
  /** True when the subscription is set to end at period end (no auto-renew). */
  cancelAtPeriodEnd: boolean;
  /** The opaque customer ref (`cus_…`), "" when absent — NEVER a name/email. */
  customer: string;
}

/**
 * A row from `list-payment-intents` (one Stripe PaymentIntent).
 *
 * The adapter projects GET /v1/payment_intents →
 * `{id, status, amount, currency, created(unix), capture_method}`. No customer
 * data is projected (rule #0).
 */
export interface PaymentIntentItem {
  /** PaymentIntent id (`pi_…`) — the opaque ref shown mono. */
  id: string;
  /** PI status ("succeeded" / "requires_payment_method" / "canceled" / …). */
  status: string;
  /** Amount in the smallest currency unit. */
  amount: number;
  /** Lower-case ISO currency code. */
  currency: string;
  /** Creation time as a Unix epoch (seconds). */
  created: number;
  /** Capture method ("automatic" / "manual"). */
  captureMethod: string;
}

/**
 * One refund inside a {@link ChargeDetail} (from `charge-detail`'s `refunds`).
 *
 * RULE #0: only the opaque refund id + amount/reason/created — never any
 * customer ref. A refund is money already moved (read-only history here).
 */
export interface RefundItem {
  /** Refund id (`re_…`) — opaque ref shown mono. */
  id: string;
  /** Refunded amount in the smallest currency unit. */
  amount: number;
  /** Stripe's refund reason ("requested_by_customer" / "fraudulent" / …), or "". */
  reason: string;
  /** Refund creation time as a Unix epoch (seconds). */
  created: number;
}

/**
 * The `charge-detail` object (param `charge_id`) — the drill-down read behind a
 * charge id in the Reconciliação roster.
 *
 * The adapter projects GET /v1/charges/{id} →
 * `{id, amount, currency, status, created(unix), refunded(bool),
 * refundedAmount(int), payment_intent, failureCode, failureMessage,
 * refunds:[{id, amount, reason, created}]}`.
 *
 * RULE #0: only opaque refs (`id`, `payment_intent`, refund ids) are read —
 * never the customer name/email Stripe carries on `billing_details`. The detail
 * view NEVER renders a customer identity; `failureMessage` is Stripe's own
 * decline string (e.g. "Your card was declined."), not customer data.
 */
export interface ChargeDetail {
  /** Charge id (`ch_…`). */
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
  /** Total amount refunded so far, in the smallest currency unit. */
  refundedAmount: number;
  /** The opaque payment_intent ref (`pi_…`), "" when absent. */
  paymentIntent: string;
  /** Stripe failure code when the charge failed (e.g. "card_declined"), or "". */
  failureCode: string;
  /** Stripe's failure message when the charge failed, or "". */
  failureMessage: string;
  /** The charge's refunds (money-already-moved history), newest first. */
  refunds: RefundItem[];
}

/** The envelope every list surface query returns: `{ items, has_more }`. */
export interface ItemsEnvelope<T> {
  items: T[];
  has_more?: boolean;
}
