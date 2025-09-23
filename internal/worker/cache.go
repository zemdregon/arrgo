package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache defines Redis-backed helpers for worker metadata lookups.
type Cache interface {
	GetWorker(ctx context.Context, id string) (Worker, bool, error)
	SaveWorker(ctx context.Context, worker Worker) error
	RemoveWorker(ctx context.Context, id string) error
	ListWorkers(ctx context.Context) ([]Worker, error)
}

// RedisCacheConfig describes the Redis cache options.
type RedisCacheConfig struct {
	Client *redis.Client
	Prefix string
	TTL    time.Duration
}

// RedisCache implements the Cache interface using Redis data structures.
type RedisCache struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

const (
	workerKeyPrefix  = "worker:"
	workerIndexKey   = "workers:index"
	defaultCacheTTL  = 5 * time.Minute
	defaultKeyPrefix = "arrgo:"
)

// NewRedisCache constructs a Redis-backed worker cache.
func NewRedisCache(cfg RedisCacheConfig) (*RedisCache, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("worker: redis client is required")
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = defaultKeyPrefix
	}
	return &RedisCache{client: cfg.Client, prefix: prefix, ttl: ttl}, nil
}

func (r *RedisCache) namespaced(key string) string {
	return r.prefix + key
}

// SaveWorker serializes the worker metadata into Redis and indexes the identifier.
func (r *RedisCache) SaveWorker(ctx context.Context, worker Worker) error {
	payload, err := json.Marshal(worker)
	if err != nil {
		return fmt.Errorf("worker: encode worker cache entry: %w", err)
	}
	key := r.namespaced(workerKeyPrefix + worker.ID)
	if err := r.client.Set(ctx, key, payload, r.ttl).Err(); err != nil {
		return fmt.Errorf("worker: cache worker: %w", err)
	}
	if err := r.client.SAdd(ctx, r.namespaced(workerIndexKey), worker.ID).Err(); err != nil {
		return fmt.Errorf("worker: index worker cache entry: %w", err)
	}
	return nil
}

// GetWorker loads a worker entry from Redis if available.
func (r *RedisCache) GetWorker(ctx context.Context, id string) (Worker, bool, error) {
	key := r.namespaced(workerKeyPrefix + id)
	raw, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return Worker{}, false, nil
		}
		return Worker{}, false, fmt.Errorf("worker: get worker cache entry: %w", err)
	}
	var worker Worker
	if err := json.Unmarshal(raw, &worker); err != nil {
		return Worker{}, false, fmt.Errorf("worker: decode cached worker: %w", err)
	}
	return worker, true, nil
}

// RemoveWorker evicts a worker entry from Redis.
func (r *RedisCache) RemoveWorker(ctx context.Context, id string) error {
	key := r.namespaced(workerKeyPrefix + id)
	if err := r.client.Del(ctx, key).Err(); err != nil && err != redis.Nil {
		return fmt.Errorf("worker: delete worker cache entry: %w", err)
	}
	if err := r.client.SRem(ctx, r.namespaced(workerIndexKey), id).Err(); err != nil {
		return fmt.Errorf("worker: remove worker index entry: %w", err)
	}
	return nil
}

// ListWorkers returns cached worker entries.
func (r *RedisCache) ListWorkers(ctx context.Context) ([]Worker, error) {
	ids, err := r.client.SMembers(ctx, r.namespaced(workerIndexKey)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("worker: list worker ids: %w", err)
	}
	workers := make([]Worker, 0, len(ids))
	for _, id := range ids {
		worker, ok, err := r.GetWorker(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		workers = append(workers, worker)
	}
	return workers, nil
}
