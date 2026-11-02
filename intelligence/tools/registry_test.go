package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExternalActionRequiresExplicitApproval(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Tool{
		Name: "submit_application",
		Risk: RiskExternalAction,
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return "submitted", nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), "submit_application", nil, false); err == nil {
		t.Fatal("expected missing approval to reject external action")
	}
	result, err := registry.Execute(context.Background(), "submit_application", nil, true)
	if err != nil || result != "submitted" {
		t.Fatalf("approved action failed: result=%v error=%v", result, err)
	}
}
