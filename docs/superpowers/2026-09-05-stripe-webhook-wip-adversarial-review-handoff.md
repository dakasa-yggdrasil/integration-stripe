# Stripe production webhook WIP: adversarial review handoff

Date: 2026-09-05  
Review target: `integration-stripe` worktree `codex/reconcile-production-webhook-20260905`  
Baseline commit: `e4006900a5390768481c13fbe3bfc124915a5d58` plus the uncommitted WIP listed below  
Review mode: read-only; this handoff is the only file created by the review

## 1. Safety and provenance

- No Stripe, AWS, Kubernetes, Yggdrasil, database, RabbitMQ, GitHub, or other
  external mutation was performed.
- No provider credential or secret value was read.
- No implementation file was changed, committed, pushed, or dispatched.
- The Stripe worktree was already dirty. Its pre-review WIP contained 15 modified
  tracked files and two untracked implementation files:
  `manifest/capability.provision_webhook_endpoint.yaml` and
  `providers/stripe/adapter/webhook_endpoint_test.go`.
- Sources were read from these local snapshots:
  - Stripe WIP: this worktree.
  - Core sensitive-output snapshot:
    `/Users/dakasa/projects/dakasa/.codex-worktrees/yggdrasil-core-sensitive-output-contract`,
    commit `bfb05e046e4c57b9fcb156158fea164fed7becff`, equal to its `origin/main`.
  - secrets-management:
    `/Users/dakasa/projects/dakasa/yggdrasil/integration-secrets-management`.
  - canonical integration contract:
    `/Users/dakasa/projects/dakasa/yggdrasil/integration-template/INTEGRATION_CONTRACT.md`.
  - DaKasa consumer and prior audit:
    `/Users/dakasa/projects/dakasa/dakasa-system`.

Path aliases used below are `CORE` for the Core worktree, `SECRETS` for
`integration-secrets-management`, `TEMPLATE` for `integration-template`, and
`DAKASA` for `dakasa-system` at the absolute locations above.

## 2. Executive verdict

**Do not enable `allow_sensitive_webhook_endpoint_creation` and do not run the
real Stripe create call yet.**

The adopt/update-only split in the adapter is directionally correct, and the WIP
does avoid putting the create-only secret into the current SDK mutation-event
path. However, the end-to-end contract described by the WIP does not exist in
the current Core or secret sink:

1. a real workflow never receives the handshake the adapter requires;
2. caller-controlled execute metadata can forge that handshake once the instance
   opt-in is enabled;
3. the secrets-management sink re-echoes the persisted plaintext as an ordinary,
   unmarked workflow result;
4. account/Connect scope is not bound to the instance or observable after the
   create;
5. DaKasa appears to need both platform-account events and connected-account
   events, while the consumer accepts only one signing secret;
6. the create path is not recoverably idempotent across the exact crash window
   that matters for a create-only secret.

Passing local tests therefore proves the adapter's isolated branches, not a safe
production resource graph.

## 3. Blocking findings

### P0: The proposed handshake has no current producer and is forgeable

The adapter authorizes creation from ordinary request metadata:

- `providers/stripe/adapter/adapter.go:829-833` checks the instance opt-in and
  calls `hasTransientSecretSinkHandshake(req.Metadata)`.
- `providers/stripe/adapter/adapter.go:995-1017` validates the shape of fields in
  that map, but it cannot establish their provenance.
- The positive unit test manufactures the metadata locally and calls the adapter
  directly (`providers/stripe/adapter/webhook_endpoint_test.go:220-287`).

Current Core does not implement the other half:

- `CORE/model/integration_runtime.go:3-10` accepts arbitrary execute metadata.
- `CORE/controllers/message/integrations.go:84-102,265-276` unmarshals it and
  forwards it unchanged to the adapter.
- `CORE/controllers/message/integrations.go:308-337` validates selectors and
  operation names, but reserves no metadata keys.
- A real workflow sends only `workflow`, `step_id`, and `source` in
  `CORE/controllers/message/workflows.go:471-482`; it never emits
  `supports_sensitive_output_paths` or `sensitive_output_sink`.

Consequences:

- the intended workflow path always fails closed at the adapter today;
- after the instance opt-in is enabled, any caller able to execute that instance
  can supply the expected metadata shape and receive the raw `secret` in a
  normal direct response;
- the adapter's checks for producer ID, family, operation, input path, and output
  path do not prove that the sink is the next workflow step.

Required before rollout: Core must reject these reserved keys on every direct
execute boundary and inject an authenticated/run-bound lease only after it has
validated the actual workflow topology. The adapter should consume that
Core-issued lease, not treat an arbitrary map as authority.

### P0: Persistence is not a terminal secret boundary

The Stripe producer correctly marks `secret` as sensitive in
`providers/stripe/adapter/adapter.go:886-898`. Core redacts the public copy, but
keeps the original result in the shared execution context for later steps
(`CORE/controllers/message/workflows.go:297-313` and
`CORE/controllers/message/workflows_sensitive_output.go:12-53`). There is no
one-step lease enforcement in that runtime.

The proposed sink then re-exposes the same value:

- AWS `ensure_secret` returns `SecretUpsertResult.Value` on create and manual
  update (`SECRETS/providers/aws/adapter/spec.go:388-419,444-461`), and its
  response helper adds no sensitive paths (`:842-852`).
- GCP does the same (`SECRETS/providers/gcp/adapter/spec.go:432-465,475-493` and
  `:790-800`).
- The output type serializes `value` normally
  (`SECRETS/family/contract/types.go:309-318`).

The Stripe handshake also does not bind:

- the secrets-management integration instance/backend;
- `secret.secret_id` (mandatory according to
  `SECRETS/docs/CAPABILITIES.md:59-75`);
- the expected DaKasa property `STRIPE_WEBHOOK_SECRET`;
- the workflow/run identity or a single-use nonce;
- adjacency, conditions, iteration cardinality, or absence of later consumers.

The consumer requires exactly that property through process environment
(`DAKASA/dakasa-enterprise-fe/backend/dakasa-enterprise-payments-api/controllers/webhook-stripe.go:19-56`)
and verifies requests with it (`:90-119`). The current WIP has no workflow proving
the store -> ExternalSecret -> pod projection chain.

Required before rollout: a leased manual `ensure_secret` write must return only
a receipt, Core must taint/redact the sink result and sanitize errors, and the
lease must bind the exact sink instance plus destination secret ID. Add an
end-to-end canary that searches durable results, events, errors, and captured
logs for the exact canary bytes.

### P0: Account/Connect topology is neither bound nor fully designed

There are two distinct Stripe concepts in the WIP:

- `connect=true` creates a platform Connect webhook for events from connected
  accounts; `connect=false` creates an account webhook for the key owner's own
  events (stripe-go v83.1.0 `webhookendpoint.go:106-123`).
- `Stripe-Account` makes the API request as one specific connected account
  (stripe-go v83.1.0 `params.go:125-129,236-240`).

The WIP accepts both at once (`connect` plus `stripe_account`) with no valid
combination matrix (`providers/stripe/adapter/adapter.go:849-876`). More
importantly, `stripe_account_id` is declared and documented as an instance-bound
field (`providers/stripe/adapter/spec.go:196-209`,
`manifest/integration_type.stripe.yaml:261-269`), but `Execute` never reads it.
Only caller input `stripe_account` sets the header. The client construction reads
the API key, base URL, and nominal API version, not the instance account ID
(`providers/stripe/adapter/adapter.go:45-72`,
`providers/stripe/adapter/client.go:69-80`).

Adoption is exact-URL-only inside whichever request scope the caller selected
(`providers/stripe/adapter/adapter.go:955-977`). The returned Stripe object has
no `connect` field (stripe-go `webhookendpoint.go:146-172`), and the WIP neither
stores nor verifies a separate immutable scope identity. Consequently an
observe/readback cannot prove whether the endpoint is platform-account,
Connect-wide, or attached to a particular account context.

There is an additional DaKasa topology question that must be resolved before
creating anything:

- the payments handler explicitly expects `invoice.paid` and
  `invoice.payment_failed` on the platform account
  (`DAKASA/.../controllers/webhook_stripe_dispatch.go:144-209`);
- it requires connected-account identity for payout events
  (`DAKASA/.../controllers/webhook_stripe_dispatch.go:217-271`);
