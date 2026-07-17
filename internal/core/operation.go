package core

import "context"

// Operation is implemented by every registered op type.
type Operation interface {
	Type() string
	Kind() OpKind
	ValidateParams(params map[string]any) error
}

// ActivityOp is an Operation executed as a Temporal activity. Both deterministic
// ops (pii.scan, score, publish) and LLM ops (llm.classify, llm.decision)
// implement this interface; the distinction is internal to each op.
type ActivityOp interface {
	Operation
	Execute(ctx context.Context, input Envelope, params map[string]any) (Envelope, error)
}

// HumanGate is an Operation handled inside RunFlow via a Temporal signal rather
// than as an activity. It produces an ApprovalRequest shown to the reviewer;
// the workflow itself records the decision on the envelope.
type HumanGate interface {
	Operation
	BuildRequest(input Envelope, params map[string]any) ApprovalRequest
}
