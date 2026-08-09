# Contributing to SupplyMize

Thanks for helping make supply-chain decisions more reliable, efficient, and explainable.

## Before you start

1. Search existing issues and discussions.
2. Open an issue for a substantial feature or contract change.
3. Keep pull requests focused on one outcome.

## Development setup

The full stack requires Node.js 22+, pnpm 11+, Go 1.22+, Python 3.11+, Rust 1.85+, and Docker with Compose.

```bash
cp .env.example .env
make bootstrap
make test
```

Individual components can be developed with only their language toolchain. Each service directory contains its own README and test command.

## Engineering rules

- Keep domain logic independent of HTTP frameworks.
- Treat `contracts/openapi` as a reviewed public boundary.
- Add a deterministic test for every changed rule.
- Return assumptions and explanations with model or solver output.
- Never log customer payloads, secrets, or precise shipment coordinates by default.
- Update an architecture decision record when a change alters a major boundary.

## Commit and pull request style

Use a short imperative subject, such as `Add lane carbon pricing`. Pull requests should explain the user impact, contract changes, validation performed, and operational considerations.

All contributors must follow the [Code of Conduct](CODE_OF_CONDUCT.md).
