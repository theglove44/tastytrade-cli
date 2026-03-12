# DevOps MI Log

This directory is the audit trail for investigation work, debugging passes, and surgical fixes.

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
