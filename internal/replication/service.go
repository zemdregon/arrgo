package replication

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

const versionTimestampFormat = "20060102T150405.000000000Z07"

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// ObjectStorageClient captures the S3-compatible primitives required for replication.
type ObjectStorageClient interface {
	PutObject(ctx context.Context, bucket, key string, body io.Reader, opts PutObjectOptions) error
	ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error)
	DeleteObject(ctx context.Context, bucket, key string) error
}

// PutObjectOptions customizes object upload behavior.
type PutObjectOptions struct {
	ContentType  string
	Metadata     map[string]string
	StorageClass string
	UploadedAt   time.Time
}

// ObjectInfo represents an object stored in a bucket.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// RetentionPolicy defines retention bounds applied to replicated objects.
type RetentionPolicy struct {
	MaxVersions int
	MaxAge      time.Duration
}

// Enabled returns true when the policy enforces at least one rule.
func (p RetentionPolicy) Enabled() bool {
	return p.MaxVersions > 0 || p.MaxAge > 0
}

// Target describes a replication destination.
type Target struct {
	Name         string
	Region       string
	Endpoint     string
	Bucket       string
	Prefix       string
	StorageClass string
	Client       ObjectStorageClient
	Retention    *RetentionPolicy
}

// Workflow orchestrates replication across one or more targets.
type Workflow struct {
	Name         string
	SourceRegion string
	Description  string
	Targets      []Target
	Retention    RetentionPolicy
}

// Config controls Service construction.
type Config struct {
	Logger    *slog.Logger
	Clock     Clock
	Workflows []Workflow
}

// Errors returned by the replication service.
var (
	ErrWorkflowExists    = errors.New("replication: workflow already exists")
	ErrUnknownWorkflow   = errors.New("replication: workflow not found")
	ErrInvalidWorkflow   = errors.New("replication: workflow invalid")
	ErrInvalidTarget     = errors.New("replication: target invalid")
	ErrContentRequired   = errors.New("replication: content is required")
	ErrObjectKeyRequired = errors.New("replication: object key is required")
	ErrWorkflowRequired  = errors.New("replication: workflow name is required")
)

// Service coordinates cross-region replication workflows.
type Service struct {
	mu        sync.RWMutex
	workflows map[string]*Workflow
	logger    *slog.Logger
	clock     Clock
}

// NewService constructs a replication service with optional pre-registered workflows.
func NewService(cfg Config) (*Service, error) {
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	svc := &Service{
		workflows: make(map[string]*Workflow),
		logger:    logger,
		clock:     clock,
	}

	for _, wf := range cfg.Workflows {
		if err := svc.RegisterWorkflow(wf); err != nil {
			return nil, err
		}
	}

	return svc, nil
}

// RegisterWorkflow registers a replication workflow.
func (s *Service) RegisterWorkflow(workflow Workflow) error {
	normalized, err := s.normalizeWorkflow(workflow)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workflows[normalized.Name]; exists {
		return ErrWorkflowExists
	}
	s.workflows[normalized.Name] = normalized
	s.logger.Info("registered replication workflow", "workflow", normalized.Name, "targets", len(normalized.Targets))
	return nil
}

// ReplicationRequest defines the data to copy across regions.
type ReplicationRequest struct {
	Workflow      string
	ObjectKey     string
	Version       string
	Content       io.ReadSeeker
	ContentLength int64
	ContentType   string
	Metadata      map[string]string
}

// TargetResult describes replication state for a single target.
type TargetResult struct {
	TargetName     string
	Region         string
	Bucket         string
	ObjectKey      string
	StorageClass   string
	ContentLength  int64
	ReplicatedAt   time.Time
	DeletedObjects []string
	Err            error
}

// ReplicationResult summarizes a replication attempt across all targets.
type ReplicationResult struct {
	Workflow    string
	Version     string
	StartedAt   time.Time
	CompletedAt time.Time
	Targets     []TargetResult
}

