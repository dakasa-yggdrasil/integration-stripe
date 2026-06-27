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

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-lg)",
  fontWeight: 500,
  color: "var(--ink)"
};

const CARD: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--sp-2)",
  padding: "var(--sp-4) var(--sp-5)",
  background: "var(--cream)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)"
};

// What this page WILL show once a dispute read exists — framed honestly as
// needs-work, NEVER fabricated as present data. Disputes are financially urgent
// (response deadlines), so faking a count here would be the worst kind of lie.
const NEEDS_WORK: Array<{ label: string; detail: string }> = [
  {
    label: "Disputas abertas & prazo de resposta",
    detail:
      "Quais chargebacks estão abertos e até quando responder. O adapter não tem uma op observe_disputes hoje — as disputas são reconstruídas a jusante a partir do log de eventos RTA (charge.dispute.created/closed), não lidas aqui. Por isso não mostramos um número — seria inventado."
  },
  {
    label: "Evidência & estado da contestação",
    detail:
      "O estado de cada disputa (needs_response / under_review / won / lost) e a evidência submetida. Chega quando um cliente /v1/disputes + parser + testes existir no adapter."
  },
  {
    label: "Alerta de prazo",
    detail:
      "O sinal de “responda até X” entra no “Precisa de você” da Home quando o passthrough de evento RTA / dispute estiver ligado. Até lá, o prazo real mora no Stripe."
  }
];

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
        <KpiTile eyebrow="Prazo mais próximo" value="—" chart={kpiSubtext("leitura needs-work", false)} />
        <KpiTile eyebrow="Estado" value="—" chart={kpiSubtext("reconstruído via RTA", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading) {
      return <LoadingState label="Preparando os links de disputa…" />;
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
            As <strong>disputas</strong> não são legíveis nesta surface: o adapter não tem uma op de leitura para elas —
            são reconstruídas a jusante a partir do log de eventos (RTA / Prometheus). Para não inventar um número de
            disputas abertas nem um prazo, esta página é <strong>deep-link</strong>: o estado real, com os prazos de
            resposta, mora no Stripe nativo.
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
                title="Link para o Stripe nativo indisponível: o host do dashboard ainda não é exposto por um surface read."
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

        {/* what's needs-work */}
        <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
          <h3 style={SECTION_TITLE}>O que falta conectar</h3>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
            {NEEDS_WORK.map((v) => (
              <div key={v.label} style={CARD}>
                <span style={{ fontWeight: 600, color: "var(--ink)" }}>{v.label}</span>
                <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>{v.detail}</span>
              </div>
            ))}
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
        subtitle="Deep-link por ora: as disputas não são lidas no adapter (reconstruídas via RTA a jusante). Nada de disputa inventada — o estado e os prazos reais estão no Stripe (↗)."
        kpis={instanceLoading ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
