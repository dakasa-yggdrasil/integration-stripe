import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, SubscriptionItem } from "./types";
import { mockEnabled, mockSubscriptions } from "./mock";

export interface SubscriptionsResult {
  items: SubscriptionItem[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits flat values; normalize every row into the strict shape.
// RULE #0: `customer` is read ONLY as the opaque Stripe ref (cus_…) the adapter
// projects — this normalizer never reads a name/email field, and there is no
// such field on the wire.
function normalize(raw: Record<string, unknown>): SubscriptionItem {
  const amount = Number(raw.amount);
  const periodEnd = Number(raw.current_period_end);
  const plan = (raw.plan ?? {}) as Record<string, unknown>;
  return {
    id: (raw.id ?? "").toString(),
    status: (raw.status ?? "").toString(),
    planNickname: (plan.nickname ?? "").toString(),
    planPriceId: (plan.price_id ?? "").toString(),
    amount: Number.isFinite(amount) ? amount : 0,
    currency: (raw.currency ?? "").toString(),
    currentPeriodEnd: Number.isFinite(periodEnd) ? periodEnd : 0,
    cancelAtPeriodEnd: raw.cancel_at_period_end === true,
    customer: (raw.customer ?? "").toString()
  };
}

/** True when a subscription is set to end at period end (no auto-renew). */
export function isCancelAtPeriodEnd(s: SubscriptionItem): boolean {
  return s.cancelAtPeriodEnd;
}

/** True when a subscription is actively billing ("active" or "trialing"). */
export function isSubscriptionActive(s: SubscriptionItem): boolean {
  const st = s.status.trim().toLowerCase();
  return st === "active" || st === "trialing";
}

/**
 * The instance's Stripe subscriptions (config-grade refs only — never customer
 * data). The optional `limit` is threaded through to the adapter.
 */
export function useSubscriptions(instanceId: string | undefined, limit = 50): SubscriptionsResult {
  const mock = mockEnabled();
  const query = useSurfaceQuery<ItemsEnvelope<SubscriptionItem>>(
    mock ? undefined : instanceId,
    "list-subscriptions",
    { limit }
  );

  if (mock) {
    return { items: mockSubscriptions(), isLoading: false, isError: false, error: null };
  }

  const raw = (query.data?.items ?? []) as unknown as Array<Record<string, unknown>>;
  return {
    items: raw.map(normalize),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