- the configured IDs are documented as connected accounts
  (`DAKASA/.../client/stripe.go:57-102`);
- the handler verifies every delivery using one `STRIPE_WEBHOOK_SECRET`.

Based on the Stripe API model, this event set appears to span an account endpoint
and a Connect endpoint, which normally have separate signing secrets. The WIP's
test creates one `connect=true` endpoint containing both an invoice and a payout
event (`providers/stripe/adapter/webhook_endpoint_test.go:220-270`) but does not
prove provider delivery semantics or consumer verification. Confirm the intended
Stripe topology. If two endpoints are required, use distinct URLs/secrets or
teach the consumer to verify against a bounded set without weakening signature
validation.

## 4. High-priority correctness findings

### P1: `provision_webhook_endpoint` is not retry-safe despite claiming idempotence

- The capability is marked idempotent in
  `providers/stripe/adapter/spec.go:412-416` and
  `manifest/integration_type.stripe.yaml:115-119,239-255`.
- Before every POST it lists by URL and errors if any match exists
  (`providers/stripe/adapter/adapter.go:849-856`). A retry after a successful
  provider create but lost response therefore cannot replay Stripe's cached
  response and cannot recover the one-time secret.
- The derived idempotency key includes only the constant `ensure_we` and URL
  (`:877`; helper `providers/stripe/adapter/client.go:83-95`). It omits event set,
  `connect`, endpoint API version, description, metadata, instance/account scope,
  and a resource generation. Reusing the same URL with different desired input
  can cause Stripe idempotency-parameter mismatch or stale replay.
- There is no explicit race recovery, lost-response test, crash compensation, or
  destroy/recreate recovery path.

This violates the canonical retry property “same input -> same output” and
adoption contract (`TEMPLATE/INTEGRATION_CONTRACT.md:110-119`). For create-only
material, the recovery design must be settled before the first live call; normal
workflow history must not become the recovery store.

### P1: Readback cannot establish exact production state

- `webhookEndpointOutput` has the richer fields
  (`providers/stripe/adapter/adapter.go:980-992`), but
  `observeWebhookEndpoints` still emits only ID, URL, status, events, and API
  version (`:1042-1106`).
- `CHANGELOG.md:22-23` and `docs/CAPABILITIES.md:214-219` claim observation also
  includes live mode, application, creation time, description, and metadata.
  The claim is false for the current implementation.
- Endpoint API version is creation-only in stripe-go
  (`webhookendpoint.go:18-29,75-90`). Ensure accepts no desired API version and
  neither reconciles nor rejects an immutable version mismatch
  (`manifest/capability.ensure_webhook_endpoint.yaml:14-24`,
  `providers/stripe/adapter/adapter.go:901-952`).
- Neither observation nor ensure establishes the authenticated account ID or
  `connect` scope.

The required DaKasa readback is exact URL, enabled status, exact event set,
live/test mode, account/scope, endpoint API version, and destination secret
projection. Current output cannot support that gate.

### P1: Creation is intentionally silent in the audit/event plane

`provision_webhook_endpoint` stays outside the SDK reconciler specifically so
the full create response is not auto-emitted
(`providers/stripe/adapter/adapter.go:823-827`). The message handler consequently
falls back to the legacy execute path for this unregistered operation
(`providers/stripe/message/execute.go:19-25,60-87`). This prevents a raw-secret
event, which is the right immediate safety choice, but leaves the successful
resource mutation with no sanitized provider event.

The canonical contract requires every mutation to emit a structured event
(`TEMPLATE/INTEGRATION_CONTRACT.md:236-303`). Add an SDK/adapter projection that
emits endpoint identity and non-secret observed state only. Projection failure
must never fall back to the raw result.

### P1: The release/schema contract is not compatible as version 2.5.0

Baseline 2.4.0 let `ensure_webhook_endpoint` POST when no ID and returned
`secret`. The WIP changes the same capability to adopt/update-only and removes
that output while adding a separate action, yet bumps only to 2.5.0
(`providers/stripe/adapter/spec.go:20-27`, `CHANGELOG.md:8-27`). Existing callers
can now fail on absent endpoints and lose an expected output field.

