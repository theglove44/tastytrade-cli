# MI-2026-03-22 Branch Closeout Helper

## Trigger

The branch closeout / handoff flow has been repeated often enough that the manual
`git status` -> `git add` -> `git commit` -> `git push -u` sequence is worth
streamlining. The commit subject changes per branch, so the helper should keep
message generation human-controlled while automating the repetitive git steps.

## Scope inspected

- current branch status / upstream bookkeeping patterns
- existing Makefile targets
- prior closeout notes in `docs/mi/README.md`
- safe place for an installable Codex skill

## Commands run

```bash
git status --short --branch
rg -n "closeout|branch.*push|push.*upstream|set-upstream|git status|git add|git commit" .
sed -n '1,240p' Makefile
sed -n '1,220p' docs/mi/MI-2026-03-15-phase3a-closeout.md
sed -n '1,220p' docs/mi/README.md
```

## Files inspected

- `Makefile`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3a-closeout.md`

## Findings

- There was no reusable branch closeout helper in the repo.
- The branch closeout flow is consistent across phases, but the commit subject is
  intentionally branch-specific.
- A small shell helper can safely automate the repetitive git mechanics without
  deciding the commit message.
- A Codex skill can act as the judgment layer that summarizes the branch work and
  generates the commit message for the helper.

## Direct answers / conclusions

- The best split is:
  - repo-local helper script for staging, commit, push, and upstream refresh
  - installable Codex skill for status inspection and commit-message generation
- The helper should stay explicit and accept a human-provided commit subject.

## Proposed surgical fix

- Add `scripts/branch_closeout.sh`.
- Add a `make closeout` wrapper for the script.
- Install a `branch-closeout` Codex skill under `~/.codex/skills/`.
- Update the MI index with this helper note.

## Files changed

- `Makefile`
- `scripts/branch_closeout.sh`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-22-branch-closeout-helper.md`

## Validation status

- `bash -n scripts/branch_closeout.sh` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current status / next steps

- Repo helper implementation is complete.
- The Codex `branch-closeout` skill is installed under `~/.codex/skills/branch-closeout/`.
- Next step is to use the helper on the next branch closeout and confirm the generated commit subject still matches the branch work.
