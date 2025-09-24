package replication

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type storedObject struct {
	data         []byte
	metadata     map[string]string
	lastModified time.Time
	storageClass string
}

type memoryStorage struct {
	mu      sync.RWMutex
	objects map[string]map[string]storedObject
	clock   Clock
}

func newMemoryStorage(clock Clock) *memoryStorage {
	return &memoryStorage{objects: make(map[string]map[string]storedObject), clock: clock}
}

func (m *memoryStorage) PutObject(ctx context.Context, bucket, key string, body io.Reader, opts PutObjectOptions) error {
	if bucket == "" || key == "" {
		return errors.New("bucket and key are required")
	}
	payload, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	uploadedAt := opts.UploadedAt
	if uploadedAt.IsZero() {
		uploadedAt = m.clock.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[bucket]; !ok {
		m.objects[bucket] = make(map[string]storedObject)
	}
	m.objects[bucket][key] = storedObject{
		data:         append([]byte(nil), payload...),
		metadata:     cloneStringMap(opts.Metadata),
		lastModified: uploadedAt,
		storageClass: opts.StorageClass,
	}
	return nil
}

func (m *memoryStorage) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	bucketObjects, ok := m.objects[bucket]
	if !ok {
		return nil, nil
	}
	infos := make([]ObjectInfo, 0)
	for key, obj := range bucketObjects {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		infos = append(infos, ObjectInfo{Key: key, Size: int64(len(obj.data)), LastModified: obj.lastModified})
	}
	return infos, nil
}

func (m *memoryStorage) DeleteObject(ctx context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[bucket]; !ok {
		return nil
	}
	delete(m.objects[bucket], key)
	return nil
}

func (m *memoryStorage) getObject(bucket, key string) (storedObject, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	bucketObjects, ok := m.objects[bucket]
	if !ok {
		return storedObject{}, false
	}
	obj, ok := bucketObjects[key]
	return obj, ok
}

func TestServiceReplicateAcrossTargets(t *testing.T) {
	clock := newFakeClock(time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC))
	primaryStorage := newMemoryStorage(clock)
	secondaryStorage := newMemoryStorage(clock)

	svc, err := NewService(Config{Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	workflow := Workflow{
		Name:         "global-media",
		SourceRegion: "us-east-1",
		Targets: []Target{
			{
				Name:         "primary-us-east",
				Region:       "us-east-1",
				Bucket:       "arrgo-primary",
				Prefix:       "media",
				StorageClass: "STANDARD",
				Client:       primaryStorage,
			},
			{
				Name:         "secondary-eu-central",
				Region:       "eu-central-1",
				Bucket:       "arrgo-secondary",
				Prefix:       "media",
				StorageClass: "STANDARD_IA",
				Client:       secondaryStorage,
			},
		},
		Retention: RetentionPolicy{MaxVersions: 3},
	}
	if err := svc.RegisterWorkflow(workflow); err != nil {
		t.Fatalf("RegisterWorkflow: %v", err)
	}

	content := []byte("the expanse - dulcinea")
	req := ReplicationRequest{
		Workflow:      "global-media",
		ObjectKey:     "series/the-expanse/s01e01.mkv",
		Content:       bytes.NewReader(content),
		ContentLength: int64(len(content)),
		ContentType:   "video/x-matroska",
		Metadata: map[string]string{
			"entity_id": "series-1",
			"episode":   "s01e01",
		},
	}

	res, err := svc.Replicate(context.Background(), req)
	if err != nil {
		t.Fatalf("Replicate: %v", err)
	}
	if res.Workflow != "global-media" {
		t.Fatalf("unexpected workflow: %s", res.Workflow)
	}
	if len(res.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(res.Targets))
	}

	for _, target := range res.Targets {
		if target.Err != nil {
			t.Fatalf("unexpected error for target %s: %v", target.TargetName, target.Err)
		}
		if target.ObjectKey == "" {
			t.Fatalf("target %s missing object key", target.TargetName)
		}
		if !strings.Contains(target.ObjectKey, res.Version) {
			t.Fatalf("target %s object key %q missing version %q", target.TargetName, target.ObjectKey, res.Version)
		}
		storage := primaryStorage
		bucket := "arrgo-primary"
		if target.TargetName == "secondary-eu-central" {
			storage = secondaryStorage
			bucket = "arrgo-secondary"
		}
		obj, ok := storage.getObject(bucket, target.ObjectKey)
		if !ok {
			t.Fatalf("target %s missing replicated object", target.TargetName)
		}
		if target.ContentLength != int64(len(content)) {
			t.Fatalf("target %s content length mismatch", target.TargetName)
		}
		if got := string(obj.data); got != string(content) {
			t.Fatalf("target %s stored data mismatch: %s", target.TargetName, got)
		}
		if obj.storageClass != target.StorageClass {
			t.Fatalf("target %s storage class mismatch", target.TargetName)
		}
	}
}

