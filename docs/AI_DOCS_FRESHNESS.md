# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: 79610c793b0e89e2243acfb4c1f11bb5a0d8d9b5
verified_diff_sha256: 5c8d532f6871040a71aee7deecc399574f2e82cf957630c6008f8534281595fa
reconciler_schema: 1
verified_at: 2026-09-13
by: Codex
note: Reconciled public RPC observations, strict items arrays, typed webhook absence, version 3.0.2, manifests, tests, and operator contracts.
