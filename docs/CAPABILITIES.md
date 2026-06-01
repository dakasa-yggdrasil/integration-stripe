# Capabilities

`integration-stripe` declares **19 executable capabilities + 1 webhook
reactor** across 10 managed resource types. Input/output schemas below are
pulled from [`manifest/capability.*.yaml`](../manifest) and
[`manifest/reactor.stripe_webhook_received.yaml`](../manifest/reactor.stripe_webhook_received.yaml);
the action catalog and resource→action mapping come from
[`providers/stripe/adapter/spec.go`](../providers/stripe/adapter/spec.go).

[back to README](../README.md) · [Usage](USAGE.md) · [Configuration](CONFIGURATION.md) · [Operations](OPERATIONS.md)

---

## Naming convention

Resource operations follow the Yggdrasil universal capability convention:

- `ensure_*` — create-or-update toward a desired state (idempotent).
- `observe_*` — read; `{id}` filter → single record, otherwise a paginated list
  (`{items, has_more}`).
- `destroy_*` — delete; `404` is treated as already-absent success.

Money-movement (`create_refund`, `create_payout`) and helper/state-transition
ops (`create_setup_intent`, `manage_connect_account`, `verify_webhook_signature`)
are kept as allowlisted action-shaped capabilities. All execute ops are marked
idempotent.

### Legacy aliases

Pre-v2 names resolve to canonical ops via a shim
([`spec.go::legacyOperationAliases`](../providers/stripe/adapter/spec.go) +
`reconcile.WithLegacyNames`), logging a `WARN`. Removal target: **v3.0.0**.

| Legacy name | Canonical |
|---|---|
| `create_payment_intent`, `confirm_payment_intent` | `ensure_payment_intent` (confirm via `{confirm:true}`) |
| `cancel_payment_intent` | `destroy_payment_intent` |
| `retrieve_payment_intent` | `observe_payment_intents` |
| `create_customer`, `update_customer` | `ensure_customer` |
| `list_customers` | `observe_customers` |
| `create_subscription`, `update_subscription` | `ensure_subscription` |
| `cancel_subscription` | `destroy_subscription` |
| `list_subscriptions` | `observe_subscriptions` |
| `list_charges` | `observe_charges` |
| `retrieve_balance` | `observe_balance` |
| `create_/update_webhook_endpoint` | `ensure_webhook_endpoint` |
| `delete_webhook_endpoint` | `destroy_webhook_endpoint` |
| `list_webhook_endpoints` | `observe_webhook_endpoints` |

---

## Resource type: `payment_intent`

Canonical prefix `thirdparty.stripe.payment_intent` · identity
`payment_intent.{id}` · default actions: ensure / observe / destroy.

### `ensure_payment_intent`
Ensure a PaymentIntent exists. Collapses create + confirm: when
`payment_intent_id` is set the call follows the confirm path; otherwise it
creates (with `confirm=true` as create-then-confirm shorthand). Idempotency key
derived from input or `sha256(amount+currency+customer)`.

| Inputs | Outputs |
|---|---|
| `payment_intent_id`, `amount` (int), `currency` (ISO-4217 lowercase), `customer`, `payment_method`, `payment_method_types` (array, default `["card"]`), `capture_method` (default `automatic`), `confirm` (bool, default `false`), `setup_future_usage`, `return_url`, `metadata` (obj), `stripe_account`, `idempotency_key` | `payment_intent_id`, `client_secret`, `status`, `amount` (int), `currency`, `next_action` (obj) |

### `observe_payment_intents`
Filter `{id}` → single record (`GET /v1/payment_intents/{id}`); otherwise
paginated list.

| Inputs | Outputs |
|---|---|
| `id`, `customer`, `limit` (int, default `10`), `stripe_account` | `items` (array), `has_more` (bool) |

### `destroy_payment_intent`
`POST /v1/payment_intents/{id}/cancel`. `404` → already-absent success.

| Inputs | Outputs |
|---|---|
| `payment_intent_id`, `ref` (SDK Destroy alias), `cancellation_reason` (enum: `duplicate`, `fraudulent`, `requested_by_customer`, `abandoned`), `stripe_account` | `payment_intent_id`, `status`, `cancellation_reason` |

---

## Resource type: `customer`

Canonical prefix `thirdparty.stripe.customer` · identity `customer.{id}` ·
default actions: ensure / observe / destroy.

### `ensure_customer`
POST new when absent, PATCH deltas when present. Idempotency key defaults to
`create_customer_<email>`.

