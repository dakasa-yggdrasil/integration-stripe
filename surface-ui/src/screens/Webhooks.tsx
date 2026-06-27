import type { CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useWebhookEndpoints,
  useStripeBase,
  webhooksHref,
  isEndpointDisabled,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import { WebhookTable } from "./webhooks-parts";
import { kpiDelta, kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .st-wh-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .st-wh-kpis { grid-template-columns: 1fr; } }
`;

const NOTE: CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  gap: "var(--sp-3)",
  padding: "var(--sp-3) var(--sp-4)",
  background: "var(--sand2)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)"
};

export function Webhooks() {
  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();

  const webhooks = useWebhookEndpoints(instanceId);

  const total = webhooks.items.length;
  const disabled = webhooks.items.filter(isEndpointDisabled).length;
  const enabled = total - disabled;
  // Distinct event types subscribed across all endpoints (wildcard counts as 1).
  const distinctEvents = new Set<string>();
  webhooks.items.forEach((e) => e.enabledEvents.forEach((ev) => distinctEvents.add(ev)));

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="st-wh-kpis">
        <KpiTile
          eyebrow="Endpoints ativos"
          value={enabled}
          delta={kpiDelta(`${disabled} desativado(s)`, disabled > 0)}
          chart={kpiSubtext(`de ${total}`, disabled > 0)}
        />
        <KpiTile
          eyebrow="Desativados"
          value={disabled}
          delta={kpiDelta("não entregando", disabled > 0)}
          chart={kpiSubtext("nenhum", disabled > 0)}
        />
        <KpiTile
          eyebrow="Tipos de evento"
          value={distinctEvents.size}
          chart={kpiSubtext("assinados", false)}
        />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || webhooks.isLoading) {
      return <LoadingState label="Lendo os webhook endpoints…" />;
    }
    if (webhooks.isError) {
      return (
        <EmptyState
          title="Não consegui ler os webhooks"
          description={
            webhooks.error instanceof Error ? webhooks.error.message : "Tente novamente em instantes."
          }
        />
      );
    }
    if (total === 0) {
      return (
        <EmptyState
          title="Nenhum webhook endpoint"
          description="Esta conta Stripe ainda não tem endpoints configurados para este token."
        />
      );
    }
    const dashHref = webhooksHref(stripeBase);
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* verify-signature diagnostic note */}
        <div style={NOTE}>
          <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
            ◦
          </span>
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
            Endpoint <strong>desativado</strong> = Stripe para de entregar; eventos se acumulam. Histórico de entregas
            no Stripe (<strong>↗</strong>).
          </span>
        </div>

        <WebhookTable endpoints={webhooks.items} stripeBase={stripeBase} />

        {/* deep-link to native webhooks settings */}
        <div>
          {dashHref ? (
            <a
              href={dashHref}
              target="_blank"
              rel="noreferrer"
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: "var(--sp-2)",
                fontSize: "var(--fs-sm)",
                fontWeight: 700,
                color: "var(--honey)",
                textDecoration: "none"
              }}
            >
              Webhooks no Stripe <span aria-hidden="true">↗</span>
            </a>
          ) : (
            <span
              title="Link para o Stripe indisponível."
              style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
            >
              Webhooks no Stripe <span aria-hidden="true">↗</span>
            </span>
          )}
        </div>
      </div>
    );
  }

  const chromeBusy = instanceLoading || webhooks.isLoading || webhooks.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Webhook Health"
        subtitle="Endpoints, entrega e eventos assinados."
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
