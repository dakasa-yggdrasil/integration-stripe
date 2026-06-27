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
  useSubscriptions,
  usePaymentIntents,
  primaryBrl,
  formatMoneyCompact,
  formatMoney,
  isCancelAtPeriodEnd,
  isSubscriptionActive,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID,
  MOCK_INSTANCE_LABEL
} from "../data";
import { KpiStrip, AttentionBand, NavGroups } from "./home-parts";
import type { NavGroupSpec } from "./home-parts";

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

// Narrow-host header reflow (by host width, not viewport).
const HEADER_REFLOW = `
  @container (max-width: 560px) {
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
  const charges = useCharges(instanceId, 50);
  const subs = useSubscriptions(instanceId, 50);
  const pis = usePaymentIntents(instanceId, 50);

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
  const esteiraHealthy = pulse.webhooksDisabled === 0;

  // Per-destination signals for the grouped nav (hard numbers + bad-signal pills).
  const cancelingSubs = subs.items.filter(isCancelAtPeriodEnd).length;
  const activeSubs = subs.items.filter(isSubscriptionActive).length;
  const needsActionPis = pis.items.filter((p) => {
    const st = p.status.trim().toLowerCase();
    return st === "requires_payment_method" || st === "requires_action" || st === "requires_confirmation";
  }).length;
  const refundedCharges = charges.items.filter((c) => c.refunded).length;
  const failedCharges = charges.items.filter((c) => c.status.trim().toLowerCase() === "failed").length;

  // The identity line: what the console IS — a payments-OPS view, with the hard
  // rule that money-movement and per-customer data are never here.
  const identityLine = [
    "Webhooks · conciliação · saldo · assinaturas · payment intents · disputas",
    "ops de pagamentos — sem dados de cliente, sem mover dinheiro"
  ].join(" · ");

  // Grouped nav — a calm, scannable index over every detail page, organized by
  // function (Ingestão / Dinheiro / Disputas). Each card carries a hard number
  // (or honest "—" for an un-readable fact); bad signals (webhook desativado,
  // assinatura a encerrar, PI aguardando ação, cobrança estornada/falha) surface
  // a status pill so the operator's eye lands on them first.
  const navGroups: NavGroupSpec[] = [
    {
      key: "ingestao",
      title: "Ingestão",
      cards: [
        {
          key: "webhooks",
          label: "Webhook Health",
          value: pulse.webhooksEnabled,
          unit: pulse.webhooks === 1 ? "endpoint" : `de ${pulse.webhooks}`,
          to: "/webhooks",
          tagLabel: pulse.webhooksDisabled > 0 ? `${pulse.webhooksDisabled} desativado(s)` : undefined,
          tagTone: "crit"
        },
        {
          key: "reconciliation",
          label: "Reconciliação",
          value: charges.items.length,
          unit: charges.items.length === 1 ? "cobrança" : "cobranças",
          to: "/reconciliation",
          tagLabel:
            failedCharges > 0
              ? `${failedCharges} falha(s)`
              : refundedCharges > 0
                ? `${refundedCharges} estorno(s)`
                : undefined,
          tagTone: failedCharges > 0 ? "crit" : "warn"
        }
      ]
    },
    {
      key: "dinheiro",
      title: "Dinheiro",
      cards: [
        {
          key: "balance",
          label: "Saldo & Payouts",
          value: formatMoneyCompact(availableBrl, "brl"),
          unit: "disponível",
          to: "/balance"
        },
        {
          key: "subscriptions",
          label: "Assinaturas",
          value: activeSubs,
          unit: subs.items.length === activeSubs ? "ativas" : `de ${subs.items.length}`,
          to: "/subscriptions",
          tagLabel: cancelingSubs > 0 ? `${cancelingSubs} a encerrar` : undefined,
          tagTone: "warn"
        },
        {
          key: "payment-intents",
          label: "Payment Intents",
          value: pis.items.length,
          unit: pis.items.length === 1 ? "intent" : "intents",
          to: "/payment-intents",
          tagLabel: needsActionPis > 0 ? `${needsActionPis} aguardando` : undefined,
          tagTone: "warn"
        }
      ]
    },
    {
      key: "disputas",
      title: "Disputas",
      cards: [
        {
          key: "disputes",
          label: "Disputas",
          value: "—",
          unit: "via ↗",
          to: "/disputes",
          tagLabel: "deep-link",
          tagTone: "neutral"
        }
      ]
    }
  ];

  return (
    <div className="atelier" style={PAGE}>
      <style>{HEADER_REFLOW}</style>

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

      {/* ---------- grouped navigation (every detail page, hard numbers) ---------- */}
      <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-4)" }}>
        <h2 style={SECTION_TITLE}>Navegação</h2>
        <NavGroups groups={navGroups} />
      </section>
    </div>
  );
}
