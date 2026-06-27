import type { CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useCollaboratorScope,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useBalance,
  useStripeBase,
  primaryBrl,
  formatMoney,
  payoutsHref,
  balanceHref,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID
} from "../data";
import { BalanceTable } from "./balance-parts";
import { GatedAction } from "./shared/GatedAction";
import { kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .st-bal-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  @container (max-width: 480px) { .st-bal-kpis { grid-template-columns: 1fr; } }
`;

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-lg)",
  fontWeight: 500,
  color: "var(--ink)"
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

export function Balance() {
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const scope = mock ? mockCollaboratorScope() : liveScope;
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("stripe");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const stripeBase = useStripeBase();

  const balance = useBalance(instanceId);

  const availBrl = primaryBrl(balance.available);
  const pendBrl = primaryBrl(balance.pending);

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="st-bal-kpis">
        <KpiTile
          eyebrow="Disponível (BRL)"
          value={formatMoney(availBrl, "brl")}
          chart={kpiSubtext("liberado p/ payout", false)}
        />
        <KpiTile
          eyebrow="Pendente (BRL)"
          value={formatMoney(pendBrl, "brl")}
          chart={kpiSubtext("a liberar", false)}
        />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || balance.isLoading) {
      return <LoadingState label="Lendo o saldo…" />;
    }
    if (balance.isError) {
      return (
        <EmptyState
          title="Não consegui ler o saldo"
          description={balance.error instanceof Error ? balance.error.message : "Tente novamente em instantes."}
        />
      );
    }
    if (balance.available.length === 0 && balance.pending.length === 0) {
      return (
        <EmptyState
          title="Sem saldo reportado"
          description="A conta ainda não expõe um saldo para este token."
        />
      );
    }

    const payHref = payoutsHref(stripeBase);
    const balHref = balanceHref(stripeBase);

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* balance per currency */}
        <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
          <h3 style={SECTION_TITLE}>Saldo por moeda</h3>
          <BalanceTable available={balance.available} pending={balance.pending} />
        </section>

        {/* payouts — deep-link out */}
        <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
          <h3 style={SECTION_TITLE}>Payouts</h3>
          <div style={NOTE}>
            <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
              ◦
            </span>
            <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
              Histórico de payouts: no Stripe (<strong>↗</strong>).
            </span>
          </div>
          <div style={{ display: "flex", gap: "var(--sp-4)", flexWrap: "wrap" }}>
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
                Payouts no Stripe <span aria-hidden="true">↗</span>
              </a>
            ) : (
              <span
                title="Link para o Stripe indisponível."
                style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
              >
                Payouts no Stripe <span aria-hidden="true">↗</span>
              </span>
            )}
            {balHref ? (
              <a
                href={balHref}
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
                Visão de saldo no Stripe <span aria-hidden="true">↗</span>
              </a>
            ) : null}
          </div>
        </section>

        {/* money-movement: create_payout — admin-tier, gated + disabled (v1) */}
        <GatedAction
          need="stripe.payouts.create"
          perms={scope.perms}
          eyebrow="Remediação"
          label="Payout manual: movimentação de dinheiro, admin, fora da v1."
          hint="create_payout é admin e fora da v1."
        />
      </div>
    );
  }

  const chromeBusy = instanceLoading || balance.isLoading || balance.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Saldo & Payouts"
        subtitle="Saldo por moeda."
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
