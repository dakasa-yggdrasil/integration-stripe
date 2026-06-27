import { Pill } from "@dakasa-yggdrasil/surface-toolkit";
import type { ChargeItem } from "../../data";
import { formatMoney, paymentIntentHref } from "../../data";
import { DeepLinkArrow } from "../shared/DeepLinkArrow";
import { StatusDot, chargeStatusTone } from "../shared/StatusDot";
import { formatCreated, relativeCreated } from "../shared/time";

export interface ChargeTableProps {
  charges: ChargeItem[];
  /** Native-Stripe host for the "↗" deep-links ("" → disabled, honest). */
  stripeBase: string;
}

// One scoped stylesheet: row hover lifts warm + reveals the "↗"; container-query
// keeps the layout from collapsing on narrow hosts (the wrapper scrolls).
//
// RULE #0: there is NO customer column. The only customer-linked data shown are
// the opaque refs (charge id, payment_intent) — never a name or email. The
// adapter omits them; this table never reintroduces them.
const TABLE_CSS = `
  .st-ch-table { container-type: inline-size; width: 100%; }
  .st-ch-scroll { overflow-x: auto; }
  .st-ch-grid { width: 100%; min-width: 760px; border-collapse: separate; border-spacing: 0; }
  .st-ch-grid th {
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
  .st-ch-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .st-ch-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
    white-space: nowrap;
  }
  .st-ch-mono { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
  .st-ch-row { transition: background 100ms ease; }
  .st-ch-row:hover { background: var(--sand); }
  .st-ch-row:hover .st-ch-arrow { color: var(--honey); }
  .st-ch-row:hover .st-ch-id { color: var(--honey); }
`;

/**
 * The recent-charges roster — reconciliation context, refs only. Columns: the
 * charge id (mono opaque ref), the amount formatted to its currency, the
 * currency code, a status dot (succeeded/pending/failed, read from the field), a
 * "estornada" pill when refunded, the created timestamp, and the payment_intent
 * (mono opaque ref). The row "↗" deep-links to the payment in native Stripe.
 * There is intentionally NO customer column (rule #0).
 */
export function ChargeTable({ charges, stripeBase }: ChargeTableProps) {
  const rows = [...charges].sort((a, b) => b.created - a.created);

  return (
    <div className="st-ch-table">
      <style>{TABLE_CSS}</style>
      <div className="st-ch-scroll">
        <table className="st-ch-grid">
          <thead>
            <tr>
              <th>Charge</th>
              <th style={{ textAlign: "right" }}>Valor</th>
              <th>Moeda</th>
              <th>Status</th>
              <th>Criada</th>
              <th>Payment intent</th>
              <th aria-label="Abrir no Stripe" />
            </tr>
          </thead>
          <tbody>
            {rows.map((c) => (
              <tr key={c.id} className="st-ch-row">
                <td>
                  <span
                    className="st-ch-id st-ch-mono"
                    style={{ fontWeight: 600, color: "var(--ink)", transition: "color 100ms ease" }}
                    title={c.id}
                  >
                    {c.id}
                  </span>
                </td>
                <td className="amount">{formatMoney(c.amount, c.currency)}</td>
                <td style={{ textTransform: "uppercase", color: "var(--mut)" }}>{c.currency || "—"}</td>
                <td>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)" }}>
                    <StatusDot tone={chargeStatusTone(c.status)} label={c.status || "—"} />
                    {c.refunded ? <Pill label="estornada" tone="warn" preserveCase /> : null}
                  </span>
                </td>
                <td>
                  <span title={relativeCreated(c.created)} style={{ color: "var(--mut)" }}>
                    {formatCreated(c.created)}
                  </span>
                </td>
                <td>
                  {c.paymentIntent ? (
                    <span className="st-ch-mono" title={c.paymentIntent}>
                      {c.paymentIntent}
                    </span>
                  ) : (
                    <span style={{ color: "var(--mut)" }}>—</span>
                  )}
                </td>
                <td style={{ textAlign: "right", width: "1.8em" }}>
                  <DeepLinkArrow
                    className="st-ch-arrow"
                    href={paymentIntentHref(stripeBase, c.paymentIntent)}
                    label={`Abrir ${c.paymentIntent || c.id} no Stripe`}
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
