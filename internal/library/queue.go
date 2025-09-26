package library

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arrgo/arrgo/internal/observability/logging"
	"github.com/redis/go-redis/v9"
)

const (
	defaultStreamName          = "arrgo:library:jobs"
	defaultConsumerGroup       = "arrgo-library-workers"
	defaultIdempotencyTTL      = 24 * time.Hour
	idempotencyPrefixBase      = "arrgo:library:idemp"
	defaultDeadLetterSuffix    = "dead"
	defaultMaxDeliveryAttempts = 5
	defaultLagAlertInterval    = time.Minute
)

// OrchestrationQueue wraps Redis Streams primitives for job orchestration.
type OrchestrationQueue struct {
	client *redis.Client

	logger *slog.Logger

	mu sync.RWMutex

	streamBase        string
	deadLetterBase    string
	idempotencyBase   string
	stream            string
	deadLetter        string
	group             string
	idempotencyPrefix string
	keyVersion        string

	idempotencyTTL      time.Duration
	maxDeliveryAttempts int

	lagAlertThreshold int64
	lagAlertInterval  time.Duration
	lastLagCheck      time.Time
}

// OrchestrationQueueConfig describes queue construction parameters.
type OrchestrationQueueConfig struct {
	Client              *redis.Client
	Stream              string
	Group               string
	Logger              *slog.Logger
	IdempotencyTTL      time.Duration
	DeadLetterStream    string
	MaxDeliveryAttempts int
	KeyVersion          string
	LagAlertThreshold   int64
	LagAlertInterval    time.Duration
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

	deadLetterBase := cfg.DeadLetterStream
	if strings.TrimSpace(deadLetterBase) == "" {
		deadLetterBase = fmt.Sprintf("%s:%s", stream, defaultDeadLetterSuffix)
	}

	maxAttempts := cfg.MaxDeliveryAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxDeliveryAttempts
	}

	lagInterval := cfg.LagAlertInterval
	if lagInterval <= 0 {
		lagInterval = defaultLagAlertInterval
	}

	queue := &OrchestrationQueue{
		client:              cfg.Client,
		logger:              logger,
		streamBase:          strings.TrimSpace(stream),
		deadLetterBase:      strings.TrimSpace(deadLetterBase),
		idempotencyBase:     idempotencyPrefixBase,
		group:               group,
		idempotencyTTL:      ttl,
		maxDeliveryAttempts: maxAttempts,
		lagAlertThreshold:   cfg.LagAlertThreshold,
		lagAlertInterval:    lagInterval,
	}
	queue.applyVersion(cfg.KeyVersion)

	return queue, nil
}

