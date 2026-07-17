package ops_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"composable-operations/internal/core"
	"composable-operations/internal/llm"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
)

// fixedLLM is a test double that returns a predetermined completion.
type fixedLLM struct{ text string }

func (f *fixedLLM) Complete(_ context.Context, _ string) (llm.Completion, error) {
	return llm.Completion{Text: f.text}, nil
}

func newRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &llm.StubClient{}))
	return reg
}

// --- RegisterBuiltins ---

func TestRegisterBuiltins_RegistersAllTypes(t *testing.T) {
	reg := newRegistry(t)
	for _, typeName := range []string{"llm.classify", "pii.scan", "score", "human.approval", "llm.decision", "publish"} {
		assert.True(t, reg.Has(typeName), "expected type %q to be registered", typeName)
	}
}

func TestRegisterBuiltins_DuplicateRegistrationFails(t *testing.T) {
	reg := newRegistry(t)
	err := ops.RegisterBuiltins(reg, &llm.StubClient{})
	require.Error(t, err)
}

// --- llm.classify ---

func TestClassifyOp_EnrichesEnvelopeWithScore(t *testing.T) {
	ctx := context.Background()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &fixedLLM{text: "0.75"}))
	op, _ := reg.Get("llm.classify")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"content": "some text"}
	params := map[string]any{
		"prompt_template": "Analyze: {{.content}}",
		"output_field":    "toxicity_score",
	}

	result, err := actOp.Execute(ctx, input, params)

	require.NoError(t, err)
	assert.Equal(t, 0.75, result["toxicity_score"])
	assert.Equal(t, "some text", result["content"], "original fields must be preserved")
}

func TestClassifyOp_ValidateParams_MissingPromptTemplate(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("llm.classify")
	err := op.ValidateParams(map[string]any{"output_field": "score"})
	require.Error(t, err)
}

func TestClassifyOp_ValidateParams_MissingOutputField(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("llm.classify")
	err := op.ValidateParams(map[string]any{"prompt_template": "tmpl"})
	require.Error(t, err)
}

// --- pii.scan ---

func TestPIIScanOp_DetectsPII(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t)
	op, _ := reg.Get("pii.scan")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"content": "Call me at john@example.com for details."}
	params := map[string]any{
		"patterns":  []any{`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`},
		"threshold": 1,
	}

	result, err := actOp.Execute(ctx, input, params)

	require.NoError(t, err)
	assert.Equal(t, true, result["pii_found"])
}

func TestPIIScanOp_NoPIIFound(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t)
	op, _ := reg.Get("pii.scan")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"content": "This is clean content."}
	params := map[string]any{
		"patterns":  []any{`\b\d{3}-\d{2}-\d{4}\b`},
		"threshold": 1,
	}

	result, err := actOp.Execute(ctx, input, params)

	require.NoError(t, err)
	assert.Equal(t, false, result["pii_found"])
}

// --- score ---

func TestScoreOp_ComputesWeightedSum(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t)
	op, _ := reg.Get("score")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{
		"toxicity_score":   0.5,
		"policy_violation": 0.2,
	}
	params := map[string]any{
		"weights": map[string]any{
			"toxicity_score":   0.6,
			"policy_violation": 0.4,
		},
	}

	result, err := actOp.Execute(ctx, input, params)

	require.NoError(t, err)
	// 0.5*0.6 + 0.2*0.4 = 0.30 + 0.08 = 0.38
	assert.InDelta(t, 0.38, result["risk_score"], 1e-9)
}

// --- human.approval ---

func TestHumanApprovalOp_BuildsRequestWithDisplayFields(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("human.approval")
	gate := op.(core.HumanGate)

	input := core.Envelope{
		"content":        "Hello world",
		"toxicity_score": 0.1,
		"risk_score":     0.05,
	}
	params := map[string]any{
		"prompt":         "Review this content",
		"display_fields": []any{"content", "risk_score"},
	}

	req := gate.BuildRequest(input, params)

	assert.Equal(t, "Review this content", req.Prompt)
	assert.Equal(t, "Hello world", req.Payload["content"])
	assert.Equal(t, 0.05, req.Payload["risk_score"])
	assert.NotContains(t, req.Payload, "toxicity_score", "non-display fields must be excluded")
}

func TestHumanApprovalOp_BuildsRequestWithFullEnvelopeWhenNoDisplayFields(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("human.approval")
	gate := op.(core.HumanGate)

	input := core.Envelope{"content": "text", "risk_score": 0.1}
	params := map[string]any{"prompt": "Review"}

	req := gate.BuildRequest(input, params)

	assert.Equal(t, len(input), len(req.Payload))
}

// --- llm.decision ---

func TestDecisionOp_EmitsDecisionField(t *testing.T) {
	ctx := context.Background()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &fixedLLM{
		text: `{"approved":true,"comment":"Looks good."}`,
	}))
	op, _ := reg.Get("llm.decision")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"risk_score": 0.2, "content": "hello"}
	params := map[string]any{
		"prompt_template": "Approve or reject based on risk score {{.risk_score}}",
	}

	result, err := actOp.Execute(ctx, input, params)

	require.NoError(t, err)
	decision, ok := result["decision"].(map[string]any)
	require.True(t, ok, "decision field must be a map")
	assert.Equal(t, true, decision["approved"])
	assert.Equal(t, "Looks good.", decision["comment"])
	assert.Equal(t, "llm", decision["by"])
}

func TestDecisionOp_EmitsSameShapeAsHumanApproval(t *testing.T) {
	ctx := context.Background()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &fixedLLM{
		text: `{"approved":false,"comment":"Too risky."}`,
	}))
	op, _ := reg.Get("llm.decision")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"risk_score": 0.9}
	params := map[string]any{"prompt_template": "Decision for risk {{.risk_score}}"}

	result, err := actOp.Execute(ctx, input, params)
	require.NoError(t, err)

	decision := result["decision"].(map[string]any)
	assert.Contains(t, decision, "approved")
	assert.Contains(t, decision, "comment")
	assert.Contains(t, decision, "by")
}

// --- publish ---

func TestPublishOp_PublishesWhenApproved(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t)
	op, _ := reg.Get("publish")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{
		"content":  "Great post",
		"decision": map[string]any{"approved": true, "by": "llm", "comment": ""},
	}

	result, err := actOp.Execute(ctx, input, nil)

	require.NoError(t, err)
	assert.Equal(t, true, result["published"])
}

func TestPublishOp_RejectsWhenNotApproved(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t)
	op, _ := reg.Get("publish")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{
		"decision": map[string]any{"approved": false, "by": "human", "comment": "rejected"},
	}

	_, err := actOp.Execute(ctx, input, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not approved")
}

func TestPublishOp_RejectsWhenDecisionMissing(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t)
	op, _ := reg.Get("publish")
	actOp := op.(core.ActivityOp)

	_, err := actOp.Execute(ctx, core.Envelope{}, nil)

	require.Error(t, err)
}
