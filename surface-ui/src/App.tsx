import { Routes, Route, Navigate } from "react-router-dom";
import { Home } from "./screens/Home";
import { Webhooks } from "./screens/Webhooks";
import { Balance } from "./screens/Balance";
import { Disputes } from "./screens/Disputes";
import { Reconciliation } from "./screens/Reconciliation";

/**
 * Collaborator-root router for the Stripe operator surface (surface #7 of the
 * 9-surface family — the FIRST payment-rail surface; EFI reuses this template).
 * Mirrors the AWS / Google Workspace / Grafana templates: the surface opens on a
 * technical account-pulse Home, with four pillar detail screens — Webhook
 * Health, Saldo & Payouts, Disputas, and Reconciliação & Refunds.
 *
 * CRITICAL RULE #0: this is a payments-OPS view for the platform team, NEVER a
 * per-customer billing UI. Customer-identifying data appears only as opaque refs
 * (charge id, payment_intent). Money-movement (refund / payout) is admin-tier and
 * OUT of v1 — rendered as a gated, disabled "Em breve" affordance, never a
 * transactional button. The warm Atelier theme is applied per-screen;
 * `BrowserRouter basename="/s/stripe"` lives in main.tsx.
 */
export function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/webhooks" element={<Webhooks />} />
      <Route path="/balance" element={<Balance />} />
      <Route path="/disputes" element={<Disputes />} />
      <Route path="/reconciliation" element={<Reconciliation />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
