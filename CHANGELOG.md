# Changelog

All notable changes to integration-stripe will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.2.0] — 2026-05-27

### Changed

- Bump yggdrasil-sdk-go v0.6.0 → v0.7.0 to pick up the public
  `reconcile.Dispatch` API.
- Production runtime migrated from `adapter.Execute` switch to
  `reconcile.Dispatch`. `cmd/adapter/main.go` now calls
  `WireReconcilers(a, defaultInstanceID(instances))` BEFORE the
  `Register("execute", ...)` chain so the SDK dispatch table owns
  routing for `ensure_/observe_/destroy_` ops. The legacy switch
  remains as the fallback path for allowlisted action helpers
  (`create_refund`, `create_setup_intent`, `create_payout`,
  `manage_connect_account`) and `verify_webhook_signature`, which
  are not registered through `RegisterReconciler`.
- §6.5 mutation event auto-emission is now LIVE for production
  traffic (previously TEST-ONLY) when `YGGDRASIL_CORE_URL`
  + `YGGDRASIL_RUN_TOKEN` are wired in the cluster manifest.
- `ExecuteHandler` signature now accepts `*adapter.Adapter`
  alongside the logger and instances map; the bridge re-wraps the
  raw observed-state JSON returned by the SDK in the legacy
  `rpcResponse{ok,data}` envelope so callers see the same wire
  shape they always have.
- `reconcile.go` dispatch helpers extract `instance_id` from the
  input payload (lifted by the ExecuteHandler bridge from
  `integration.instance_id`) per-call, with the registration-time
  fallback preserving single-instance test flows.

## [2.1.0] — 2026-05-27

### Added

- Bump yggdrasil-sdk-go v0.5.0 → v0.6.0 to pick up the additive
  `sdk/events` package + the new `reconcile.WithEmitter` /
  `WithProvider` / `WithInstanceID` options.
- `WireReconcilers` now wires a §6.5 mutation-event emitter via
  `events.NewHTTPEmitter()` when `YGGDRASIL_CORE_URL` is set;
  degrades to `events.NoopEmitter{}` otherwise. Successful
  `Ensure()` / `Destroy()` invocations auto-emit
  `stripe.<resource>.ensured` / `stripe.<resource>.destroyed`
  events to yggdrasil-core `POST /api/v1/events` per
  INTEGRATION_CONTRACT.md §6.5.
- The emitter is env-driven (`YGGDRASIL_CORE_URL` +
  `YGGDRASIL_RUN_TOKEN`) so the adapter stays Lego-compliant — no
  broker / secret-store / cloud is hardcoded.

### Notes

- Production `main()` still uses the legacy `message.Execute`
  switch as the runtime dispatch path; emission activates once the
  runtime moves onto SDK reconcile dispatch in a follow-up cycle.
  The wiring is in place so the v0.6.0 emitter is a no-op flip
  away.
- Emission is best-effort (per `reconcile.WithEmitter` docstring):
  failures log WARN but never fail the capability call.

## [2.0.0] — 2026-05-27

### BREAKING CHANGES — Yggdrasil universal capability naming convention

Aligned with the universal capability naming convention. Resource
operations use the canonical `ensure_/observe_/destroy_` prefix
triple; legacy v1 names route through `WithLegacyNames` shim with a
WARN log entry (removal target: v3.0.0).

#### Managed resource types

- `payment_intent`: `ensure_/observe_/destroy_`
- `customer`: `ensure_/observe_/destroy_`
- `subscription`: `ensure_/observe_/destroy_`
- `webhook_endpoint`: `ensure_/observe_/destroy_`

### Migration

SDK reconcile dispatch is now wired (`WireReconcilers`) as the
Go-API expression of the convention; production runtime continues
to use the legacy `message.Execute` switch for backward compat.
