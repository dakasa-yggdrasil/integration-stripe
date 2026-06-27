import { useMemo, useState, type CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  usePaymentIntents,
  useStripeBase,
  paymentIntentsHref,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import type { PaymentIntentItem } from "../data";
import { PaymentIntentsTable } from "./payment-intents-parts";
import { FilterPills } from "./shared/FilterPills";
import type { FilterOption } from "./shared/FilterPills";
import { kpiDelta, kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .st-pi-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .st-pi-kpis { grid-template-columns: 1fr; } }
`;

const FILTER_LABEL: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.1em",
  textTransform: "uppercase",
  color: "var(--mut)",
  marginBottom: "var(--sp-2)",
  display: "block"
};

const NOTE: CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  gap: "var(--sp-3)",
  padding: "var(--sp-3) var(--sp-4)",
  background: "var(--sand2)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)"
};

/** Count PIs whose status (case-insensitive) is in the given set. */
function countStatus(items: PaymentIntentItem[], set: Set<string>): number {
  return items.filter((p) => set.has(p.status.trim().toLowerCase())).length;
}

const NEEDS_ACTION = new Set(["requires_payment_method", "requires_action", "requires_confirmation"]);

export function PaymentIntents() {
  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();

  const pis = usePaymentIntents(instanceId, 50);

  const [statusFilter, setStatusFilter] = useState<string | null>(null);

  const total = pis.items.length;
  const succeeded = countStatus(pis.items, new Set(["succeeded"]));
  const needsAction = countStatus(pis.items, NEEDS_ACTION);

  const statuses = useMemo(
    () =>
      Array.from(new Set(pis.items.map((p) => p.status.trim()).filter((s) => s !== ""))).sort((a, b) =>
        a.localeCompare(b)
      ),
    [pis.items]
  );

  const filtered = useMemo<PaymentIntentItem[]>(() => {
    if (statusFilter === null) return pis.items;
    return pis.items.filter((p) => p.status.trim() === statusFilter);
  }, [pis.items, statusFilter]);

  const statusOptions: FilterOption[] = [
    { value: null, label: "Todos os status", count: total },
    ...statuses.map((s) => ({
      value: s,
      label: s,
      count: pis.items.filter((x) => x.status.trim() === s).length
    }))
  ];

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="st-pi-kpis">
        <KpiTile eyebrow="Total" value={total} chart={kpiSubtext("refs, sem dados de cliente", false)} />
        <KpiTile eyebrow="Aprovados" value={succeeded} chart={kpiSubtext("succeeded na janela", false)} />
        <KpiTile
          eyebrow="Aguardando ação"
          value={needsAction}
          delta={kpiDelta("requires_*", needsAction > 0)}
          chart={kpiSubtext("nenhum", needsAction > 0)}
        />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || pis.isLoading) {
      return <LoadingState label="Lendo os payment intents…" />;
    }
    if (pis.isError) {
      return (
        <EmptyState
          title="Não consegui ler os payment intents"
          description={pis.error instanceof Error ? pis.error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (total === 0) {
      return (
        <EmptyState
          title="Nenhum payment intent"
          description="A conta ainda não expõe payment intents visíveis para este token."
        />
      );
    }

    const dashHref = paymentIntentsHref(stripeBase);

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* rule-#0 reminder */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Só a ref opaca (<code>pi_…</code>) — status, valor, captura. Sem dados de pagador. Detalhe via{" "}
          <strong>↗</strong> no Stripe.
        </p>

        <div style={NOTE}>
          <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
            ◦
          </span>
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
            <code>requires_*</code>: parado esperando o pagador. <code>requires_capture</code>: autorizado, não capturado.
            Capturar/cancelar no Stripe (<strong>↗</strong>).
          </span>
        </div>

        {/* status filter */}
        <section>
          <span style={FILTER_LABEL}>Status</span>
          <FilterPills
            ariaLabel="Filtrar por status"
            options={statusOptions}
            selected={statusFilter}
            onSelect={setStatusFilter}
          />
        </section>

        {/* payment intents table */}
        {filtered.length === 0 ? (
          <EmptyState title="Nenhum payment intent com esse status" description="Escolha outro status para ver mais." />
        ) : (
          <PaymentIntentsTable paymentIntents={filtered} stripeBase={stripeBase} />
        )}

        {/* deep-link to native payments */}
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
              Pagamentos no Stripe <span aria-hidden="true">↗</span>
            </a>
          ) : (
            <span
              title="Link para o Stripe indisponível."
              style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
            >
              Pagamentos no Stripe <span aria-hidden="true">↗</span>
            </span>
          )}
        </div>
      </div>
    );
  }

  const chromeBusy = instanceLoading || pis.isLoading || pis.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Payment Intents"
        subtitle="Status, valor e captura — só refs opacas."
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