The canonical lifecycle contract requires breaking changes only at a major bump
and a compatibility shim for one minor cycle
(`TEMPLATE/INTEGRATION_CONTRACT.md:336-343`). Security motivation does not make
the wire change backward-compatible; release it under an explicit major/compat
plan.

`provision_webhook_endpoint` also violates Core's canonical prefix regex and is
not currently allowlisted (`CORE/manifest/capability_naming.go:14-16,93-117`,
`CORE/config/capability_naming_allowlist.yaml:1-28`). Phase 1 currently warns
rather than blocks, but the resource/action naming decision must be explicit
before registration.

### P1: No deployable DaKasa producer/sink resource graph is present

Repository search found the handshake terms only in Stripe prose, the adapter,
and its unit test. There is no production workflow or integration instance that
binds:

- `https://enterprise.dakasa.me/payment/webhook/stripe`;
- the complete consumer event set (`invoice.paid`, `invoice.payment_failed`,
  `balance.available`, `payout.paid`, `payout.reconciliation_completed`,
  `payout.failed`, `payout.canceled`, `charge.dispute.created`,
  `charge.dispute.closed`, `charge.refunded`);
- the exact live Stripe account/scope;
- the exact secrets-management destination and AWS region;
- `STRIPE_WEBHOOK_SECRET` through ExternalSecret and the payments pod;
- scheduled readback/drift alerting and a provider-signed delivery canary.

Those production requirements are recorded at
`DAKASA/docs/superpowers/2026-09-05-third-party-resource-contract-audit-handoff.md:61-106`.
An adapter implementation alone is not rollout evidence.

## 5. Medium-priority findings

### P2: Input schemas and runtime parsing fail open on wrong types

The capability manifests do not mark URL/event inputs as required. Runtime does
check their emptiness, but several optional fields silently become absent on a
wrong JSON type (`providers/stripe/adapter/adapter.go:1252-1302`), and metadata
values are coerced with `fmt.Sprint` (`:1339-1357`). Core direct execute validates
neither capability input schema nor reserved metadata (`CORE/controllers/message/integrations.go:308-337`).

Use strict decoding/validation for all security- and scope-relevant fields,
reject unknown or ill-typed handshake fields, and make required inputs explicit
in the capability manifest.

### P2: API-version configuration is documented as consumed but is not applied

`stripe_api_version` is declared as “sent via the Stripe-Version header”
(`providers/stripe/adapter/spec.go:210-219`, `docs/CONFIGURATION.md:41-47`).
`NewStripeClient` explicitly discards the value because stripe-go's public
`BackendConfig` has no such field (`providers/stripe/adapter/client.go:19-40`).
This is broader than webhook provisioning, but it invalidates the instance
schema's consumed/readback claim. Either implement an actual per-request header
contract or document the SDK-pinned behavior precisely.

### P2: Adjacent destroy contract still converts absence into an error

The action catalog and capability docs promise `404 -> already-absent success`
(`providers/stripe/adapter/spec.go:416`, `docs/CAPABILITIES.md:221-222`), while
`destroyWebhookEndpoint` returns every Stripe delete error unchanged
(`providers/stripe/adapter/adapter.go:1109-1128`). This pre-existing mismatch can
surface a spurious failure/500 on safe retries and should be corrected or the
contract changed.

### P2: Generic webhook-server docs conflict with the DaKasa destination

`docs/USAGE.md:155-178` first describes provisioning the application endpoint,
then says Stripe posts to the adapter route `/webhooks/stripe/{instance_id}`.
For DaKasa production the audited destination is the payments API route
`/payment/webhook/stripe`, not the integration's generic inbound reactor.
Separate the two supported modes and ensure the production workflow cannot pick
the wrong destination from documentation.

## 6. Controls that are correct and should be preserved

- `ensure_webhook_endpoint` no longer creates, rejects absent or ambiguous exact
  URL adoption, and requires an explicit event set
  (`providers/stripe/adapter/adapter.go:783-820`).
- Provisioning requires both an instance opt-in and a second gate; the default is
  fail-closed (`:823-846`). Keep the opt-in `false` through all prerequisite
  deployments and tests.
- The create response checks for a non-empty signing secret and marks exactly
  `secret` as sensitive (`:879-898`).
