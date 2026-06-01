# Configuration

Every configurable field of `integration-stripe`, derived from the adapter's
`Describe()` schema
([`providers/stripe/adapter/spec.go`](../providers/stripe/adapter/spec.go)), the
`integration_type` manifest
([`manifest/integration_type.stripe.yaml`](../manifest/integration_type.stripe.yaml)),
and the worker's env/config loaders
([`cmd/adapter/main.go`](../cmd/adapter/main.go),
[`providers/stripe/config/config.go`](../providers/stripe/config/config.go)).

[back to README](../README.md) · [Usage](USAGE.md) · [Capabilities](CAPABILITIES.md) · [Operations](OPERATIONS.md)

---

## Credential schema

Mode `inline`. No field is hard-required by the schema (`required: []`) — but
**at least one of `stripe_api_key` / `stripe_secret_key` must be supplied**;
the adapter reads the canonical name first and falls back to the alias.

| Field | Type | Secret | Required | Notes |
|---|---|---|---|---|
| `stripe_api_key` | string | yes | one-of | Canonical API key, `sk_live_*` / `rk_live_*`. Prefer restricted keys (`rk_`). |
| `stripe_secret_key` | string | yes | one-of | Alias for `stripe_api_key`. Accepted so secret stores that already name the value `stripe_secret_key` work without renaming. Read **only if** `stripe_api_key` is absent. |
| `stripe_webhook_secret` | string | yes | no | Webhook endpoint signing secret (`whsec_*`). Declared on **both** the credential and instance schemas. |

> **Manifest vs adapter:** the Go `Describe()` declares `stripe_api_key` +
> `stripe_secret_key` in the credential schema; the YAML manifest additionally
> lists `stripe_webhook_secret` under `credential_schema.properties`. The
> adapter's instance schema also declares `stripe_webhook_secret`. Either
> placement is accepted by core; the credential is resolved from the instance's
> `credentials_ref`.

## Instance schema

Mode `inline`. Fields the adapter reads at execute time, plus operator-metadata
fields declared so `yggdrasil-core`'s config validator does not reject the
instance config.

| Field | Type | Secret | Default | Consumed? | Notes |
|---|---|---|---|---|---|
| `stripe_account_id` | string | no | — | yes | Optional Stripe Connect account ID. Sets the `Stripe-Account` header on Connect calls. |
| `stripe_api_version` | string | no | `2024-12-18.acacia` | yes | Sent via the `Stripe-Version` header. |
| `stripe_webhook_secret` | string | yes | — | yes (webhook server) | Per-instance webhook signing secret (`whsec_*`). |
| `webhook_tolerance_seconds` | integer | no | `300` | yes (webhook server) | Max clock skew between the webhook `t=` timestamp and verification. |
| `base_url` | string (`uri`) | no | — | no | Operator-injected RPC base-URL override used by the validation harness. The adapter ignores it. |
| `environment` | string | no | — | no | Free-form tag (`sandbox`/`production`) for operator filtering. Ignored by the adapter. |
| `provider` | string | no | `stripe` | no | Provider-name echo, kept for instance ergonomics. Ignored by the adapter. |

> `base_url`, `environment`, and `provider` exist solely to open core's
> validator gate — core rejects undeclared `spec.config` fields. The canonical
> **adapter-side** RPC/Stripe base-URL override hook is the per-request config
> field `stripe_api_base_url` (read by `Execute()` from
> `integration.instance_spec.config`), not `base_url`.

## `credentials_ref` usage

Instances reference secrets rather than inlining them. The `keys` map binds
secret-store keys to schema fields:

```yaml
spec:
  credentials_ref:
    secret_name: dakasa-stripe-credentials
    keys:
      stripe_api_key: api_key            # schema field  ← secret key
      stripe_webhook_secret: webhook_secret
```

Secrets are stored as `json:"-"` in
[`config.InstanceConfig`](../providers/stripe/config/config.go) — they never
appear in JSON responses, logs, or RTA envelopes. The webhook server routes by
`instance_id` (the path segment of `/webhooks/stripe/{instance_id}`) and looks
up the per-instance webhook secret for HMAC verification.

## Runtime environment variables

Read by the worker entrypoint
([`cmd/adapter/main.go`](../cmd/adapter/main.go)),
config loader
([`config.go`](../providers/stripe/config/config.go)), and the reconcile-event
emitter ([`reconcile.go`](../providers/stripe/adapter/reconcile.go)).

| Variable | Default | Purpose |
|---|---|---|
| `ADAPTER_PORT` | `8081` | RPC listener (`/rpc/describe`, `/rpc/execute`). |
| `WEBHOOK_PORT` | `8082` | Inbound Stripe webhook receiver. |
| `HEALTHCHECK_PORT` | `8080` | `/healthz`, `/readyz`, `/metrics`. |
| `STRIPE_INSTANCES_CONFIG` | unset | Path to a multi-tenant instance JSON file (`{"instances":[{...}]}`). Used by the dakasa-system pod that injects the instance set via a ConfigMap/Secret. |
| `STRIPE_API_KEY` | unset | Single-tenant fallback API key (used when `STRIPE_INSTANCES_CONFIG` is unset). |
| `STRIPE_WEBHOOK_SECRET` | unset | Single-tenant fallback webhook signing secret. |
| `STRIPE_ACCOUNT_ID` | unset | Single-tenant fallback Stripe Connect account ID. |
| `STRIPE_INSTANCE_ID` | `default` | Instance ID assigned to the single-tenant fallback instance. |
| `STRIPE_API_BASE` | Stripe prod | Backend URL override (used by tests and `stripe-mock`). Consumed by the Stripe client builder. |
| `YGGDRASIL_CORE_URL` | unset | When set, the §6.5 mutation-event emitter posts to core's `/api/v1/events`; otherwise a no-op emitter is used. |
| `YGGDRASIL_RUN_TOKEN` | unset | Auth token paired with `YGGDRASIL_CORE_URL` for event emission. |

> The Lego principle: no broker / secret-store / cloud is hardcoded. The
> emitter target (`YGGDRASIL_CORE_URL`) and Stripe backend (`STRIPE_API_BASE` /
> `stripe_api_base_url`) are all env/config-driven, so the adapter does not
> couple to `api.stripe.com` or any specific core deployment.

### Multi-tenant instance JSON

When `STRIPE_INSTANCES_CONFIG` points at a file:

```json
{
  "instances": [
    {
      "instance_id": "dakasa",
      "stripe_api_key": "sk_live_...",
      "stripe_webhook_secret": "whsec_...",
      "stripe_account_id": "",
      "stripe_api_version": "2024-12-18.acacia",
      "webhook_tolerance_seconds": 300
    }
  ]
}
```

Entries without an `instance_id` are skipped. Unset `stripe_api_version`
defaults to `2024-12-18.acacia` and unset `webhook_tolerance_seconds` defaults
to `300` (`InstanceConfig.WithDefaults`).
