package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements the Cache interface using Redis as the backing store.
type RedisCache struct {
	client *redis.Client
	prefix string
}

// RedisCacheConfig describes the Redis-backed cache options.
type RedisCacheConfig struct {
	Client *redis.Client
	Prefix string
}

// NewRedisCache constructs a Redis-backed cache implementation.
func NewRedisCache(cfg RedisCacheConfig) (*RedisCache, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("metadata: redis client is required")
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "arrgo:"
	}

	return &RedisCache{
		client: cfg.Client,
		prefix: prefix,
	}, nil
}

func (r *RedisCache) namespaced(key string) string {
	return r.prefix + key
}

// Fetch retrieves metadata from Redis.
func (r *RedisCache) Fetch(ctx context.Context, key string) (Metadata, bool, error) {
	namespaced := r.namespaced(key)
	raw, err := r.client.Get(ctx, namespaced).Bytes()
	if err != nil {
		if err == redis.Nil {
			return Metadata{}, false, nil
		}
		return Metadata{}, false, fmt.Errorf("metadata: redis get %q: %w", namespaced, err)
	}

	var metadata Metadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return Metadata{}, false, fmt.Errorf("metadata: decode cached metadata: %w", err)
	}

	return metadata, true, nil
}

// Save stores metadata in Redis with the provided TTL.
func (r *RedisCache) Save(ctx context.Context, key string, value Metadata, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("metadata: encode metadata for cache: %w", err)
	}

	namespaced := r.namespaced(key)
	if err := r.client.Set(ctx, namespaced, payload, ttl).Err(); err != nil {
		return fmt.Errorf("metadata: redis set %q: %w", namespaced, err)
	}

	return nil
}
