# AGENTS

> **Source of truth is `providers/stripe/adapter/spec.go`**, not this
> file. When in doubt about the live contract (capabilities, versions,
> transport), trust `Describe()` and the `Operation*` / `Reactor*`
> constants in `spec.go`. This file is a map; the code is the territory.
> See `CLAUDE.md` for the expanded context.

## Repo role

`integration-stripe` is the standalone **Stripe leaf adapter** for the
Yggdrasil control plane. `integration_type stripe`, namespace `global`,
domain `payments`. It exposes an honest adapter contract through
`describe` and runs capabilities through `execute` over `http_json` RPC;
inbound Stripe webhooks are handled by a separate reactor server.

Multi-tenant: one Stripe account = one `integration_instance` (its own
API key + webhook secret); Stripe Connect via the optional
`stripe_account_id` instance field.

## Capability surface

19 executable ops + 1 reactor — authoritative list is
`SupportedExecuteOperations` in `spec.go`:

- Canonical `ensure_`/`observe_`/`destroy_` triples for `payment_intent`,
  `customer`, `subscription`, `webhook_endpoint`; `observe_` only for
  `charge` and `balance`.
- Allowlisted action helpers: `create_refund`, `create_payout`
  (money-movement), `create_setup_intent`, `manage_connect_account`,
  `verify_webhook_signature`.
- Reactor `stripe_webhook_received` — framework-invoked, NOT dispatchable
  via `execute`.
- Pre-v2.0.0 names still accepted via `legacyOperationAliases` /
  `ResolveOperation`; don't drop an alias without checking callers.

## Webhook reactor (HMAC `t=,v1=`)

- `providers/stripe/adapter/webhook_server.go` serves
  `POST /webhooks/stripe/{instance_id}` — separate from the SDK RPC mux.
  Verify signature → dedup (`instance_id:event_id`, 24h) → **200 before
  emit** → async RTA emit.
- `providers/stripe/adapter/hmac.go` is a **local** Stripe-Signature
  verifier with typed errors. The repo deliberately does NOT use the
  SDK's `sig/hmac` or `webhookhttp` packages (both exist in
  yggdrasil-sdk-go v0.8.3) — keep the local error/dedup contract; tests
  encode it.

## Transport, versions, ports

- Transport `http_json`; endpoints `/rpc/describe`, `/rpc/execute`;
  timeout 30s.
- Ports (`cmd/adapter/main.go`): RPC `ADAPTER_PORT` default **8081**,
  webhook `WEBHOOK_PORT` default **8082**, health `HEALTHCHECK_PORT`
  default **8080** (`/healthz`, `/readyz`, `/metrics`).
- `AdapterVersion` in `spec.go` ≈ **2.4.0** (read the constant);
  `StripeAPIVersion` = `2024-12-18.acacia`.
- SDK pin `yggdrasil-sdk-go` ≈ **v0.8.3** (`go.mod`); Stripe client
  `stripe-go/v83`; Go 1.25.

## Non-negotiable rules

- Keep the plugin standalone — no yggdrasil-core / monorepo runtime
  imports. Wire types live in `family/contract/types.go`.
- `describe` must stay aligned with what `execute` accepts;
  `pkg/contractcheck` enforces it in CI — don't silence it.
- Add/rename a capability → update `spec.go`, tests, and `docs/` in the
  same change.
- Prefer failing fast over silent degradation (no swallowed
  signature/emit errors).
- Runtime behavior only; business authority stays in yggdrasil-core.

## Manifest may be stale

`manifest/` is maintained separately from `spec.go` and can drift (e.g.
`integration_type.stripe.yaml` version `2.2.4` vs `spec.go` `2.4.0`). Do
NOT edit `manifest/` as part of a docs/context change — reconcile it as
its own deliberate step. There is no `examples/` dir and no top-level
`INTEGRATION_CONTRACT.md` / `SURFACE_CONTRACT.md` in this repo; adopter
docs are under `docs/`.

## Commands

- `go test ./...`
- `task config`
- `task build:image`
- `task up` / `task down`
