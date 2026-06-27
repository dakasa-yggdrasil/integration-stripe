import { useWebhookEndpoints, isEndpointDisabled } from "./useWebhookEndpoints";
import { useBalance } from "./useBalance";
import type { BalanceAmount } from "./types";

export interface StripePulse {
  /** Total configured webhook endpoints. */
  webhooks: number;
  /** Endpoints actively delivering (status === "enabled"). */
  webhooksEnabled: number;
  /** Endpoints Stripe is NOT delivering to — the readable "precisa de você". */
  webhooksDisabled: number;
  /** Available balance buckets per currency (smallest unit). */
  available: BalanceAmount[];
  /** Pending balance buckets per currency (smallest unit). */
  pending: BalanceAmount[];
  isLoading: boolean;
  isError: boolean;
}

/**
 * One derived read of the Stripe account's OPS posture, composed from the two
 * always-on reads (webhook endpoints + balance). Powers the technical Home
 * headline + KPI strip. Every value is a bare, real fact an operator can act on.
 *
 * Deliberately NO `disputes` and NO `signatureFailures` here — the adapter has
 * no observe op for disputes (reconstructed downstream from the RTA event log),
 * and signature-failure rate lives on the adapter's /metrics health port with no
 * surface passthrough yet. Fabricating either count would be a lie; the Home
 * shows them honestly as "— via ↗" / "— sem passthrough".
 *
 * Charges are NOT folded into the pulse — they're a reconciliation roster, not a
 * headline count (a "recent N charges" number reads like throughput, which it
 * isn't). The Reconciliação page reads them directly.
 */
export function useStripePulse(instanceId: string | undefined): StripePulse {
  const webhooks = useWebhookEndpoints(instanceId);
  const balance = useBalance(instanceId);

  const disabled = webhooks.items.filter(isEndpointDisabled).length;

  return {
    webhooks: webhooks.items.length,
    webhooksEnabled: webhooks.items.length - disabled,
    webhooksDisabled: disabled,
    available: balance.available,
    pending: balance.pending,
    isLoading: webhooks.isLoading || balance.isLoading,
    isError: webhooks.isError || balance.isError
  };
}

/** Sum a balance bucket's BRL entry (the primary display currency), in cents. */
export function primaryBrl(buckets: BalanceAmount[]): number {
  return buckets
    .filter((b) => b.currency.trim().toLowerCase() === "brl")
    .reduce((acc, b) => acc + b.amount, 0);
}
