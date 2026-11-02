package jobqueue

import (
	"context"
	"testing"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/lrcconfig"
)

func TestDispatchTools_NilDBError(t *testing.T) {
	ctx := context.Background()

	// Both jq nil and jq.db nil should fail gracefully
	var nilJQ *JobQueue
	if _, err := nilJQ.DispatchTools(ctx, ToolDispatchParams{}); err == nil {
		t.Error("expected error when jq is nil, got nil")
	}

	emptyJQ := &JobQueue{}
	if _, err := emptyJQ.DispatchTools(ctx, ToolDispatchParams{}); err == nil {
		t.Error("expected error when jq.db is nil, got nil")
	}
}

func TestToolDispatchParams_FieldInitialization(t *testing.T) {
	params := ToolDispatchParams{
		OrgID:         123,
		ReviewID:      456,
		PlanCode:      "team_plan",
		DiffZipBase64: "dGVzdA==",
		LocalDiffs:   []lib.LocalCodeDiff{{NewPath: "main.go"}},
		LRCBundle:     lrcconfig.Bundle{Files: map[string][]byte{}},
		Logger:        nil,
		EventSink:     nil,
	}

	if params.OrgID != 123 || params.ReviewID != 456 || params.PlanCode != "team_plan" {
		t.Errorf("ToolDispatchParams fields not properly initialized: %+v", params)
	}
	if len(params.LocalDiffs) != 1 || params.LocalDiffs[0].NewPath != "main.go" {
		t.Errorf("ToolDispatchParams LocalDiffs mismatch: %+v", params.LocalDiffs)
	}
}
