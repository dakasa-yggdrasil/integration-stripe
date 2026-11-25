# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: e8c36ef7c5770a6744356d08905de21c5c437754
verified_diff_sha256: 9a58c30a7af3236d936f9019d67290a98c27cc483493c3d58b38c8945e268715
reconciler_schema: 1
verified_at: 2026-09-13
by: Codex
note: Reconciled public RPC observations, strict items arrays, typed webhook absence, version 3.0.2, manifests, tests, and operator contracts.

sdk010_original_context_drain_review: Reconciled original caller context, exact Run/WaitDrained ownership, actual listener-error cancellation, retained ABI/provider/idempotency/auth behavior, explicit example grace and separate runtime desired adoption. CI-only actual provider/Reply/settlement/startup checks qualify Source; no local tests/builds/imports or Native occurred. Genuine551 input bytes remain equal to the signed code commit and raw generated provenance is retained.
