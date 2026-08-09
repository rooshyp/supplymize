package requestid

import (
	"context"
	"strings"
	"testing"
)

func TestNewReturnsUniquePrefixedIDs(t *testing.T) {
	first, err := New("plan")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second, err := New("plan")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !strings.HasPrefix(first, "plan_") || len(first) != len("plan_")+32 {
		t.Fatalf("first ID = %q", first)
	}
	if first == second {
		t.Fatalf("IDs are equal: %q", first)
	}
}

func TestContextRoundTrip(t *testing.T) {
	ctx := WithContext(context.Background(), "req_123")
	if got := FromContext(ctx); got != "req_123" {
		t.Fatalf("FromContext() = %q", got)
	}
	if got := FromContext(context.Background()); got != "" {
		t.Fatalf("FromContext(empty) = %q", got)
	}
}
