# Changelog

All notable changes to integration-stripe will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased — 2026-05-27

### Changed

- **`.github/workflows/release.yml`**: build image on every push to
  `main` (matches the integration-efi pattern). Previously the
  workflow only triggered on tag push, forcing every cycle to either
  bump a tag OR manually dispatch the workflow. Tag-push + manual
  dispatch triggers remain in place; the additional main-branch
  trigger removes the friction so adapter rolls stay declarative.
- Tagging strategy aligned with integration-efi via
  `docker/metadata-action@v5`:
  - `branch-main-latest` + `sha-<short>` + `edge` on main pushes
  - `v<version>` + `latest` on tag push
  - optional `${{ inputs.tag }}` on workflow_dispatch
- Tagging now uses `docker/metadata-action@v5` instead of a
  hand-rolled `Resolve version` step — keeps the tag matrix
  consistent across all adapters in the GHCR namespace.

## [2.3.1] — 2026-05-27

### Changed

- **Bumped `yggdrasil-sdk-go` v0.8.0 → v0.8.1**. SDK patch fixes
  destroy resource_id inference for §6.5 mutation events. Pre-v0.8.1
  stripe destroy events (e.g. `stripe.customer.destroyed`,
  `stripe.subscription.destroyed`) emitted with empty resource_id
  because `makeDestroyFn` only extracted ref from `{"ref":...}` while
  stripe sends `{"customer_id":"cus_..."}`, `{"subscription_id":"sub_..."}`,
  etc. yggdrasil-core rejected with HTTP 400. After v0.8.1 the SDK
  infers ref from `<resource>_id`; stripe destroy events now land in
  `event_log` with the correct id. No adapter source change required.

## [2.3.0] — 2026-05-27

### Changed

- **Bumped `yggdrasil-sdk-go` v0.7.0 → v0.8.0**. New SDK ships
  `DestroyWithDesired[D]` — an opt-in interface that lets reconcilers
  see the FULL desired payload during destruction (including the
  reserved bridge keys `__instance_credentials` / `__instance_config`
  / `__request_auth` per INTEGRATION_CONTRACT.md §5.b).
- **`paymentIntentReconciler`, `customerReconciler`,
  `subscriptionReconciler`, `webhookEndpointReconciler` implement
  `DestroyWithDesired`**. The new method merges the inbound ref into
  the desired payload the SDK forwards (so `payment_intent_id` /
  `customer_id` / `subscription_id` / `id` is present for handlers),
  then routes through the existing `dispatch()` →
  `buildExecuteRequest()` → `Execute()` chain that ensure_* / observe_*
  already use. Credentials reach destroy_* handlers identically to
  ensure_* / observe_*.

### Tests

- New `TestCustomerReconciler_DestroyWithDesired_ForwardsCredentials`
  proves the SDK v0.8.0 + DestroyWithDesired combination propagates
  `InstanceCredsKey` through the dispatch helper into Execute().

## [2.2.4] — 2026-05-27

### Fixed

- **`instance_schema` did not declare operator-injected metadata
  fields** (`base_url` / `environment` / `provider`). The validation
  instance `stripe-dakasa-validation` (and any real operator template)
  carries these on `spec.config` for ergonomics — base_url for the
  RPC endpoint override on the harness pod, environment for
  sandbox/production tagging, provider as a provider-name echo.
  yggdrasil-core's instance-config validator rejects undeclared
  fields during dispatch with `integration config field "<x>" is not
  declared in the integration_type schema`. Surfaced during the
  v2.2.3 smoke when the credential_schema gate finally passed.
- Declared `base_url`, `environment`, `provider` as additional
  string properties on `InstanceSchema.Properties`. The adapter
  ignores fields it does not consume; this only opens the validator
  gate. The canonical adapter-side override hook for the RPC base
  URL remains `stripe_api_base_url` (see `clientForExecuteConfig`).
- Covered by `TestSpec_InstanceSchemaDeclaresOperatorMetadata`.
- `AdapterVersion` bumped 2.2.3 → 2.2.4 (patch — additive schema
  fields, no client-side breaking change). YAML manifest
  `spec.version` / `spec.adapter.version` / `spec.adapter.image_tag`
  aligned to 2.2.4.

