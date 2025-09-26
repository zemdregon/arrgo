package library

import (
	"maps"
	"slices"
	"strings"
	"time"
)

// RequestStatus captures the lifecycle state of a library request.
type RequestStatus string

const (
	// RequestStatusPending indicates the request is awaiting approval.
	RequestStatusPending RequestStatus = "pending"
	// RequestStatusApproved indicates the request has been approved and queued for automation.
	RequestStatusApproved RequestStatus = "approved"
	// RequestStatusDenied indicates the request was rejected by an approver.
	RequestStatusDenied RequestStatus = "denied"
	// RequestStatusCancelled indicates the requester withdrew the request prior to fulfillment.
	RequestStatusCancelled RequestStatus = "cancelled"
)

// Request represents a media intake or automation request awaiting review.
type Request struct {
	ID             string
	Kind           string
	Title          string
	RequestedBy    string
	Notes          string
	Policy         string
	ExternalIDs    map[string]string
	QuotaRemaining int
	SubmittedAt    time.Time
	UpdatedAt      time.Time
	SLADeadline    time.Time
	Status         RequestStatus

	Approvals     []RequestApprovalEvent
	Notifications []RequestNotification
}

// RequestDecision represents an approval outcome applied to a request.
type RequestDecision string

const (
	RequestDecisionApproved  RequestDecision = "approved"
	RequestDecisionDenied    RequestDecision = "denied"
	RequestDecisionCancelled RequestDecision = "cancelled"
)

// RequestApprovalEvent captures an approval or denial that transitions a request.
type RequestApprovalEvent struct {
	ID        string
	RequestID string
	Decision  RequestDecision
	Actor     string
	Notes     string
	DecidedAt time.Time
}

// RequestNotificationStatus captures the delivery outcome for notification fan-out.
type RequestNotificationStatus string

const (
	RequestNotificationStatusQueued RequestNotificationStatus = "queued"
	RequestNotificationStatusSent   RequestNotificationStatus = "sent"
	RequestNotificationStatusFailed RequestNotificationStatus = "failed"
)

// RequestNotification records notification delivery attempts linked to a request timeline.
type RequestNotification struct {
	ID           string
	RequestID    string
	Channel      string
	Target       string
	Status       RequestNotificationStatus
	AttemptedAt  time.Time
	ErrorMessage string
}

func cloneRequest(req Request) Request {
	cloned := req
	if len(req.ExternalIDs) > 0 {
		cloned.ExternalIDs = maps.Clone(req.ExternalIDs)
	}
	if len(req.Approvals) > 0 {
		cloned.Approvals = make([]RequestApprovalEvent, len(req.Approvals))
		for i := range req.Approvals {
			cloned.Approvals[i] = cloneRequestApproval(req.Approvals[i])
		}
	}
	if len(req.Notifications) > 0 {
		cloned.Notifications = make([]RequestNotification, len(req.Notifications))
		for i := range req.Notifications {
			cloned.Notifications[i] = cloneRequestNotification(req.Notifications[i])
		}
	}
	return cloned
}

func cloneRequestApproval(event RequestApprovalEvent) RequestApprovalEvent {
	cloned := event
	cloned.Notes = strings.Clone(event.Notes)
	return cloned
}

func cloneRequestNotification(notification RequestNotification) RequestNotification {
	cloned := notification
	cloned.ErrorMessage = strings.Clone(notification.ErrorMessage)
	return cloned
}

func canonicalizeRequest(req Request) Request {
	canonical := cloneRequest(req)
	canonical.ID = strings.TrimSpace(canonical.ID)
	canonical.Kind = strings.ToLower(strings.TrimSpace(canonical.Kind))
	canonical.Title = strings.TrimSpace(canonical.Title)
	canonical.RequestedBy = strings.TrimSpace(canonical.RequestedBy)
	canonical.Notes = strings.TrimSpace(canonical.Notes)
	canonical.Policy = strings.TrimSpace(canonical.Policy)
	canonical.Status = normalizeRequestStatus(canonical.Status)
	canonical.SubmittedAt = canonical.SubmittedAt.UTC()
	canonical.UpdatedAt = canonical.UpdatedAt.UTC()
	canonical.SLADeadline = canonical.SLADeadline.UTC()
	canonical.ExternalIDs = normalizeExternalIDs(canonical.ExternalIDs)
	canonical.Approvals = nil
	canonical.Notifications = nil
	if canonical.Title == "" {
		canonical.Title = canonical.Kind
	}
	if canonical.UpdatedAt.IsZero() || canonical.UpdatedAt.Before(canonical.SubmittedAt) {
		canonical.UpdatedAt = canonical.SubmittedAt
	}
	return canonical
}

