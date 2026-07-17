package core

// Envelope is the payload threaded through each step in a flow. Ops enrich it
// by reading existing fields and writing new ones; they never remove fields.
type Envelope map[string]any

// OpKind distinguishes how a step is executed by RunFlow.
type OpKind string

const (
	KindActivity  OpKind = "activity"
	KindHumanGate OpKind = "human_gate"
)

// RetryPolicy mirrors a subset of Temporal's ActivityRetryPolicy, expressed in
// step params so flows can tune retry behaviour without code changes.
type RetryPolicy struct {
	MaxAttempts            int     `yaml:"max_attempts"`
	InitialIntervalSeconds float64 `yaml:"initial_interval_seconds"`
}

// StepConfig is one step entry in a FlowDefinition.
type StepConfig struct {
	ID          string         `yaml:"id"`
	Type        string         `yaml:"type"`
	Params      map[string]any `yaml:"params"`
	RetryPolicy *RetryPolicy   `yaml:"retry_policy,omitempty"`
}

// FlowDefinition is the parsed, validated form of a flow YAML file.
type FlowDefinition struct {
	Name     string       `yaml:"name"`
	MockData string       `yaml:"mock_data,omitempty"`
	Steps    []StepConfig `yaml:"steps"`
}

// ApprovalRequest is produced by a HumanGate op and surfaced by GET /runs/{id}
// so a reviewer has the context they need to decide.
type ApprovalRequest struct {
	RunID   string         `json:"run_id"`
	StepID  string         `json:"step_id"`
	Flow    string         `json:"flow"`
	Prompt  string         `json:"prompt"`
	Payload map[string]any `json:"payload"`
}

// ApprovalDecision is the signal payload sent by POST /runs/{id}/approvals.
// The workflow accepts it only when StepID matches the currently-suspended step.
type ApprovalDecision struct {
	StepID   string `json:"step_id"`
	Approved bool   `json:"approved"`
	Comment  string `json:"comment"`
	By       string `json:"by"`
}

// StepStatus tracks the execution state of a single step in a run.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
)

// StepResult is one element in the ordered step list returned by GET /runs/{id}.
type StepResult struct {
	StepID string         `json:"step_id"`
	Status StepStatus     `json:"status"`
	Output map[string]any `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// RunState is the top-level state of a flow run.
type RunState string

const (
	RunRunning         RunState = "running"
	RunWaitingApproval RunState = "waiting_approval"
	RunCompleted       RunState = "completed"
	RunFailed          RunState = "failed"
)

// RunStatus is the full response shape for GET /runs/{id}.
type RunStatus struct {
	RunID           string           `json:"run_id"`
	Flow            string           `json:"flow"`
	State           RunState         `json:"state"`
	Steps           []StepResult     `json:"steps"`
	ApprovalRequest *ApprovalRequest `json:"approval_request,omitempty"`
	Result          map[string]any   `json:"result,omitempty"`
	Error           string           `json:"error,omitempty"`
}

// RunSummary is one item in the GET /runs?state=... list response.
type RunSummary struct {
	RunID string   `json:"run_id"`
	Flow  string   `json:"flow"`
	State RunState `json:"state"`
}
