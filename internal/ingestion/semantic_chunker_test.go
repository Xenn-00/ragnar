package ingestion

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEmbedder struct {
	vectors map[string][]float32
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if v, ok := m.vectors[text]; ok {
		return v, nil
	}

	return []float32{0.5, 0.2}, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, t := range texts {
		if v, ok := m.vectors[t]; ok {
			result[i] = v
		} else {
			result[i] = []float32{0.5, 0.5}
		}
	}

	return result, nil
}

func TestSemanticChunker_EmptyDocument(t *testing.T) {
	emb := &mockEmbedder{}
	c := NewSemanticChunker(context.Background(), emb, SemanticChunkerConfig{})
	chunks := c.Chunk(&ParsedDocument{Content: ""})
	assert.Nil(t, chunks)
}

func TestSemanticChunker_SingleSentence(t *testing.T) {
	emb := &mockEmbedder{}
	c := NewSemanticChunker(context.Background(), emb, SemanticChunkerConfig{})
	chunks := c.Chunk(&ParsedDocument{Content: "PostgreSQL is a relational database."})
	require.Len(t, chunks, 1)
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, "semantic", chunks[0].Metadata["chunker"])
}

func TestSemanticChunker_BoundaryDetected(t *testing.T) {
	// 2 different topics -> low similarity -> must be into 2 chunk
	emb := &mockEmbedder{
		vectors: map[string][]float32{
			"PostgreSQL supports ACID transactions.": {1.0, 0.0},
			"It uses MVCC for concurrency.":          {0.9, 0.1}, // similar -> same chunk
			"Python is a programming language.":      {0.0, 1.0}, // different topic -> boundary
			"It is dynamically typed.":               {0.1, 0.9}, // similar Python
		},
	}
	// debug: check, is split result exact match with map key
	sents := splitSentences("PostgreSQL supports ACID transactions. It uses MVCC for concurrency. Python is a programming language. It is dynamically typed.")
	for i, s := range sents {
		_, ok := emb.vectors[s]
		t.Logf("sent[%d] = %q → in map: %v", i, s, ok)
	}

	c := NewSemanticChunker(context.Background(), emb, SemanticChunkerConfig{
		Threshold: 0.75,
		MinSents:  1,
	})

	doc := &ParsedDocument{
		Content: "PostgreSQL supports ACID transactions. It uses MVCC for concurrency. Python is a programming language. It is dynamically typed.",
	}
	chunks := c.Chunk(doc)
	require.Len(t, chunks, 2)
	assert.Contains(t, chunks[0].Content, "PostgreSQL")
	assert.Contains(t, chunks[1].Content, "Python")
}

func TestSemanticChunker_MaxSentsEnforced(t *testing.T) {
	// all sentences similar (high similarity), but maxSents = 2
	emb := &mockEmbedder{
		vectors: map[string][]float32{},
	}

	// default vector {0.5, 0.5} for all -> similarity = 1.0
	c := NewSemanticChunker(context.Background(), emb, SemanticChunkerConfig{
		Threshold: 0.5,
		MinSents:  1,
		MaxSents:  2,
	})

	doc := &ParsedDocument{
		Content: "Sentence one. Sentence two. Sentence three. Sentence four.",
	}

	chunks := c.Chunk(doc)
	for _, ch := range chunks {
		sentCount := ch.Metadata["sentence_count"].(int)
		assert.LessOrEqual(t, sentCount, 2)
	}
}

func TestSemanticChunker_MetadataFields(t *testing.T) {
	emb := &mockEmbedder{}
	c := NewSemanticChunker(context.Background(), emb, SemanticChunkerConfig{Threshold: 0.8})
	chunks := c.Chunk(&ParsedDocument{Content: "Hello world. This is a test."})
	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.Equal(t, "semantic", ch.Metadata["chunker"])
		assert.NotNil(t, ch.Metadata["threshold"])
		assert.NotNil(t, ch.Metadata["sentence_count"])
	}
}

func TestCosineSimilarity(t *testing.T) {
	assert.InDelta(t, 1.0, cosineSimilarity([]float32{1, 0}, []float32{1, 0}), 0.001)
	assert.InDelta(t, 0.0, cosineSimilarity([]float32{1, 0}, []float32{0, 1}), 0.001)
	assert.Equal(t, 0.0, cosineSimilarity([]float32{}, []float32{}))
}

func TestSplitSentencesDebug(t *testing.T) {
	sents := splitSentences("PostgreSQL supports ACID transactions. It uses MVCC for concurrency. Python is a programming language. It is dynamically typed.")
	t.Logf("sentences: %v", sents)
	assert.Len(t, sents, 4)
}
