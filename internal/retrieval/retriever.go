package retrieval

import (
	"context"
	"fmt"
	"sort"

	"github.com/Xenn-00/rag-engine/internal/store"
	"github.com/Xenn-00/rag-engine/pkg/provider/embedder"
)

const (
	defaultTopK     = 5
	defaultMinScore = 0.3 // throw away the chunks with similarity < 0.3
)

type RetrieveRequest struct {
	TopK     int
	MinScore float64
}

type Retriever struct {
	embedder   embedder.Embedder
	chunkStore *store.ChunkStore
}

func NewRetriever(embedder embedder.Embedder, chunkStore *store.ChunkStore) *Retriever {
	return &Retriever{
		embedder:   embedder,
		chunkStore: chunkStore,
	}
}

// Retrieve embed query + similarity search + rerank
// Return filtered and sorted top result
func (r *Retriever) Retrieve(ctx context.Context, query string, req RetrieveRequest) ([]store.SearchResult, error) {
	req = r.applyDefaults(req)

	// 1. embed query
	queryVec, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. similarity search
	results, err := r.chunkStore.SimilaritySearch(ctx, queryVec, req.TopK)
	if err != nil {
		return nil, fmt.Errorf("similarity search: %w", err)
	}

	// 3. rerank
	return r.rerank(results, req.MinScore), nil
}

// rerank filter chunks below the minScore and sort by score desending.
// es ist ein simplisches heuristic reranker - reich es für v1 aus.
// later could be upgraded to cross-encoder model
func (r *Retriever) rerank(results []store.SearchResult, minScore float64) []store.SearchResult {
	filtered := results[:0] // reuse slice, zero allocation
	for _, r := range results {
		if r.Score >= minScore {
			filtered = append(filtered, r)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	return filtered
}

func (r *Retriever) applyDefaults(req RetrieveRequest) RetrieveRequest {
	if req.TopK <= 0 {
		req.TopK = defaultTopK
	}

	if req.MinScore <= 0 {
		req.MinScore = defaultMinScore
	}

	return req
}
