# Claude Code Context: integration-stripe

> **Source of truth is the code, not this file.** When this document and
> the adapter disagree, trust `providers/stripe/adapter/spec.go` —
> specifically `Describe()` and the `Operation*` / `Reactor*` constants.
> Everything below is a human-readable map; `spec.go` is what
> yggdrasil-core actually handshakes against.

## What this repo is

`integration-stripe` is the **Stripe leaf adapter** for the Yggdrasil
control plane (`github.com/dakasa-yggdrasil/integration-stripe`, Go,
Apache 2.0). It turns Stripe into a declarative Yggdrasil integration:
yggdrasil-core speaks `http_json` RPC to it (`/rpc/describe`,
`/rpc/execute`), the adapter translates capabilities into Stripe REST
calls (via `stripe-go/v83`), and it pushes inbound Stripe webhook
deliveries back into core as RTA event envelopes.

- **Domain:** `payments` (money movement — payments, subscriptions,
  refunds, payouts, Connect). This is provider-side resource management,
  not end-user business: the adapter manages the *company's* Stripe
  account configuration. (Charging an end user lives in the backend, not
  here.)
- **`integration_type`:** `stripe`, **namespace `global`** (see
  `manifest/integration_type.stripe.yaml`). Instances are per-namespace
  (e.g. `integration-stripe-dakasa` in ns `dakasa`).
- **Multi-tenant by design:** one Stripe account = one
  `integration_instance`, each with its own API key + webhook signing
  secret. Connect is supported via the optional `stripe_account_id`
  instance field (sets the `Stripe-Account` header).
- **Provider / type:** both named `stripe` (see the `Provider` /
  `IntegrationType` constants in `spec.go`).

## Capability surface

19 executable operations + 1 reactor. The full, authoritative list is
`SupportedExecuteOperations` and the `Operation*` / `Reactor*` constants
in `spec.go`; do not hand-maintain a second copy here. Shape:

- **Canonical triples** (`ensure_`/`observe_`/`destroy_`, per the
  Yggdrasil universal naming convention) for `payment_intent`,
  `customer`, `subscription`, `webhook_endpoint`; read-only `observe_`
  for `charge` and `balance`.
- **Allowlisted action-shaped helpers** that don't collapse into the
  triple: `create_refund` and `create_payout` (money-movement),
  `create_setup_intent`, `manage_connect_account`,
  `verify_webhook_signature` (pure HMAC helper).
- **Reactor:** `stripe_webhook_received` — NOT dispatchable via
  `execute`; framework-invoked by the webhook server on inbound delivery.

**Legacy aliases.** Pre-v2.0.0 names (`create_payment_intent`,
`cancel_subscription`, `list_charges`, `retrieve_balance`, …) are still
accepted and mapped to their canonical v2 op by
`legacyOperationAliases` / `ResolveOperation` in `spec.go` (the bool
return flags that a legacy name was used so the caller can WARN). The
SDK's `reconcile.WithLegacyNames` gives the same behavior on the
reconcile path. Don't remove an alias without checking who still sends
the old name.

## Webhook reactor (HMAC `t=,v1=`)

The reactor is served **separately from the SDK RPC mux**:

- `providers/stripe/adapter/webhook_server.go` — `WebhookServer` routes
  `POST /webhooks/stripe/{instance_id}`, reads the raw body (max 64 KiB),
  verifies the signature, dedups by `instance_id:event_id` (in-memory
  `sync.Map`, 24h TTL per spec §2), returns **200 BEFORE emitting** the
  RTA envelope (so a slow downstream doesn't trigger Stripe's retry
  storm), then emits asynchronously via the `RTAEmitter`.
- `providers/stripe/adapter/hmac.go` — `VerifySignature` is a **local,
  hand-rolled** implementation of the Stripe-Signature algorithm
  (`signed_payload = "{ts}.{body}"`, `HMAC_SHA256`, constant-time
  compare over all `v1=` candidates, `t=` tolerance window). It returns
  typed errors (`ErrSignatureMissingT`, `ErrSignatureMissingV1`,
  `ErrSignatureExpired`, `ErrSignatureMismatch`, `ErrInvalidTimestamp`)
  that the tests assert on concretely to defeat any silent-pass refactor.

**Note — the SDK ships equivalents this repo deliberately does NOT use.**
`yggdrasil-sdk-go` v0.8.3 has both `sig/hmac/stripe.go` and
`webhookhttp/server.go`. This adapter keeps its own `hmac.go` +
`webhook_server.go` instead, so the reactor surface stays under local
control (typed errors, dedup map, 200-before-emit ordering). If you're
tempted to "just use the SDK helper", confirm it matches the local
error/dedup contract first — the tests encode the local one.

## Repo layout (real)

