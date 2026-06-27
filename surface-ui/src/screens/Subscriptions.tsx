import { useMemo, useState, type CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useSubscriptions,
  useStripeBase,
  subscriptionsHref,
  isCancelAtPeriodEnd,
  isSubscriptionActive,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import type { SubscriptionItem } from "../data";
import { SubscriptionsTable } from "./subscriptions-parts";
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
  .st-su-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .st-su-kpis { grid-template-columns: 1fr; } }
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

export function Subscriptions() {
  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();

  const subs = useSubscriptions(instanceId, 50);

  const [statusFilter, setStatusFilter] = useState<string | null>(null);

  const total = subs.items.length;
  const activeCount = subs.items.filter(isSubscriptionActive).length;
  const cancelingCount = subs.items.filter(isCancelAtPeriodEnd).length;

  const statuses = useMemo(
    () =>
      Array.from(new Set(subs.items.map((s) => s.status.trim()).filter((s) => s !== ""))).sort((a, b) =>
        a.localeCompare(b)
      ),
    [subs.items]
  );

  const filtered = useMemo<SubscriptionItem[]>(() => {
    if (statusFilter === null) return subs.items;
    return subs.items.filter((s) => s.status.trim() === statusFilter);
  }, [subs.items, statusFilter]);

  const statusOptions: FilterOption[] = [
    { value: null, label: "Todos os status", count: total },
    ...statuses.map((s) => ({
      value: s,
      label: s,
      count: subs.items.filter((x) => x.status.trim() === s).length
    }))
  ];

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="st-su-kpis">
        <KpiTile eyebrow="Ativas" value={activeCount} chart={kpiSubtext(`de ${total}`, false)} />
        <KpiTile
          eyebrow="A cancelar"
          value={cancelingCount}
          delta={kpiDelta("encerram no período", cancelingCount > 0)}
          chart={kpiSubtext("nenhuma", cancelingCount > 0)}
        />
        <KpiTile eyebrow="Total" value={total} chart={kpiSubtext("refs, sem dados de cliente", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || subs.isLoading) {
      return <LoadingState label="Lendo as assinaturas…" />;
    }
    if (subs.isError) {
      return (
        <EmptyState
          title="Não consegui ler as assinaturas"
          description={subs.error instanceof Error ? subs.error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (total === 0) {
      return (
        <EmptyState
          title="Nenhuma assinatura"
          description="A conta ainda não expõe assinaturas visíveis para este token."
        />
      );
    }

    const dashHref = subscriptionsHref(stripeBase);

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* rule-#0 reminder */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Visão de <strong>ops de pagamentos</strong>: só referências opacas (<code>sub_…</code>, <code>price_…</code>,{" "}
          <code>cus_…</code>) — <strong>sem coluna de cliente</strong>, sem nome ou e-mail. Para gerenciar uma
          assinatura, cada <strong>↗</strong> abre o registro no Stripe.
        </p>

        <div style={NOTE}>
          <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
            ◦
          </span>
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
            Uma assinatura marcada <strong>encerra no período</strong> não renova ao fim do ciclo atual — o{" "}
            <em>cancel_at_period_end</em> está ligado. Cobrança, alteração de plano e cancelamento são movimentação de
            conta e ficam no Stripe nativo (<strong>↗</strong>); esta surface é leitura.
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

        {/* subscriptions table */}
        {filtered.length === 0 ? (
          <EmptyState title="Nenhuma assinatura com esse status" description="Escolha outro status para ver mais." />
        ) : (
          <SubscriptionsTable subscriptions={filtered} stripeBase={stripeBase} />
        )}

        {/* deep-link to native subscriptions */}
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
              Assinaturas no Stripe <span aria-hidden="true">↗</span>
            </a>
          ) : (
            <span
              title="Link para o Stripe nativo indisponível: o host do dashboard ainda não é exposto por um surface read."
              style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
            >
              Assinaturas no Stripe <span aria-hidden="true">↗</span>
            </span>
          )}
        </div>
      </div>
    );
  }

  const chromeBusy = instanceLoading || subs.isLoading || subs.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Assinaturas"
        subtitle="As assinaturas recorrentes da conta — status, plano, valor e renovação, por referência opaca. Sem dados de cliente; gerir é no Stripe (↗)."
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
