import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, PaymentIntentItem } from "./types";
import { mockEnabled, mockPaymentIntents } from "./mock";

export interface PaymentIntentsResult {
  items: PaymentIntentItem[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits flat values; normalize every row into the strict shape.
// RULE #0: no customer field is projected or read — only opaque refs + facts.
function normalize(raw: Record<string, unknown>): PaymentIntentItem {
  const amount = Number(raw.amount);
  const created = Number(raw.created);
  return {
    id: (raw.id ?? "").toString(),
    status: (raw.status ?? "").toString(),
    amount: Number.isFinite(amount) ? amount : 0,
    currency: (raw.currency ?? "").toString(),
    created: Number.isFinite(created) ? created : 0,
    captureMethod: (raw.capture_method ?? "").toString()
  };
}

/**
 * The instance's recent PaymentIntents (opaque refs + status/amount facts only,
 * never customer data). The optional `limit` is threaded through to the adapter.
 */
export function usePaymentIntents(instanceId: string | undefined, limit = 50): PaymentIntentsResult {
  const mock = mockEnabled();
  const query = useSurfaceQuery<ItemsEnvelope<PaymentIntentItem>>(
    mock ? undefined : instanceId,
    "list-payment-intents",
    { limit }
  );

  if (mock) {
    return { items: mockPaymentIntents(), isLoading: false, isError: false, error: null };
  }

  const raw = (query.data?.items ?? []) as unknown as Array<Record<string, unknown>>;
  return {
    items: raw.map(normalize),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
