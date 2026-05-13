package ingestion

import (
	"strings"
)

const (
	defaultChunkSize    = 512 // in words, not in character
	defaultChunkOverlap = 50  // overlap between chunk inside the word
)

// Chunker = cut the long text into some other smaller chunks
type FixedSizeChunker struct {
	chunkSize    int
	chunkOverlap int
}

func NewFixedSizeChunker(chunkSize, chunkOverlap int) *FixedSizeChunker {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	if chunkOverlap < 0 || chunkOverlap >= chunkSize {
		chunkOverlap = defaultChunkOverlap
	}

	return &FixedSizeChunker{
		chunkSize:    chunkSize,
		chunkOverlap: chunkOverlap,
	}
}

// Chunk cutting text into a slice of Chunk in fixed-size + overlap.
// the cutting unit is a word, not a character - more semantic.
//
//	Overlap ensures that the context in the boundary chunk is not lost.
//
// Example with chunkSize=5, overlap=2
// words: [a b c d e f g h i]
// chunk0: [a b c d e]
// chunk1: [d e f g h] <- overlap 2 words from the chunk before
// chunk2: [g h i]
func (c *FixedSizeChunker) Chunk(doc *ParsedDocument) []Chunk {
	words := strings.Fields(doc.Content)
	if len(words) == 0 {
		return nil
	}

	var chunks []Chunk
	step := c.chunkSize - c.chunkOverlap
	index := 0

	for start := 0; start < len(words); start += step {
		end := start + c.chunkSize
		if end > len(words) {
			end = len(words)
		}

		content := strings.Join(words[start:end], " ")

		metadata := map[string]any{
			"chunk_size":    c.chunkSize,
			"chunk_overlap": c.chunkOverlap,
		}

		// If PDF, add the estimated pages number to metadata
		if doc.Pages > 0 {
			estimatedPage := estimatePage(start, len(words), doc.Pages)
			metadata["estimated_page"] = estimatedPage
		}

		chunks = append(chunks, Chunk{
			Content:    content,
			ChunkIndex: index,
			Metadata:   metadata,
		})

		index++

		// wenn es schon am Ende des Unterlages hat, dann hält es auf
		if end == len(words) {
			break
		}
	}

	return chunks
}

// estimatePage = the estimation for pages based on the word positioning inside the document.
// This is just an approximation - enough for metadata, not for the precise navigation
func estimatePage(wordStart, totalWords, totalPages int) int {
	if totalPages == 0 || totalWords == 0 {
		return 1
	}

	ratio := float64(wordStart) / float64(totalWords)
	page := int(ratio*float64(totalPages)) + 1
	if page > totalPages {
		page = totalPages
	}
	return page
}
