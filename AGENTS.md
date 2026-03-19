# Repository Guidelines

## Project Structure & Module Organization
`main.go` is the CLI entrypoint. Cobra commands live in `cmd/`; keep one command or closely related surface per file, using lowercase snake case names such as `submit_state_compare.go`. Shared application logic belongs in `internal/`, which is organized by domain (`client`, `store`, `streamer`, `reconciler`, `valuation`, `exchange/tastytrade`, etc.). Runtime configuration is loaded from `config/config.go`. Operator runbooks and implementation notes live in `docs/` and `docs/mi/`; older checklist material is in `doc/`.

## Build, Test, and Development Commands
Use the Makefile targets:

- `make build`: build the CLI as `./tt`.
- `make test`: run all Go tests with the race detector and a 60s timeout.
- `make lint`: run `golangci-lint` across the module.
- `make clean`: remove the binary and clear the Go test cache.
- `make env-check`: confirm required `TASTYTRADE_*` variables are set.
- `make run-sandbox`: build and smoke-test against the sandbox account.

For direct Go workflows, `go test ./...` and `go build .` should behave the same as the Make targets.

## Coding Style & Naming Conventions
Target Go `1.22` and keep code `gofmt`-clean. Follow standard Go formatting: tabs for indentation, mixedCaps for exported names, short lowercase package names, and descriptive file names like `broker_orders.go` or `market_data_symbol.go`. Keep CLI wiring in `cmd/` thin; push reusable logic into `internal/`. Use concise package comments where behavior or shutdown order is non-obvious.

## Testing Guidelines
Tests are standard Go `_test.go` files placed next to the code they cover, for example `cmd/watch_test.go` and `internal/store/store_test.go`. Prefer table-driven tests for command behavior and policy logic. Use `t.TempDir()` and `t.Setenv()` for stateful paths and environment isolation. Run `make test` before opening a PR; run `go test ./... -race` again after touching concurrency, streamers, or persistence code.

## Commit & Pull Request Guidelines
Recent history uses short imperative subjects such as `Add broker order detail inspection command` and `Polish broker order detail rendering`. Keep commits focused and descriptive; phase or feature labels are fine when they add context. PRs should explain the user-visible or operator-visible change, list validation performed, and link the relevant runbook or issue. Include command output or screenshots only when the change affects CLI rendering or docs.

## Security & Configuration Tips
Start from `.env.example`. Never commit `.env`, secrets, refresh tokens, or live credentials. Credentials are expected in the OS keychain via `tt login`. Treat `TASTYTRADE_RATE_ORDERS_RPS`, live-trading flags, and the kill switch as safety-critical defaults, not convenience settings.

## Project Purpose

This repository is building a safety-first operator CLI for tastytrade workflows.

The CLI is intended to help a human operator:
- inspect broker state
- validate and gate risky actions
- compare local and broker truth
- understand order state clearly
- reconcile manually when needed

It is not intended to become:
- a fully automated trading bot
- an aggressive reconciliation engine
- a broad broker action console
- a speculative “smart” automation layer

## Roadmap Discipline

Work phase-by-phase.

Rules:
- keep slices tightly scoped
- prefer minimal surfaces over new command sprawl
- avoid overbuilding
- do not silently start the next phase
- recommend the next slice only after the current slice is reviewed

## Branch / Concurrency Rules

Only one active roadmap feature branch may change core behavior at a time.

Parallel work is allowed only for:
- docs and runbooks for already-merged features
- tests for already-merged or stable behavior
- read-only investigation/spec work for future phases
- read-only review of current diffs

Do not run concurrent implementation branches that both touch:
- shared `cmd/...` command surfaces
- exchange interfaces
- broker order view models
- reconciler behavior
- submit safety behavior

## Core Safety Constraints

Unless explicitly in scope, do not introduce:
- broker mutation
- cancel/replace
- automatic reconciliation
- local persisted state mutation
- submit safety regressions
- soft automation
- speculative redesigns

## Implementation Style

Prefer:
- small diffs
- existing command surfaces
- reuse of current models/helpers
- operator-friendly errors
- stable JSON shapes
- focused tests
- light docs/help updates only where needed

Avoid:
- raw payload dumping
- broad refactors
- new abstractions without clear payoff
- hidden behavior changes
- mixing unrelated cleanups into roadmap slices

## Reporting Format

When finishing a task, return:
1. concise summary of what changed
2. files changed
3. notable design decisions
4. validation results
5. explicit out-of-scope items intentionally not implemented
6. merge-readiness verdict

## Subagent Role Expectations

Builder:
- implements only the active slice
- does not redesign the system
- does not start later phases

Reviewer:
- read-only by default
- focuses on correctness, scope creep, regression risk, output drift, and missing tests
- critiques; does not rewrite unless explicitly asked

Investigator:
- read-only only
- inspects the codebase and proposes the smallest safe next slice
- does not change code
