# Select Mutation Testing Framework

- **Status:** accepted
- **Date:** 2026-08-05

## Context

The project needs mutation testing to verify that tests actually catch defects,
not just execute code. Code coverage measures what runs; mutation testing
measures what matters — whether a test fails when code is broken.

Requirements:
- Minimal friction to adopt — no source code changes requested from the team
- Customizable execution pipeline (CI integration, custom test commands)
- Parallelizable across packages to keep CI times reasonable
- Out-of-the-box mutation score reporting (MSI)
- Sufficient mutator coverage to be meaningful

## Options Considered

- **go-mutesting** (avito-tech/go-mutesting) — Standalone CLI binary with 30+
  built-in mutators, JSON/HTML reports, and external exec script support.
  Drop-in activation via `go-mutesting ./...`.
- **ooze** (gtramontina/ooze) — Library-based framework imported into
  `mutation_test.go` files. Fewer mutators (14), built-in parallel execution,
  and minimum threshold enforcement.

## Decision

We will use **go-mutesting** (avito-tech/go-mutesting) as the mutation testing
framework.

## Consequences

- **Positive: Simplicity.** Run a single CLI command against any package. No
  build tags, no library imports, no boilerplate.
- **Positive: No project code changes.** The tool works against unmodified
  source trees. Adoption is a CI configuration change, not a code change.
- **Positive: Easy to customize.** Execution is pluggable via `--exec` bash
  scripts with environment variables (`MUTATE_CHANGED`, `MUTATE_PACKAGE`,
  etc.). Teams can wire in custom test commands, linters, or integration
  checks without forking the tool.
- **Positive: Parallelizable by package.** While go-mutesting lacks built-in
  parallelism, mutations can be run in parallel by invoking the tool against
  separate packages concurrently (e.g., GNU parallel, CI matrix jobs).
- **Positive: MSI out of the box.** The framework calculates Mutation Score
  Indicator automatically and produces JSON/HTML reports suitable for CI
  artifacts and trend tracking.
- **Negative: No native parallelism.** Each mutation is evaluated sequentially
  within a single invocation. Large packages will be slow. Mitigated by
  running per-package in parallel.
- **Negative: No threshold enforcement.** The tool cannot fail CI below a
  minimum MSI. A wrapper script must compare the reported score against a
  threshold.
- **Negative: Blacklist fragility.** MD5-based mutation blacklists break when
  source code changes. Inline annotations (`mutator-disable-*`) are preferred.