func TestServiceRetentionMaxVersions(t *testing.T) {
	clock := newFakeClock(time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC))
	storage := newMemoryStorage(clock)

	svc, err := NewService(Config{Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	workflow := Workflow{
		Name: "library-archive",
		Targets: []Target{{
			Name:   "archive",
			Region: "us-west-2",
			Bucket: "arrgo-archive",
			Prefix: "archive",
			Client: storage,
		}},
		Retention: RetentionPolicy{MaxVersions: 2},
	}
	if err := svc.RegisterWorkflow(workflow); err != nil {
		t.Fatalf("RegisterWorkflow: %v", err)
	}

	content := []byte("data")
	var lastResult ReplicationResult
	for i := 0; i < 3; i++ {
		req := ReplicationRequest{
			Workflow:      "library-archive",
			ObjectKey:     "movies/blade-runner-2049.mkv",
			Content:       bytes.NewReader(content),
			ContentLength: int64(len(content)),
		}
		res, err := svc.Replicate(context.Background(), req)
		if err != nil {
			t.Fatalf("Replicate iteration %d: %v", i, err)
		}
		lastResult = res
		clock.Advance(time.Hour)
	}

	infos, err := storage.ListObjects(context.Background(), "arrgo-archive", "archive/movies")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 retained versions, got %d", len(infos))
	}
	if len(lastResult.Targets) != 1 {
		t.Fatalf("expected single target result, got %d", len(lastResult.Targets))
	}
	if len(lastResult.Targets[0].DeletedObjects) != 1 {
		t.Fatalf("expected 1 deleted object, got %d", len(lastResult.Targets[0].DeletedObjects))
	}
}

func TestServiceRetentionMaxAge(t *testing.T) {
	clock := newFakeClock(time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC))
	storage := newMemoryStorage(clock)

	svc, err := NewService(Config{Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	workflow := Workflow{
		Name: "age-based",
		Targets: []Target{{
			Name:   "long-term",
			Region: "ap-southeast-1",
			Bucket: "arrgo-long-term",
			Prefix: "media",
			Client: storage,
		}},
		Retention: RetentionPolicy{MaxAge: 24 * time.Hour},
	}
	if err := svc.RegisterWorkflow(workflow); err != nil {
		t.Fatalf("RegisterWorkflow: %v", err)
	}

	content := []byte("archive")
	req := ReplicationRequest{
		Workflow:      "age-based",
		ObjectKey:     "music/daft-punk/one-more-time.flac",
		Content:       bytes.NewReader(content),
		ContentLength: int64(len(content)),
	}
	if _, err := svc.Replicate(context.Background(), req); err != nil {
		t.Fatalf("initial replicate: %v", err)
	}
	clock.Advance(48 * time.Hour)
	res, err := svc.Replicate(context.Background(), req)
	if err != nil {
		t.Fatalf("second replicate: %v", err)
	}

	infos, err := storage.ListObjects(context.Background(), "arrgo-long-term", "media/music/daft-punk")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 retained version, got %d", len(infos))
	}
	if len(res.Targets) != 1 {
		t.Fatalf("expected single target, got %d", len(res.Targets))
	}
	if len(res.Targets[0].DeletedObjects) != 1 {
		t.Fatalf("expected 1 deleted object due to max age, got %d", len(res.Targets[0].DeletedObjects))
	}
}

func TestReplicateUnknownWorkflow(t *testing.T) {
	svc, err := NewService(Config{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Replicate(context.Background(), ReplicationRequest{Workflow: "missing", ObjectKey: "path", Content: bytes.NewReader([]byte("a")), ContentLength: 1})
	if !errors.Is(err, ErrUnknownWorkflow) {
		t.Fatalf("expected ErrUnknownWorkflow, got %v", err)
	}
}

func TestRegisterWorkflowValidation(t *testing.T) {
	svc, err := NewService(Config{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	err = svc.RegisterWorkflow(Workflow{Name: "", Targets: []Target{{Name: "x", Client: newMemoryStorage(systemClock{})}}})
	if !errors.Is(err, ErrInvalidWorkflow) {
		t.Fatalf("expected ErrInvalidWorkflow, got %v", err)
	}

	err = svc.RegisterWorkflow(Workflow{Name: "invalid", Targets: nil})
	if err == nil {
		t.Fatalf("expected error for missing targets")
	}
}
