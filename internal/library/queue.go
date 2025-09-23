package library

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultStreamName     = "arrgo:library:jobs"
	defaultConsumerGroup  = "arrgo-library-workers"
	defaultIdempotencyTTL = 24 * time.Hour
	idempotencyPrefix     = "arrgo:library:idemp:"
)

// OrchestrationQueue wraps Redis Streams primitives for job orchestration.
type OrchestrationQueue struct {
	client            *redis.Client
	stream            string
	group             string
	logger            *slog.Logger
	idempotencyPrefix string
	idempotencyTTL    time.Duration
}

// OrchestrationQueueConfig describes queue construction parameters.
type OrchestrationQueueConfig struct {
	Client         *redis.Client
	Stream         string
	Group          string
	Logger         *slog.Logger
	IdempotencyTTL time.Duration
}

// ClaimedJob represents a job fetched from the Redis consumer group.
type ClaimedJob struct {
	Job      Job
	StreamID string
}

// NewOrchestrationQueue configures a Redis Streams-backed queue.
func NewOrchestrationQueue(cfg OrchestrationQueueConfig) (*OrchestrationQueue, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("library: redis client is required for orchestration queue")
	}

	stream := cfg.Stream
	if stream == "" {
		stream = defaultStreamName
	}

	group := cfg.Group
	if group == "" {
		group = defaultConsumerGroup
	}

	ttl := cfg.IdempotencyTTL
	if ttl <= 0 {
		ttl = defaultIdempotencyTTL
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &OrchestrationQueue{
		client:            cfg.Client,
		stream:            stream,
		group:             group,
		logger:            logger,
		idempotencyPrefix: idempotencyPrefix,
		idempotencyTTL:    ttl,
	}, nil
}

// Initialize ensures the backing stream and consumer group exist.
func (q *OrchestrationQueue) Initialize(ctx context.Context) error {
	if err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "$").Err(); err != nil {
		if !isBusyGroupError(err) {
			return fmt.Errorf("library: create consumer group %q: %w", q.group, err)
		}
	}
	return nil
}

// Enqueue pushes a job onto the orchestration stream.
func (q *OrchestrationQueue) Enqueue(ctx context.Context, job Job) (string, error) {
	return q.enqueue(ctx, job, false)
}

// Claim retrieves pending jobs for the provided consumer.
func (q *OrchestrationQueue) Claim(ctx context.Context, consumer string, count int, block time.Duration) ([]ClaimedJob, error) {
	if consumer == "" {
		return nil, fmt.Errorf("library: consumer name is required")
	}

	args := &redis.XReadGroupArgs{
		Group:    q.group,
		Consumer: consumer,
		Streams:  []string{q.stream, ">"},
	}
	if count > 0 {
		args.Count = int64(count)
	}
	if block > 0 {
		args.Block = block
	}

	streams, err := q.client.XReadGroup(ctx, args).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("library: claim jobs: %w", err)
	}

	total := 0
	for _, stream := range streams {
		total += len(stream.Messages)
	}
	claimed := make([]ClaimedJob, 0, total)
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			job, err := q.deserializeJob(msg)
			if err != nil {
				q.logger.Warn("library: discard malformed job", "streamID", msg.ID, "error", err)
				if ackErr := q.client.XAck(ctx, q.stream, q.group, msg.ID).Err(); ackErr != nil {
					q.logger.Warn("library: ack malformed job failed", "streamID", msg.ID, "error", ackErr)
				}
				continue
			}
			claimed = append(claimed, ClaimedJob{
				Job:      job,
				StreamID: msg.ID,
			})
		}
	}

	return claimed, nil
}

// Ack marks a job as completed for the consumer group.
func (q *OrchestrationQueue) Ack(ctx context.Context, streamID string) error {
	if streamID == "" {
		return fmt.Errorf("library: stream id is required")
	}
	if err := q.client.XAck(ctx, q.stream, q.group, streamID).Err(); err != nil {
		return fmt.Errorf("library: ack job %q: %w", streamID, err)
	}
	if err := q.client.XDel(ctx, q.stream, streamID).Err(); err != nil {
		return fmt.Errorf("library: delete job %q: %w", streamID, err)
	}
	return nil
}

