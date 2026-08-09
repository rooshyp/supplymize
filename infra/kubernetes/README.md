# Kubernetes deployment

This Kustomize base deploys the four stateless SupplyMize workloads with restricted pod security, probes, resource boundaries, horizontal scaling, disruption budgets, and deny-by-default network policy.

## Deploy

Replace the `:latest` image references with immutable release digests in an environment overlay, then apply:

```bash
kubectl apply -k infra/kubernetes
kubectl rollout status deployment/control-plane -n supplymize
```

No ingress controller or cloud-specific resource is assumed. Label the namespace that contains your ingress controller before exposing the console and control plane:

```bash
kubectl label namespace ingress-nginx supplymize.io/edge-access=true
```

Likewise, label the namespace containing Prometheus with `supplymize.io/observability-access=true`. Production overlays should add authenticated TLS ingress, workload identity, topology spread constraints, immutable image digests, and provider-specific autoscaling metrics.