## [2.2.3] — 2026-05-27

### Fixed

- **`integration_type.stripe.yaml` credential_schema only declared
  `stripe_api_key`** — the v2.2.2 Go-level `Describe()` declares
  both `stripe_api_key` and `stripe_secret_key`, and `adapter.Execute`
  reads either field with canonical-first preference, but the YAML
  manifest registered into yggdrasil-core via
  `POST /api/v1/manifests?kind=integration_type` was missing
  `stripe_secret_key`. Core validates incoming instance configs
  against the YAML's `credential_schema.properties` BEFORE the
  request reaches the adapter and rejects with `integration
  credentials field "stripe_secret_key" is not declared in the
  integration_type schema`. Operators using AWS Secrets Manager (or
  any secret-store) entries that follow the `stripe_secret_key`
  naming convention could not bind to a stripe instance even though
  the adapter would have happily resolved the alias.
- Declare `stripe_secret_key` as `type: string, secret: true` in
  `manifest/integration_type.stripe.yaml` credential_schema —
  alongside `stripe_api_key`. The alias is now reachable end-to-end
  (validator pass → adapter read → canonical-preference resolution).
- Covered by `TestManifest_IntegrationTypeYAML_DeclaresBothKeyAliases`
  and the existing Go-level `TestSpec_CredentialSchemaDeclaresBothKeyAliases`
  — both pin the credential_schema shape so adapter Go declarations
  and YAML stay aligned across future schema changes.
- `AdapterVersion` bumped 2.2.2 → 2.2.3 (patch — additive schema
  field, no client-side breaking change; the alias was already
  accepted by `adapter.Execute` since v2.2.2).
- Manifest YAML `spec.version` / `spec.adapter.version` /
  `spec.adapter.image_tag` aligned to 2.2.3 so the registered
  catalog row matches the new image.

## [2.2.2] — 2026-05-27

### Fixed

- **Execute() unconditionally called clientForInstance with an empty
  apiKey** — pre-existing structural bug surfaced during the cycle
  #243 bridge sweep and deferred from v2.2.1 to a separate cycle.
  `providers/stripe/adapter/adapter.go::Execute` invoked
  `clientForInstance(req.Integration.InstanceID, "", "", StripeAPIVersion)`
  with a hardcoded empty `apiKey`, so `NewStripeClient("")` returned
  `"stripe api key is required"` and EVERY stripe write capability
  (`ensure_customer`, `ensure_payment_intent`, `ensure_subscription`,
  `ensure_webhook_endpoint`, `destroy_*`, plus the kept allowlisted
  action helpers `create_refund` / `create_setup_intent` /
  `create_payout` / `manage_connect_account`) failed at the apiKey
  gate before reaching the Stripe HTTP boundary — regardless of
  whether the v2.2.1 bridge fix had rehydrated credentials onto
  `req.Integration.Spec.Credentials`.
- Fix: Execute() now reads `stripe_api_key` from
  `req.Integration.Spec.Credentials` and optional
  `stripe_api_base_url` / `stripe_api_version` from
  `req.Integration.Spec.Config`, threading all three into
  `clientForInstance`. The credentials map is rehydrated upstream by
  `buildExecuteRequest` (v2.2.1) from the wire-level
  `integration.instance_spec.credentials` field. The `stripe_api_base_url`
  config field doubles as an override hook for stripe-mock /
  httptest.Server / private Stripe-API-compatible endpoints
  (Lego-principle aligned — does not couple to api.stripe.com).
- Covered by `TestExecute_ReadsAPIKeyFromCredentials` and
  `TestExecute_MissingAPIKeyRejected` in
  `providers/stripe/adapter/execute_credentials_test.go`. The first
  test reproduces the exact `stripe api key is required` failure on
  unfixed code and asserts `Authorization: Bearer <canary>` reaches
  the in-process Stripe HTTP server after the fix.
- `AdapterVersion` bumped 2.2.1 → 2.2.2 (patch — no API surface
  change; the credential is one already declared in the v1.0.0
  `credential_schema`).

### Added — credential field alias

