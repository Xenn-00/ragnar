package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFixedSizeChunker_Defaults(t *testing.T) {
	t.Run("invalid chunkSize falls back to default", func(t *testing.T) {
		c := NewFixedSizeChunker(0, 50)
		assert.Equal(t, defaultChunkSize, c.chunkSize)
	})

	t.Run("negative chunkSize falls back to default", func(t *testing.T) {
		c := NewFixedSizeChunker(-10, 50)
		assert.Equal(t, defaultChunkSize, c.chunkSize)
	})

	t.Run("negative overlap falls back to default", func(t *testing.T) {
		c := NewFixedSizeChunker(100, -1)
		assert.Equal(t, defaultChunkOverlap, c.chunkOverlap)
	})

	t.Run("overlap >= chunkSize falls back to default", func(t *testing.T) {
		c := NewFixedSizeChunker(100, 100)
		assert.Equal(t, defaultChunkOverlap, c.chunkOverlap)
	})

	t.Run("valid params are accepted as-is", func(t *testing.T) {
		c := NewFixedSizeChunker(200, 20)
		assert.Equal(t, 200, c.chunkSize)
		assert.Equal(t, 20, c.chunkOverlap)
	})
}

func TestChunker_Chunk_EmptyDocument(t *testing.T) {
	c := NewFixedSizeChunker(5, 2)
	doc := &ParsedDocument{Content: "", Pages: 0}

	chunks := c.Chunk(doc)
	assert.Nil(t, chunks, "empty content should return nil")
}

func TestChunker_Chunk_WhitespaceOnly(t *testing.T) {
	c := NewFixedSizeChunker(5, 2)
	doc := &ParsedDocument{Content: "	\n\t ", Pages: 0}

	chunks := c.Chunk(doc)
	assert.Nil(t, chunks, "whitespace-only content should return nil")
}

func TestChunker_Chunk_OverlapBehavior(t *testing.T) {
	// words: [a b c d e f g h i]
	// chunkSize=5, overlap=2 -> step=3
	// chunk0: [a b c d e]
	// chunk1: [d e f g h] 	<- overlap 2 words from chunk0
	// chunk2: [g h i]	 	<- overlap 2 words from chunk1
	c := NewFixedSizeChunker(5, 2)
	doc := &ParsedDocument{
		Content: "a b c d e f g h i",
		Pages:   0,
	}

	chunks := c.Chunk(doc)
	require.Len(t, chunks, 3)

	assert.Equal(t, "a b c d e", chunks[0].Content)
	assert.Equal(t, "d e f g h", chunks[1].Content)
	assert.Equal(t, "g h i", chunks[2].Content)
}

func TestChunker_Chunk_IndexSequential(t *testing.T) {
	c := NewFixedSizeChunker(3, 1)
	doc := &ParsedDocument{Content: "a b c d e f g h i", Pages: 0}

	chunks := c.Chunk(doc)
	for i, chunk := range chunks {
		assert.Equal(t, i, chunk.ChunkIndex, "chunk index should be sequential")
	}
}

func TestChunker_Chunk_MetadataPresent(t *testing.T) {
	c := NewFixedSizeChunker(5, 2)
	doc := &ParsedDocument{
		Content: "a b c d e f g",
		Pages:   0,
	}

	chunks := c.Chunk(doc)
	require.NotEmpty(t, chunks)

	for _, chunk := range chunks {
		assert.Equal(t, 5, chunk.Metadata["chunk_size"])
		assert.Equal(t, 2, chunk.Metadata["chunk_overlap"])
	}
}

func TestChunker_Chunk_PDFEstimatedPage(t *testing.T) {
	c := NewFixedSizeChunker(5, 0)
	// 10 words, 2 pages
	doc := &ParsedDocument{
		Content: "a b c d e f g h i j",
		Pages:   2,
	}

	chunks := c.Chunk(doc)
	require.NotEmpty(t, chunks)

	for _, chunk := range chunks {
		_, ok := chunk.Metadata["estimated_page"]
		assert.True(t, ok, "PDF document should have estimated_page in metadata")
	}
}

func TestChunker_Chunk_NoPDFPageMetadata(t *testing.T) {
	c := NewFixedSizeChunker(5, 0)
	// 10 words, 2 pages
	doc := &ParsedDocument{
		Content: "a b c d e f g h i j",
		Pages:   0, // plain text, no pages
	}

	chunks := c.Chunk(doc)
	require.NotEmpty(t, chunks)

	for _, chunk := range chunks {
		_, ok := chunk.Metadata["estimated_page"]
		assert.False(t, ok, "non-PDF document should have estimated_page in metadata")
	}
}

func TestChunker_Chunk_ExactlyOneChunk(t *testing.T) {
	c := NewFixedSizeChunker(10, 2)
	doc := &ParsedDocument{Content: "a b c", Pages: 0}

	chunks := c.Chunk(doc)
	require.Len(t, chunks, 1)
	assert.Equal(t, "a b c", chunks[0].Content)
}

func TestChunker_Chunk_LastChunkContainsRemainder(t *testing.T) {
	// chunkSize=4, overlap=0 -> step=4
	// words: [a b c d e]
	// chunk0: [a b c d]
	// chunk1: [e]
	c := NewFixedSizeChunker(4, 0)
	doc := &ParsedDocument{Content: "a b c d e", Pages: 0}

	chunks := c.Chunk(doc)
	require.Len(t, chunks, 2)
	assert.Equal(t, "a b c d", chunks[0].Content)
	assert.Equal(t, "e", chunks[1].Content)
}

func TestEstimatePage(t *testing.T) {
	tests := []struct {
		name       string
		wordStart  int
		totalWords int
		totalPages int
		expected   int
	}{
		{"first word, 2 pages", 0, 100, 2, 1},
		{"middle word, 2 pages", 50, 100, 2, 2},
		{"last word, 2 pages", 99, 100, 2, 2},
		{"zero totalPages returns 1", 50, 100, 0, 1},
		{"zero totalWords returns 1", 0, 0, 5, 1},
		{"never exceeds totalPages", 99, 100, 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimatePage(tt.wordStart, tt.totalWords, tt.totalPages)
			assert.Equal(t, tt.expected, result)
		})
	}
}
