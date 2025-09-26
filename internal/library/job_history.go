package library

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// JobHistoryRecord captures a state transition or noteworthy event for a job attempt.
type JobHistoryRecord struct {
	Attempt    int       `json:"attempt"`
	State      string    `json:"state"`
	Message    string    `json:"message,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// JobHistoryRetention defines retention policies applied when recording history entries.
type JobHistoryRetention struct {
	MaxEntries int
	MaxAge     time.Duration
}

// JobListCursor describes the continuation point for keyset pagination of jobs.
type JobListCursor struct {
	SubmittedAt time.Time
	JobID       string
}

// ListJobsOptions tunes repository-backed job pagination queries.
type ListJobsOptions struct {
	Limit  int
	Cursor *JobListCursor
}

// ListJobsResult conveys a single page of jobs and the cursor for subsequent pages.
type ListJobsResult struct {
	Jobs       []Job
	NextCursor *JobListCursor
}

// jobCursorDelimiter separates the timestamp and identifier components within a cursor payload.
const jobCursorDelimiter = ":"

// encodeJobCursor serializes the cursor into an opaque token for API consumers.
func encodeJobCursor(cursor *JobListCursor) string {
	if cursor == nil {
		return ""
	}
	payload := strings.Join([]string{
		strconv.FormatInt(cursor.SubmittedAt.UTC().UnixNano(), 10),
		cursor.JobID,
	}, jobCursorDelimiter)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// decodeJobCursor parses an API token into a cursor structure.
func decodeJobCursor(token string) (*JobListCursor, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("library: decode job cursor: %w", err)
	}
	parts := strings.SplitN(string(raw), jobCursorDelimiter, 2)
	if len(parts) != 2 {
		return nil, errors.New("library: invalid job cursor payload")
	}
	nano, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("library: parse job cursor timestamp: %w", err)
	}
	submittedAt := time.Unix(0, nano).UTC()
	jobID := strings.TrimSpace(parts[1])
	if jobID == "" {
		return nil, errors.New("library: job cursor missing job id")
	}
	return &JobListCursor{SubmittedAt: submittedAt, JobID: jobID}, nil
}