| Inputs | Outputs |
|---|---|
| `customer_id` (set → PATCH path), `email`, `name`, `phone`, `metadata` (obj), `stripe_account`, `idempotency_key` | `customer_id`, `email`, `created` (int), `updated` (bool) |

### `observe_customers`
`{id}` → single; `{email}` → list-by-email; else paginated list.

| Inputs | Outputs |
|---|---|
| `id`, `email`, `limit` (int, default `10`), `stripe_account` | `items` (array), `has_more` (bool) |

### `destroy_customer`
`DELETE /v1/customers/{id}`. `404` → already-absent success.

| Inputs | Outputs |
|---|---|
| `customer_id`, `ref` (Destroy alias), `stripe_account` | `customer_id`, `deleted` (bool) |

---

## Resource type: `subscription`

Canonical prefix `thirdparty.stripe.subscription` · identity
`subscription.{id}` · default actions: ensure / observe / destroy.

### `ensure_subscription`
POST when absent, PATCH deltas when present. Defaults `payment_behavior` to
`default_incomplete` (SCA-friendly).

| Inputs | Outputs |
|---|---|
| `subscription_id` (set → PATCH), `customer`, `items` (array of obj), `payment_behavior` (default `default_incomplete`), `trial_end` (int, unix), `cancel_at_period_end` (bool), `metadata` (obj), `stripe_account`, `idempotency_key` | `subscription_id`, `status`, `latest_invoice`, `cancel_at_period_end` (bool), `canceled_at` (int) |

### `observe_subscriptions`
`{id}` → single; `{customer}` → list-by-customer; else paginated list.

| Inputs | Outputs |
|---|---|
| `id`, `customer`, `limit` (int, default `10`), `stripe_account` | `items` (array), `has_more` (bool) |

### `destroy_subscription`
`DELETE /v1/subscriptions/{id}` immediate; `{cancel_at_period_end:true}` for the
graceful update path. `404` → already-absent success.

| Inputs | Outputs |
|---|---|
| `subscription_id`, `ref` (Destroy alias), `cancel_at_period_end` (bool, default `false`), `stripe_account` | `subscription_id`, `status`, `cancel_at_period_end` (bool), `canceled_at` (int) |

---

## Resource type: `charge`

Canonical prefix `thirdparty.stripe.charge` · identity `charge.{id}` ·
default actions: `observe_charges`, `create_refund`.

### `observe_charges`
Read-only. Filter by customer or payment_intent, with cursor pagination.

| Inputs | Outputs |
|---|---|
| `customer`, `payment_intent`, `limit` (int, default `10`), `starting_after`, `stripe_account` | `items` (array), `has_more` (bool) |

---

## Resource type: `refund`

Canonical prefix `thirdparty.stripe.refund` · identity `refund.{id}` ·
default action: `create_refund`.

### `create_refund`
Money-movement action (allowlisted — not collapsed into `ensure_refund`).
Refund a charge or PaymentIntent partially or fully.

| Inputs | Outputs |
|---|---|
| `charge` (or `payment_intent` — one required), `amount` (int, omit for full), `reason`, `metadata` (obj), `stripe_account`, `idempotency_key` | `refund_id`, `status`, `amount` (int), `charge` |

---

## Resource type: `balance`

Canonical prefix `thirdparty.stripe.balance` · identity `balance.{account}` ·
default action: `observe_balance`.

### `observe_balance`
`GET /v1/balance`. The Balance object is a singleton per account (no list).

| Inputs | Outputs |
|---|---|
| `stripe_account` | `available` (array), `pending` (array) |

---

## Resource type: `webhook_endpoint`

Canonical prefix `thirdparty.stripe.webhook_endpoint` · identity
`webhook_endpoint.{id}` · default actions: ensure / observe / destroy +
`verify_webhook_signature`.

### `ensure_webhook_endpoint`
POST when absent, PATCH (URL / `enabled_events`) when present.

| Inputs | Outputs |
|---|---|
| `id` (set → PATCH), `url`, `enabled_events` (array, default `["*"]`), `description`, `metadata` (obj), `stripe_account`, `idempotency_key` | `id`, `url`, `status`, `enabled_events` (array), `secret` (returned on create only) |

### `observe_webhook_endpoints`
`{id}` → single; otherwise paginated list.

| Inputs | Outputs |
|---|---|
| `id`, `limit` (int, default `10`), `stripe_account` | `items` (array), `has_more` (bool) |

### `destroy_webhook_endpoint`
`DELETE /v1/webhook_endpoints/{id}`. `404` → already-absent success.

