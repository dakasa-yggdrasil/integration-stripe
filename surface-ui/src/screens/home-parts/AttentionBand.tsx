import type { CSSProperties } from "react";
import { Chip, Pill, LoadingState } from "@dakasa-yggdrasil/surface-toolkit";
import type { WebhookEndpointsResult } from "../../data";
import { isEndpointDisabled } from "../../data";

export interface AttentionBandProps {
  webhooks: WebhookEndpointsResult;
}

const ROW: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--sp-3)",
  padding: "var(--sp-3) var(--sp-4)",
  background: "var(--cream)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)",
  minWidth: 0
};

/**
 * The euphemized "Precisa de você" band. The tone is supportive, never "ALERTA".
 *
 * HONEST by construction: disputes and signature-failures are NOT readable yet
 * (no observe op for disputes; signature-failure rate lives on /metrics with no
 * surface passthrough), so we never lead with a fabricated "you have N disputes".
 * The one real, readable signal today is a webhook endpoint Stripe has stopped
 * delivering to (status !== "enabled") — events silently pile up there, so that
 * leads. When nothing is readable-critical, we say so plainly and note what
 * lands once the RTA/metrics passthrough is wired.
 */
export function AttentionBand({ webhooks }: AttentionBandProps) {
  if (webhooks.isLoading) {
    return <LoadingState label="Lendo a saúde dos webhooks…" />;
  }

  const disabled = webhooks.items.filter(isEndpointDisabled).slice(0, 6);

  if (disabled.length === 0) {
    return (
      <p
        style={{
          margin: 0,
          display: "flex",
          alignItems: "center",
          gap: "var(--sp-2)",
          fontSize: "var(--fs-md)",
          color: "var(--mut)",
          lineHeight: 1.5
        }}
      >
        <span aria-hidden="true" style={{ color: "var(--ok)", fontWeight: 700 }}>
          ✓
        </span>
        <span>Nada precisa de você. Webhooks entregando.</span>
      </p>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
        {disabled.map((e) => (
          <div key={e.id || e.url} style={ROW}>
            <div style={{ display: "flex", flexDirection: "column", gap: "2px", minWidth: 0, flex: 1 }}>
              <span
                style={{
                  fontFamily: "var(--font-mono, var(--font-body))",
                  fontSize: "var(--fs-sm)",
                  fontWeight: 500,
                  color: "var(--ink)",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap"
                }}
                title={e.url}
              >
                {e.url || e.id}
              </span>
              <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)" }}>
                <Chip label="webhook" tone="team" />
                <Pill label="não entregando" tone="crit" preserveCase />
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