// Replicate copies the provided payload to all workflow targets.
func (s *Service) Replicate(ctx context.Context, req ReplicationRequest) (ReplicationResult, error) {
	result := ReplicationResult{}
	if strings.TrimSpace(req.Workflow) == "" {
		return result, ErrWorkflowRequired
	}
	if strings.TrimSpace(req.ObjectKey) == "" {
		return result, ErrObjectKeyRequired
	}
	if req.Content == nil {
		return result, ErrContentRequired
	}

	s.mu.RLock()
	workflow, ok := s.workflows[strings.TrimSpace(req.Workflow)]
	s.mu.RUnlock()
	if !ok {
		return result, ErrUnknownWorkflow
	}

	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = s.clock.Now().UTC().Format(versionTimestampFormat)
	}

	startedAt := s.clock.Now()
	result = ReplicationResult{
		Workflow:  workflow.Name,
		Version:   version,
		StartedAt: startedAt,
		Targets:   make([]TargetResult, 0, len(workflow.Targets)),
	}

	var errs error
	for _, target := range workflow.Targets {
		targetResult := TargetResult{
			TargetName:    target.Name,
			Region:        target.Region,
			Bucket:        target.Bucket,
			StorageClass:  target.StorageClass,
			ContentLength: req.ContentLength,
		}

		objectKey, retentionPrefix, matchSuffix := buildObjectKey(target.Prefix, req.ObjectKey, version)
		if _, err := req.Content.Seek(0, io.SeekStart); err != nil {
			wrapped := fmt.Errorf("replication: reset content for target %q: %w", target.Name, err)
			targetResult.Err = wrapped
			errs = errors.Join(errs, wrapped)
			result.Targets = append(result.Targets, targetResult)
			continue
		}

		body := io.Reader(req.Content)
		if req.ContentLength >= 0 {
			body = io.LimitReader(req.Content, req.ContentLength)
		}

		uploadedAt := s.clock.Now()
		options := PutObjectOptions{
			ContentType:  req.ContentType,
			Metadata:     cloneStringMap(req.Metadata),
			StorageClass: target.StorageClass,
			UploadedAt:   uploadedAt,
		}

		if err := target.Client.PutObject(ctx, target.Bucket, objectKey, body, options); err != nil {
			wrapped := fmt.Errorf("replication: put object on target %q: %w", target.Name, err)
			targetResult.Err = wrapped
			errs = errors.Join(errs, wrapped)
			result.Targets = append(result.Targets, targetResult)
			continue
		}

		targetResult.ObjectKey = objectKey
		targetResult.ReplicatedAt = uploadedAt

		policy := workflow.Retention
		if target.Retention != nil {
			policy = *target.Retention
		}
		if policy.Enabled() {
			deleted, err := s.enforceRetention(ctx, target, policy, objectKey, retentionPrefix, matchSuffix, uploadedAt)
			if err != nil {
				wrapped := fmt.Errorf("replication: enforce retention on target %q: %w", target.Name, err)
				targetResult.Err = wrapped
				errs = errors.Join(errs, wrapped)
			} else {
				targetResult.DeletedObjects = deleted
			}
		}

		result.Targets = append(result.Targets, targetResult)
	}

	result.CompletedAt = s.clock.Now()
	return result, errs
}

// normalizeWorkflow validates and copies workflow data.
func (s *Service) normalizeWorkflow(workflow Workflow) (*Workflow, error) {
	name := strings.TrimSpace(workflow.Name)
	if name == "" {
		return nil, ErrInvalidWorkflow
	}
	if len(workflow.Targets) == 0 {
		return nil, fmt.Errorf("%w: at least one target required", ErrInvalidWorkflow)
	}
	if workflow.Retention.MaxVersions < 0 {
		return nil, fmt.Errorf("%w: max versions cannot be negative", ErrInvalidWorkflow)
	}
	if workflow.Retention.MaxAge < 0 {
		return nil, fmt.Errorf("%w: max age cannot be negative", ErrInvalidWorkflow)
	}

	normalized := &Workflow{
		Name:         name,
		SourceRegion: strings.TrimSpace(workflow.SourceRegion),
		Description:  strings.TrimSpace(workflow.Description),
		Retention:    workflow.Retention,
	}
	normalized.Targets = make([]Target, len(workflow.Targets))
	for i, target := range workflow.Targets {
		normTarget, err := s.normalizeTarget(target)
		if err != nil {
			return nil, err
		}
		normalized.Targets[i] = normTarget
	}
	return normalized, nil
}