```
cmd/adapter/main.go               # 3 listeners: RPC (8081), webhook (8082), health (8080)
providers/stripe/
  adapter/
    spec.go                       # Describe() contract + Operation*/Reactor* consts + AdapterVersion (SOURCE OF TRUTH)
    adapter.go, client.go         # Stripe REST client + execute dispatch
    reconcile.go                  # SDK reconcile dispatch table (WireReconcilers, §6.5 mutation events)
    event_router.go               # eventTypeToRTAKey() — Stripe event type → RTA routing key
    webhook_server.go             # inbound webhook reactor (NOT the SDK webhookhttp)
    hmac.go                       # local Stripe-Signature verify (NOT the SDK sig/hmac)
    connect.go                    # Stripe Connect (manage_connect_account)
    metrics.go                    # prometheus counters (sig failures, dedup, RTA emit)
    spec_test.go, hmac_test.go, webhook_server_test.go, contractcheck_test.go, ...
  config/                         # InstanceConfig, LoadInstances (STRIPE_INSTANCES_CONFIG)
  message/                        # SDK RPC handlers: describe.go, execute.go, rpc.go (local {ok,data,error} envelope)
family/contract/types.go          # local copy of the core wire types (AdapterDescribeResponse, ...) — keeps adapter standalone
pkg/contractcheck/                # public describe-vs-execute drift linter (used in CI + contractcheck_test.go)
manifest/                         # integration_type + per-capability + reactor manifests (see staleness note below)
testdata/stripe-events/           # sample Stripe event payloads for webhook tests
integration_tests/webhook_test.go # end-to-end webhook flow test
yggdrasil-quickstart.yaml         # quickstart bundle for `yggdrasil install`
docs/                             # USAGE / CONFIGURATION / CAPABILITIES / OPERATIONS / DEVELOPMENT
```

There is **no `examples/` dir and no top-level `INTEGRATION_CONTRACT.md`
/ `SURFACE_CONTRACT.md`** in this repo — those live in the yggdrasil
monorepo. Adopter-facing docs are under `docs/`.

## Transport & versions

- **Transport:** `http_json` (declared in `Describe().Adapter.Transport`
  and the manifest). Endpoints `/rpc/describe` + `/rpc/execute`,
  `timeout_seconds: 30`.
- **Ports** (`cmd/adapter/main.go`):
  - RPC: `ADAPTER_PORT`, default **8081** (SDK `adapter.New(...).ListenHTTP`).
  - Webhook: `WEBHOOK_PORT`, default **8082** (local `WebhookServer`).
  - Health/metrics: `HEALTHCHECK_PORT`, default **8080**
    (`/healthz`, `/readyz`, `/metrics`).
- **AdapterVersion:** `spec.go` `const AdapterVersion` ≈ **2.4.0**
  (read the constant; this number moves). `StripeAPIVersion` pins the
  Stripe API version (`2024-12-18.acacia`) — bumping it requires a full
  integration-test cycle + version bump.
- **SDK pin:** `go.mod` `dakasa-yggdrasil/yggdrasil-sdk-go` ≈ **v0.8.3**
  (read `go.mod`). Stripe client: `stripe/stripe-go/v83`. Go 1.25.

## Manifest is generated/maintained separately — may be stale

`manifest/` holds the integration_type + per-capability + reactor
manifests that get published. It is **not** auto-derived from `spec.go`
at build time, so it can drift. As of this writing
`manifest/integration_type.stripe.yaml` declares
`spec.version` / `adapter.version` = **2.2.4** while `spec.go`
`AdapterVersion` is **2.4.0** — that's a known drift.

**Do NOT edit `manifest/` as part of a CLAUDE.md / docs change.** If you
need the manifest reconciled to `spec.go`, do it as its own deliberate
change (and verify against `pkg/contractcheck`). When in doubt about the
live contract, trust `Describe()` in `spec.go`, not the manifest YAML.

## Non-negotiable rules

- **Standalone.** No imports of yggdrasil-core / monorepo runtime types.
  The wire types live locally in `family/contract/types.go`.
- **`describe` stays aligned with `execute`.** `pkg/contractcheck`
  enforces this in CI (`contractcheck_test.go`, `spec_test.go`); don't
  silence it.
- **Rename/add a capability → update `spec.go`, tests, docs, and (as a
  separate deliberate step) the manifest in the same effort.**
- **Fail fast.** No swallowing signature/verify errors, no silent
  emit-loss — emit failures increment `StripeRTAEmitErrors` and log.
- **Runtime behavior only.** Business authority stays in yggdrasil-core.

## Validation

```bash
go test ./...        # unit + integration (webhook flow, hmac, contractcheck)
task config          # render/validate manifest + quickstart
task build:image
task up / task down  # local stack via docker-compose
```
