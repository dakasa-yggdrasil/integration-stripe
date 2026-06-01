# Development

Build, test, CI, repo layout, and the describe/execute contract for
`integration-stripe`.

[back to README](../README.md) · [Usage](USAGE.md) · [Configuration](CONFIGURATION.md) · [Capabilities](CAPABILITIES.md) · [Operations](OPERATIONS.md)

---

## Prerequisites

- Go **1.25** (`go.mod`).
- Docker (for `stripe-mock` and the container build).
- [`task`](https://taskfile.dev) (optional — for the `Taskfile.yml` shortcuts).

## Build & test

```bash
go mod download
go vet ./...
go test ./... -race          # unit + integration tests
go build ./cmd/adapter       # → ./adapter binary
```

Integration tests that hit the Stripe client use `stripe-mock`. The CI workflow
runs a `stripe-mock` service container and sets `STRIPE_API_BASE` +
`STRIPE_API_KEY=sk_test_ci`; locally, `docker compose up` provides the same
(`STRIPE_API_BASE=http://stripe-mock:12111`, `STRIPE_API_KEY=sk_test_compose`).

### Taskfile shortcuts

```bash
task test          # go test ./...
task build:image   # docker build (see note)
task up            # docker compose up --build -d
task down          # docker compose down --remove-orphans
task logs          # docker compose logs -f
```

> **Stale Taskfile bits (code-verified):** `task build:image` tags the image
> `integration-template:local` (carried over from the scaffold — the published
> image is `ghcr.io/dakasa-yggdrasil/integration-stripe`). `task up`/`config`
> reference `docker-compose.standalone.yml` and a `.env` (via `dotenv` + the
> `COMPOSE_ARGS` var) that are **not present** in this repo; only
> `docker-compose.yml` exists. Use `docker compose up --build` directly for the
> local stack until the Taskfile is reconciled.

## Container

`Dockerfile` is a two-stage build (`golang:1.25-bookworm` →
`distroless/base-debian12:nonroot`) producing `/app/integration-stripe`,
exposing `8080` (health), `8081` (RPC), `8082` (webhook). Build context must be
the repo root.

## Repo layout

```
cmd/adapter/main.go              # entrypoint: 3 listeners (RPC, webhook, health) + reconciler wiring
providers/stripe/
  adapter/                       # the provider implementation
    spec.go                      # Describe() contract + capability constants + legacy aliases
    adapter.go                   # Execute() switch (legacy/fallback path)
    client.go                    # stripe-go/v83 client builder (per-instance, base-URL override)
    connect.go                   # manage_connect_account (Connect Express/Custom)
    reconcile.go                 # SDK reconciler wiring (ensure/observe/destroy) + §6.5 emitter
    webhook_server.go            # /webhooks/stripe/{instance_id} reactor server
    hmac.go                      # local Stripe-Signature HMAC verification (t=, v1=)
    event_router.go              # event_type → RTA routing key map
    metrics.go                   # 11 Prometheus series
  config/config.go               # per-instance config loader (env + JSON file)
  message/                       # SDK-shaped RPC handlers
    describe.go                  # DescribeHandler (version handshake)
    execute.go                   # ExecuteHandler (reconcile.Dispatch bridge → Execute fallback)
    rpc.go                       # success/failure helpers, Handler type
family/contract/types.go         # local wire protocol types (no yggdrasil-core import)
pkg/contractcheck/               # PUBLIC describe-contract lint (importable by other adapters)
manifest/
  integration_type.stripe.yaml   # the type manifest (provider, schemas, resource_types, action_catalog)
  instance.stripe.yaml           # two-tenant instance sample (dakasa + acme-corp)
  capability.*.yaml              # 19 capability IO manifests
  reactor.stripe_webhook_received.yaml  # the webhook reactor manifest
integration_tests/webhook_test.go
testdata/stripe-events/          # sample Stripe event payloads for tests
RUNBOOK.staging.md               # staging acceptance runbook
```

## SDK dependencies

The adapter is built on `github.com/dakasa-yggdrasil/yggdrasil-sdk-go v0.8.3`
and uses these SDK packages (verified from imports):

| SDK package | Used for |
|---|---|
| `adapter` | `adapter.New(...)`, `ListenHTTP`, `Register`, signal handler. |
| `rpc` | `rpc.Delivery` envelope for the describe/execute handlers. |
| `sdk/reconcile` | `RegisterReconciler` + `reconcile.Dispatch` for ensure/observe/destroy routing and legacy-name shims. |
| `sdk/events` | `events.NewHTTPEmitter()` / `NoopEmitter{}` for §6.5 mutation-event auto-emission. |

> **Not used:** the SDK's `sig/hmac` and `webhookhttp` packages. This adapter
> ships its own webhook server (`webhook_server.go`) and its own HMAC
> verification (`hmac.go`). Stripe API calls go through `stripe-go/v83`
> directly.

## Describe / execute contract

- **`describe`** ([`message/describe.go`](../providers/stripe/message/describe.go))
  returns `adapter.Describe()`. Core calls it before every execute to verify the
  adapter version (`2.4.0`) and provider (`stripe`). A mismatched
  `expected_version` yields `version_mismatch`.
- **`execute`** ([`message/execute.go`](../providers/stripe/message/execute.go))
  first routes the envelope through `reconcile.Dispatch` (activating §6.5
  mutation-event emission for `ensure_/observe_/destroy_` ops). Operations with
  no registered reconciler — the allowlisted action helpers (`create_refund`,
  `create_setup_intent`, `create_payout`, `manage_connect_account`,
  `verify_webhook_signature`) — fall back to the legacy `adapter.Execute`
  switch. Per-request credentials (`stripe_api_key`/`stripe_secret_key`) and
  config are lifted from `integration.instance_spec` through the bridge.

### Contract drift lint (`pkg/contractcheck`)

`Describe()` is the single source of truth and three of its pieces —
`SupportedExecuteOperations`, `ResourceTypes`, and `ActionCatalog` — must stay
in sync or core rejects the adapter at registration with `version_mismatch` /
`action_catalog_mismatch`. [`pkg/contractcheck`](../pkg/contractcheck) is the
public, importable lint that catches this drift; it is exercised by the
`TestContractCheck` test and by other DaKasa adapters that import it.

**When you add or rename a capability, update all of these in the same change:**
the constant + `SupportedExecuteOperations` in `spec.go`, the resource type's
`DefaultActions`, the `ActionCatalog` entry, the matching
`manifest/capability.*.yaml`, the `integration_type.stripe.yaml`
`action_catalog`/`resource_types`, tests, and the docs in
[CAPABILITIES.md](CAPABILITIES.md).

## CI

`.github/workflows/`:

| Workflow | What it does |
|---|---|
| `ci.yml` | `go vet`, `go test ./... -race` (against a `stripe-mock` service container), `go build ./cmd/adapter`; a separate `contractcheck` job runs `TestContractCheck`. |
| `release.yml` | Builds + pushes `ghcr.io/dakasa-yggdrasil/integration-stripe` on tag push (`v*` → `v<version>` + `latest`) and on `main` push (`edge`, `branch-main-latest`, `sha-<short>`). Tags via `docker/metadata-action@v5`. |
| `emit-deploy-event.yml` | POSTs a deploy event into yggdrasil-core. |

## Compatibility & version notes

| Component | Version |
|---|---|
| `yggdrasil-sdk-go` | `v0.8.3` |
| Adapter version (`spec.go`) | `2.4.0` |
| `integration_type` manifest | `2.2.4` |
| Latest released `CHANGELOG` entry | `2.3.1` (plus an Unreleased section) |
| `stripe-go` | `v83 v83.1.0` |
| Stripe API version | `2024-12-18.acacia` |

> These three version sources (`spec.go` `AdapterVersion`, the manifest
> `spec.version`/`image_tag`, and the changelog) are **not aligned**. The
> describe handshake reports `2.4.0`; the published image manifest references
> `2.2.4`. Reconcile before cutting a release so the registered catalog row,
> the image tag, and the describe response agree.
