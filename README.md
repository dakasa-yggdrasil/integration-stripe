<div align="center">

# integration-stripe

**Yggdrasil integration adapter for [Stripe](https://stripe.com) — multi-tenant
payments, subscriptions, Connect, and HMAC-verified webhooks.**

[![ci](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/ci.yml/badge.svg)](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/ci.yml)
[![release](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/release.yml/badge.svg)](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/release.yml)
![go](https://img.shields.io/badge/go-1.25-00ADD8)
![license](https://img.shields.io/badge/license-Apache--2.0-blue)

A single Go binary that turns Stripe into a declarative Yggdrasil integration:
`ensure_/observe_/destroy_` capabilities + a webhook reactor.
· [Usage](docs/USAGE.md) · [Configuration](docs/CONFIGURATION.md) · [Capabilities](docs/CAPABILITIES.md) · [Operations](docs/OPERATIONS.md) · [Development](docs/DEVELOPMENT.md)

</div>

---

## What it is

`integration-stripe` is a leaf adapter in the **Yggdrasil** ecosystem —
[Yggdrasil is a self-hosted control plane for declarative workflows +
integrations over your whole stack](https://github.com/dakasa-yggdrasil/yggdrasil-core)
(think *Backstage, but more complete and scalable*: an orchestration engine +
versioned manifest catalog + RBAC/policy + OAuth/OIDC + a pluggable integration
ecosystem). You write YAML; Yggdrasil persists, runs, and audits it.

This adapter is the Stripe plug. `yggdrasil-core` speaks to it over
`http_json` RPC (`/rpc/describe`, `/rpc/execute`); the adapter translates
Yggdrasil capabilities into Stripe API calls (via `stripe-go/v83`) and pushes
Stripe webhook deliveries back into core as workflow-triggering events. It is
**multi-tenant** by design: one Stripe account = one `integration_instance`,
each with its own API key and webhook signing secret.

## Where it fits

```mermaid
flowchart LR
  subgraph YG["Yggdrasil control plane"]
    core["yggdrasil-core<br/>(workflows · catalog · events)"]
  end
  core -- "http_json RPC<br/>/rpc/describe · /rpc/execute" --> ad["integration-stripe<br/>(this adapter)"]
  ad -- "stripe-go/v83<br/>REST" --> stripe["Stripe API"]
  stripe -- "POST /webhooks/stripe/{instance_id}<br/>HMAC t=,v1=" --> ad
  ad -- "RTA event envelope" --> core
```

See [docs/OPERATIONS.md](docs/OPERATIONS.md) for the full webhook sequence.

## Family → type → instance → provider

```mermaid
flowchart TD
  prov["provider: stripe<br/>domain: payments"]
  type["integration_type: stripe<br/>(global) · adapter http_json"]
  inst1["instance: integration-stripe-dakasa<br/>(ns dakasa)"]
  inst2["instance: integration-stripe-client-acme<br/>(ns acme-corp · Connect acct_1AcmeXYZ)"]
  type --> prov
  inst1 --> type
  inst2 --> type
  inst1 -. "secret: dakasa-stripe-credentials" .-> sec1["stripe_api_key · stripe_webhook_secret"]
  inst2 -. "secret: acme-stripe-credentials" .-> sec2["stripe_api_key · stripe_webhook_secret"]
```

## Capabilities

The adapter declares **21 executable operations + 1 inbound webhook reactor**
across 10 managed resource types. The catalog classifies 20 operations as
grantable capabilities and two as framework reactors (`on_surface_query` is
executable only through the framework). Resource operations follow the
Yggdrasil universal naming convention (`ensure_/observe_/destroy_`).

| Capability | Resource type | Category |
|---|---|---|
| `ensure_payment_intent` | `payment_intent` | capability |
| `observe_payment_intents` | `payment_intent` | capability |
| `destroy_payment_intent` | `payment_intent` | capability |
| `ensure_customer` | `customer` | capability |
| `observe_customers` | `customer` | capability |
| `destroy_customer` | `customer` | capability |
| `ensure_subscription` | `subscription` | capability |
| `observe_subscriptions` | `subscription` | capability |
| `destroy_subscription` | `subscription` | capability |
| `observe_charges` | `charge` | capability |
| `observe_balance` | `balance` | capability |
| `ensure_webhook_endpoint` | `webhook_endpoint` | capability |
| `provision_webhook_endpoint` | `webhook_endpoint` | capability (break glass) |
| `observe_webhook_endpoints` | `webhook_endpoint` | capability |
| `destroy_webhook_endpoint` | `webhook_endpoint` | capability |
| `create_refund` | `refund` | capability (money movement) |
| `create_setup_intent` | `setup_intent` | capability |
| `create_payout` | `payout` | capability (money movement) |
| `manage_connect_account` | `connect_account` | capability (Connect) |
| `verify_webhook_signature` | `webhook_endpoint` | capability (helper) |
| `on_surface_query` | `webhook_endpoint` | **reactor** (framework-invoked) |
| `stripe_webhook_received` | `webhook_endpoint` | **reactor** (framework-invoked) |

> Pre-v2 names (`create_payment_intent`, `confirm_payment_intent`,
> `cancel_subscription`, `list_charges`, …) still resolve through a legacy-alias
> shim that maps them to the canonical operation with a `WARN` log
> (removal target: v4.0.0). Full input/output schemas in
> [docs/CAPABILITIES.md](docs/CAPABILITIES.md).

## Quick start (local)

```bash
# Boot the adapter against stripe-mock (no real Stripe account needed):
docker compose up --build

# Adapter listens on three ports:
#   :8081  RPC      /rpc/describe  /rpc/execute
#   :8082  Webhook  /webhooks/stripe/{instance_id}
#   :8080  Health   /healthz  /readyz  /metrics

# Verify the describe handshake core uses to pin the adapter version:
curl -s localhost:8081/rpc/describe | jq '{provider, version: .adapter.version, caps: (.action_catalog | length)}'

# Run the test suite:
go test ./... -race
```

End-to-end walkthrough (install → configure an instance → run a workflow →
verify) lives in [docs/USAGE.md](docs/USAGE.md).

## Configuration

Credentials and instance settings are declared in the adapter's
`Describe()` schema and the `integration_type` manifest. Secrets are pulled from
a secret store via `credentials_ref` and never logged or round-tripped through
JSON.

| Field | Where | Type | Secret | Notes |
|---|---|---|---|---|
| `stripe_api_key` | credential | string | yes | `sk_live_*` / `rk_live_*`. Canonical. |
| `stripe_secret_key` | credential | string | yes | Alias for `stripe_api_key` (read only if the canonical is absent). |
| `stripe_webhook_secret` | credential / instance | string | yes | Webhook signing secret (`whsec_*`). |
| `stripe_account_id` | instance | string | no | Optional bound Connect account; callers cannot override it. |
| `stripe_api_version` | instance | string | no | Default `2025-10-29.clover`. |
| `webhook_tolerance_seconds` | instance | integer | no | Default `300`. HMAC timestamp window. |

Full field reference (incl. operator-metadata fields and runtime env vars) in
[docs/CONFIGURATION.md](docs/CONFIGURATION.md).

## Usage

A minimal workflow that ensures a Stripe customer exists and creates a
PaymentIntent (real capability names):

```yaml
apiVersion: yggdrasil.io/v1alpha1
kind: workflow
metadata:
  name: stripe-charge-customer
  namespace: dakasa
spec:
  steps:
    - id: customer
      capability: ensure_customer
      integration: { namespace: dakasa, name: integration-stripe-dakasa }
      input:
        email: "buyer@example.com"
        name: "Acme Buyer"
    - id: intent
      capability: ensure_payment_intent
      integration: { namespace: dakasa, name: integration-stripe-dakasa }
      input:
        amount: 4990            # cents
        currency: "brl"
        customer: "{{ steps.customer.output.customer_id }}"
        confirm: true
```

See [docs/USAGE.md](docs/USAGE.md) for the full journey.

## Webhooks & reactor

Stripe delivers events to `POST /webhooks/stripe/{instance_id}` on the webhook
port. The adapter verifies the `Stripe-Signature` header (HMAC-SHA256 over
`{t}.{body}`, comparing each `v1=` digest within the per-instance tolerance
window), deduplicates by `instance_id:event_id` (24h in-memory window), answers
`200` to Stripe **before** emitting, then maps the event type to an RTA routing
key and emits an envelope to `yggdrasil-core`.

```mermaid
sequenceDiagram
  participant S as Stripe
  participant W as Webhook server (:8082)
  participant R as Reactor (eventTypeToRTAKey)
  participant C as yggdrasil-core /api/v1/events
  participant F as Workflow

  S->>W: POST /webhooks/stripe/{instance_id}<br/>Stripe-Signature: t=...,v1=...
  W->>W: VerifySignature(body, header, secret, tolerance)
  W->>W: dedup check (instance_id:event_id, 24h)
  W-->>S: 200 OK (before emit)
  W->>R: route event_type → rta.* key
  R->>C: emit RTA envelope (routing_key, payload, livemode)
  C->>F: trigger subscribed workflow
```

The HMAC verification is implemented locally in
[`providers/stripe/adapter/hmac.go`](providers/stripe/adapter/hmac.go).
The event-type → routing-key map (18 known types + `rta.stripe.unhandled_event`
catch-all) is in
[`providers/stripe/adapter/event_router.go`](providers/stripe/adapter/event_router.go).
See [docs/OPERATIONS.md](docs/OPERATIONS.md) for the staging runbook.

## Operations

- `GET /healthz` — liveness (always `200`).
- `GET /readyz` — readiness (`200`).
- `GET /metrics` — 11 Prometheus series (request latency/errors, webhook
  received/dedup/signature-failures, RTA emit, execute latency, API-key-valid
  gauge, dedup-map size).

Details and troubleshooting in [docs/OPERATIONS.md](docs/OPERATIONS.md).

## Development

```bash
go test ./...          # unit + integration tests
go build ./cmd/adapter # build the binary
task up                # local stack via docker compose
```

Repo layout, the describe/execute contract, and `pkg/contractcheck` are
documented in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Compatibility

| Component | Version |
|---|---|
| `yggdrasil-sdk-go` | `v0.8.3` (`go.mod`) |
| Adapter version (`spec.go`) | `3.0.0` |
| `integration_type` manifest version | `3.0.0` |
| Stripe Go SDK | `stripe-go/v83 v83.1.0` |
| Pinned Stripe API version | `2025-10-29.clover` |
| Go | `1.25` |

> The adapter version constant, published-image manifest, and current changelog
> entry are aligned at `3.0.0`. The describe handshake reports `3.0.0`.

## License

Apache-2.0 — see [LICENSE](LICENSE).

---

*Part of [Yggdrasil](https://github.com/dakasa-yggdrasil/yggdrasil-core). Last
updated 2026-06-01.*
