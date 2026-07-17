package ops_test

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"composable-operations/internal/core"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
	"composable-operations/internal/testutil"
)

// fixedLLM returns a predetermined message for every prompt, for unit tests
// that need precise control over the LLM response.
type fixedLLM struct{ text string }

func (f *fixedLLM) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(f.text, nil), nil
}

func (f *fixedLLM) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func newRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &testutil.StubChatModel{}))
	return reg
}

// --- RegisterBuiltins ---

func TestRegisterBuiltins_RegistersAllTypes(t *testing.T) {
	reg := newRegistry(t)
	for _, typeName := range []string{"metrics.check", "logs.check", "llm.analyze", "human.approval", "llm.decision", "remediate"} {
		assert.True(t, reg.Has(typeName), "expected type %q to be registered", typeName)
	}
}

func TestRegisterBuiltins_DuplicateRegistrationFails(t *testing.T) {
	reg := newRegistry(t)
	err := ops.RegisterBuiltins(reg, &testutil.StubChatModel{})
	require.Error(t, err)
}

// --- metrics.check ---

func TestMetricsCheckOp_PassesThroughEnvelope(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("metrics.check")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"trigger": "alert", "service": "payment-api", "cpu_usage": 0.92}

	result, err := actOp.Execute(context.Background(), input, map[string]any{})

	require.NoError(t, err)
	assert.Equal(t, "payment-api", result["service"])
	assert.Equal(t, 0.92, result["cpu_usage"])
	assert.Equal(t, "alert", result["trigger"])
}

// --- logs.check ---

func TestLogsCheckOp_PassesThroughEnvelope(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("logs.check")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{
		"service": "api",
		"logs":    []any{"ERROR: timeout", "WARN: circuit breaker open"},
	}

	result, err := actOp.Execute(context.Background(), input, map[string]any{})

	require.NoError(t, err)
	logs, ok := result["logs"].([]any)
	require.True(t, ok)
	assert.Len(t, logs, 2)
	assert.Equal(t, "ERROR: timeout", logs[0])
	assert.Equal(t, "api", result["service"], "original fields must be preserved")
}

// --- llm.analyze ---

func TestAnalyzeOp_ParsesStructuredAnalysis(t *testing.T) {
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &fixedLLM{
		text: `{"root_cause":"db timeout","severity":"critical","recommended_action":"restart db"}`,
	}))
	op, _ := reg.Get("llm.analyze")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"service": "api", "cpu_usage": 0.9}
	params := map[string]any{"prompt_template": "Analyze service {{.service}} with CPU {{.cpu_usage}}"}

	result, err := actOp.Execute(context.Background(), input, params)

	require.NoError(t, err)
	analysis, ok := result["analysis"].(map[string]any)
	require.True(t, ok, "analysis field must be a map")
	assert.Equal(t, "db timeout", analysis["root_cause"])
	assert.Equal(t, "critical", analysis["severity"])
	assert.Equal(t, "restart db", analysis["recommended_action"])
	assert.Equal(t, "api", result["service"], "original fields must be preserved")
}

func TestAnalyzeOp_ValidateParams_MissingPromptTemplate(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("llm.analyze")
	require.Error(t, op.ValidateParams(map[string]any{}))
}

// --- human.approval ---

func TestHumanApprovalOp_BuildsRequestWithDisplayFields(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("human.approval")
	gate := op.(core.HumanGate)

	input := core.Envelope{
		"service":    "payment-api",
		"cpu_usage":  0.92,
		"error_rate": 0.15,
		"analysis":   map[string]any{"root_cause": "db timeout"},
	}
	params := map[string]any{
		"prompt":         "Review incident",
		"display_fields": []any{"service", "analysis"},
	}

	req := gate.BuildRequest(input, params)

	assert.Equal(t, "Review incident", req.Prompt)
	assert.Equal(t, "payment-api", req.Payload["service"])
	assert.NotNil(t, req.Payload["analysis"])
	assert.NotContains(t, req.Payload, "cpu_usage", "non-display fields must be excluded")
}

func TestHumanApprovalOp_BuildsRequestWithFullEnvelopeWhenNoDisplayFields(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("human.approval")
	gate := op.(core.HumanGate)

	input := core.Envelope{"service": "api", "cpu_usage": 0.9}
	params := map[string]any{"prompt": "Review"}

	req := gate.BuildRequest(input, params)

	assert.Equal(t, len(input), len(req.Payload))
}

// --- llm.decision ---

func TestDecisionOp_EmitsApprovedDecision(t *testing.T) {
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &fixedLLM{
		text: `{"approved":true,"comment":"Remediation is warranted."}`,
	}))
	op, _ := reg.Get("llm.decision")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"analysis": map[string]any{"root_cause": "db timeout", "severity": "critical", "recommended_action": "restart"}}
	params := map[string]any{"prompt_template": "Should we remediate? Root cause: {{index .analysis \"root_cause\"}}. Respond with JSON: {\"approved\": bool, \"comment\": string}"}

	result, err := actOp.Execute(context.Background(), input, params)

	require.NoError(t, err)
	decision, ok := result["decision"].(map[string]any)
	require.True(t, ok, "decision field must be a map")
	assert.Equal(t, true, decision["approved"])
	assert.Equal(t, "Remediation is warranted.", decision["comment"])
	assert.Equal(t, "llm", decision["by"])
}

func TestDecisionOp_EmitsRejectedDecision(t *testing.T) {
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &fixedLLM{
		text: `{"approved":false,"comment":"Severity too low for auto-remediation."}`,
	}))
	op, _ := reg.Get("llm.decision")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{"analysis": map[string]any{"severity": "low"}}
	params := map[string]any{"prompt_template": "Decide: {{index .analysis \"severity\"}}. JSON: {\"approved\": bool, \"comment\": string}"}

	result, err := actOp.Execute(context.Background(), input, params)

	require.NoError(t, err)
	decision := result["decision"].(map[string]any)
	assert.Equal(t, false, decision["approved"])
	assert.Equal(t, "llm", decision["by"])
}

// --- remediate ---

func TestRemediateOp_RemediatesWhenApproved(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("remediate")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{
		"analysis": map[string]any{"recommended_action": "restart payment-api"},
		"decision": map[string]any{"approved": true, "by": "human", "comment": ""},
	}

	result, err := actOp.Execute(context.Background(), input, map[string]any{"endpoint": "https://ops.internal/remediate"})

	require.NoError(t, err)
	assert.Equal(t, true, result["remediated"])
}

func TestRemediateOp_FailsWhenNotApproved(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("remediate")
	actOp := op.(core.ActivityOp)

	input := core.Envelope{
		"decision": map[string]any{"approved": false, "by": "llm", "comment": "severity too low"},
	}

	_, err := actOp.Execute(context.Background(), input, map[string]any{"endpoint": "https://ops.internal/remediate"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not approved")
}

func TestRemediateOp_FailsWhenDecisionMissing(t *testing.T) {
	reg := newRegistry(t)
	op, _ := reg.Get("remediate")
	actOp := op.(core.ActivityOp)

	_, err := actOp.Execute(context.Background(), core.Envelope{}, map[string]any{"endpoint": "https://ops.internal/remediate"})

	require.Error(t, err)
}
