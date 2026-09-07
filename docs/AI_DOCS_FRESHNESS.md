# AI docs freshness stamp

Records the commit an AI (or agent-assisted human) last reconciled these docs at.
The docs-freshness CI reads it: a PR that bumps it is trusted and the AI is skipped
(economy path). See the "Docs freshness" rule in AGENTS.md / CLAUDE.md.

Before a PR: update stale docs, set verified_at_commit to your branch tip.
On arrival: if this is behind the code you touch, reconcile the docs FIRST.

verified_at_commit: eddc04d8759a950017b40feb545636f286cc57a9
verified_diff_sha256: 919cc4df1c0f0d3722eda2eb7ca59a3db4b52a169d63fe9735c39936b522645a
reconciler_schema: 1
verified_at: 2026-09-06
by: Codex
note: Reconciled passive fixed-queue AMQP startup, canonical Core instance identity hydration, safe Stripe webhook provisioning, SDK pin integrity, manifests, and operator documentation.