// Retry re-queues a job for another execution attempt.
func (q *OrchestrationQueue) Retry(ctx context.Context, streamID string, job Job) (string, error) {
	newID, err := q.enqueue(ctx, job, true)
	if err != nil {
		return "", err
	}
	if err := q.client.XAck(ctx, q.stream, q.group, streamID).Err(); err != nil {
		return "", fmt.Errorf("library: ack job %q for retry: %w", streamID, err)
	}
	if err := q.client.XDel(ctx, q.stream, streamID).Err(); err != nil {
		return "", fmt.Errorf("library: delete job %q for retry: %w", streamID, err)
	}
	return newID, nil
}

func (q *OrchestrationQueue) enqueue(ctx context.Context, job Job, skipIdempotency bool) (string, error) {
	if job.ID == "" {
		return "", fmt.Errorf("library: job id is required")
	}
	if !skipIdempotency && job.IdempotencyKey != "" {
		key := q.idempotencyKey(job.IdempotencyKey)
		created, err := q.client.SetNX(ctx, key, job.ID, q.idempotencyTTL).Result()
		if err != nil {
			return "", fmt.Errorf("library: set idempotency key: %w", err)
		}
		if !created {
			return "", ErrDuplicateJob
		}
	}

	attempt := job.Attempt + 1
	fields := map[string]any{
		"job_id":          job.ID,
		"job_type":        job.JobType,
		"target_id":       job.TargetID,
		"payload_uri":     job.PayloadURI,
		"idempotency_key": job.IdempotencyKey,
		"submitted_at":    job.SubmittedAt.Format(time.RFC3339Nano),
		"attempt":         strconv.Itoa(attempt),
	}

	id, err := q.client.XAdd(ctx, &redis.XAddArgs{Stream: q.stream, Values: fields}).Result()
	if err != nil {
		return "", fmt.Errorf("library: enqueue job: %w", err)
	}
	return id, nil
}

func (q *OrchestrationQueue) deserializeJob(msg redis.XMessage) (Job, error) {
	jobID, err := getStringField(msg, "job_id")
	if err != nil {
		return Job{}, err
	}
	jobType, err := getStringField(msg, "job_type")
	if err != nil {
		return Job{}, err
	}
	targetID, err := getStringField(msg, "target_id")
	if err != nil {
		return Job{}, err
	}
	payloadURI, err := getStringField(msg, "payload_uri")
	if err != nil {
		return Job{}, err
	}
	idempotencyKey, err := getStringField(msg, "idempotency_key")
	if err != nil {
		return Job{}, err
	}
	submittedAtRaw, err := getStringField(msg, "submitted_at")
	if err != nil {
		return Job{}, err
	}
	submittedAt, err := time.Parse(time.RFC3339Nano, submittedAtRaw)
	if err != nil {
		return Job{}, fmt.Errorf("library: parse submitted_at: %w", err)
	}
	attemptRaw, err := getStringField(msg, "attempt")
	if err != nil {
		return Job{}, err
	}
	attempt, err := strconv.Atoi(attemptRaw)
	if err != nil {
		return Job{}, fmt.Errorf("library: parse attempt: %w", err)
	}

	return Job{
		ID:             jobID,
		JobType:        jobType,
		TargetID:       targetID,
		PayloadURI:     payloadURI,
		IdempotencyKey: idempotencyKey,
		SubmittedAt:    submittedAt,
		Attempt:        attempt,
		State:          JobStateQueued,
	}, nil
}

func getStringField(msg redis.XMessage, key string) (string, error) {
	raw, ok := msg.Values[key]
	if !ok {
		return "", fmt.Errorf("library: missing field %q", key)
	}
	switch v := raw.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return fmt.Sprint(v), nil
	}
}

func (q *OrchestrationQueue) idempotencyKey(key string) string {
	return q.idempotencyPrefix + key
}

func isBusyGroupError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}
