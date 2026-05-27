# Staging Runbook — integration-stripe v1.0.0

Verifies the end-to-end webhook reactor against real Stripe test mode in
staging. Acceptance: 100 events delivered → DLQ stays at 0.

## Pre-flight

- [ ] Integration type + dakasa instance registered (Task 42).
- [ ] Stripe dashboard staging webhook endpoint configured: `https://staging.dakasa.io/webhooks/stripe/dakasa`.
- [ ] `enterprise-payments-api` staging is consuming new RTA routing keys.
- [ ] `webhook_event(provider, event_id) UNIQUE` constraint present in staging DB.

## Execution

```bash
# 50 payment_intent.succeeded
for i in $(seq 1 50); do stripe trigger payment_intent.succeeded; done

# 25 charge.refunded
for i in $(seq 1 25); do stripe trigger charge.refunded; done

# 25 customer.subscription.deleted
for i in $(seq 1 25); do stripe trigger customer.subscription.deleted; done
```

## Verification

```bash
# 1. Adapter received 100
curl -s http://staging-stripe-adapter:8080/metrics \
  | awk '/stripe_webhook_received_total/ {sum+=$NF} END {print sum}'   # expect 100

# 2. Zero signature failures
curl -s http://staging-stripe-adapter:8080/metrics \
  | grep stripe_webhook_signature_failures_total   # expect 0

# 3. RTA emit success = 100
curl -s http://staging-stripe-adapter:8080/metrics \
  | awk '/stripe_rta_emit_total/ {sum+=$NF} END {print sum}'   # expect 100

# 4. DLQ depth on the enterprise-payments-api consumer queues
yggdrasil workflow run rabbitmq_describe_topology --vhost dakasa | jq '.queues[] | select(.name | endswith(".dlq")) | {name, messages}'
# expect every dlq messages=0
```

## Rollback

```bash
# Revert ingress binding to integration-webhooks-external Stripe provider.
yggdrasil workflow run put_file_contents \
  --repo dakasa-co/dakasa-system \
  --path deploy/overlays/production/stripe-ingress-binding.yaml \
  --content "$(cat manifests/stripe-binding-old.yaml)"
```

Stripe retries failed events for 72h; no data loss expected for short rollback windows.
