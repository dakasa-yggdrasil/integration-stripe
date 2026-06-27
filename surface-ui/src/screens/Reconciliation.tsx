import { useMemo, useState, type CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useCollaboratorScope,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useCharges,
  useStripeBase,
  primaryBrl,
  formatMoney,
  paymentsHref,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID
} from "../data";
import type { ChargeItem } from "../data";
import { ChargeTable } from "./reconciliation-parts";
import { GatedAction } from "./shared/GatedAction";
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
  .st-rc-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .st-rc-kpis { grid-template-columns: 1fr; } }
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

export function Reconciliation() {
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const scope = mock ? mockCollaboratorScope() : liveScope;
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();

  const charges = useCharges(instanceId, 50);

  const [statusFilter, setStatusFilter] = useState<string | null>(null);

  const total = charges.items.length;
  const refundedCount = charges.items.filter((c) => c.refunded).length;
  const succeededBrl = primaryBrl(
    charges.items
      .filter((c) => c.status.trim().toLowerCase() === "succeeded" && c.currency.trim().toLowerCase() === "brl")
      .map((c) => ({ amount: c.amount, currency: c.currency }))
  );

  const statuses = useMemo(
    () =>
      Array.from(new Set(charges.items.map((c) => c.status.trim()).filter((s) => s !== ""))).sort((a, b) =>
        a.localeCompare(b)
      ),
    [charges.items]
  );

  const filtered = useMemo<ChargeItem[]>(() => {
    if (statusFilter === null) return charges.items;
    return charges.items.filter((c) => c.status.trim() === statusFilter);
  }, [charges.items, statusFilter]);

  const statusOptions: FilterOption[] = [
    { value: null, label: "Todos os status", count: total },
    ...statuses.map((s) => ({
      value: s,
      label: s,
      count: charges.items.filter((c) => c.status.trim() === s).length
    }))
  ];

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="st-rc-kpis">
        <KpiTile eyebrow="Cobranças recentes" value={total} chart={kpiSubtext("refs, sem dados de cliente", false)} />
        <KpiTile
          eyebrow="Estornadas"
          value={refundedCount}
          delta={kpiDelta("revisar", refundedCount > 0)}
          chart={kpiSubtext("nenhuma", refundedCount > 0)}
        />
        <KpiTile
          eyebrow="Aprovadas (BRL)"
          value={formatMoney(succeededBrl, "brl")}
          chart={kpiSubtext("na janela exibida", false)}
        />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || charges.isLoading) {
      return <LoadingState label="Lendo as cobranças recentes…" />;
    }
    if (charges.isError) {
      return (
        <EmptyState
          title="Não consegui ler as cobranças"
          description={charges.error instanceof Error ? charges.error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (total === 0) {
      return (
        <EmptyState
          title="Nenhuma cobrança recente"
          description="A conta ainda não expõe cobranças visíveis para este token."
        />
      );
    }

    const payHref = paymentsHref(stripeBase);

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* rule-#0 reminder */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Só refs opacas (<code>id</code>, <code>payment_intent</code>) — sem dados de pagador. Detalhe via{" "}
          <strong>↗</strong> no Stripe.
        </p>

        <div style={NOTE}>
          <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
            ◦
          </span>
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
            Ledger de conciliação: em breve.
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

        {/* charges table */}
        {filtered.length === 0 ? (
          <EmptyState title="Nenhuma cobrança com esse status" description="Escolha outro status para ver mais." />
        ) : (
          <ChargeTable charges={filtered} stripeBase={stripeBase} />
        )}

        {/* deep-link to native payments */}
        <div>
          {payHref ? (
            <a
              href={payHref}
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

        {/* money-movement: create_refund — admin-tier, gated + disabled (v1) */}
        <GatedAction
          need="stripe.refunds.create"
          perms={scope.perms}
          eyebrow="Remediação"
          label="Estorno: movimentação de dinheiro, admin, fora da v1."
          hint="create_refund é admin e fora da v1."
        />
      </div>
    );
  }

  const chromeBusy = instanceLoading || charges.isLoading || charges.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Reconciliação & Refunds"
        subtitle="Cobranças recentes — só refs opacas."
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