| Inputs | Outputs |
|---|---|
| `id`, `ref` (Destroy alias), `stripe_account` | `id`, `deleted` (bool) |

### `verify_webhook_signature`
Standalone HMAC-SHA256 verification of a `Stripe-Signature` header. Read-only
helper (allowlisted).

| Inputs | Outputs |
|---|---|
| `payload` (string, required), `stripe_signature` (string, required), `endpoint_secret`, `tolerance_seconds` (int, default `300`) | `valid` (bool), `event_id`, `event_type`, `timestamp` (int) |

---

## Resource type: `setup_intent`

Canonical prefix `thirdparty.stripe.setup_intent` · identity
`setup_intent.{id}` · default action: `create_setup_intent`.

### `create_setup_intent`
Create a SetupIntent to save a payment method for future use.

| Inputs | Outputs |
|---|---|
| `customer`, `payment_method`, `payment_method_types` (array, default `["card"]`), `usage` (default `off_session`), `metadata` (obj), `stripe_account`, `idempotency_key` | `setup_intent_id`, `client_secret`, `status` |

---

## Resource type: `payout`

Canonical prefix `thirdparty.stripe.payout` · identity `payout.{id}` ·
default action: `create_payout`.

### `create_payout`
Money-movement action (allowlisted). Create a payout to a bank account.

| Inputs | Outputs |
|---|---|
| `amount` (int, required), `currency` (string, required), `method` (default `standard`), `stripe_account` (required for Connect), `metadata` (obj), `idempotency_key` | `payout_id`, `status`, `arrival_date` (int), `method` |

---

## Resource type: `connect_account`

Canonical prefix `thirdparty.stripe.connect_account` · identity
`connect_account.{id}` · default action: `manage_connect_account`.

### `manage_connect_account`
Marketplace/Connect operation. Phase 1 supports `create` / `get` / `update` of
Express/Custom accounts; any other operation returns `unsupported_operation`.
Implemented in
[`providers/stripe/adapter/connect.go`](../providers/stripe/adapter/connect.go).

| Inputs | Outputs |
|---|---|
| `operation` (string, required, enum `create`/`get`/`update`), `account_id` (required for get/update), `type` (default `express`), `country` (default `BR`), `email`, `capabilities` (array), `metadata` (obj) | `account_id`, `type`, `country`, `charges_enabled` (bool), `payouts_enabled` (bool), `details_submitted` (bool) |

---

## Reactor: `stripe_webhook_received`

**Framework-invoked — NOT dispatchable via `execute`.** Inbound Stripe delivery
at `POST /webhooks/stripe/{instance_id}`. HMAC-SHA256 verified; payload routed
to an RTA key via `eventTypeToRTAKey()`. Source:
[`webhook_server.go`](../providers/stripe/adapter/webhook_server.go),
[`event_router.go`](../providers/stripe/adapter/event_router.go).

| Inputs | Outputs (RTA envelope) |
|---|---|
| _(none — invoked by the webhook server)_ | `routing_key`, `instance_id`, `stripe_event_id`, `event_type`, `livemode` (bool), `payload` (obj) |

### Event-type → routing-key map

18 known Stripe event types route to a specific key; anything else falls into
the catch-all `rta.stripe.unhandled_event`.

| Stripe event type | RTA routing key |
|---|---|
| `payment_intent.succeeded` | `rta.payments.intent_succeeded` |
| `payment_intent.payment_failed` | `rta.payments.intent_failed` |
| `payment_intent.canceled` | `rta.payments.intent_canceled` |
| `payment_intent.requires_action` | `rta.payments.intent_requires_action` |
| `charge.refunded` | `rta.payments.refunded` |
| `charge.dispute.created` | `rta.payments.dispute_created` |
| `charge.dispute.closed` | `rta.payments.dispute_closed` |
| `invoice.paid` | `rta.payments.invoice_paid` |
| `invoice.payment_failed` | `rta.payments.invoice_failed` |
| `balance.available` | `rta.payments.balance_available` |
| `payout.paid` | `rta.payments.payout_paid` |
| `payout.failed` | `rta.payments.payout_failed` |
| `payout.reconciliation_completed` | `rta.payments.payout_reconciliation_completed` |
| `customer.subscription.deleted` | `rta.subscriptions.cancelled` |
| `customer.subscription.updated` | `rta.subscriptions.updated` |
| `customer.subscription.trial_will_end` | `rta.subscriptions.trial_ending` |
| `account.updated` | `rta.connect.account_updated` |
| `account.application.deauthorized` | `rta.connect.deauthorized` |
| _(any other)_ | `rta.stripe.unhandled_event` |
