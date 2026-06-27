import type { CSSProperties, ReactNode } from "react";
import { useParams, useLocation, Link } from "react-router-dom";
import {
  TierTwoShell,
  KpiTile,
  Pill,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useChargeDetail,
  useStripeBase,
  formatMoney,
  chargeHref,
  paymentIntentHref,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import { RefundsTable } from "./charge-parts";
import { StatusDot, chargeStatusTone } from "./shared/StatusDot";
import { DeepLinkArrow } from "./shared/DeepLinkArrow";
import { formatCreated, relativeCreated } from "./shared/time";
import { kpiDelta, kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .st-cd-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .st-cd-kpis { grid-template-columns: 1fr; } }
`;

const BACK_LINK: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: "var(--sp-1)",
  fontSize: "var(--fs-sm)",
  fontWeight: 600,
  color: "var(--mut)",
  textDecoration: "none",
  transition: "color 100ms ease"
};

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-lg)",
  fontWeight: 500,
  color: "var(--ink)"
};

const FAILURE_CARD: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--sp-2)",
  padding: "var(--sp-4) var(--sp-5)",
  background: "var(--sand2)",
  border: "1px solid var(--crit)",
  borderRadius: "var(--r-md)"
};

const MONO: CSSProperties = { fontFamily: "var(--font-mono, var(--font-body))" };

/** A consistent section frame: heading (+count) then its body. */
function Section({ title, count, children }: { title: string; count?: number; children: ReactNode }) {
  return (
    <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: "var(--sp-2)" }}>
        <h3 style={SECTION_TITLE}>{title}</h3>
        {count !== undefined ? (
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", fontWeight: 600 }}>{count}</span>
        ) : null}
      </div>
      {children}
    </section>
  );
}

/**
 * The charge drill-down (`/charge/:id`) — opened from a charge id in the
 * Reconciliação roster. Reads `charge-detail` (param `charge_id`) and renders a
 * header (id, amount, status, created), a failure card when the charge failed
 * (Stripe's own code/message — NOT customer data), and a refunds section
 * (money-already-moved history). RULE #0: opaque refs only; no customer
 * identity anywhere. Back-link carries the query string (e.g. `?mock`) so DEV
 * review survives the round trip to/from Reconciliação.
 */
export function ChargeDetail() {
  const params = useParams<{ id: string }>();
  const chargeId = params.id ?? "";
  const { search } = useLocation();

  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();

  const { detail, isLoading, isError, error } = useChargeDetail(instanceId, chargeId);

  const backLink = (
    <Link to={`/reconciliation${search}`} style={BACK_LINK} className="st-cd-back">
      <span aria-hidden="true">←</span>
      <span>Reconciliação</span>
    </Link>
  );

  const failed = (detail?.status ?? "").trim().toLowerCase() === "failed";

  const subtitle =
    detail && !isLoading && !isError ? (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
        {backLink}
        <div style={{ display: "flex", alignItems: "center", gap: "var(--sp-3)", flexWrap: "wrap" }}>
          <StatusDot tone={chargeStatusTone(detail.status)} label={detail.status || "—"} />
          {detail.refunded ? <Pill label="estornada" tone="warn" preserveCase /> : null}
          <span style={{ color: "var(--mut)", fontSize: "var(--fs-sm)" }} title={relativeCreated(detail.created)}>
            {formatCreated(detail.created)}
          </span>
          <DeepLinkArrow href={chargeHref(stripeBase, detail.id)} label={`Abrir ${detail.id} no Stripe`} />
        </div>
      </div>
    ) : (
      backLink
    );

  const kpis =
    detail && !isLoading && !isError ? (
      <div style={{ containerType: "inline-size", width: "100%" }}>
        <style>{KPI_GRID}</style>
        <div className="st-cd-kpis">
          <KpiTile eyebrow="Valor" value={formatMoney(detail.amount, detail.currency)} chart={kpiSubtext("cobrado", false)} />
          <KpiTile
            eyebrow="Estornado"
            value={formatMoney(detail.refundedAmount, detail.currency)}
            delta={kpiDelta("revisar", detail.refundedAmount > 0)}
            chart={kpiSubtext("nenhum", detail.refundedAmount > 0)}
          />
          <KpiTile
            eyebrow="Estornos"
            value={detail.refunds.length}
            chart={kpiSubtext(detail.refunds.length === 1 ? "registro" : "registros", false)}
          />
        </div>
      </div>
    ) : undefined;

  function body() {
    if (instanceLoading || isLoading) {
      return <LoadingState label="Lendo o detalhe da cobrança…" />;
    }
    if (isError) {
      return (
        <EmptyState
          title="Não consegui ler esta cobrança"
          description={error instanceof Error ? error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (!detail) {
      return (
        <EmptyState
          title="Cobrança não encontrada"
          description={`Nenhuma cobrança "${chargeId}" visível para este token.`}
        />
      );
    }

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        <style>{".st-cd-back:hover { color: var(--honey); }"}</style>

        {/* rule-#0 reminder */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Detalhe por <strong>referência opaca</strong> (<code>{detail.id}</code>) — <strong>sem dados de cliente</strong>.
          A mensagem de falha, quando há, é o texto do próprio Stripe (não é dado de cliente). Estornar é admin e fica
          fora da v1; o estado real está no Stripe (<strong>↗</strong>).
        </p>

        {/* failure card (only when failed) */}
        {failed ? (
          <Section title="Falha">
            <div style={FAILURE_CARD}>
              <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)" }}>
                <span style={{ fontWeight: 600, color: "var(--ink)" }}>Cobrança recusada</span>
                {detail.failureCode ? <Pill label={detail.failureCode} tone="crit" preserveCase /> : null}
              </span>
              <span style={{ fontSize: "var(--fs-sm)", color: "var(--body)", lineHeight: 1.5 }}>
                {detail.failureMessage || "O Stripe não retornou uma mensagem de falha para esta cobrança."}
              </span>
            </div>
          </Section>
        ) : null}

        {/* identity refs */}
        <Section title="Referências">
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
            <RefRow label="Charge" value={detail.id} />
            <RefRow
              label="Payment intent"
              value={detail.paymentIntent}
              href={paymentIntentHref(stripeBase, detail.paymentIntent)}
            />
          </div>
        </Section>

        {/* refunds */}
        <Section title="Estornos" count={detail.refunds.length}>
          {detail.refunds.length === 0 ? (
            <EmptyState
              title="Sem estornos"
              description="Nenhum estorno registrado para esta cobrança."
            />
          ) : (
            <RefundsTable refunds={detail.refunds} currency={detail.currency} />
          )}
        </Section>
      </div>
    );
  }

  const chromeBusy = instanceLoading || isLoading || isError || !detail;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Cobrança"
        title={chargeId || "Cobrança"}
        subtitle={subtitle}
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}

// A label + opaque-ref row, with an optional "↗" out to native Stripe.
function RefRow({ label, value, href }: { label: string; value: string; href?: string }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--sp-3)",
        padding: "var(--sp-2) var(--sp-3)",
        background: "var(--sand)",
        border: "1px solid var(--line)",
        borderRadius: "var(--r-sm)",
        minWidth: 0
      }}
    >
      <span
        style={{
          fontSize: "var(--fs-xs)",
          fontWeight: 700,
          letterSpacing: "0.06em",
          textTransform: "uppercase",
          color: "var(--mut)",
          width: "8.5em",
          flex: "0 0 auto"
        }}
      >
        {label}
      </span>
      <span
        style={{ ...MONO, fontSize: "var(--fs-sm)", color: "var(--ink)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", flex: 1 }}
        title={value}
      >
        {value || "—"}
      </span>
      {value && href !== undefined ? (
        <DeepLinkArrow href={href} label={`Abrir ${value} no Stripe`} />
      ) : null}
    </div>
  );
}
