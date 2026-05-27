# integration-stripe

[![ci](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/ci.yml/badge.svg)](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/ci.yml)
[![release](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/release.yml/badge.svg)](https://github.com/dakasa-yggdrasil/integration-stripe/actions/workflows/release.yml)

Community-grade Yggdrasil integration adapter for the [Stripe](https://stripe.com)
global payments API. Built on `yggdrasil-sdk-go` v0.4.0; runs as a single
Go binary with three HTTP listeners (RPC, inbound webhook receiver,
health/metrics).

## Capabilities (14 total)

13 executable + 1 reactor:

| Capability                  | Description                                                                   |
|-----------------------------|-------------------------------------------------------------------------------|
| `create_payment_intent`     | Create a PaymentIntent for one-time payments.                                 |
| `confirm_payment_intent`    | Confirm an existing PaymentIntent; surfaces `next_action` for 3DS.            |
| `cancel_payment_intent`     | Cancel a PaymentIntent. Idempotent.                                           |
| `create_customer`           | Create a Customer.                                                            |
| `update_customer`           | Update Customer fields (email/name/phone/metadata).                           |
| `create_subscription`       | Create a Subscription with `payment_behavior=default_incomplete`.             |
| `cancel_subscription`       | Cancel immediately (DELETE) or at period end (POST update).                   |
| `create_refund`             | Refund a charge or PaymentIntent partially or fully.                          |
| `create_setup_intent`       | Create a SetupIntent to save a payment method (`usage=off_session` default).  |
| `list_charges`              | List charges by customer or PaymentIntent (read-only).                        |
| `create_payout`             | Create a payout to a bank account (Connect-aware).                            |
| `manage_connect_account`    | Phase 1: create / get / update Stripe Connect Express/Custom accounts.        |
| `verify_webhook_signature`  | Standalone HMAC SHA256 verification of a Stripe-Signature header.             |
| `stripe_webhook_received`*  | Reactor — receives inbound Stripe webhook deliveries.                         |

\* Reactor; framework-invoked via `POST /webhooks/stripe/{instance_id}`. Not dispatchable via `execute`.

## Multi-tenant

Each Stripe account = one `integration_instance`. The webhook receiver
routes by `/webhooks/stripe/{instance_id}` path segment and looks up
secrets from the per-instance config. See
[`manifest/instance.stripe.yaml`](manifest/instance.stripe.yaml) for a
two-tenant sample (`dakasa` + `client-acme-corp`).

## Stripe API version

Pinned at **`2024-12-18.acacia`** as a constant in
`providers/stripe/adapter/spec.go`. Bumping requires a full integration
test cycle plus an adapter version bump.

## Quickstart

```bash
# Local dev with stripe-mock:
docker compose up

# Run unit + integration tests:
go test ./... -race

# Build the binary:
go build ./cmd/adapter
```

Environment variables consumed by `cmd/adapter`:

| Var                          | Default       | Purpose                                                |
|------------------------------|---------------|--------------------------------------------------------|
| `ADAPTER_PORT`               | `8081`        | SDK RPC listener (`/rpc/describe`, `/rpc/execute`).    |
| `WEBHOOK_PORT`               | `8082`        | Inbound Stripe webhook receiver.                       |
| `HEALTHCHECK_PORT`           | `8080`        | `/healthz`, `/readyz`, `/metrics`.                     |
| `STRIPE_INSTANCES_CONFIG`    | unset         | Path to multi-tenant instance JSON.                    |
| `STRIPE_API_KEY`             | unset         | Single-tenant fallback API key.                        |
| `STRIPE_WEBHOOK_SECRET`      | unset         | Single-tenant fallback webhook signing secret.         |
| `STRIPE_ACCOUNT_ID`          | unset         | Single-tenant fallback Stripe Connect account.         |
| `STRIPE_API_BASE`            | Stripe prod   | Backend URL override (used by tests and `stripe-mock`).|

## Observability

Adapter exposes 11 Prometheus series at `/metrics` on the health port:

```
stripe_request_duration_seconds        stripe_webhook_received_total
stripe_request_errors_total            stripe_webhook_signature_failures_total
stripe_webhook_dedup_total             stripe_rta_emit_total
stripe_rta_emit_errors_total           stripe_execute_requests_total
stripe_execute_duration_seconds        stripe_api_key_valid
stripe_dedup_map_size
```

## Event routing

Inbound Stripe events are routed to RTA keys by
[`providers/stripe/adapter/event_router.go`](providers/stripe/adapter/event_router.go).
18 known event types + 1 catch-all `rta.stripe.unhandled_event`.

## License

Apache 2.0 — see [LICENSE](LICENSE).
