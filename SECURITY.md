# Security policy

## Supported versions

SupplyMize is pre-1.0. Security fixes are applied to the latest `main` branch and the most recent tagged release.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability reporting on this repository. Include the affected component, reproduction steps, impact, and any suggested mitigation.

You should receive an acknowledgement within three business days. We aim to provide an initial severity assessment within seven business days and will coordinate disclosure after a fix is available.

## Scope priorities

High-priority reports include authentication or authorization bypass, remote code execution, cross-tenant data access, secret exposure, unsafe deserialization, dependency-chain compromise, and manipulation of plans without an audit signal.

## Operational assumptions

The reference stack is a development deployment. Production operators must place the public API behind authenticated TLS ingress, use a managed secret store, restrict backend network paths, enable centralized audit logs, and apply organization-specific retention requirements.
