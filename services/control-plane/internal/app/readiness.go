package app

import (
	"context"
	"fmt"
)

type ReadyChecker interface {
	Ready(context.Context) error
}

type NamedChecker struct {
	Name    string
	Checker ReadyChecker
}

type Readiness struct {
	dependencies []NamedChecker
}

func NewReadiness(dependencies ...NamedChecker) *Readiness {
	copyOfDependencies := append([]NamedChecker(nil), dependencies...)
	return &Readiness{dependencies: copyOfDependencies}
}

// Check probes dependencies concurrently so readiness latency is bounded by
// the slowest dependency rather than the sum of all dependency timeouts.
func (r *Readiness) Check(ctx context.Context) map[string]error {
	if r == nil || len(r.dependencies) == 0 {
		return map[string]error{}
	}
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(r.dependencies))
	for _, dependency := range r.dependencies {
		dependency := dependency
		go func() {
			if dependency.Checker == nil {
				results <- result{name: dependency.Name, err: fmt.Errorf("checker is not configured")}
				return
			}
			results <- result{name: dependency.Name, err: dependency.Checker.Ready(ctx)}
		}()
	}

	checks := make(map[string]error, len(r.dependencies))
	for range r.dependencies {
		result := <-results
		checks[result.name] = result.err
	}
	return checks
}
