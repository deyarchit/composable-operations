package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"composable-operations/internal/api"
	"composable-operations/internal/core"
	"composable-operations/internal/loader"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
	"composable-operations/internal/testutil"
)

// apiClient is a typed HTTP client for the API under test.
type apiClient struct {
	base string
	http *http.Client
}

func newAPIClient(srv *httptest.Server) *apiClient {
	return &apiClient{base: srv.URL, http: srv.Client()}
}

func (c *apiClient) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, c.base+path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&v))
	return v
}

// --- mock TemporalClient ---

type mockTemporalClient struct {
	startErr    error
	queryStatus *core.RunStatus
	queryErr    error
	signalErr   error
	capturedSig *core.ApprovalDecision
}

func (m *mockTemporalClient) ExecuteWorkflow(_ context.Context, _ client.StartWorkflowOptions, _ any, _ ...any) (client.WorkflowRun, error) {
	return nil, m.startErr
}

func (m *mockTemporalClient) QueryWorkflow(_ context.Context, _ string, _ string, _ string, _ ...any) (converter.EncodedValue, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if m.queryStatus == nil {
		return nil, errors.New("no status configured")
	}
	return &encodedStatus{status: m.queryStatus}, nil
}

func (m *mockTemporalClient) SignalWorkflow(_ context.Context, _ string, _ string, _ string, arg any) error {
	if d, ok := arg.(core.ApprovalDecision); ok {
		m.capturedSig = &d
	}
	return m.signalErr
}

func (m *mockTemporalClient) ListWorkflow(_ context.Context, _ *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	return &workflowservice.ListWorkflowExecutionsResponse{}, nil
}

// encodedStatus satisfies converter.EncodedValue for query responses.
type encodedStatus struct {
	status *core.RunStatus
}

func (e *encodedStatus) Get(valuePtr any) error {
	out, ok := valuePtr.(*core.RunStatus)
	if !ok {
		return errors.New("expected *core.RunStatus")
	}
	*out = *e.status
	return nil
}

func (e *encodedStatus) HasValue() bool { return e.status != nil }

// --- test server setup ---

func newTestServer(t *testing.T, temporal *mockTemporalClient, flowsDir string) *httptest.Server {
	t.Helper()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &testutil.StubChatModel{}))
	ldr := loader.New(flowsDir, reg)
	h := api.NewHandler(temporal, ldr)
	e := echo.New()
	e.HideBanner = true
	api.RegisterRoutes(e, h)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv
}

func newFlowsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	yaml := `
name: test-flow
steps:
  - id: check-metrics
    type: metrics.check
    params: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test-flow.yaml"), []byte(yaml), 0o600))
	return dir
}

// --- tests ---

func TestStartRun_UnknownFlow(t *testing.T) {
	srv := newTestServer(t, &mockTemporalClient{}, t.TempDir())
	c := newAPIClient(srv)

	t.Run("When starting a run for a flow that does not exist", func(t *testing.T) {
		resp := c.do(t, http.MethodPost, "/flows/does-not-exist/runs", map[string]any{})

		t.Run("Then it returns bad request", func(t *testing.T) {
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})
}

func TestStartRun_ValidFlow(t *testing.T) {
	dir := newFlowsDir(t)
	srv := newTestServer(t, &mockTemporalClient{}, dir)
	c := newAPIClient(srv)

	t.Run("When starting a run for a known flow", func(t *testing.T) {
		resp := c.do(t, http.MethodPost, "/flows/test-flow/runs", map[string]any{
			"input": map[string]any{"trigger": "alert"},
		})

		t.Run("Then it returns 201 with a run_id", func(t *testing.T) {
			require.Equal(t, http.StatusCreated, resp.StatusCode)
			body := decodeBody[map[string]any](t, resp)
			assert.NotEmpty(t, body["run_id"])
		})
	})
}

func TestGetRun_ReturnsStatus(t *testing.T) {
	status := &core.RunStatus{
		RunID: "run-123",
		Flow:  "test-flow",
		State: core.RunCompleted,
		Steps: []core.StepResult{{StepID: "check-metrics", Status: core.StepCompleted}},
	}
	srv := newTestServer(t, &mockTemporalClient{queryStatus: status}, newFlowsDir(t))
	c := newAPIClient(srv)

	t.Run("When querying a completed run", func(t *testing.T) {
		resp := c.do(t, http.MethodGet, "/runs/run-123", nil)

		t.Run("Then it returns 200 with per-step status", func(t *testing.T) {
			require.Equal(t, http.StatusOK, resp.StatusCode)
			body := decodeBody[core.RunStatus](t, resp)
			assert.Equal(t, "completed", string(body.State))
			assert.Len(t, body.Steps, 1)
		})
	})
}

func TestGetRun_QueryError(t *testing.T) {
	srv := newTestServer(t, &mockTemporalClient{queryErr: errors.New("not found")}, t.TempDir())
	c := newAPIClient(srv)

	resp := c.do(t, http.MethodGet, "/runs/bad-id", nil)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSubmitApproval_SendsSignal(t *testing.T) {
	mock := &mockTemporalClient{}
	srv := newTestServer(t, mock, t.TempDir())
	c := newAPIClient(srv)

	t.Run("When submitting an approval with step_id", func(t *testing.T) {
		resp := c.do(t, http.MethodPost, "/runs/run-123/approvals", map[string]any{
			"step_id":  "human-review",
			"approved": true,
			"comment":  "Looks good",
		})

		t.Run("Then it returns 204 and sends the signal", func(t *testing.T) {
			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
			require.NotNil(t, mock.capturedSig)
			assert.Equal(t, "human-review", mock.capturedSig.StepID)
			assert.True(t, mock.capturedSig.Approved)
			assert.Equal(t, "Looks good", mock.capturedSig.Comment)
		})
	})
}

func TestSubmitApproval_MissingStepID(t *testing.T) {
	srv := newTestServer(t, &mockTemporalClient{}, t.TempDir())
	c := newAPIClient(srv)

	resp := c.do(t, http.MethodPost, "/runs/run-123/approvals", map[string]any{
		"approved": true,
	})

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
