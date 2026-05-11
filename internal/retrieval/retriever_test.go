package retrieval

import (
	"testing"

	"github.com/Xenn-00/ragnar/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- applyDefaults ---

func TestApplyDefaults_ZeroTopK(t *testing.T) {
	r := &Retriever{}
	req := r.applyDefaults(RetrieveRequest{TopK: 0, MinScore: 0.5})
	assert.Equal(t, defaultTopK, req.TopK)
}

func TestApplyDefaults_NegativeTopK(t *testing.T) {
	r := &Retriever{}
	req := r.applyDefaults(RetrieveRequest{TopK: -3, MinScore: 0.5})
	assert.Equal(t, defaultTopK, req.TopK)
}

func TestApplyDefaults_ZeroMinScore(t *testing.T) {
	r := &Retriever{}
	req := r.applyDefaults(RetrieveRequest{TopK: 5, MinScore: 0})
	assert.Equal(t, defaultMinScore, req.MinScore)
}

func TestApplyDefaults_NegativeMinScore(t *testing.T) {
	r := &Retriever{}
	req := r.applyDefaults(RetrieveRequest{TopK: 5, MinScore: -0.5})
	assert.Equal(t, defaultMinScore, req.MinScore)
}

func TestApplyDefaults_ValidValuesUnchanged(t *testing.T) {
	r := &Retriever{}
	req := r.applyDefaults(RetrieveRequest{TopK: 5, MinScore: 0.5})
	assert.Equal(t, 5, req.TopK)
	assert.Equal(t, 0.5, req.MinScore)
}

func TestApplyDefaults_BothZero(t *testing.T) {
	r := &Retriever{}
	req := r.applyDefaults(RetrieveRequest{})
	assert.Equal(t, defaultTopK, req.TopK)
	assert.Equal(t, defaultMinScore, req.MinScore)
}

// --- rerank ---

func TestRerank_FiltersBelowMinScore(t *testing.T) {
	r := &Retriever{}
	results := []store.SearchResult{
		{Score: 0.8},
		{Score: 0.2}, // below threshold
		{Score: 0.5},
		{Score: 0.1}, // below threshold
		{Score: 0.2}, // below threshold
	}

	filtered := r.rerank(results, 0.3)

	require.Len(t, filtered, 2)
	for _, res := range filtered {
		assert.GreaterOrEqual(t, res.Score, 0.3)
	}
}

func TestRerank_SortsByScoreDescending(t *testing.T) {
	r := &Retriever{}
	results := []store.SearchResult{
		{Score: 0.5},
		{Score: 0.9},
		{Score: 0.7},
	}

	filtered := r.rerank(results, 0.0)

	require.Len(t, filtered, 3)
	assert.Equal(t, 0.9, filtered[0].Score)
	assert.Equal(t, 0.7, filtered[1].Score)
	assert.Equal(t, 0.5, filtered[2].Score)
}

func TestRerank_ExactlyOnThreshold_IsIncluded(t *testing.T) {
	r := &Retriever{}
	results := []store.SearchResult{
		{Score: 0.3}, // exactly on threshold — should be included
		{Score: 0.29},
	}

	filtered := r.rerank(results, 0.3)

	require.Len(t, filtered, 1)
	assert.Equal(t, 0.3, filtered[0].Score)
}

func TestRerank_EmptyInput(t *testing.T) {
	r := &Retriever{}
	filtered := r.rerank([]store.SearchResult{}, 0.3)
	assert.Empty(t, filtered)
}

func TestRerank_AllFilteredOut(t *testing.T) {
	r := &Retriever{}
	results := []store.SearchResult{
		{Score: 0.1},
		{Score: 0.2},
	}

	filtered := r.rerank(results, 0.5)
	assert.Empty(t, filtered)
}

func TestRerank_AllPass(t *testing.T) {
	r := &Retriever{}
	results := []store.SearchResult{
		{Score: 0.9},
		{Score: 0.8},
		{Score: 0.7},
	}

	filtered := r.rerank(results, 0.3)
	require.Len(t, filtered, 3)
}

func TestRerank_PreservesChunkContent(t *testing.T) {
	r := &Retriever{}
	results := []store.SearchResult{
		{Score: 0.9, Content: "first chunk"},
		{Score: 0.4, Content: "second chunk"},
		{Score: 0.1, Content: "third chunk"}, // filtered
	}

	filtered := r.rerank(results, 0.3)

	require.Len(t, filtered, 2)
	// sorted descending — first chunk should be at index 0
	assert.Equal(t, "first chunk", filtered[0].Content)
	assert.Equal(t, "second chunk", filtered[1].Content)
}
