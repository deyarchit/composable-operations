package api

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"

	"composable-operations/internal/core"
	"composable-operations/internal/engine"
)

type startRunRequest struct {
	Input map[string]any `json:"input"`
}

type startRunResponse struct {
	RunID string `json:"run_id"`
}

// startRun handles POST /flows/{name}/runs.
// It loads and validates the definition, then starts a RunFlow workflow.
func (h *Handler) startRun(c echo.Context) error {
	name := c.Param("name")

	def, err := h.loader.Load(name)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("load flow %q: %v", name, err))
	}

	var req startRunRequest
	if bindErr := c.Bind(&req); bindErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	input := core.Envelope(req.Input)
	if input == nil {
		input = core.Envelope{}
	}

	runID := uuid.New().String()
	_, err = h.temporal.ExecuteWorkflow(
		c.Request().Context(),
		client.StartWorkflowOptions{
			ID:        runID,
			TaskQueue: engine.TaskQueue,
		},
		"RunFlow",
		engine.FlowInput{Definition: *def, Input: input},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("start workflow: %v", err))
	}

	return c.JSON(http.StatusCreated, startRunResponse{RunID: runID})
}

// getRun handles GET /runs/{id}: queries the workflow for its current status.
func (h *Handler) getRun(c echo.Context) error {
	runID := c.Param("id")

	resp, err := h.temporal.QueryWorkflow(
		c.Request().Context(),
		runID, "",
		engine.QueryStatus,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("query run %q: %v", runID, err))
	}

	var status core.RunStatus
	if err := resp.Get(&status); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "decode status")
	}

	return c.JSON(http.StatusOK, status)
}

type approvalRequest struct {
	StepID   string `json:"step_id"`
	Approved bool   `json:"approved"`
	Comment  string `json:"comment"`
}

// submitApproval handles POST /runs/{id}/approvals: sends the approval signal.
func (h *Handler) submitApproval(c echo.Context) error {
	runID := c.Param("id")

	var req approvalRequest
	if bindErr := c.Bind(&req); bindErr != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.StepID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "step_id is required")
	}

	decision := core.ApprovalDecision{
		StepID:   req.StepID,
		Approved: req.Approved,
		Comment:  req.Comment,
		By:       "human",
	}

	if err := h.temporal.SignalWorkflow(
		c.Request().Context(),
		runID, "",
		engine.SignalApproval,
		decision,
	); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("signal run %q: %v", runID, err))
	}

	return c.NoContent(http.StatusNoContent)
}

// listRuns handles GET /runs?state=waiting_approval.
// It lists open workflows and queries each for state, filtering to the
// requested state. This is N+1 but acceptable for the demo scale.
func (h *Handler) listRuns(c echo.Context) error {
	stateFilter := c.QueryParam("state")

	listResp, err := h.temporal.ListWorkflow(
		c.Request().Context(),
		&workflowservice.ListWorkflowExecutionsRequest{
			Query:    "ExecutionStatus='Running'",
			PageSize: 100,
		},
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("list workflows: %v", err))
	}

	var summaries []core.RunSummary
	for _, info := range listResp.Executions {
		runID := info.Execution.WorkflowId

		resp, err := h.temporal.QueryWorkflow(c.Request().Context(), runID, "", engine.QueryStatus)
		if err != nil {
			continue
		}
		var status core.RunStatus
		if err := resp.Get(&status); err != nil {
			continue
		}

		if stateFilter != "" && string(status.State) != stateFilter {
			continue
		}

		summaries = append(summaries, core.RunSummary{
			RunID: runID,
			Flow:  status.Flow,
			State: status.State,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{"runs": summaries})
}
