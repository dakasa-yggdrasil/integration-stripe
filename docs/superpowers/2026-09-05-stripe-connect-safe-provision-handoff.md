# Stripe Connect safe provisioning handoff

Date: 2026-09-05

Worktree: `/Users/dakasa/projects/dakasa/.codex-worktrees/integration-stripe-prod-webhook`

Branch: `codex/reconcile-production-webhook-20260905`

Base: `e400690`

## Decision

GO for review and release as integration-stripe 3.0.0 together with the Core
one-step lease and integration-secrets-management 1.3.2.

NO-GO for a provider POST until the production Payments Connect receiver is
deployed, the target secrets-management instance and its exact production IAM
write prefix are verified live, and the three released adapters pass the Core
handshake. No provider write was performed from this worktree.

## Contract completed

`ensure_webhook_endpoint` is adoption and reconciliation only. It can select
one existing endpoint by exact ID or exact URL, but it never creates an
endpoint or claims that an already lost signing secret can be recovered.

`provision_webhook_endpoint` is the only create path. It is non-idempotent and
requires both:

1. the instance opt-in `allow_sensitive_webhook_endpoint_creation=true`;
2. the exact Core-issued v1 transient next-step sink handshake.

It remains outside the SDK reconciler mutation event path, so the one-time
Stripe signing secret is not serialized into an automatic mutation event.
The response declares `sensitive_output_paths: [secret]` for the Core lease.
Ambiguous completion must be resolved by observe, explicit destroy if needed,
and a deliberate recreate. The operation is absent from every idempotent
operation list in code and manifest. Caller-supplied idempotency keys are
rejected; the adapter derives the key from canonical intent so identical
concurrent workflow calls cannot select distinct replay keys. The operator
must set one stable `webhook_endpoint_provisioning_generation` on the instance
for the attempt. After an explicit destroy, that generation must change before
recreation so Stripe cannot replay the cached response for the deleted endpoint.

The transport propagates the Core cancellation context into Stripe. Provider
errors for this operation are replaced before adapter logging and RPC output,
and every stripe-go backend uses a null SDK logger so malformed provider bodies
cannot leak a one-time value to stderr before that replacement.

## Scope and ownership rules

Provisioning requires an explicit `connect` boolean and the exact Stripe API
version `2025-10-29.clover`. Connect cannot be combined with a
`Stripe-Account` header. An instance-level connected account is validated as
an `acct_` identifier, injected into requests, and cannot be overridden by the
caller.

Created endpoints receive reserved provider metadata for the Yggdrasil scope
and instance identity. Adoption refuses ambiguous URL matches, mismatched
events, unprovable scope, mismatched integration ownership, account conflicts,
and attempts to replace reserved metadata. Adoption requires an explicit
`connect` assertion. Observation returns provider-backed scope, account, live
mode, API version, status, events, and metadata.

The intended production Connect endpoint has exactly these events:

- `invoice.paid`
- `invoice.payment_failed`
- `balance.available`
- `charge.dispute.created`
- `charge.dispute.closed`
- `charge.refunded`
- `payout.paid`
- `payout.reconciliation_completed`
- `payout.failed`
- `payout.canceled`

## Live read-only evidence

The production Stripe account currently exposes one live webhook endpoint,
`we_1TM7yfCd06GHzNk90JA3jIqw`, at the legacy URL
`https://enterprise.dakasa.me/payment/webhook/stripe`. It is disabled, has no
Yggdrasil ownership metadata, does not prove Connect scope, lacks
`payout.paid` and `payout.canceled`, and includes obsolete events. Its signing
secret cannot be proven to match the current stored secret.

Treat it as disabled legacy state. Do not enable, adopt, or mutate it for the
new path. The new endpoint target is
`https://enterprise.dakasa.me/payment/webhook/stripe/connect`.

Do not point this DaKasa release at the adapter's generic
`/webhooks/stripe/{instance_id}` reactor. That separate path does not yet carry
the top-level Stripe `event.account` through its RTA envelope. Payments
`/payment/webhook/stripe/connect` preserves and validates it before side
effects.

The production platform credential was read-only validated against Stripe and
the three declared connected accounts were accessible. No credential or
signing secret value is recorded here.

## Validation

Passed before handoff:

```text
GOWORK=off go test ./... -race -count=1
GOWORK=off go vet ./...
git diff --check
```

`task config` is not a valid gate on the current main branch because the task
references a missing `.env.example`; this is pre-existing repository debt.

## Ordered rollout

1. Deploy the Payments Connect route with no provider endpoint yet.
2. Release and deploy the Core lease, this adapter, and
   integration-secrets-management 1.3.2.
3. Verify the live Stripe instance is platform scoped, has a unique stable
   provisioning generation for this attempt, and the live secrets instance
   resolves to family `secrets-management` with `ensure_secret`.
4. Verify exact-prefix IAM write access for
   `dakasa/production/webhooks/stripe-connect-signing-secret` in `us-east-1`.
5. Temporarily enable the Stripe instance opt-in, run the workflow once, then
   immediately disable the opt-in.
6. Project the dedicated standalone secret into Payments and perform a
   RollingUpdate.
7. Observe the new endpoint and require exact URL, ten events, live mode,
   Connect scope metadata, and enabled status.
8. Send a provider-signed test from Stripe and require HTTP 2xx plus expected
   application telemetry. A locally forged signature is not sufficient.
9. Leave the disabled legacy endpoint untouched until the new delivery path is
   proven. Its later deletion is a separate explicit provider action. Any
   recovery recreation after destroy requires a new provisioning generation.
