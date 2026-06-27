import { Pill } from "@dakasa-yggdrasil/surface-toolkit";
import type { PaymentIntentItem } from "../../data";
import { formatMoney, paymentIntentHref } from "../../data";
import { DeepLinkArrow } from "../shared/DeepLinkArrow";
import { StatusDot, paymentIntentStatusTone } from "../shared/StatusDot";
import { formatCreated, relativeCreated } from "../shared/time";

export interface PaymentIntentsTableProps {
  paymentIntents: PaymentIntentItem[];
  /** Native-Stripe host for the "↗" deep-links ("" → disabled, honest). */
  stripeBase: string;
}

// One scoped stylesheet: row hover lifts warm + reveals the "↗"; container-query
// keeps the layout from collapsing on narrow hosts (the wrapper scrolls).
//
// RULE #0: there is NO customer column. The only ref shown is the opaque PI id;
// the adapter projects no customer data, and this table never reintroduces it.
const TABLE_CSS = `
  .st-pi-table { container-type: inline-size; width: 100%; }
  .st-pi-scroll { overflow-x: auto; }
  .st-pi-grid { width: 100%; min-width: 760px; border-collapse: separate; border-spacing: 0; }
  .st-pi-grid th {
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
  .st-pi-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .st-pi-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
    white-space: nowrap;
  }
  .st-pi-mono { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
  .st-pi-row { transition: background 100ms ease; }
  .st-pi-row:hover { background: var(--sand); }
  .st-pi-row:hover .st-pi-arrow { color: var(--honey); }
  .st-pi-row:hover .st-pi-id { color: var(--honey); }
`;

/**
 * The PaymentIntents roster — opaque refs + status/amount facts only. Columns:
 * the PI id (mono opaque ref), a status dot (read from the field), the amount
 * formatted to its currency, the currency code, the created timestamp, and the
 * capture method (a "manual" pill when capture is manual — the readable "ainda
 * por capturar" hint). The row "↗" deep-links to the payment in native Stripe.
 * There is intentionally NO customer column (rule #0).
 */
export function PaymentIntentsTable({ paymentIntents, stripeBase }: PaymentIntentsTableProps) {
  const rows = [...paymentIntents].sort((a, b) => b.created - a.created);

  return (
    <div className="st-pi-table">
      <style>{TABLE_CSS}</style>
      <div className="st-pi-scroll">
        <table className="st-pi-grid">
          <thead>
            <tr>
              <th>Payment intent</th>
              <th>Status</th>
              <th style={{ textAlign: "right" }}>Valor</th>
              <th>Moeda</th>
              <th>Criado</th>
              <th>Captura</th>
              <th aria-label="Abrir no Stripe" />
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => {
              const manual = p.captureMethod.trim().toLowerCase() === "manual";
              return (
                <tr key={p.id} className="st-pi-row">
                  <td>
                    <span
                      className="st-pi-id st-pi-mono"
                      style={{ fontWeight: 600, color: "var(--ink)", transition: "color 100ms ease" }}
                      title={p.id}
                    >
                      {p.id}
                    </span>
                  </td>
                  <td>
                    <StatusDot tone={paymentIntentStatusTone(p.status)} label={p.status || "—"} />
                  </td>
                  <td className="amount">{formatMoney(p.amount, p.currency)}</td>
                  <td style={{ textTransform: "uppercase", color: "var(--mut)" }}>{p.currency || "—"}</td>
                  <td>
                    <span title={relativeCreated(p.created)} style={{ color: "var(--mut)" }}>
                      {formatCreated(p.created)}
                    </span>
                  </td>
                  <td>
                    {p.captureMethod ? (
                      manual ? (
                        <Pill label="manual" tone="warn" preserveCase />
                      ) : (
                        <span style={{ color: "var(--mut)" }}>{p.captureMethod}</span>
                      )
                    ) : (
                      <span style={{ color: "var(--mut)" }}>—</span>
                    )}
                  </td>
                  <td style={{ textAlign: "right", width: "1.8em" }}>
                    <DeepLinkArrow
                      className="st-pi-arrow"
                      href={paymentIntentHref(stripeBase, p.id)}
                      label={`Abrir ${p.id} no Stripe`}
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
