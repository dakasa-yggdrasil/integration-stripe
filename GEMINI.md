# GEMINI

> **Source of truth is `providers/stripe/adapter/spec.go`**, not this
> file. For the live contract (capabilities, versions, transport) trust
> `Describe()` and the `Operation*` / `Reactor*` constants in `spec.go`.
> Read `CLAUDE.md` for the full context and `AGENTS.md` for the
> rules-of-engagement summary.

## What this is

`integration-stripe`: the standalone **Stripe leaf adapter** for the
Yggdrasil control plane. `integration_type stripe`, namespace `global`,
domain `payments`. yggdrasil-core ↔ adapter over `http_json` RPC; inbound
Stripe webhooks handled by a separate reactor server. Multi-tenant (one
Stripe account = one instance), Stripe Connect via optional
`stripe_account_id`.

## Capability surface

19 executable ops + 1 reactor (`stripe_webhook_received`, framework-only).
Authoritative list: `SupportedExecuteOperations` in `spec.go`. Canonical
`ensure_`/`observe_`/`destroy_` triples (payment_intent, customer,
subscription, webhook_endpoint) + `observe_` (charge, balance) +
allowlisted helpers (`create_refund`, `create_payout`,
`create_setup_intent`, `manage_connect_account`,
`verify_webhook_signature`). Pre-v2.0.0 names accepted via
`legacyOperationAliases` / `ResolveOperation`.

## Webhook reactor (HMAC `t=,v1=`)

`POST /webhooks/stripe/{instance_id}` is served by the **local**
`webhook_server.go` + `hmac.go` (typed errors, `instance_id:event_id`
dedup, 200-before-emit). The repo deliberately does NOT use the SDK's
`sig/hmac` or `webhookhttp` packages even though both exist in
yggdrasil-sdk-go v0.8.3.

## Transport, versions, ports

- `http_json`; `/rpc/describe`, `/rpc/execute`; timeout 30s.
- Ports: RPC `ADAPTER_PORT` 8081, webhook `WEBHOOK_PORT` 8082, health
  `HEALTHCHECK_PORT` 8080.
- `AdapterVersion` ≈ 2.4.0 (read the constant); SDK pin
  `yggdrasil-sdk-go` ≈ v0.8.3; Stripe client `stripe-go/v83`; Go 1.25.

## Rules

- Keep it standalone (no yggdrasil-core/monorepo runtime imports; wire
  types in `family/contract/types.go`).
- Keep `describe` aligned with `execute` (`pkg/contractcheck` enforces
  it in CI).
- Validate any capability change against `spec.go`, tests, and `docs/`.
- Runtime behavior only; business authority stays in yggdrasil-core.

## Manifest may be stale

`manifest/` is maintained separately and can drift from `spec.go` (e.g.
version `2.2.4` vs `2.4.0`). Do NOT edit `manifest/` as part of a
docs/context change. No `examples/` dir and no top-level
`INTEGRATION_CONTRACT.md` / `SURFACE_CONTRACT.md` exist here; adopter
docs live under `docs/`.
