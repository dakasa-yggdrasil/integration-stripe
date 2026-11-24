# Original work and planned shutdown

The Source candidate selects signed common SDK `v0.10.0`, commit
`6dbb2733974cf04384369ee249804ac874c82bbe`. The existing SDK v0.9.1 fixed-queue
and terminal-refusal contracts remain in that release. No provider operation,
authentication, idempotency key, event field, retry policy or public wire type
changes here. This Source change does not update a running image or establish
its current OCI provenance.

The same SDK runtime calls Run once and then WaitDrained with a background wait
context. Its final original completion owns admitted handlers, replies and Ack/Nack.
A nonnil completion remains unqualified and holds the process; it is never replaced
by a fresh Run or cleanup. The original RPC timeout remains 30 seconds; the SDK's
first shutdown wait is 31 seconds. A side listener failure retains its cause and
cancels the same admission context, instead of calling Fatal from its goroutine.
Only after the original completion is qualified can the process exit with that
original Run/listener error.

Stripe forwards the existing context through all four reconcile dispatchers into
ExecuteContext; legacy RPC fallback and stripe-go provider calls already carry it.
The independent webhook keeps its signature checks, deduplication, original
200-before-async-emit response, event fields and5-second emitter context. Planned
webhook shutdown closes admission and joins the original HTTP handlers, async
emitter and eviction work. A wait deadline reports incomplete work without
cancelling that emitter or replacing its original join. All side listeners share
the same 10-second first auxiliary-shutdown budget; after a timeout, main joins
the same original shutdown with a background wait.

The 45-second Kubernetes/Compose example grace covers 30s handler +1s first-wait
margin +10s shared auxiliary shutdown (including the 5-second emitter) +4s exit.

A handler or emitter that ignores its original deadline can exceed any finite
grace. The wait remains incomplete; process or network loss can leave effects
unknown. Cancellation never proves absence of a remote mutation, and this work
does not introduce automatic retry, exactly-once effects or horizontal-mutation
qualification.

CI-only validation runs the existing provider/contract/security suites, actual
loopback provider cancellation across reconcile and legacy paths, planned stop
with provider I/O then the original Reply/Ack blocked, and actual SDK Run-overrun,
startup-failure and listener-bind-failure fixtures. No local test/build or live
provider call belongs to this change.

Root owns review, merge, immutable image publication and live adoption. Normal
desired/recovery producers must carry the same qualified image, Source tuple and
explicit grace together through the exact receiving Deployment UID/RV. Historical
prepared26 bodies are preserved and must not be replayed to apply this successor.
A later reconciliation must not restore the old image or the shorter grace.
