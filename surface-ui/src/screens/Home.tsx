import type { CSSProperties } from "react";
import {
  useCollaboratorScope,
  useDefaultInstance,
  LoadingState,
  EmptyState
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useStripePulse,
  useWebhookEndpoints,
  useCharges,
  primaryBrl,
  formatMoneyCompact,
  formatMoney,
  isEndpointDisabled,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID,
  MOCK_INSTANCE_LABEL
} from "../data";
import type { WebhookEndpointItem } from "../data";
import { KpiStrip, AttentionBand, PillarPreview } from "./home-parts";
import type { PillarRow } from "./home-parts";

/* ---------------------------------------------------------------- layout */

const PAGE: CSSProperties = {
  containerType: "inline-size",
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)",
  display: "flex",
  flexDirection: "column",
  gap: "var(--sp-6)",
  fontFamily: "var(--font-body)",
  color: "var(--body)"
};

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-xl)",
  fontWeight: 500,
  color: "var(--ink)"
};

const EYEBROW: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.2em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

// Pillar grid: 4 columns → 2 → 1 by host width (not viewport).
const PILLAR_GRID = `
  .st-home-pillars {
    display: grid;
    gap: var(--sp-4);
    grid-template-columns: repeat(4, minmax(0, 1fr));
    align-items: stretch;
  }
  @container (max-width: 900px) {
    .st-home-pillars { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  @container (max-width: 560px) {
    .st-home-pillars { grid-template-columns: 1fr; }
    .st-home-header { flex-direction: column; align-items: flex-start; }
  }
`;

/* ---------------------------------------------------------------- helpers */

// Technical, one-line read of the account — bare payments-OPS facts joined by
// "·". No editorializing. "esteira saudável" only when every webhook delivers.
function headline(parts: {
  label: string;
  esteiraHealthy: boolean;
  webhooksEnabled: number;
  availableBrl: number;
}): string {
  return [
    parts.label || "Stripe",
    parts.esteiraHealthy ? "esteira saudável" : "esteira com pendência",
    `${parts.webhooksEnabled} ${parts.webhooksEnabled === 1 ? "webhook ativo" : "webhooks ativos"}`,
    `saldo ${formatMoneyCompact(parts.availableBrl, "brl")}`
  ].join(" · ");
}

function webhookRows(items: WebhookEndpointItem[]): PillarRow[] {
  // Governance first: disabled endpoints. If none, a representative sample.
  const disabled = items.filter(isEndpointDisabled);
  const shown = (disabled.length > 0 ? disabled : items).slice(0, 3);
  return shown.map((e) => {
    const off = isEndpointDisabled(e);
    return {
      key: e.id || e.url,
      title: e.url || e.id,
      sub: `${e.enabledEvents.length} ${e.enabledEvents.length === 1 ? "evento" : "eventos"}`,
      tagLabel: off ? "desativado" : "ativo",
      tagTone: off ? ("crit" as const) : ("ok" as const)
    };
  });
}

/* ---------------------------------------------------------------- screen */

