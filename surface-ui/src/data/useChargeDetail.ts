import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ChargeDetail, RefundItem } from "./types";
import { mockEnabled, mockChargeDetail } from "./mock";

export interface ChargeDetailResult {
  detail: ChargeDetail | null;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// One refund row from the `charge-detail` object's `refunds` array.
// RULE #0: only the opaque refund id + amount/reason/created are read.
function normalizeRefund(raw: Record<string, unknown>): RefundItem {
  const amount = Number(raw.amount);
  const created = Number(raw.created);
  return {
    id: (raw.id ?? "").toString(),
    amount: Number.isFinite(amount) ? amount : 0,
    reason: (raw.reason ?? "").toString(),
    created: Number.isFinite(created) ? created : 0
  };
}

// The adapter returns the `charge-detail` object (not a list envelope). Normalize
// the flat object + its refunds[] into the strict shape. RULE #0: only opaque
// refs are read; `failureMessage` is Stripe's own decline string, not customer
// data, and there is intentionally no billing_details read here.
function normalize(raw: Record<string, unknown>): ChargeDetail {
  const amount = Number(raw.amount);
  const created = Number(raw.created);
  const refundedAmount = Number(raw.refundedAmount);
  const rawRefunds = raw.refunds;
  const refunds = Array.isArray(rawRefunds)
    ? (rawRefunds as Array<Record<string, unknown>>).map(normalizeRefund)
    : [];
  return {
    id: (raw.id ?? "").toString(),
    amount: Number.isFinite(amount) ? amount : 0,
    currency: (raw.currency ?? "").toString(),
    status: (raw.status ?? "").toString(),
    created: Number.isFinite(created) ? created : 0,
    refunded: raw.refunded === true,
    refundedAmount: Number.isFinite(refundedAmount) ? refundedAmount : 0,
    paymentIntent: (raw.payment_intent ?? "").toString(),
    failureCode: (raw.failureCode ?? "").toString(),
    failureMessage: (raw.failureMessage ?? "").toString(),
    refunds: refunds.sort((a, b) => b.created - a.created)
  };
}

/**
 * The `charge-detail` drill-down read for a single charge id (the param
 * `charge_id`). The query stays disabled until both an instance handle and a
 * non-empty charge id are present. Under `?mock` the network is bypassed and the
 * fixture detail is returned (scripted for the refunded + failed charges, a
 * synthesized succeeded detail otherwise). RULE #0: opaque refs only.
 */
export function useChargeDetail(
  instanceId: string | undefined,
  chargeId: string
): ChargeDetailResult {
  const mock = mockEnabled();
  const hasCharge = chargeId.trim() !== "";
  const query = useSurfaceQuery<Record<string, unknown>>(
    mock || !hasCharge ? undefined : instanceId,
    "charge-detail",
    { charge_id: chargeId }
  );

  if (mock) {
    return {
      detail: hasCharge ? mockChargeDetail(chargeId) : null,
      isLoading: false,
      isError: false,
      error: null
    };
  }

  const data = query.data;
  return {
    detail: data && (data.id ?? "").toString() !== "" ? normalize(data) : null,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
