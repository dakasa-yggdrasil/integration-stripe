import type { CSSProperties } from "react";
import { KpiTile } from "@dakasa-yggdrasil/surface-toolkit";
import type { StripePulse } from "../../data";
import { primaryBrl, formatMoneyCompact } from "../../data";
import { kpiDelta, kpiSubtext } from "../shared/kpiQualifier";

export interface KpiStripProps {
  pulse: StripePulse;
}

// Dense, responsive grid of KpiTiles. Reflows by the host width (container
// query), never the viewport — five terse payments-OPS facts that read the same
// on a wide console or a narrow panel.
const GRID = `
  .st-kpi-strip {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
  @container (max-width: 1040px) {
    .st-kpi-strip { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  }
  @container (max-width: 560px) {
    .st-kpi-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
`;

const WRAP: CSSProperties = { containerType: "inline-size", width: "100%" };

/**
 * KPI polish: the directional arrow misleads on a static fact, so neutral/good
 * facts carry a plain muted subtext (no arrow) via the `chart` slot, and only a
 * genuinely bad signal (a disabled webhook endpoint) gets a `delta` with the
 * crit ↓.
 *
 * Honest handling of the two un-readable facts — NEVER a fabricated number:
 *  - Disputas shows "— via ↗" (the adapter has no observe op for disputes; they
 *    are reconstructed downstream from the RTA event log, opened on the Stripe
 *    Dashboard).
 *  - Falhas de assinatura shows "— sem passthrough" (the signature-failure rate
 *    lives on the adapter's /metrics health port; no core passthrough wired yet).
 */
export function KpiStrip({ pulse }: KpiStripProps) {
  const disabledBad = pulse.webhooksDisabled > 0;
  const availBrl = primaryBrl(pulse.available);
  const pendBrl = primaryBrl(pulse.pending);

  return (
    <div style={WRAP}>
      <style>{GRID}</style>
      <div className="st-kpi-strip">
        <KpiTile
          eyebrow="Webhooks ativos"
          value={pulse.webhooksEnabled}
          delta={kpiDelta(`${pulse.webhooksDisabled} desativado(s)`, disabledBad)}
          chart={kpiSubtext(`de ${pulse.webhooks} endpoint(s)`, disabledBad)}
        />
        <KpiTile
          eyebrow="Saldo disponível"
          value={formatMoneyCompact(availBrl, "brl")}
          chart={kpiSubtext("liberado p/ payout", false)}
        />
        <KpiTile
          eyebrow="Pendente"
          value={formatMoneyCompact(pendBrl, "brl")}
          chart={kpiSubtext("a liberar", false)}
        />
        <KpiTile eyebrow="Disputas" value="—" chart={kpiSubtext("via ↗ no Stripe", false)} />
        <KpiTile eyebrow="Falhas de assinatura" value="—" chart={kpiSubtext("sem passthrough", false)} />
      </div>
    </div>
  );
}
