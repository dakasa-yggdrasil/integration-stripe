import type { RefundItem } from "../../data";
import { formatMoney } from "../../data";
import { formatCreated, relativeCreated } from "../shared/time";

export interface RefundsTableProps {
  refunds: RefundItem[];
  /** The charge's currency — refunds share it (smallest-unit amounts). */
  currency: string;
}

// One scoped stylesheet, matching the charge/PI tables: row hover lifts warm;
// container-query keeps the layout from collapsing on narrow hosts.
//
// RULE #0: a refund is money already moved — read-only history. Only the opaque
// refund id + amount/reason/created are shown; never any customer ref.
const TABLE_CSS = `
  .st-rf-table { container-type: inline-size; width: 100%; }
  .st-rf-scroll { overflow-x: auto; }
  .st-rf-grid { width: 100%; min-width: 560px; border-collapse: separate; border-spacing: 0; }
  .st-rf-grid th {
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
  .st-rf-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .st-rf-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
    white-space: nowrap;
  }
  .st-rf-mono { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
  .st-rf-row:hover { background: var(--sand); }
`;

/** Pretty-print a Stripe refund reason ("requested_by_customer" → readable). */
function refundReason(reason: string): string {
  const r = reason.trim().toLowerCase();
  switch (r) {
    case "requested_by_customer":
      return "solicitado pelo cliente";
    case "fraudulent":
      return "fraude";
    case "duplicate":
      return "duplicado";
    case "":
      return "—";
    default:
      return reason;
  }
}

/**
 * The refunds roster inside the charge drill-down — money-already-moved history,
 * newest first. Columns: the refund id (mono opaque ref), the amount formatted
 * to the charge's currency, the reason, and the created timestamp. Refs only
 * (rule #0).
 */
export function RefundsTable({ refunds, currency }: RefundsTableProps) {
  const rows = [...refunds].sort((a, b) => b.created - a.created);

  return (
    <div className="st-rf-table">
      <style>{TABLE_CSS}</style>
      <div className="st-rf-scroll">
        <table className="st-rf-grid">
          <thead>
            <tr>
              <th>Estorno</th>
              <th style={{ textAlign: "right" }}>Valor</th>
              <th>Motivo</th>
              <th>Criado</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id} className="st-rf-row">
                <td>
                  <span className="st-rf-mono" style={{ color: "var(--ink)", fontWeight: 600 }} title={r.id}>
                    {r.id}
                  </span>
                </td>
                <td className="amount">{formatMoney(r.amount, currency)}</td>
                <td style={{ color: "var(--mut)" }}>{refundReason(r.reason)}</td>
                <td>
                  <span title={relativeCreated(r.created)} style={{ color: "var(--mut)" }}>
                    {formatCreated(r.created)}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
