package embedder

import (
	"context"
	"fmt"

	"github.com/Xenn-00/rag-engine/internal/cache"
	"github.com/Xenn-00/rag-engine/internal/metrics"
)

// cachedEmbedder is a decorator that wraps an Embedder with Redis cache.
// this pattern is called the "decorator patter" - all the logic cache fully seperated from
// logic embedding, so OllamaEmbedder always stay clean without any caching logic from Redis.
type cachedEmbedder struct {
	embedder Embedder
	cache    *cache.EmbeddingCache
}

func NewCached(embedder Embedder, cache *cache.EmbeddingCache) Embedder {
	return &cachedEmbedder{
		embedder: embedder,
		cache:    cache,
	}
}

func (c *cachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// 1. Try to get embedding from cache
	cached, err := c.cache.Get(ctx, text)
	if err != nil {
		// cache error - fallback to embedding provider
		// this is intentional, when redis is down, the app should still running
		cached = nil
	}

	if cached != nil {
		metrics.EmbeddingCacheHits.Inc()
		return cached, nil
	}

	metrics.EmbeddingCacheMisses.Inc()

	// 2. Cache miss - get embedding from provider
	embedding, err := c.embedder.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embed text: %w", err)
	}

	// 3. Store the embedding in cache
	_ = c.cache.Set(ctx, text, embedding)
	return embedding, nil
}

func (c *cachedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	missIndexes := make([]int, 0)
	missTexts := make([]string, 0)

	// 1. Try to get embedding from cache for all texts
	for i, text := range texts {
		cached, err := c.cache.Get(ctx, text)
		if err != nil || cached == nil {
			// cache miss - mark it as "should be embedded"
			metrics.EmbeddingCacheMisses.Inc()
			missIndexes = append(missIndexes, i)
			missTexts = append(missTexts, text)
			continue
		}
		metrics.EmbeddingCacheHits.Inc()
		results[i] = cached
	}

	if len(missTexts) == 0 {
		// full cache hit
		return results, nil
	}

	// 2. batch embed only the missed texts
	embeddings, err := c.embedder.EmbedBatch(ctx, missTexts)
	if err != nil {
		return nil, fmt.Errorf("batch embed texts: %w", err)
	}

	// 3. fill the results and store it to cache
	for i, embedding := range embeddings {
		originalIndex := missIndexes[i]
		results[originalIndex] = embedding
		_ = c.cache.Set(ctx, missTexts[i], embedding)
	}

	return results, nil
}
