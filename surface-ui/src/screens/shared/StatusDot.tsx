import type { CSSProperties } from "react";

export type DotTone = "ok" | "warn" | "crit" | "mut";

const TONE_VAR: Record<DotTone, string> = {
  ok: "var(--ok)",
  warn: "var(--warn, var(--honey))",
  crit: "var(--crit, #b3261e)",
  mut: "var(--mut)"
};

export interface StatusDotProps {
  tone: DotTone;
  /** Visible label next to the dot. */
  label: string;
  /** Accessible title (defaults to the label). */
  title?: string;
}

/**
 * A small colored status dot + label. Honest by construction: the tone is passed
 * in by the caller from a REAL field (webhook endpoint status, charge status) —
 * this component never invents a health color. There is no green "all good" dot
 * for anything the adapter can't actually read.
 */
export function StatusDot({ tone, label, title }: StatusDotProps) {
  const dot: CSSProperties = {
    width: 8,
    height: 8,
    borderRadius: "50%",
    background: TONE_VAR[tone],
    flex: "0 0 auto"
  };
  return (
    <span
      title={title ?? label}
      style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)", minWidth: 0 }}
    >
      <span aria-hidden="true" style={dot} />
      <span
        style={{
          fontSize: "var(--fs-sm)",
          color: "var(--body)",
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap"
        }}
      >
        {label}
      </span>
    </span>
  );
}

/** Map a Stripe charge status to a dot tone. */
export function chargeStatusTone(status: string): DotTone {
  switch (status.trim().toLowerCase()) {
    case "succeeded":
      return "ok";
    case "pending":
      return "warn";
    case "failed":
      return "crit";
    default:
      return "mut";
  }
}

/** Map a Stripe subscription status to a dot tone. */
export function subscriptionStatusTone(status: string): DotTone {
  switch (status.trim().toLowerCase()) {
    case "active":
    case "trialing":
      return "ok";
    case "past_due":
    case "unpaid":
    case "incomplete":
      return "warn";
    case "canceled":
    case "incomplete_expired":
      return "crit";
    default:
      return "mut";
  }
}

/** Map a Stripe PaymentIntent status to a dot tone. */
export function paymentIntentStatusTone(status: string): DotTone {
  switch (status.trim().toLowerCase()) {
    case "succeeded":
      return "ok";
    case "processing":
    case "requires_capture":
    case "requires_action":
    case "requires_confirmation":
      return "warn";
    case "requires_payment_method":
    case "canceled":
      return "crit";
    default:
      return "mut";
  }
}
