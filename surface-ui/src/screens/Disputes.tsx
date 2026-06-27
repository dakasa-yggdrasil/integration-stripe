import type { CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import { useStripeBase, disputesHref, mockEnabled, MOCK_INSTANCE_ID } from "../data";
import { kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .st-dp-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .st-dp-kpis { grid-template-columns: 1fr; } }
`;


export function Disputes() {
  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();
  const href = disputesHref(stripeBase);

  // No dispute data is read at all (deep-link-only), so the instance handle is
  // only used to keep the chrome consistent; we never query dispute state.
  void instanceId;

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="st-dp-kpis">
        <KpiTile eyebrow="Disputas abertas" value="—" chart={kpiSubtext("via ↗ no Stripe", false)} />
        <KpiTile eyebrow="Prazo mais próximo" value="—" chart={kpiSubtext("no Stripe", false)} />
        <KpiTile eyebrow="Estado" value="—" chart={kpiSubtext("no Stripe", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading) {
      return <LoadingState label="Carregando…" />;
    }
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* the honest framing + the prominent deep-link */}
        <section
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--sp-4)",
            padding: "var(--sp-5) var(--sp-6)",
            background: "var(--sand2)",
            border: "1px solid var(--line)",
            borderRadius: "var(--r-lg)"
          }}
        >
          <p style={{ margin: 0, fontSize: "var(--fs-md)", color: "var(--body)", lineHeight: 1.55 }}>
            Disputas: estado e prazos no Stripe (<strong>↗</strong>).
          </p>
          <div>
            {href ? (
              <a
                href={href}
                target="_blank"
                rel="noreferrer"
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: "var(--sp-2)",
                  padding: "var(--sp-2) var(--sp-4)",
                  borderRadius: "var(--r-sm)",
                  border: "1px solid var(--honey)",
                  background: "var(--honey)",
                  color: "var(--cream)",
                  fontFamily: "var(--font-body)",
                  fontSize: "var(--fs-sm)",
                  fontWeight: 700,
                  textDecoration: "none"
                }}
              >
                Abrir Disputes no Stripe <span aria-hidden="true">↗</span>
              </a>
            ) : (
              <span
                title="Link para o Stripe indisponível."
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: "var(--sp-2)",
                  padding: "var(--sp-2) var(--sp-4)",
                  borderRadius: "var(--r-sm)",
                  border: "1px solid var(--line)",
                  background: "var(--cream)",
                  color: "var(--mut)",
                  fontSize: "var(--fs-sm)",
                  fontWeight: 700,
                  cursor: "not-allowed",
                  opacity: 0.7
                }}
              >
                Abrir Disputes no Stripe <span aria-hidden="true">↗</span>
              </span>
            )}
          </div>
        </section>
      </div>
    );
  }

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Disputas"
        subtitle="Deep-link para o Stripe."
        kpis={instanceLoading ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
