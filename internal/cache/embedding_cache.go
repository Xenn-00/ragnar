package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Xenn-00/rag-engine/internal/config"
	"github.com/redis/go-redis/v9"
)

const (
	embeddingKeyPrefix = "embed:"
	defaultTTL         = 24 * time.Hour
)

type EmbeddingCache struct {
	client *redis.Client
	ttl    time.Duration
}

type EmbeddingCacheInterface interface {
	Get(ctx context.Context, text string) ([]float32, error)
	Set(ctx context.Context, text string, embedding []float32) error
}

func NewEmbeddingCache(cfg *config.RedisConfig) *EmbeddingCache {
	client := redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
	})

	return &EmbeddingCache{
		client: client,
		ttl:    defaultTTL,
	}
}

func (c *EmbeddingCache) Close() error {
	return c.client.Close()
}

func (c *EmbeddingCache) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}

	return nil
}

// Get retrieves a cached embedding by text content.
// Returns (nil, nil) if cache miss - not an error
func (c *EmbeddingCache) Get(ctx context.Context, text string) ([]float32, error) {
	key := c.buildKey(text)

	val, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		// cache miss
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get embedding: %w", err)
	}

	var embedding []float32
	if err := json.Unmarshal(val, &embedding); err != nil {
		return nil, fmt.Errorf("unmarshal embedding: %w", err)
	}

	return embedding, nil
}

// Set stores an embedding in the cache with the associated text content as the key.
func (c *EmbeddingCache) Set(ctx context.Context, text string, embedding []float32) error {
	key := c.buildKey(text)

	val, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	if err := c.client.Set(ctx, key, val, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis set embedding: %w", err)
	}

	return nil
}

// buildKey constructs the Redis key for a given text content
func (c *EmbeddingCache) buildKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%s%x", embeddingKeyPrefix, hash)
}
