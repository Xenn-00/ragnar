package embedder

import "context"

// Embedder is an interface for all embedding providers
// No matter what embedding provider we use (Ollama, OpenAI, etc), it should implement this interface
type Embedder interface {
	// Embed produces a vector embedding from 1 or more text inputs.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch produces vector embeddings from a batch of text inputs.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}
