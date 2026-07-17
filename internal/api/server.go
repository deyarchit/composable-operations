package api //nolint:revive // 'api' is the conventional package name for HTTP handler packages

import (
	"context"

	"github.com/labstack/echo/v4"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"composable-operations/internal/loader"
)

// TemporalClient is the narrow slice of client.Client the API handlers need.
// Defining it here (rather than using client.Client directly) allows tests to
// provide a minimal mock without implementing the full Temporal client interface.
type TemporalClient interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow any, args ...any) (client.WorkflowRun, error)
	QueryWorkflow(ctx context.Context, workflowID string, runID string, queryType string, args ...any) (converter.EncodedValue, error)
	SignalWorkflow(ctx context.Context, workflowID string, runID string, signalName string, arg any) error
	ListWorkflow(ctx context.Context, request *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error)
}

// Handler holds dependencies for all API routes.
type Handler struct {
	temporal TemporalClient
	loader   *loader.Loader
}

// NewHandler constructs an API handler.
func NewHandler(temporal TemporalClient, ldr *loader.Loader) *Handler {
	return &Handler{temporal: temporal, loader: ldr}
}

// RegisterRoutes mounts all API routes onto e.
func RegisterRoutes(e *echo.Echo, h *Handler) {
	e.POST("/flows/:name/runs", h.startRun)
	e.GET("/runs", h.listRuns)
	e.GET("/runs/:id", h.getRun)
	e.POST("/runs/:id/approvals", h.submitApproval)
}