func normalizeExternalIDs(ids map[string]string) map[string]string {
	if len(ids) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(ids))
	for key, value := range ids {
		trimmedKey := strings.ToLower(strings.TrimSpace(key))
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		normalized[trimmedKey] = trimmedValue
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeRequestStatus(status RequestStatus) RequestStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(RequestStatusApproved):
		return RequestStatusApproved
	case string(RequestStatusDenied):
		return RequestStatusDenied
	case string(RequestStatusCancelled):
		return RequestStatusCancelled
	default:
		return RequestStatusPending
	}
}

func canonicalizeRequestApproval(event RequestApprovalEvent) RequestApprovalEvent {
	canonical := cloneRequestApproval(event)
	canonical.ID = strings.TrimSpace(canonical.ID)
	canonical.RequestID = strings.TrimSpace(canonical.RequestID)
	canonical.Actor = strings.TrimSpace(canonical.Actor)
	canonical.Notes = strings.TrimSpace(canonical.Notes)
	canonical.Decision = normalizeRequestDecision(canonical.Decision)
	canonical.DecidedAt = canonical.DecidedAt.UTC()
	return canonical
}

func normalizeRequestDecision(decision RequestDecision) RequestDecision {
	switch strings.ToLower(strings.TrimSpace(string(decision))) {
	case string(RequestDecisionApproved):
		return RequestDecisionApproved
	case string(RequestDecisionDenied):
		return RequestDecisionDenied
	case string(RequestDecisionCancelled):
		return RequestDecisionCancelled
	default:
		return ""
	}
}

func canonicalizeRequestNotification(notification RequestNotification) RequestNotification {
	canonical := cloneRequestNotification(notification)
	canonical.ID = strings.TrimSpace(canonical.ID)
	canonical.RequestID = strings.TrimSpace(canonical.RequestID)
	canonical.Channel = strings.ToLower(strings.TrimSpace(canonical.Channel))
	canonical.Target = strings.TrimSpace(canonical.Target)
	canonical.Status = normalizeNotificationStatus(canonical.Status)
	canonical.AttemptedAt = canonical.AttemptedAt.UTC()
	canonical.ErrorMessage = strings.TrimSpace(canonical.ErrorMessage)
	return canonical
}

func normalizeNotificationStatus(status RequestNotificationStatus) RequestNotificationStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(RequestNotificationStatusSent):
		return RequestNotificationStatusSent
	case string(RequestNotificationStatusFailed):
		return RequestNotificationStatusFailed
	default:
		return RequestNotificationStatusQueued
	}
}

func requestStatusFromDecision(decision RequestDecision) RequestStatus {
	switch decision {
	case RequestDecisionApproved:
		return RequestStatusApproved
	case RequestDecisionDenied:
		return RequestStatusDenied
	case RequestDecisionCancelled:
		return RequestStatusCancelled
	default:
		return RequestStatusPending
	}
}

func isTerminalRequestStatus(status RequestStatus) bool {
	switch status {
	case RequestStatusApproved, RequestStatusDenied, RequestStatusCancelled:
		return true
	default:
		return false
	}
}

func sortApprovals(events []RequestApprovalEvent) []RequestApprovalEvent {
	if len(events) <= 1 {
		return events
	}
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, func(a, b RequestApprovalEvent) int {
		if a.DecidedAt.Before(b.DecidedAt) {
			return -1
		}
		if a.DecidedAt.After(b.DecidedAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return sorted
}

func sortNotifications(events []RequestNotification) []RequestNotification {
	if len(events) <= 1 {
		return events
	}
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, func(a, b RequestNotification) int {
		if a.AttemptedAt.Before(b.AttemptedAt) {
			return -1
		}
		if a.AttemptedAt.After(b.AttemptedAt) {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return sorted
}