func (s *Service) normalizeTarget(target Target) (Target, error) {
	if target.Client == nil {
		return Target{}, fmt.Errorf("%w: client is required for target %q", ErrInvalidTarget, target.Name)
	}
	if target.Retention != nil {
		if target.Retention.MaxVersions < 0 {
			return Target{}, fmt.Errorf("%w: max versions cannot be negative for target %q", ErrInvalidTarget, target.Name)
		}
		if target.Retention.MaxAge < 0 {
			return Target{}, fmt.Errorf("%w: max age cannot be negative for target %q", ErrInvalidTarget, target.Name)
		}
		policy := *target.Retention
		target.Retention = &policy
	}

	target.Name = strings.TrimSpace(target.Name)
	target.Region = strings.TrimSpace(target.Region)
	target.Endpoint = strings.TrimSpace(target.Endpoint)
	target.Bucket = strings.TrimSpace(target.Bucket)
	target.Prefix = strings.Trim(target.Prefix, "/")
	target.StorageClass = strings.TrimSpace(target.StorageClass)
	return target, nil
}

func (s *Service) enforceRetention(ctx context.Context, target Target, policy RetentionPolicy, currentKey, retentionPrefix, matchSuffix string, now time.Time) ([]string, error) {
	objects, err := target.Client.ListObjects(ctx, target.Bucket, retentionPrefix)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	filtered := make([]ObjectInfo, 0, len(objects))
	for _, obj := range objects {
		if obj.Key == "" {
			continue
		}
		if matchSuffix != "" && !strings.HasSuffix(obj.Key, matchSuffix) {
			continue
		}
		filtered = append(filtered, obj)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].LastModified.Equal(filtered[j].LastModified) {
			return filtered[i].Key > filtered[j].Key
		}
		return filtered[i].LastModified.After(filtered[j].LastModified)
	})

	toDelete := make(map[string]struct{})
	if policy.MaxVersions > 0 && len(filtered) > policy.MaxVersions {
		for _, obj := range filtered[policy.MaxVersions:] {
			toDelete[obj.Key] = struct{}{}
		}
	}
	if policy.MaxAge > 0 {
		cutoff := now.Add(-policy.MaxAge)
		for _, obj := range filtered {
			if obj.LastModified.Before(cutoff) {
				toDelete[obj.Key] = struct{}{}
			}
		}
	}

	if len(toDelete) == 0 {
		return nil, nil
	}

	deleted := make([]string, 0, len(toDelete))
	for key := range toDelete {
		if key == currentKey {
			continue
		}
		if err := target.Client.DeleteObject(ctx, target.Bucket, key); err != nil {
			return nil, fmt.Errorf("delete object %q: %w", key, err)
		}
		deleted = append(deleted, key)
	}
	sort.Strings(deleted)
	return deleted, nil
}

func buildObjectKey(prefix, objectKey, version string) (objectPath string, retentionPrefix string, matchSuffix string) {
	cleanedPrefix := strings.Trim(prefix, "/")
	segments := splitPath(objectKey)

	var dirSegments []string
	var fileName string
	if len(segments) > 0 {
		dirSegments = segments[:len(segments)-1]
		fileName = segments[len(segments)-1]
	}

	prefixSegments := make([]string, 0, len(dirSegments)+1)
	if cleanedPrefix != "" {
		prefixSegments = append(prefixSegments, cleanedPrefix)
	}
	prefixSegments = append(prefixSegments, dirSegments...)

	if fileName != "" {
		matchSuffix = "/" + fileName
	}

	retentionPrefix = strings.Join(prefixSegments, "/")
	if retentionPrefix != "" {
		retentionPrefix += "/"
	}

	objectSegments := make([]string, 0, len(prefixSegments)+2)
	objectSegments = append(objectSegments, prefixSegments...)
	if version != "" {
		objectSegments = append(objectSegments, version)
	}
	if fileName != "" {
		objectSegments = append(objectSegments, fileName)
	}
	objectPath = strings.Join(objectSegments, "/")
	return objectPath, retentionPrefix, matchSuffix
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return cleaned
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
