# DevOps MI Log

This directory is the audit trail for investigation work, debugging passes, and surgical fixes.

## Primary runbooks

- `../live-submit-safety-runbook.md`
  - single primary runbook for the full Phase 3C live submit safety chain and operator workflow
- `../manual-reconciliation-runbook.md`
  - single primary runbook for the current manual reconciliation workflow built from the read-only Phase 4/5A surfaces
- `../broker-order-inspection.md`
  - read-only Phase 4A guide for broker-facing order inspection commands
- `../local-vs-broker-order-comparison.md`
  - read-only Phase 4B/4C/5A guide for advisory local vs broker order comparison, summaries, filters, and recommended next actions

## Naming convention

Use one markdown file per issue / incident:

- `MI-YYYY-MM-DD-short-slug.md`

Examples:

- `MI-2026-03-11-auth-runtime-and-parity.md`
- `MI-2026-03-12-accounts-empty-output.md`

## Rules

- If a check belongs to an existing issue, append to that issue's file.
- If it is a new issue, create a new MI file.
- Keep entries factual and code-based.
- Record:
  - trigger / symptom
  - scope inspected
  - commands run
  - files inspected
  - findings
  - answer / conclusion
  - proposed surgical fix
  - validation status

## Current index

- `MI-2026-03-11-auth-runtime-and-parity.md`
  - auth compatibility investigation, parity fixes, token persistence fixes
  - source detail also preserved in legacy note: `docs/auth-fix-review.md`
- `MI-2026-03-12-accounts-empty-output.md`
  - accounts endpoint parsing investigation
- `MI-2026-03-12-phase2c-import-diff-review.md`
  - phase2c import diff review, architectural additions, and regression risks
- `MI-2026-03-12-phase3a-reconciler-import-review.md`
  - phase3a reconciler import review, safe selective merge, and validation
- `MI-2026-03-12-watch-command-import-review.md`
  - watch-command import review, Cobra shutdown risk, and metrics env-var mismatch
- `MI-2026-03-12-market-streamer-auth-investigation.md`
  - DXLink quote-token auth investigation and likely AUTH_STATE sequencing bug
- `MI-2026-03-13-market-watchdog-closed-market-fix.md`
  - closed-market stale-watchdog churn fix and startup open-positions metric correction
- `MI-2026-03-13-phase3a-reconciler-result-model.md`
  - structured reconciler run result model, status metrics, and summary logging
- `MI-2026-03-13-phase3a-reconciler-latest-status-surface.md`
  - lightweight in-memory latest-result status surface for reconciler runs
- `MI-2026-03-13-phase3a-watch-reconciler-status-output.md`
  - tt watch status output now surfaces the latest reconciler snapshot
- `MI-2026-03-13-phase3a-watch-operational-heartbeat.md`
  - compact tt watch heartbeat combining streamer and reconciler runtime health
- `MI-2026-03-13-last-quote-metric-stuck-at-zero.md`
  - investigation and fix for live quote metric staying at zero due to DXLink symbol format mismatch
- `MI-2026-03-13-phase3a-reconcile-outcome-policy.md`
  - conservative operational handling policy for reconciler outcomes and watch heartbeat policy state
- `MI-2026-03-15-phase3a-closeout.md`
  - branch closeout, docs hygiene, stray artifact cleanup, and Phase 3B handoff recommendation
- `MI-2026-03-15-phase3b-decision-gating.md`
  - lightweight reconcile-policy decision gating threaded into dry-run while leaving read-only paths untouched
- `MI-2026-03-15-phase3b-decision-gating-watch-surface.md`
  - extend the same gate into tt watch as an explicit operator-visible workflow surface
- `MI-2026-03-15-live-submit-minimal-path.md`
  - first minimal live-submit command reusing safety checks, decision gating, and intent logging
- `MI-2026-03-15-live-submit-operator-confirmation.md`
  - explicit human confirmation and JSON-mode acknowledgement for live order submission
