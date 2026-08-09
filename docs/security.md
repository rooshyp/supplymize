# Security and threat model

## Protected assets

- Demand history, inventory positions, supplier performance, and lane rates
- Forecast and optimization parameters
- Recommended and approved operating plans
- Service credentials and deployment configuration

## Trust boundaries

The browser-to-control-plane edge is untrusted. Internal service traffic is trusted only after workload identity is established in production. Source-system data is untrusted even when it comes from an authenticated connector because malformed or extreme values can manipulate a plan.

## Primary threats and controls

| Threat | Control |
| --- | --- |
| Oversized or adversarial payload | Request-size limits, bounded collections, strict validation, deadlines |
| Prompt-like data influencing decisions | Models use numeric feature contracts; untrusted text is not executed as instruction |
| Cost manipulation through extreme values | Finite ranges, non-negative domain types, explicit objective breakdown |
| Cross-tenant plan exposure | Authentication and tenant scoping at ingress and persistence layers (roadmap) |
| Service impersonation | Kubernetes network policy now; mTLS/workload identity for production |
| Secret leakage | No secrets in examples, logs, images, or repository; use a managed secret store |
| Supply-chain compromise | Pinned lockfiles, Dependabot updates, CodeQL, and verified container builds in CI |

## Data minimization

The planning core needs aggregated demand and network constraints. Customer PII is not part of the API contract. Logs contain request IDs, timings, result counts, and status—not full request or response bodies.

## Model and solver safety

Forecast responses include uncertainty and assumptions. Optimization output includes unfulfilled demand and binding constraints. The UI treats recommendations as reviewable proposals; autonomous execution is outside the current trust boundary.
