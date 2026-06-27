import type { CSSProperties } from "react";

export interface DeepLinkArrowProps {
  /** The native-Stripe URL to open, or "" when the host isn't wired yet. */
  href: string;
  /** Accessible label, e.g. "Abrir no Stripe". */
  label: string;
  /** Class on the rendered element so a parent row's :hover can warm the ↗. */
  className?: string;
  /** Inline overrides (e.g. textAlign on a table cell). */
  style?: CSSProperties;
}

const BASE: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  fontWeight: 700,
  fontSize: "var(--fs-md)",
  textDecoration: "none",
  lineHeight: 1
};

/**
 * The icon-only "↗" deep-link OUT to the native Stripe Dashboard — the single
 * affordance that enforces CRITICAL RULE #0: this console never moves money and
 * never shows per-customer billing, so "act on this in Stripe" (inspect a
 * payment, open a dispute, run a refund) is always this link out.
 *
 * Honest degradation: when `href === ""` the Stripe Dashboard host isn't wired
 * through a surface read yet (the live case today), so we render a DISABLED,
 * muted "↗" with a tooltip that says so — we never point the arrow at a guessed
 * URL. Under `?mock` the host is supplied so the affordance is a real link.
 *
 * Opens in a new tab with `rel="noreferrer"`. Icon-only per the craft playbook
 * (the "↗" carries the meaning; no "abrir" word clutter).
 */
export function DeepLinkArrow({ href, label, className, style }: DeepLinkArrowProps) {
  if (href === "") {
    return (
      <span
        className={className}
        aria-hidden="true"
        title="Link para o Stripe indisponível."
        style={{ ...BASE, color: "var(--mut)", opacity: 0.4, cursor: "not-allowed", ...style }}
      >
        ↗
      </span>
    );
  }
  return (
    <a
      className={className}
      href={href}
      target="_blank"
      rel="noreferrer"
      aria-label={label}
      title={label}
      style={{ ...BASE, color: "inherit", ...style }}
    >
      ↗
    </a>
  );
}
