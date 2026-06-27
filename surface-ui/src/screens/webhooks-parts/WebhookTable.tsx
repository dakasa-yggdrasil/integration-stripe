import { useState } from "react";
import { Chip } from "@dakasa-yggdrasil/surface-toolkit";
import type { WebhookEndpointItem } from "../../data";
import { webhookEndpointHref, isEndpointDisabled } from "../../data";
import { DeepLinkArrow } from "../shared/DeepLinkArrow";
import { StatusDot } from "../shared/StatusDot";

export interface WebhookTableProps {
  endpoints: WebhookEndpointItem[];
  /** Native-Stripe host for the "↗" deep-links ("" → disabled, honest). */
  stripeBase: string;
}

// One scoped stylesheet: row hover lifts warm + reveals the "↗"; container-query
// keeps the layout from collapsing on narrow hosts (the wrapper scrolls).
const TABLE_CSS = `
  .st-wh-table { container-type: inline-size; width: 100%; }
  .st-wh-scroll { overflow-x: auto; }
  .st-wh-grid { width: 100%; min-width: 720px; border-collapse: separate; border-spacing: 0; }
  .st-wh-grid th {
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
  .st-wh-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .st-wh-row { transition: background 100ms ease; }
  .st-wh-row:hover { background: var(--sand); }
  .st-wh-row:hover .st-wh-arrow { color: var(--honey); }
  .st-wh-row:hover .st-wh-url { color: var(--honey); }
  .st-wh-url { font-family: var(--font-mono, var(--font-body)); }
`;

/** Up to N event chips + a "+M" overflow chip; the full list opens on click. */
function EventChips({ events }: { events: string[] }) {
  const [open, setOpen] = useState(false);
  const MAX = 3;
  if (events.length === 0) {
    return <span style={{ color: "var(--mut)" }}>nenhum</span>;
  }
  const isWildcard = events.length === 1 && events[0] === "*";
  if (isWildcard) {
    return <Chip label="todos os eventos" tone="team" preserveCase />;
  }
  const shown = open ? events : events.slice(0, MAX);
  const overflow = events.length - shown.length;
  return (
    <span style={{ display: "inline-flex", flexWrap: "wrap", gap: "var(--sp-1)", alignItems: "center" }}>
      {shown.map((ev) => (
        <Chip key={ev} label={ev} tone="neutral" preserveCase />
      ))}
      {overflow > 0 ? (
        <button
          type="button"
          onClick={() => setOpen(true)}
          title="Mostrar todos os eventos"
          style={{
            fontFamily: "var(--font-body)",
            fontSize: "var(--fs-xs)",
            fontWeight: 700,
            padding: "var(--sp-1) var(--sp-2)",
            borderRadius: "999px",
            border: "1px solid var(--line)",
            background: "var(--cream)",
            color: "var(--mut)",
            cursor: "pointer"
          }}
        >
          +{overflow}
        </button>
      ) : null}
    </span>
  );
}

/**
 * The webhook endpoints roster — the real data page, the contract's canonical
 * readable signal. Columns: the endpoint URL (mono; links nowhere itself — the
 * row's "↗" is the only navigation, OUT to the native Stripe webhook detail), a
 * status dot (enabled = ok / disabled = crit, read straight from the field), the
 * subscribed event-type count as chips, the API version, and the "↗" deep-link.
 */
export function WebhookTable({ endpoints, stripeBase }: WebhookTableProps) {
  // Disabled endpoints first (they need attention), then by URL.
  const rows = [...endpoints].sort((a, b) => {
    const da = isEndpointDisabled(a) ? 0 : 1;
    const db = isEndpointDisabled(b) ? 0 : 1;
    if (da !== db) return da - db;
    return (a.url || a.id).localeCompare(b.url || b.id);
  });

  return (
    <div className="st-wh-table">
      <style>{TABLE_CSS}</style>
      <div className="st-wh-scroll">
        <table className="st-wh-grid">
          <thead>
            <tr>
              <th>Endpoint</th>
              <th>Status</th>
              <th>Eventos</th>
              <th>API version</th>
              <th aria-label="Abrir no Stripe" />
            </tr>
          </thead>
          <tbody>
            {rows.map((e) => {
              const off = isEndpointDisabled(e);
              return (
                <tr key={e.id || e.url} className="st-wh-row">
                  <td style={{ maxWidth: 360 }}>
                    <span
                      className="st-wh-url"
                      style={{
                        fontWeight: 600,
                        color: "var(--ink)",
                        transition: "color 100ms ease",
                        display: "block",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap"
                      }}
                      title={e.url}
                    >
                      {e.url || e.id}
                    </span>
                  </td>
                  <td>
                    <StatusDot
                      tone={off ? "crit" : "ok"}
                      label={off ? "desativado" : "ativo"}
                      title={off ? "Stripe não está entregando a este endpoint" : "Stripe está entregando"}
                    />
                  </td>
                  <td>
                    <EventChips events={e.enabledEvents} />
                  </td>
                  <td>
                    {e.apiVersion ? (
                      <span style={{ fontFamily: "var(--font-mono, var(--font-body))", color: "var(--mut)" }}>
                        {e.apiVersion}
                      </span>
                    ) : (
                      <span style={{ color: "var(--mut)" }}>padrão</span>
                    )}
                  </td>
                  <td style={{ textAlign: "right", width: "1.8em" }}>
                    <DeepLinkArrow
                      className="st-wh-arrow"
                      href={webhookEndpointHref(stripeBase, e.id)}
                      label={`Abrir "${e.url || e.id}" no Stripe`}
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
