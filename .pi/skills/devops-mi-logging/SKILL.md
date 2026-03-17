---
name: devops-mi-logging
description: Creates and maintains DevOps-style MI markdown logs for debugging, investigations, surgical fixes, verification passes, and incident-style code audits in this repository. Use whenever inspecting a bug, comparing patches, applying a surgical fix, validating changes, or answering operational root-cause questions so findings are written to docs/mi and kept auditable.
---

# DevOps MI Logging

Use this skill whenever working on debugging, root-cause analysis, patch review, surgical fixes, or validation in this repository.

## Objective

Maintain an auditable markdown trail in `docs/mi/` so investigation history can be reviewed like DevOps incident / MI records.

## Repo Convention

- MI logs live in `docs/mi/`
- one issue per file
- filename format:
  - `MI-YYYY-MM-DD-short-slug.md`
- keep `docs/mi/README.md` updated when a new MI is created

## Required Behavior

For every investigation/check/debug task:

1. Decide whether this belongs to an existing MI or a new one.
2. Record findings before finishing the task.
3. If code changes are made, record:
   - files changed
   - summary of change
   - validation commands
   - validation outcome
4. If analysis only, record that no code change was applied.

## Suggested MI Sections

- Trigger / symptom
- Scope inspected
- Commands run
- Files inspected
- Findings
- Direct answers / conclusions
- Proposed surgical fix
- Files changed
- Validation status
- Current status / next steps

## Current Known MI Files

- `docs/mi/MI-2026-03-11-auth-runtime-and-parity.md`
- `docs/mi/MI-2026-03-12-accounts-empty-output.md`

## Validation Commands

When applicable, record these exactly:

```bash
go build ./...
go vet ./...
go test ./...
```

Also record any issue-specific commands used during investigation.

## Notes

- Prefer append/update over overwriting prior history.
- Keep entries factual, implementation-based, and auditable from code and commands.
- Use the smallest practical code change when a surgical fix is requested.
