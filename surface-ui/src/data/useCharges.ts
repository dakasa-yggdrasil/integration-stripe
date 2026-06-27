import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, ChargeItem } from "./types";
import { mockEnabled, mockCharges } from "./mock";

export interface ChargesResult {
  items: ChargeItem[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits flat values; normalize every row into the strict shape.
// RULE #0: only opaque refs (id, payment_intent) ever appear — the adapter never
// projects customer name/email, and this normalizer never reads such a field.
function normalize(raw: Record<string, unknown>): ChargeItem {
  const amount = Number(raw.amount);
  const created = Number(raw.created);
  return {
    id: (raw.id ?? "").toString(),
    amount: Number.isFinite(amount) ? amount : 0,
    currency: (raw.currency ?? "").toString(),
    status: (raw.status ?? "").toString(),
    created: Number.isFinite(created) ? created : 0,
    refunded: raw.refunded === true,
    paymentIntent: (raw.payment_intent ?? "").toString()
  };
}

/**
 * Recent charges for reconciliation context (config-grade refs only — never
 * customer data). The optional `limit` is threaded through to the adapter.
 */
export function useCharges(instanceId: string | undefined, limit = 25): ChargesResult {
  const mock = mockEnabled();
  const query = useSurfaceQuery<ItemsEnvelope<ChargeItem>>(
    mock ? undefined : instanceId,
    "list-charges",
    { limit }
  );

  if (mock) {
    return { items: mockCharges(), isLoading: false, isError: false, error: null };
  }

  const raw = (query.data?.items ?? []) as unknown as Array<Record<string, unknown>>;
  return {
    items: raw.map(normalize),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
