package library

import "context"

// QuotaEvaluator runs quota checks before a request is approved.
type QuotaEvaluator interface {
	EvaluateQuota(ctx context.Context, request Request) (QuotaEvaluationResult, error)
}

// QuotaEvaluationResult captures the outcome of a quota check.
type QuotaEvaluationResult struct {
	// Remaining indicates how many requests remain after the evaluation. When nil,
	// the evaluator chose not to update the persisted quota counter.
	Remaining *int
}

// AutomationPreviewer produces an automation preview that runs before approval.
type AutomationPreviewer interface {
	PreviewAutomation(ctx context.Context, request Request) (AutomationPreviewResult, error)
}

// AutomationPreviewResult captures the previewed automation fan-out.
type AutomationPreviewResult struct {
	Jobs []AutomationPreviewJob
}

// AutomationPreviewJob represents a single automation job previewed for a request.
type AutomationPreviewJob struct {
	JobType  string
	TargetID string
	Summary  string
}