- The `credential_schema` now accepts `stripe_secret_key` as a
  recognized alias for `stripe_api_key`. Execute() reads
  `stripe_api_key` first then falls back to `stripe_secret_key` —
  operator secret-store entries that follow that naming convention
  (e.g. AWS Secrets Manager `dakasa/validation/yggdrasil-stripe-secret`
  with field `stripe_secret_key`) work WITHOUT renaming the secret.
  The Lego principle (§2) argues against forcing every operator to
  rename their secret-store entries to match the canonical field name;
  the alias is a one-line accommodation.
- The canonical `stripe_api_key` wins when both fields are present
  (predictable for operators rotating to the canonical name).
- Covered by
  `TestExecute_ReadsAPIKeyFromStripeSecretKeyAlias` and
  `TestExecute_StripeAPIKeyPrefersCanonicalOverAlias`.

### Closes

- The `DONE_WITH_CONCERNS` note from v2.2.1 ("`adapter.Execute` /
  `clientForInstance` carry a pre-existing structural bug independent
  of the bridge fix") — bridge fix + Execute() rehydration now
  compose to make stripe writes work end-to-end in production.

## [2.2.1] — 2026-05-27

### Fixed

- **Bridge credentials forwarding (necessary but NOT sufficient)**: the
  `providers/stripe/message/execute.go::buildSDKDelivery` bridge was
  dropping `instance_spec.config` / `instance_spec.credentials` /
  `req.Auth` when re-marshalling inbound envelopes into the SDK-shaped
  reconcile envelope. Per-resource dispatch helpers
  (`paymentIntentReconciler.dispatch`, `customerReconciler.dispatch`,
  `subscriptionReconciler.dispatch`, `webhookEndpointReconciler.dispatch`)
  then synthesized empty-Spec/Auth requests into `Execute()`. Same root
  cause as integration-github cycle #243 (`892811e` / `eb49c75` fix). Fix
  mirrors the canonical pattern: bridge stashes the three context maps
  under reserved keys (`InstanceConfigKey` / `InstanceCredsKey` /
  `InstanceAuthKey`, `"__instance_config"` / `"__instance_credentials"`
  / `"__request_auth"`), shared `buildExecuteRequest` helper rehydrates
  them into `AdapterExecuteIntegrationRequest` before each dispatch
  Execute() call, and reserved keys are stripped from forwarded Input
  so handlers only see operator-supplied fields.

### Known issue — DEFERRED to a follow-up cycle (DONE_WITH_CONCERNS)

- `adapter.Execute` / `clientForInstance` carry a **pre-existing
  structural bug** independent of the bridge fix above:
  `clientForInstance(req.Integration.InstanceID, "", "", StripeAPIVersion)`
  passes an empty `apiKey` unconditionally, and the per-instance
  `config.LoadInstances` map captured by `cmd/adapter/main.go` is held
  by `message.ExecuteHandler` as `_ = instances` (line 27 comment claims
  "per-instance config consumed inside adapter.Execute via
  clientForInstance" but no such wiring exists). In production
  `NewStripeClient("")` returns `"stripe api key is required"` and ALL
  stripe writes fail regardless of bridge state. The bridge fix shipped
  in this release is necessary (the secondary bug fix will need
  `req.Integration.Spec.Credentials["stripe_api_key"]` to reach
  `Execute()` to work), but not sufficient. Follow-up cycle should wire
  `instances` map through to `clientForInstance` or refactor
  `clientForInstance` to read from the synthesized
  `AdapterExecuteIntegrationRequest.Integration.Spec.Credentials`.

### Tests

- `TestPaymentIntentReconciler_Dispatch_ForwardsInstanceCredentials`,
  `TestPaymentIntentReconciler_Dispatch_FallbackInstanceID`,
  `TestPaymentIntentReconciler_Dispatch_NilReservedMapsTolerated`
  (`providers/stripe/adapter/reconcile_dispatch_test.go`) — assert the
  in-tree `buildExecuteRequest` helper rehydrates instance_spec +
  req.Auth from the reserved-key forwarded payload, falls back to the
  reconciler-bound instance_id when the payload carries no override,
  and tolerates nil reserved maps without panic.

### Changed

- `AdapterVersion` bumped 2.2.0 → 2.2.1 (patch — no API surface change).

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
