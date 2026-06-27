import type { CSSProperties } from "react";
import { CapabilityGate } from "@dakasa-yggdrasil/surface-toolkit";

export interface GatedActionProps {
  /** Capability the viewer must hold for this affordance to render at all. */
  need: string;
  /** The viewer's held perms (from useCollaboratorScope().perms). */
  perms: string[];
  /** Eyebrow (e.g. "Remediação"). */
  eyebrow: string;
  /** The ops-remediation line — what this WOULD do, framed as remediation. */
  label: string;
  /** The disabled button caption (defaults to "Em breve"). */
  cta?: string;
  /** Tooltip on the disabled button. */
  hint?: string;
}

const CARD: CSSProperties = {
  background: "var(--sand2)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)",
  padding: "var(--sp-4) var(--sp-5)",
  fontSize: "var(--fs-sm)",
  color: "var(--body)"
};

const EYEBROW: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.2em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

/**
 * The money-movement affordance, gated + disabled. RULE #0: refund / payout are
 * admin-tier and OUT of v1, so this is NEVER a transactional button — it renders
 * only for a viewer who holds the capability (so the read-first console doesn't
 * tease an action they can't have), and even then it is a disabled "Em breve"
 * framed as ops-remediation, not a "Pay" / "Refund" call to action. When v1.x
 * wires the write path, this becomes the live control — every page already gates
 * through here.
 */
export function GatedAction({ need, perms, eyebrow, label, cta = "Em breve", hint }: GatedActionProps) {
  return (
    <CapabilityGate need={need} perms={perms} fallback={null}>
      <section style={CARD}>
        <span style={EYEBROW}>{eyebrow}</span>
        <div
          style={{
            marginTop: "var(--sp-2)",
            display: "flex",
            alignItems: "center",
            gap: "var(--sp-3)",
            flexWrap: "wrap"
          }}
        >
          <span style={{ flex: 1, minWidth: 0, color: "var(--mut)", lineHeight: 1.5 }}>{label}</span>
          <button
            type="button"
            disabled
            title={hint ?? "Movimentação de dinheiro chega numa próxima etapa (admin)."}
            style={{
              fontFamily: "var(--font-body)",
              fontSize: "var(--fs-xs)",
              fontWeight: 600,
              padding: "var(--sp-1) var(--sp-3)",
              borderRadius: "var(--r-sm)",
              border: "1px solid var(--line)",
              background: "var(--cream)",
              color: "var(--mut)",
              cursor: "not-allowed",
              whiteSpace: "nowrap"
            }}
          >
            {cta}
          </button>
        </div>
      </section>
    </CapabilityGate>
  );
}
