import { Pill } from "@dakasa-yggdrasil/surface-toolkit";
import type { SubscriptionItem } from "../../data";
import { formatMoney, subscriptionHref } from "../../data";
import { DeepLinkArrow } from "../shared/DeepLinkArrow";
import { StatusDot, subscriptionStatusTone } from "../shared/StatusDot";
import { formatDate, relativeWhen } from "../shared/time";

export interface SubscriptionsTableProps {
  subscriptions: SubscriptionItem[];
  /** Native-Stripe host for the "↗" deep-links ("" → disabled, honest). */
  stripeBase: string;
}

// One scoped stylesheet: row hover lifts warm + reveals the "↗"; container-query
// keeps the layout from collapsing on narrow hosts (the wrapper scrolls).
//
// RULE #0: there is NO customer-name column. The only customer-linked datum is
// the OPAQUE customer ref (cus_…), shown mono — never a name or email. The
// adapter omits identities; this table never reintroduces them.
const TABLE_CSS = `
  .st-sub-table { container-type: inline-size; width: 100%; }
  .st-sub-scroll { overflow-x: auto; }
  .st-sub-grid { width: 100%; min-width: 820px; border-collapse: separate; border-spacing: 0; }
  .st-sub-grid th {
    text-align: left;
    font-family: var(--font-body);
    font-size: var(--fs-xs);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--mut);
    padding: var(--sp-2) var(--sp-3);
    border-bottom: 1px solid var(--line);
    white-space: nowrap;
  }
  .st-sub-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .st-sub-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
    white-space: nowrap;
  }
  .st-sub-mono { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
  .st-sub-row { transition: background 100ms ease; }
  .st-sub-row:hover { background: var(--sand); }
  .st-sub-row:hover .st-sub-arrow { color: var(--honey); }
  .st-sub-row:hover .st-sub-id { color: var(--honey); }
`;

/**
 * The subscriptions roster — config-grade refs only. Columns: the subscription
 * id (mono opaque ref), a status dot (read from the field), the plan (nickname +
 * price id), the recurring amount formatted to its currency, the current
 * period-end date (+ relative hint), and a "encerra no período" pill when
 * cancel_at_period_end is set. The row "↗" deep-links to the subscription in
 * native Stripe. There is intentionally NO customer-name column (rule #0).
 */
export function SubscriptionsTable({ subscriptions, stripeBase }: SubscriptionsTableProps) {
  const rows = [...subscriptions].sort((a, b) => a.currentPeriodEnd - b.currentPeriodEnd);

  return (
    <div className="st-sub-table">
      <style>{TABLE_CSS}</style>
      <div className="st-sub-scroll">
        <table className="st-sub-grid">
          <thead>
            <tr>
              <th>Assinatura</th>
              <th>Status</th>
              <th>Plano</th>
              <th style={{ textAlign: "right" }}>Valor</th>
              <th>Renova</th>
              <th aria-label="Abrir no Stripe" />
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr key={s.id} className="st-sub-row">
                <td>
                  <span
                    className="st-sub-id st-sub-mono"
                    style={{ fontWeight: 600, color: "var(--ink)", transition: "color 100ms ease" }}
                    title={s.id}
                  >
                    {s.id}
                  </span>
                </td>
                <td>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)" }}>
                    <StatusDot tone={subscriptionStatusTone(s.status)} label={s.status || "—"} />
                    {s.cancelAtPeriodEnd ? <Pill label="encerra no período" tone="warn" preserveCase /> : null}
                  </span>
                </td>
                <td>
                  <span style={{ display: "flex", flexDirection: "column", minWidth: 0 }}>
                    <span
                      style={{
                        color: "var(--ink)",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap"
                      }}
                      title={s.planNickname || s.planPriceId}
                    >
                      {s.planNickname || "—"}
                    </span>
                    {s.planPriceId ? (
                      <span className="st-sub-mono" style={{ fontSize: "var(--fs-xs)" }} title={s.planPriceId}>
                        {s.planPriceId}
                      </span>
                    ) : null}
                  </span>
                </td>
                <td className="amount">{formatMoney(s.amount, s.currency)}</td>
                <td>
                  {s.currentPeriodEnd > 0 ? (
                    <span style={{ display: "flex", flexDirection: "column", minWidth: 0 }}>
                      <span style={{ color: "var(--body)" }}>{formatDate(s.currentPeriodEnd)}</span>
                      <span style={{ fontSize: "var(--fs-xs)", color: "var(--mut)" }}>
                        {relativeWhen(s.currentPeriodEnd)}
                      </span>
                    </span>
                  ) : (
                    <span style={{ color: "var(--mut)" }}>—</span>
                  )}
                </td>
                <td style={{ textAlign: "right", width: "1.8em" }}>
                  <DeepLinkArrow
                    className="st-sub-arrow"
                    href={subscriptionHref(stripeBase, s.id)}
                    label={`Abrir ${s.id} no Stripe`}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