- Provisioning stays out of the current full-observed SDK mutation event path;
  do not re-register it until a tested projection primitive exists.
- Existing unit tests prove: absent ensure does not POST; missing/malformed
  handshake does not call Stripe; the secret output path is exact; and create
  parameters are encoded as expected.
- No source path was found that explicitly logs `we.Secret` or places it in an
  adapter error. Preserve this property and add byte-exact log/event assertions.

## 7. Required implementation and rollout order

1. **Keep every sensitive-create opt-in false.** No production Stripe POST.
2. **Resolve endpoint topology.** Establish whether platform plus connected
   account events require two endpoints; decide URLs, event sets, and one or more
   signing secrets accordingly.
3. **Implement Core authority.** Reserve metadata keys on direct execute,
   statically validate a single adjacent sink, bind workflow/run/producer/sink/
   destination, inject a single-use lease, and erase access after consumption.
4. **Harden secrets-management.** A leased manual write returns a receipt without
   `value`; errors and logs are canary-safe; Core redacts the sink defensively.
5. **Bind Stripe identity and scope.** Validate the API key's account and mode,
   disallow ambiguous `connect`/`Stripe-Account` combinations, and make
   observation prove immutable scope or record a safe provider-backed identity
   that can be verified.
6. **Design crash recovery.** Derive an idempotency key from the complete canonical
   create intent and test success-response loss, retry, race, and explicit
   compensation without using durable plaintext as recovery state.
7. **Complete readback and audit projection.** Return the documented non-secret
   fields, fail on immutable drift, and emit only a sanitized mutation receipt.
8. **Resolve SemVer and capability naming.** Register the change under an
   explicit compatible/major migration and approved naming rule.
9. **Add the DaKasa resources.** Exact live instance, exact URL(s), exact event
   allowlist(s), exact secret destination(s), ExternalSecret/readiness proof,
   scheduled observation, and alerting.
10. **Run an isolated byte-exact canary, then request explicit authorization for
    the real create.** Immediately read back provider state and send a
    provider-signed callback through the real consumer before turning the opt-in
    off again.

## 8. Acceptance tests still required

At minimum, test across real Core + Stripe adapter + secrets-management adapter
with a fake Stripe server and isolated fake secret backend:

- direct execute with forged reserved metadata is rejected by Core;
- a real non-adjacent, conditional, iterative, fan-out, or wrong-target sink does
  not receive a lease and causes no Stripe POST;
- exactly one valid adjacent sink can create and receives the secret once;
- producer and sink public/durable results are redacted and the sink response has
  no `value`;
- exact canary bytes are absent from workflow rows, integration responses,
  adapter/Core/sink logs, mutation events, error strings, traces, and DLQ bodies;
- sink failure, timeout, process crash after provider success, response loss,
  duplicate delivery, and concurrent create all have documented outcomes;
- idempotent retry cannot create a duplicate and can either complete persistence
  safely or enter an explicit recoverable/compensation state;
- platform and Connect delivery tests use provider-shaped signed events and prove
  the intended invoice/payout routing plus signature verification;
- observe verifies exact URL, status, event set, live mode, API version, account/
  Connect scope, and secret projection readiness;
- sanitized mutation event includes resource ID and instance identity but never
  the secret.

## 9. Validation performed

All commands were local and non-mutating with respect to external systems:

- `GOWORK=off go test -count=1 ./...`: PASS.
- `GOWORK=off go test -race -count=1 ./providers/stripe/adapter ./providers/stripe/message`: PASS.
- `GOWORK=off go vet ./...`: PASS.
- `git diff --check`: PASS.
- `gofmt -l` on changed Go files: no output.

These results do not include a Core/sink integration test, live Stripe request,
provider-signed callback, secrets projection, production readback, or canary
absence proof.

## 10. Go/no-go gate

The current WIP is **NO-GO for production create** and **acceptable only as a
disabled adapter-side draft**. A merge/release may still be unsafe even with the
opt-in false if its 2.5.0 contract silently breaks existing callers; resolve the
SemVer/compatibility decision first. The real Stripe mutation remains separately
blocked until every P0 item and the acceptance suite above are closed with
production-shaped evidence and explicit authorization.
