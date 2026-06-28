import { Routes, Route, Navigate } from "react-router-dom";
import { useCollaboratorScope, SurfaceViewGate, LoadingState } from "@dakasa-yggdrasil/surface-toolkit";
import { mockEnabled, mockCollaboratorScope } from "./data";
import { Home } from "./screens/Home";
import { Webhooks } from "./screens/Webhooks";
import { Balance } from "./screens/Balance";
import { Disputes } from "./screens/Disputes";
import { Reconciliation } from "./screens/Reconciliation";
import { Subscriptions } from "./screens/Subscriptions";
import { PaymentIntents } from "./screens/PaymentIntents";
import { ChargeDetail } from "./screens/ChargeDetail";

/**
 * Collaborator-root router for the Stripe operator surface (surface #7 of the
 * 9-surface family — the FIRST payment-rail surface; EFI reuses this template).
 * Mirrors the AWS / Google Workspace / Grafana templates: the surface opens on a
 * technical account-pulse Home with grouped navigation, fanning out to the
 * detail screens — Webhook Health, Saldo & Payouts, Assinaturas, Payment
 * Intents, Disputas, and Reconciliação & Refunds — plus a charge drill-down
 * (`/charge/:id`) opened from the Reconciliação roster.
 *
 * CRITICAL RULE #0: this is a payments-OPS view for the platform team, NEVER a
 * per-customer billing UI. Customer-identifying data appears only as opaque refs
 * (charge id, payment_intent, subscription id, customer id) — never a name or
 * email. Money-movement (refund / payout) is admin-tier and OUT of v1 —
 * rendered as a gated, disabled "Em breve" affordance, never a transactional
 * button. The warm Atelier theme is applied per-screen;
 * `BrowserRouter basename="/s/stripe"` lives in main.tsx.
 */
export function App() {
  // Resolve the viewer scope once at the root (mock-aware: `?mock` stubs an
  // admin/all-perms scope so live-review still renders the full surface).
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const scope = mock ? mockCollaboratorScope() : liveScope;

  if (scope.isLoading) {
    return (
      <div className="atelier" style={{ padding: "var(--sp-7)" }}>
        <LoadingState label="Carregando…" />
      </div>
    );
  }

  return (
    <SurfaceViewGate provider="stripe" perms={scope.perms} surfaceTitle="Stripe">
      <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/webhooks" element={<Webhooks />} />
      <Route path="/balance" element={<Balance />} />
      <Route path="/subscriptions" element={<Subscriptions />} />
      <Route path="/payment-intents" element={<PaymentIntents />} />
      <Route path="/disputes" element={<Disputes />} />
      <Route path="/reconciliation" element={<Reconciliation />} />
      <Route path="/charge/:id" element={<ChargeDetail />} />
      <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </SurfaceViewGate>
  );
}