export function Home() {
  // The collaborator + instance context is resolved over the network (/me,
  // provisioning-status, manifests). Under `?mock` we stub it entirely so the
  // surface renders fully offline for live-review — admin tier, every Stripe
  // perm, a fake instance handle. The hooks stay called unconditionally to keep
  // hook order stable; only their values are overridden. Dead code in prod (the
  // gate is DEV + `?mock`), and the real (non-mock) path below is untouched.
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");

  const scope = mock ? mockCollaboratorScope() : liveScope;
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const instanceLabel = mock ? MOCK_INSTANCE_LABEL : "Stripe";

  const pulse = useStripePulse(instanceId);
  const webhooks = useWebhookEndpoints(instanceId);
  const charges = useCharges(instanceId, 5);

  if (scope.isLoading || instanceLoading) {
    return (
      <div className="atelier" style={{ padding: "var(--sp-7)" }}>
        <LoadingState label="Carregando…" />
      </div>
    );
  }

  if (scope.isError) {
    return (
      <div className="atelier" style={{ padding: "var(--sp-7)" }}>
        <EmptyState
          title="Não consegui carregar seu contexto"
          description="Falha ao resolver colaborador e instância. Recarregue em instantes."
        />
      </div>
    );
  }

  const availableBrl = primaryBrl(pulse.available);
  const pendingBrl = primaryBrl(pulse.pending);
  const esteiraHealthy = pulse.webhooksDisabled === 0;

  // The identity line: what the console IS — a payments-OPS view, with the hard
  // rule that money-movement and per-customer data are never here.
  const identityLine = [
    "Webhooks · saldo & payouts · disputas · conciliação",
    "ops de pagamentos — sem dados de cliente, sem mover dinheiro"
  ].join(" · ");

  return (
    <div className="atelier" style={PAGE}>
      <style>{PILLAR_GRID}</style>

      {/* ---------- header (account identity) ---------- */}
      <header
        className="st-home-header"
        style={{ display: "flex", justifyContent: "space-between", gap: "var(--sp-5)", alignItems: "flex-start" }}
      >
        <div style={{ minWidth: 0 }}>
          <span style={EYEBROW}>Conta</span>
          <div
            style={{
              fontFamily: "var(--font-heading)",
              fontSize: "var(--fs-xl)",
              fontWeight: 500,
              color: "var(--ink)",
              lineHeight: 1.15,
              marginTop: "var(--sp-1)"
            }}
          >
            {instanceLabel}
          </div>
          <div style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", fontFamily: "var(--font-mono, var(--font-body))" }}>
            {identityLine}
          </div>
        </div>
        <div style={{ textAlign: "right", fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.7 }}>
          <span style={EYEBROW}>Disponível</span>
          <div
            style={{
              fontFamily: "var(--font-heading)",
              fontSize: "var(--fs-md)",
              color: "var(--body)",
              marginTop: "var(--sp-1)"
            }}
            title={formatMoney(availableBrl, "brl")}
          >
            {formatMoneyCompact(availableBrl, "brl")}
          </div>
        </div>
      </header>

      {/* ---------- technical headline ---------- */}
      <section>
        <h1
          style={{
            margin: 0,
            fontFamily: "var(--font-heading)",
            fontSize: "var(--fs-xl)",
            fontWeight: 400,
            lineHeight: 1.3,
            letterSpacing: "-0.01em",
            color: "var(--ink)"
          }}
        >
          {headline({
            label: instanceLabel,
            esteiraHealthy,
            webhooksEnabled: pulse.webhooksEnabled,
            availableBrl
          })}
        </h1>
      </section>

      {/* ---------- KPI strip ---------- */}
      <section>
        <KpiStrip pulse={pulse} />
      </section>

      {/* ---------- precisa de você (euphemized — honest readable signal) ---------- */}
      <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
        <h2 style={SECTION_TITLE}>Precisa de você</h2>
        <AttentionBand webhooks={webhooks} />
      </section>

      {/* ---------- pillars (hard numbers) ---------- */}
      <section>
        <div className="st-home-pillars">
          <PillarPreview
            kicker="Webhook Health"
            value={pulse.webhooksEnabled}
            unit={pulse.webhooks === 1 ? "endpoint" : `de ${pulse.webhooks}`}
            rows={webhookRows(webhooks.items)}
            emptyLabel="Nenhum webhook endpoint configurado."
            to="/webhooks"
          />
          <PillarPreview
            kicker="Saldo & Payouts"
            value={formatMoneyCompact(availableBrl, "brl")}
            unit="disponível"
            rows={[
              {
                key: "bal-pending",
                title: "Pendente a liberar",
                sub: formatMoney(pendingBrl, "brl")
              },
              {
                key: "bal-payouts",
                title: "Histórico de payouts",
                sub: "Leitura ainda não conectada — needs-work.",
                tagLabel: "needs-work",
                tagTone: "neutral"
              }
            ]}
            emptyLabel="Sem saldo reportado."
            to="/balance"
          />
          <PillarPreview
            kicker="Disputas"
            value="—"
            unit="via ↗"
            rows={[
              {
                key: "disputes-note",
                title: "Disputas & prazos",
                sub: "Sem leitura no adapter — abra no Stripe nativo.",
                tagLabel: "deep-link",
                tagTone: "neutral"
              }
            ]}
            emptyLabel="Sem leitura de disputas."
            to="/disputes"
          />
          <PillarPreview
            kicker="Reconciliação"
            value={charges.items.length}
            unit={charges.items.length === 1 ? "cobrança recente" : "cobranças recentes"}
            rows={[
              {
                key: "recon-note",
                title: "Cobranças recentes",
                sub: "Refs opacas (id, payment_intent) — sem dados de cliente."
              }
            ]}
            emptyLabel="Sem cobranças recentes."
            to="/reconciliation"
          />
        </div>
      </section>
    </div>
  );
}