- `MI-2026-03-15-phase3c-pre-submit-policy-hardening.md`
  - final fail-closed pre-submit policy boundary for live order transmission
- `MI-2026-03-15-phase3c-duplicate-submit-protection.md`
  - minimal fail-closed duplicate-submit protection for approved live order intents
- `MI-2026-03-15-phase3c-approval-expiry-and-stale-submit-protection.md`
  - fail-closed freshness policy for approval and confirmation at the live submit boundary
- `MI-2026-03-15-phase3c-submit-denial-diagnostics.md`
  - compact operator-visible final-boundary denial diagnostics for live submit
- `MI-2026-03-15-phase3c-restart-recovery-semantics.md`
  - fail-closed restart/recovery semantics for uncertain or in-flight live submit state
- `MI-2026-03-15-phase3c-submit-state-inspection-reset.md`
  - operator-only inspection and explicit local reset workflow for persisted submit safety state
- `MI-2026-03-15-phase3c-runbook-consolidation.md`
  - consolidates the full Phase 3C live submit safety model into one primary runbook
- `MI-2026-03-15-phase4a-broker-order-state-inspection.md`
  - initial read-only broker-facing order inspection slice for live/open and recent orders
- `MI-2026-03-15-phase4b-local-vs-broker-order-comparison.md`
  - initial read-only advisory comparison between local persisted submit state and broker-visible order state
- `MI-2026-03-15-phase4c-broker-comparison-summary-and-filters.md`
  - extends advisory local-vs-broker comparison with deterministic summary counts and light filters
- `MI-2026-03-15-phase4-branch-cleanup-and-handoff.md`
  - final tidy-up, merge-readiness review, and handoff guidance after completing Phase 4A/4B/4C
- `MI-2026-03-15-phase5a-manual-reconciliation-guidance.md`
  - read-only operator guidance layer mapping comparison outcomes to recommended next actions
- `MI-2026-03-15-phase5b-manual-reconciliation-runbook-consolidation.md`
  - consolidates the manual reconciliation workflow into one primary runbook with light help/discoverability cleanup
- `MI-2026-03-15-phase6a-broker-order-detail-inspection.md`
  - adds a minimal read-only broker order detail lookup by canonical broker order ID
- `MI-2026-03-15-phase6b-enriched-single-order-detail-rendering.md`
  - modest operator-focused polish pass for the human-readable single-order detail rendering
- `MI-2026-03-15-phase6c-bounded-broker-order-context-expansion.md`
  - bounded single-order detail context expansion using existing per-leg fill data already present in the current order model
- `MI-2026-03-18-phase6d-terminal-state-broker-order-reason-visibility.md`
  - terminal-state broker context visibility added to broker order detail rendering using optional shaped terminal fields
- `MI-2026-03-18-phase7a-compare-to-detail-operator-handoff.md`
  - adds a minimal advisory handoff from submit-state compare results to the existing broker-order detail command
- `MI-2026-03-18-phase7b-next-step-handoff-after-compare.md`
  - adds a minimal human-readable re-inspection handoff for compare results without broker_order_id
- `MI-2026-03-19-phase7c-no-more-compare-detail.md`
  - concludes the compare/detail lane is complete enough and recommends the next work elsewhere in the workflow
- `MI-2026-03-19-phase8a-submit-state-clear-workflow.md`
  - tightens the manual post-verification submit-state clear workflow with clearer operator guidance and runbook alignment
- `MI-2026-03-19-phase8b-submit-state-target-visibility.md`
  - adds a focused read-only local submit-state inspection target filter for pre-clear verification
- `MI-2026-03-20-phase8c-submit-state-clear-confirmation-context.md`
  - tightens the final clear confirmation so the operator sees the exact local target being removed
- `MI-2026-03-20-phase8d-submit-state-clear-result-clarity.md`
  - clarifies the post-clear result so the operator sees the exact target removed or that nothing was cleared
