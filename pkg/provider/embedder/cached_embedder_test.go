package embedder

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type MockEmbedder struct {
	mock.Mock
}

type MockEmbeddingCache struct {
	mock.Mock
}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	args := m.Called(ctx, text)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	args := m.Called(ctx, texts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *MockEmbeddingCache) Get(ctx context.Context, text string) ([]float32, error) {
	args := m.Called(ctx, text)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockEmbeddingCache) Set(ctx context.Context, text string, embedding []float32) error {
	args := m.Called(ctx, text, embedding)
	return args.Error(0)
}

func (m *MockEmbeddingCache) Ping(ctx context.Context) error {
	arg := m.Called(ctx)
	if arg.Get(0) == nil {
		return arg.Error(1)
	}

	return arg.Get(0).(error)
}

func (m *MockEmbeddingCache) Close() error {
	if m == nil {
		return nil
	}

	return m.Close()
}

// --- Embed tests ---

func TestCachedEmbedder_Embed_CacheHit(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	cached := []float32{0.1, 0.2, 0.3}
	mockCache.On("Get", mock.Anything, "hello").Return(cached, nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	result, err := ce.Embed(context.Background(), "hello")

	require.NoError(t, err)
	assert.Equal(t, cached, result)

	// underlying embedder should NOT be called on cache hit
	mockEmbed.AssertNotCalled(t, "Embed", mock.Anything, mock.Anything)
}

func TestCachedEmbedder_Embed_CacheMiss(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	embedding := []float32{0.4, 0.5, 0.6}
	mockCache.On("Get", mock.Anything, "hello").Return(nil, nil)
	mockEmbed.On("Embed", mock.Anything, "hello").Return(embedding, nil)
	mockCache.On("Set", mock.Anything, "hello", embedding).Return(nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	result, err := ce.Embed(context.Background(), "hello")

	require.NoError(t, err)
	assert.Equal(t, embedding, result)

	mockEmbed.AssertCalled(t, "Embed", mock.Anything, "hello")
	mockCache.AssertCalled(t, "Set", mock.Anything, "hello", embedding)
}

func TestCachedEmbedder_Embed_CacheError_FallsBackToProvider(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	embedding := []float32{0.7, 0.8, 0.9}
	// Redis is down - cache returns error
	mockCache.On("Get", mock.Anything, "hello").Return(nil, errors.New("redis: connection refused"))
	mockEmbed.On("Embed", mock.Anything, "hello").Return(embedding, nil)
	mockCache.On("Set", mock.Anything, "hello", embedding).Return(nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	result, err := ce.Embed(context.Background(), "hello")

	// should still succeed — graceful degradation
	require.NoError(t, err)
	assert.Equal(t, embedding, result)
}

func TestCachedEmbedder_Embed_ProviderError(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	mockCache.On("Get", mock.Anything, "hello").Return(nil, nil)
	mockEmbed.On("Embed", mock.Anything, "hello").Return(nil, errors.New("ollama: model not found"))

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	result, err := ce.Embed(context.Background(), "hello")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "embed text")
}

// --- EmbedBatch tests ---

func TestCachedEmbedder_EmbedBatch_AllCacheHit(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	e1 := []float32{0.1, 0.2}
	e2 := []float32{0.3, 0.4}

	mockCache.On("Get", mock.Anything, "text1").Return(e1, nil)
	mockCache.On("Get", mock.Anything, "text2").Return(e2, nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	results, err := ce.EmbedBatch(context.Background(), []string{"text1", "text2"})

	require.NoError(t, err)
	assert.Equal(t, e1, results[0])
	assert.Equal(t, e2, results[1])

	// provider should not be called at all
	mockEmbed.AssertNotCalled(t, "EmbedBatch", mock.Anything, mock.Anything)
}

func TestCachedEmbedder_EmbedBatch_PartialCacheMiss(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	e1 := []float32{0.1, 0.2}
	e2 := []float32{0.3, 0.4}

	// text1 is cached, text2 is a miss
	mockCache.On("Get", mock.Anything, "text1").Return(e1, nil)
	mockCache.On("Get", mock.Anything, "text2").Return(nil, nil)

	// only text2 should be sent to provider
	mockEmbed.On("EmbedBatch", mock.Anything, []string{"text2"}).Return([][]float32{e2}, nil)
	mockCache.On("Set", mock.Anything, "text2", e2).Return(nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	results, err := ce.EmbedBatch(context.Background(), []string{"text1", "text2"})

	require.NoError(t, err)
	assert.Equal(t, e1, results[0]) // from cache
	assert.Equal(t, e2, results[1]) // from provider

	mockEmbed.AssertCalled(t, "EmbedBatch", mock.Anything, []string{"text2"})
}

func TestCachedEmbedder_EmbedBatch_AllCacheMiss(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	e1 := []float32{0.1, 0.2}
	e2 := []float32{0.3, 0.4}

	mockCache.On("Get", mock.Anything, "text1").Return(nil, nil)
	mockCache.On("Get", mock.Anything, "text2").Return(nil, nil)
	mockEmbed.On("EmbedBatch", mock.Anything, []string{"text1", "text2"}).Return([][]float32{e1, e2}, nil)
	mockCache.On("Set", mock.Anything, "text1", e1).Return(nil)
	mockCache.On("Set", mock.Anything, "text2", e2).Return(nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	results, err := ce.EmbedBatch(context.Background(), []string{"text1", "text2"})

	require.NoError(t, err)
	assert.Equal(t, e1, results[0])
	assert.Equal(t, e2, results[1])
}

func TestCachedEmbedder_EmbedBatch_ProviderError(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	mockCache.On("Get", mock.Anything, "text1").Return(nil, nil)
	mockEmbed.On("EmbedBatch", mock.Anything, []string{"text1"}).Return(nil, errors.New("ollama: timeout"))

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	results, err := ce.EmbedBatch(context.Background(), []string{"text1"})

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "batch embed texts")
}

func TestCachedEmbedder_EmbedBatch_OrderPreserved(t *testing.T) {
	mockEmbed := new(MockEmbedder)
	mockCache := new(MockEmbeddingCache)

	// 4 texts: index 0,2 cached — index 1,3 miss
	e0 := []float32{0.0}
	e1 := []float32{0.1}
	e2 := []float32{0.2}
	e3 := []float32{0.3}

	mockCache.On("Get", mock.Anything, "t0").Return(e0, nil)
	mockCache.On("Get", mock.Anything, "t1").Return(nil, nil)
	mockCache.On("Get", mock.Anything, "t2").Return(e2, nil)
	mockCache.On("Get", mock.Anything, "t3").Return(nil, nil)

	mockEmbed.On("EmbedBatch", mock.Anything, []string{"t1", "t3"}).Return([][]float32{e1, e3}, nil)
	mockCache.On("Set", mock.Anything, "t1", e1).Return(nil)
	mockCache.On("Set", mock.Anything, "t3", e3).Return(nil)

	ce := &cachedEmbedder{embedder: mockEmbed, cache: mockCache}
	results, err := ce.EmbedBatch(context.Background(), []string{"t0", "t1", "t2", "t3"})

	require.NoError(t, err)
	assert.Equal(t, e0, results[0])
	assert.Equal(t, e1, results[1])
	assert.Equal(t, e2, results[2])
	assert.Equal(t, e3, results[3])
}
