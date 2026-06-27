import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { BalanceAmount, BalanceResult } from "./types";
import { mockEnabled, mockBalanceAvailable, mockBalancePending } from "./mock";

export interface BalanceQueryResult {
  available: BalanceAmount[];
  pending: BalanceAmount[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits `{available:[{amount,currency}], pending:[...]}`. Normalize
// each bucket defensively: amount → number (smallest unit), currency → string.
function normalizeBucket(raw: unknown): BalanceAmount[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((entry) => {
    const row = (entry ?? {}) as Record<string, unknown>;
    const amount = Number(row.amount);
    return {
      amount: Number.isFinite(amount) ? amount : 0,
      currency: (row.currency ?? "").toString()
    };
  });
}

/**
 * The current Stripe balance snapshot: available + pending arrays, one entry per
 * currency. Amounts are in the SMALLEST currency unit (cents) exactly as Stripe
 * returns them — the UI formats per-currency at render time (see formatMoney).
 */
export function useBalance(instanceId: string | undefined): BalanceQueryResult {
  const mock = mockEnabled();
  const query = useSurfaceQuery<BalanceResult>(
    mock ? undefined : instanceId,
    "get-balance",
    {}
  );

  if (mock) {
    return {
      available: mockBalanceAvailable(),
      pending: mockBalancePending(),
      isLoading: false,
      isError: false,
      error: null
    };
  }

  return {
    available: normalizeBucket(query.data?.available),
    pending: normalizeBucket(query.data?.pending),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
