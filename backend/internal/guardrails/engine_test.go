package guardrails_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/guardrails"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/guardrails/pii"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

// fakeResolver returns a fixed Policy regardless of scope. Used by tests so we
// don't need a SQL backend to exercise the engine.
type fakeResolver struct {
	policy guardrails.Policy
}

func (f *fakeResolver) GetByScope(_ context.Context, tenantID string, scope store.GuardrailScope, scopeID string) (store.GuardrailPolicy, error) {
	if scope != store.GuardrailScopeGlobal {
		return store.GuardrailPolicy{}, store.ErrNotFound
	}
	cfg, _ := guardrails.MarshalPolicy(f.policy)
	return store.GuardrailPolicy{
		TenantID: tenantID, Scope: scope, ScopeID: scopeID,
		Enabled: true, Config: cfg,
	}, nil
}

// TestEngine_Inbound_PIIMasksUserMessage exercises the full PII flow end-to-end:
// the engine resolves a global PII policy, runs the PII detector against a
// request containing an email and NIK, and verifies that the user message in
// the live request is rewritten in place. This is the regression test that
// catches mutation propagation bugs between the detector and the pipeline.
func TestEngine_Inbound_PIIMasksUserMessage(t *testing.T) {
	enabled := true
	policy := guardrails.Policy{
		Enabled: &enabled,
		PII: &guardrails.PIIConfig{
			Enabled:  true,
			Strategy: guardrails.PIIStrategyReplace, // <EMAIL_ADDRESS> etc.
			MinScore: 0.5,
		},
	}
	resolver := guardrails.NewResolver(&fakeResolver{policy: policy}, 0)
	engine := guardrails.NewEngine(guardrails.EngineConfig{
		Resolver:  resolver,
		Detectors: []guardrails.Detector{pii.New()},
	})

	req := &core.ChatRequest{
		Metadata: core.RequestMetadata{TenantID: "default"},
		Messages: []core.Message{
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{{
					Type: core.PartText,
					Text: "Please email john@example.com about NIK 3201202001900001",
				}},
			},
		},
	}

	res := engine.Inbound(context.Background(), req)
	if res.Action != guardrails.ActionMask {
		t.Fatalf("expected ActionMask, got %s; decisions=%+v", res.Action, res.Decisions)
	}

	got := req.Messages[0].TextContent()
	if strings.Contains(got, "john@example.com") {
		t.Errorf("email not masked: %q", got)
	}
	if strings.Contains(got, "3201202001900001") {
		t.Errorf("NIK not masked: %q", got)
	}
	if !strings.Contains(got, "<EMAIL_ADDRESS>") {
		t.Errorf("expected <EMAIL_ADDRESS> token in output: %q", got)
	}
	if !strings.Contains(got, "<ID_NIK>") {
		t.Errorf("expected <ID_NIK> token in output: %q", got)
	}
}

// TestEngine_Inbound_PIIBlocks confirms strategy=block surfaces ActionBlock.
func TestEngine_Inbound_PIIBlocks(t *testing.T) {
	policy := guardrails.Policy{
		PII: &guardrails.PIIConfig{Enabled: true, Strategy: guardrails.PIIStrategyBlock, MinScore: 0.5},
	}
	resolver := guardrails.NewResolver(&fakeResolver{policy: policy}, 0)
	engine := guardrails.NewEngine(guardrails.EngineConfig{
		Resolver:  resolver,
		Detectors: []guardrails.Detector{pii.New()},
	})

	req := &core.ChatRequest{
		Metadata: core.RequestMetadata{TenantID: "default"},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "my email is x@example.com"}}},
		},
	}
	res := engine.Inbound(context.Background(), req)
	if res.Action != guardrails.ActionBlock {
		t.Fatalf("expected ActionBlock, got %s", res.Action)
	}
}

// TestEngine_NilSafe confirms a nil engine doesn't panic and produces
// ActionAllow — important because the pipeline calls into the engine
// unconditionally.
func TestEngine_NilSafe(t *testing.T) {
	var e *guardrails.Engine
	req := &core.ChatRequest{Messages: []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}}}}
	res := e.Inbound(context.Background(), req)
	if res.Action != guardrails.ActionAllow {
		t.Errorf("nil engine should ActionAllow, got %s", res.Action)
	}
}