// Initialize ensures the backing stream and consumer group exist.
func (q *OrchestrationQueue) Initialize(ctx context.Context) error {
	stream, group := q.currentStreamAndGroup()
	if err := q.client.XGroupCreateMkStream(ctx, stream, group, "$").Err(); err != nil {
		if !isBusyGroupError(err) {
			return fmt.Errorf("library: create consumer group %q: %w", group, err)
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

	streamName, group := q.currentStreamAndGroup()
	q.maybeAlertLag(ctx)

	args := &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{streamName, ">"},
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
	for _, streamResult := range streams {
		total += len(streamResult.Messages)
	}
	claimed := make([]ClaimedJob, 0, total)
	for _, streamResult := range streams {
		for _, msg := range streamResult.Messages {
			job, err := q.deserializeJob(msg)
			if err != nil {
				logging.WithContext(ctx, q.logger).Warn("library: discard malformed job",
					"stream_id", msg.ID,
					"error", err,
				)
				if ackErr := q.client.XAck(ctx, streamName, group, msg.ID).Err(); ackErr != nil {
					logging.WithContext(ctx, q.logger).Warn("library: ack malformed job failed",
						"stream_id", msg.ID,
						"error", ackErr,
					)
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
	stream, group := q.currentStreamAndGroup()
	if err := q.client.XAck(ctx, stream, group, streamID).Err(); err != nil {
		return fmt.Errorf("library: ack job %q: %w", streamID, err)
	}
	if err := q.client.XDel(ctx, stream, streamID).Err(); err != nil {
		return fmt.Errorf("library: delete job %q: %w", streamID, err)
	}
	return nil
}

// Delete removes a job from the stream without acknowledging it.
func (q *OrchestrationQueue) Delete(ctx context.Context, streamID string) error {
	if streamID == "" {
		return fmt.Errorf("library: stream id is required")
	}
	stream, _ := q.currentStreamAndGroup()
	if err := q.client.XDel(ctx, stream, streamID).Err(); err != nil {
		return fmt.Errorf("library: delete job %q: %w", streamID, err)
	}
	return nil
}

// Retry re-queues a job for another execution attempt.
func (q *OrchestrationQueue) Retry(ctx context.Context, streamID string, job Job) (string, error) {
	if streamID == "" {
		return "", fmt.Errorf("library: stream id is required")
	}

	if q.maxAttemptsReached(job.Attempt) {
		if err := q.moveToDeadLetter(ctx, streamID, job); err != nil {
			return "", err
		}
		return "", ErrJobDeadLettered
	}

	newID, err := q.enqueue(ctx, job, true)
	if err != nil {
		return "", err
	}

	stream, group := q.currentStreamAndGroup()
	if err := q.client.XAck(ctx, stream, group, streamID).Err(); err != nil {
		return "", fmt.Errorf("library: ack job %q for retry: %w", streamID, err)
	}
	if err := q.client.XDel(ctx, stream, streamID).Err(); err != nil {
		return "", fmt.Errorf("library: delete job %q for retry: %w", streamID, err)
	}
	return newID, nil
}

func (q *OrchestrationQueue) enqueue(ctx context.Context, job Job, skipIdempotency bool) (string, error) {
	if job.ID == "" {
		return "", fmt.Errorf("library: job id is required")
	}
	if !skipIdempotency && job.IdempotencyKey != "" {
		prefix, ttl := q.currentIdempotency()
		key := prefix + job.IdempotencyKey
		created, err := q.client.SetNX(ctx, key, job.ID, ttl).Result()
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

	stream, _ := q.currentStreamAndGroup()
	id, err := q.client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: fields}).Result()
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

func (q *OrchestrationQueue) currentStreamAndGroup() (string, string) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.stream, q.group
}

func (q *OrchestrationQueue) currentIdempotency() (string, time.Duration) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.idempotencyPrefix, q.idempotencyTTL
}

func (q *OrchestrationQueue) applyVersion(version string) {
	sanitized := sanitizeKeyVersion(version)
	q.mu.Lock()
	defer q.mu.Unlock()
	q.keyVersion = sanitized
	q.stream = buildRedisKey(q.streamBase, sanitized)
	q.deadLetter = buildRedisKey(q.deadLetterBase, sanitized)
	q.idempotencyPrefix = buildRedisPrefix(q.idempotencyBase, sanitized)
}

// RotateKeyspace updates the stream, dead-letter, and idempotency namespaces.
func (q *OrchestrationQueue) RotateKeyspace(ctx context.Context, version string) error {
	q.applyVersion(version)
	stream, group := q.currentStreamAndGroup()
	if err := q.client.XGroupCreateMkStream(ctx, stream, group, "$").Err(); err != nil {
		if !isBusyGroupError(err) {
			return fmt.Errorf("library: rotate keyspace for group %q: %w", group, err)
		}
	}
	q.mu.RLock()
	deadStream := q.deadLetter
	currentVersion := q.keyVersion
	q.mu.RUnlock()
	logging.WithContext(ctx, q.logger).Info("library: rotated orchestration queue keyspace",
		"stream", stream,
		"dead_letter_stream", deadStream,
		"consumer_group", group,
		"key_version", currentVersion,
	)
	return nil
}

func (q *OrchestrationQueue) maxAttemptsReached(attempt int) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.maxDeliveryAttempts <= 0 {
		return false
	}
	return attempt >= q.maxDeliveryAttempts
}

func (q *OrchestrationQueue) moveToDeadLetter(ctx context.Context, streamID string, job Job) error {
	stream, group := q.currentStreamAndGroup()

	q.mu.RLock()
	deadStream := q.deadLetter
	q.mu.RUnlock()

	reason := strings.TrimSpace(job.ErrorMessage)
	if reason == "" {
		reason = "max attempts exceeded"
	}

	failedAt := time.Now().UTC().Format(time.RFC3339Nano)
	fields := map[string]any{
		"job_id":          job.ID,
		"job_type":        job.JobType,
		"target_id":       job.TargetID,
		"payload_uri":     job.PayloadURI,
		"idempotency_key": job.IdempotencyKey,
		"submitted_at":    job.SubmittedAt.Format(time.RFC3339Nano),
		"attempt":         strconv.Itoa(job.Attempt),
		"failed_at":       failedAt,
		"failure_reason":  reason,
	}

	if err := q.client.XAdd(ctx, &redis.XAddArgs{Stream: deadStream, Values: fields}).Err(); err != nil {
		return fmt.Errorf("library: append to dead-letter stream: %w", err)
	}

	if err := q.client.XAck(ctx, stream, group, streamID).Err(); err != nil {
		return fmt.Errorf("library: ack job %q for dead-letter: %w", streamID, err)
	}
	if err := q.client.XDel(ctx, stream, streamID).Err(); err != nil {
		return fmt.Errorf("library: delete job %q for dead-letter: %w", streamID, err)
	}

	logging.WithContext(ctx, q.logger).Warn("library: job moved to dead-letter stream",
		"stream_id", streamID,
		"dead_letter_stream", deadStream,
		"attempt", job.Attempt,
		"reason", reason,
	)
	return nil
}

func (q *OrchestrationQueue) maybeAlertLag(ctx context.Context) {
	threshold := q.lagAlertThreshold
	if threshold <= 0 {
		return
	}

	now := time.Now()

	q.mu.Lock()
	if !q.lastLagCheck.IsZero() && now.Sub(q.lastLagCheck) < q.lagAlertInterval {
		q.mu.Unlock()
		return
	}
	stream := q.stream
	group := q.group
	q.lastLagCheck = now
	q.mu.Unlock()

	consumers, err := q.client.XInfoConsumers(ctx, stream, group).Result()
	if err != nil {
		logging.WithContext(ctx, q.logger).Warn("library: fetch consumer lag stats failed",
			"stream", stream,
			"group", group,
			"error", err,
		)
		return
	}

	var totalPending int64
	var maxIdle time.Duration
	for _, consumer := range consumers {
		totalPending += consumer.Pending
		if consumer.Idle > maxIdle {
			maxIdle = consumer.Idle
		}
	}

	if totalPending < threshold {
		return
	}

	logging.WithContext(ctx, q.logger).Warn("library: consumer group lag threshold exceeded",
		"stream", stream,
		"group", group,
		"pending", totalPending,
		"max_idle", maxIdle,
		"threshold", threshold,
	)
}

func buildRedisKey(base, version string) string {
	trimmed := strings.Trim(strings.TrimSpace(base), ":")
	if trimmed == "" {
		return ""
	}
	sanitized := sanitizeKeyVersion(version)
	if sanitized == "" {
		return trimmed
	}
	return trimmed + ":" + sanitized
}

func buildRedisPrefix(base, version string) string {
	key := buildRedisKey(base, version)
	if key == "" {
		return ""
	}
	return key + ":"
}

func sanitizeKeyVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.Trim(trimmed, ":")
	if trimmed == "" {
		return ""
	}
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		case r == ':' || r == '/':
			return '-'
		default:
			return -1
		}
	}, trimmed)
	return strings.Trim(sanitized, ":")
}

func isBusyGroupError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "BUSYGROUP")
}
