package app

import (
	"context"
	"errors"
	"testing"
)

type readyFunc func(context.Context) error

func (function readyFunc) Ready(ctx context.Context) error { return function(ctx) }

func TestReadinessChecksEveryDependencyConcurrently(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	checker := func(name string, err error) ReadyChecker {
		return readyFunc(func(context.Context) error {
			started <- name
			<-release
			return err
		})
	}
	wantError := errors.New("not ready")
	readiness := NewReadiness(
		NamedChecker{Name: "forecast", Checker: checker("forecast", nil)},
		NamedChecker{Name: "optimizer", Checker: checker("optimizer", wantError)},
	)
	resultChannel := make(chan map[string]error, 1)
	go func() { resultChannel <- readiness.Check(context.Background()) }()

	seen := map[string]bool{}
	seen[<-started] = true
	seen[<-started] = true
	if !seen["forecast"] || !seen["optimizer"] {
		t.Fatalf("started = %#v", seen)
	}
	close(release)
	results := <-resultChannel
	if results["forecast"] != nil || !errors.Is(results["optimizer"], wantError) {
		t.Fatalf("Check() = %#v", results)
	}
}

func TestReadinessReportsMissingChecker(t *testing.T) {
	results := NewReadiness(NamedChecker{Name: "missing"}).Check(context.Background())
	if results["missing"] == nil {
		t.Fatal("Check() missing checker error = nil")
	}
}
