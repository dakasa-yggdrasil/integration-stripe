import type { CSSProperties } from "react";
import type { BalanceAmount } from "../../data";
import { formatMoney } from "../../data";

export interface BalanceTableProps {
  available: BalanceAmount[];
  pending: BalanceAmount[];
}

const TABLE_CSS = `
  .st-bal-table { container-type: inline-size; width: 100%; }
  .st-bal-scroll { overflow-x: auto; }
  .st-bal-grid { width: 100%; min-width: 420px; border-collapse: separate; border-spacing: 0; }
  .st-bal-grid th {
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
  .st-bal-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-md);
    color: var(--body);
  }
  .st-bal-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
  }
`;

const SUBHEAD: CSSProperties = {
  fontSize: "var(--fs-xs)",
  fontWeight: 700,
  letterSpacing: "0.1em",
  textTransform: "uppercase",
  color: "var(--honey)"
};

// Merge available + pending into one row per currency (the two arrays Stripe
// returns are independent — a currency may appear in either).
function byCurrency(available: BalanceAmount[], pending: BalanceAmount[]) {
  const map = new Map<string, { available: number; pending: number }>();
  for (const a of available) {
    const c = a.currency.trim().toLowerCase();
    const cur = map.get(c) ?? { available: 0, pending: 0 };
    cur.available += a.amount;
    map.set(c, cur);
  }
  for (const p of pending) {
    const c = p.currency.trim().toLowerCase();
    const cur = map.get(c) ?? { available: 0, pending: 0 };
    cur.pending += p.amount;
    map.set(c, cur);
  }
  return Array.from(map.entries()).sort((a, b) => a[0].localeCompare(b[0]));
}

/**
 * The balance snapshot, one row per currency: available + pending, formatted to
 * the currency from Stripe's smallest-unit integers (BRL/USD shown correctly,
 * never raw cents). No money is moved here — this is a read.
 */
export function BalanceTable({ available, pending }: BalanceTableProps) {
  const rows = byCurrency(available, pending);
  if (rows.length === 0) {
    return <p style={{ margin: 0, color: "var(--mut)", fontSize: "var(--fs-sm)" }}>Sem saldo reportado.</p>;
  }
  return (
    <div className="st-bal-table">
      <style>{TABLE_CSS}</style>
      <div className="st-bal-scroll">
        <table className="st-bal-grid">
          <thead>
            <tr>
              <th>Moeda</th>
              <th style={{ textAlign: "right" }}>
                <span style={SUBHEAD}>Disponível</span>
              </th>
              <th style={{ textAlign: "right" }}>
                <span style={SUBHEAD}>Pendente</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map(([currency, v]) => (
              <tr key={currency}>
                <td style={{ fontWeight: 600, color: "var(--ink)", textTransform: "uppercase" }}>{currency}</td>
                <td className="amount">{formatMoney(v.available, currency)}</td>
                <td className="amount" style={{ color: "var(--mut)" }}>
                  {formatMoney(v.pending, currency)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
