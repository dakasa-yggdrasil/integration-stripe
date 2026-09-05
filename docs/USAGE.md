# Usage

End-to-end journey for `integration-stripe`: install the adapter, register the
integration type, configure a per-tenant instance, run a workflow that calls a
real capability, and verify the run.

> New to Yggdrasil? Start at
> [yggdrasil-core](https://github.com/dakasa-yggdrasil/yggdrasil-core). This
> adapter is the Stripe plug — `yggdrasil-core` drives it over HTTP or AMQP RPC
> and receives Stripe webhook events back as workflow triggers.

See also: [Configuration](CONFIGURATION.md) · [Capabilities](CAPABILITIES.md) ·
[Operations](OPERATIONS.md) · [Development](DEVELOPMENT.md) ·
[back to README](../README.md).

---

## 1. Run the adapter

### Local (stripe-mock, no real account)

```bash
docker compose up --build
```

This starts two containers:

- `stripe-mock` — Stripe's official mock server on `:12111`.
- `adapter` — the integration, wired to the mock via `STRIPE_API_BASE`, with a
  single-tenant `STRIPE_API_KEY=sk_test_compose`.

The adapter exposes three listeners (see [Operations](OPERATIONS.md) for ports):

| Port | Purpose | Routes |
|---|---|---|
| `8081` | RPC (SDK) | `/rpc/describe`, `/rpc/execute` |
| `8082` | Webhook receiver | `/webhooks/stripe/{instance_id}` |
| `8080` | Health / metrics | `/healthz`, `/readyz`, `/metrics` |

### Production image

```
ghcr.io/dakasa-yggdrasil/integration-stripe:v3.0.1
```

Published by `.github/workflows/release.yml` on tag push (`v*`) and on pushes to
`main` (`edge`, `branch-main-latest`, `sha-<short>`).

## 2. Confirm the describe handshake

Before every execute, `yggdrasil-core` calls `/rpc/describe` to verify the
adapter version against the stored `integration_type` manifest. You can call it
directly:

```bash
curl -s localhost:8081/rpc/describe | jq '{
  provider,
  version: .adapter.version,
  transport: .adapter.transport,
  capabilities,
  resource_types: [.resource_types[].name],
  action_count: (.action_catalog | length)
}'
```

Expected in the default HTTP mode: `provider: "stripe"`, version `3.0.1`,
`transport: "http_json"`,
`capabilities: ["describe","execute"]`, 10 resource types, and 22
action-catalog entries (20 capabilities + 2 framework reactors).

## 3. Register the integration type

Apply the `integration_type` manifest into core (the canonical source is
[`manifest/integration_type.stripe.yaml`](../manifest/integration_type.stripe.yaml)):

```bash
yggdrasil apply -f manifest/integration_type.stripe.yaml
```

This registers the `stripe` type in the `global` namespace, including its
credential schema, instance schema, 10 resource types, and the action catalog.

## 4. Configure a per-tenant instance

Each Stripe account is one `integration_instance`. Secrets are referenced (not
inlined) via `credentials_ref` so they stay in your secret store.

```yaml
apiVersion: yggdrasil.io/v1alpha1
kind: integration_instance
metadata:
  name: integration-stripe-dakasa
  namespace: dakasa
spec:
  type_ref: { namespace: global, name: stripe }
  config:
    stripe_account_id: ""                  # set acct_* for Stripe Connect
    stripe_api_version: "2025-10-29.clover"
    webhook_tolerance_seconds: 300
  credentials_ref:
    secret_name: dakasa-stripe-credentials
    keys:
      stripe_api_key: api_key
      stripe_webhook_secret: webhook_secret
```

```bash
yggdrasil apply -f instance-dakasa.yaml
```

A second tenant just gets its own instance + secret — see the two-tenant sample
in [`manifest/instance.stripe.yaml`](../manifest/instance.stripe.yaml)
(`dakasa` + `acme-corp`, the latter scoped to Connect account `acct_1AcmeXYZ`).

## 5. Run a workflow

Resource operations use the canonical `ensure_/observe_/destroy_` names. This
workflow ensures a customer, then creates and confirms a PaymentIntent:

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
        amount: 4990              # smallest currency unit (cents)
        currency: "brl"
        customer: "{{ steps.customer.output.customer_id }}"
        payment_method_types: ["card"]
        confirm: true
```

```bash
yggdrasil apply -f workflow.yaml
yggdrasil workflow run stripe-charge-customer --namespace dakasa
```

`ensure_customer` returns `customer_id`, `email`, `created`, `updated`.
`ensure_payment_intent` returns `payment_intent_id`, `client_secret`, `status`,
`amount`, `currency`, and `next_action` (populated when 3DS/SCA is required).
Full IO schemas: [Capabilities](CAPABILITIES.md).

## 6. Wire up webhooks

Treat a provider-side endpoint and its signing secret as one resource graph:

1. Call `observe_webhook_endpoints` and match the exact destination URL.
2. If exactly one endpoint exists, call `ensure_webhook_endpoint` with its
   exact event allowlist and desired `disabled` state. Ensure adopts or updates
   only and never returns a secret.
3. If none exists, use `provision_webhook_endpoint` only inside a compatible
   Core workflow whose immediately following step is a
   `secrets-management/ensure_secret` sink. The integration instance must also
   set `allow_sensitive_webhook_endpoint_creation=true` and a stable
   `webhook_endpoint_provisioning_generation` for this attempt. Core keeps
   `secret` transient, sends it only to that sink, and redacts the persisted run.
   Pass `connect` explicitly and pin `api_version: 2025-10-29.clover`.
4. Return the instance opt-in to `false`, observe the exact endpoint, and run a
   provider-signed callback canary before declaring the integration ready.
5. For a Yggdrasil-owned reactor destination, Stripe POSTs to
   `/webhooks/stripe/{instance_id}` and the adapter emits an RTA event. For an
   application backend such as DaKasa payments, provision that backend's exact
   public route instead. Do not substitute the adapter reactor URL for the
   business receiver.

Do not run the create action from a direct CLI call or a Core that lacks the
transient sink contract. Do not copy the create-only secret through workflow
inputs, logs, events, or normal observation output. Do not supply a custom
`idempotency_key`; the adapter owns the deterministic key for this create-only
operation.

If provider creation completed but the response was lost before persistence,
do not guess or retry after an unbounded delay. Observe the exact endpoint,
destroy that newly created endpoint before application traffic is launched,
change `webhook_endpoint_provisioning_generation`, and run a fresh adjacent
producer/sink workflow. Reusing the generation may replay the cached response
for the deleted endpoint. Stripe creates webhook endpoints enabled, so the
receiving route must already be deployed.

## 7. Verify the run

```bash
# Adapter-side: count webhook deliveries + RTA emits
curl -s localhost:8080/metrics | grep -E 'stripe_(webhook_received|rta_emit)_total'

# Signature failures should be zero
curl -s localhost:8080/metrics | grep stripe_webhook_signature_failures_total

# Execute latency / volume
curl -s localhost:8080/metrics | grep stripe_execute_requests_total
```

For the full staging acceptance procedure (100 events → DLQ stays 0), see the
[staging runbook](../RUNBOOK.staging.md) and [Operations](OPERATIONS.md).
