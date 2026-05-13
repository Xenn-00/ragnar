package ingestion

import (
	"context"
	"math"
	"regexp"
	"strings"

	"github.com/Xenn-00/ragnar/pkg/provider/embedder"
)

const (
	defaultSemanticThreshold    = 0.75
	defaultMinSentencesPerChunk = 2
	defaultMaxSentencesPerChunk = 20
)

// SemanticChunker splits text at topic boundaries detected via embedding similarity.
// Sentences with cosine similarity below threshold are treated as a chunk boundary.
type SemanticChunker struct {
	embedder  embedder.Embedder
	ctx       context.Context
	threshold float64
	minSents  int
	maxSents  int
}

type SemanticChunkerConfig struct {
	Threshold float64 // cosine similarity drop boundary, default 0.75
	MinSents  int     // minimum sentences per chunk, default 2
	MaxSents  int     // maximum sentences per chunk, default 20
}

func NewSemanticChunker(ctx context.Context, emb embedder.Embedder, cfg SemanticChunkerConfig) *SemanticChunker {
	if cfg.Threshold <= 0 || cfg.Threshold > 1 {
		cfg.Threshold = defaultSemanticThreshold
	}

	if cfg.MinSents <= 0 {
		cfg.MinSents = defaultMinSentencesPerChunk
	}

	if cfg.MaxSents <= 0 || cfg.MaxSents < cfg.MinSents {
		cfg.MaxSents = defaultMaxSentencesPerChunk
	}

	return &SemanticChunker{
		embedder:  emb,
		ctx:       ctx,
		threshold: cfg.Threshold,
		minSents:  cfg.MinSents,
		maxSents:  cfg.MaxSents,
	}
}

func (s *SemanticChunker) Chunk(doc *ParsedDocument) []Chunk {
	sentences := splitSentences(doc.Content)
	if len(sentences) == 0 {
		return nil
	}

	// single sentence -> direct one chunk
	if len(sentences) == 1 {
		return []Chunk{{
			Content:    sentences[0],
			ChunkIndex: 0,
			Metadata:   s.metadata(1),
		}}
	}

	embeddings, err := s.embedder.EmbedBatch(s.ctx, sentences)
	if err != nil || len(embeddings) != len(sentences) {
		// fallback: one chunk per maxSents sentences
		return s.fallbackChunk(sentences)
	}

	// detect boundaries
	boundaries := []int{0} // always start from index 0
	currentLen := 0

	for i := 0; i < len(sentences)-1; i++ {
		currentLen++
		sim := cosineSimilarity(embeddings[i], embeddings[i+1])
		isTopicShift := sim < s.threshold
		isTooLong := currentLen >= s.maxSents

		if (isTopicShift && currentLen >= s.minSents) || isTooLong {
			boundaries = append(boundaries, i+1)
			currentLen = 0
		}
	}

	// build chunks from boundaries
	var chunks []Chunk
	for idx, start := range boundaries {
		end := len(sentences)
		if idx+1 < len(boundaries) {
			end = boundaries[idx+1]
		}

		content := strings.Join(sentences[start:end], " ")
		chunks = append(chunks, Chunk{
			Content:    content,
			ChunkIndex: idx,
			Metadata:   s.metadata(end - start),
		})
	}

	return chunks
}

func (s *SemanticChunker) metadata(sentenceCount int) map[string]any {
	return map[string]any{
		"chunker":        "semantic",
		"threshold":      s.threshold,
		"sentence_count": sentenceCount,
	}
}

// fallbackChunk being used when EmbedBatch miss - group per maxSents
func (s *SemanticChunker) fallbackChunk(sentences []string) []Chunk {
	var chunks []Chunk
	for i := 0; i < len(sentences); i += s.maxSents {
		end := i + s.maxSents
		if end > len(sentences) {
			end = len(sentences)
		}
		chunks = append(chunks, Chunk{
			Content:    strings.Join(sentences[i:end], " "),
			ChunkIndex: len(chunks),
			Metadata: map[string]any{
				"chunker": "semantic_fallback",
				"reason":  "embed_error",
			},
		})
	}

	return chunks
}

// splitSentences seperating text based on sentence boundary (.!?)
var sentenceEnd = regexp.MustCompile(`([.!?])\s+`)

func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	parts := sentenceEnd.Split(text, -1)
	seps := sentenceEnd.FindAllString(text, -1)

	var sentences []string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// reattach punctuation to the sentence
		if i < len(seps) {
			part = part + strings.TrimSpace(seps[i])
		}
		sentences = append(sentences, part)
	}

	return sentences
}

// cosineSimilarity calculate cosine similarity between 2 vectors float32
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return dot / denom
}
