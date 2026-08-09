# ADR 0001: Use a polyglot monorepo with contract-first boundaries

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

SupplyMize needs a product UI, a resilient orchestration edge, interpretable forecasting, and a fast constrained optimizer. Forcing these workloads into one runtime would reduce initial repository variety but would not simplify their long-term ownership or tooling.

## Decision

Use one repository with four runtime boundaries:

- TypeScript for the planning console
- Go for the control plane
- Python for forecasting
- Rust for optimization

Services communicate through versioned JSON/HTTP contracts stored under `contracts/openapi`. Domain logic stays framework-independent inside each service.

## Consequences

### Positive

- Each workload uses an appropriate ecosystem.
- Contract and integration changes are reviewable in one pull request.
- CI can validate the whole planning path.
- Teams can deploy services independently.

### Trade-offs

- Contributors need multiple toolchains for full local validation.
- Contract drift must be caught by CI and integration tests.
- Shared concepts require deliberate schema ownership instead of language-level imports.

These costs are addressed with containers, one-command Make targets, checked-in API contracts, and adapter tests.
