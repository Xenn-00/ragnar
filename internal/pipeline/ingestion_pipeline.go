package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Xenn-00/ragnar/internal/ingestion"
	"github.com/Xenn-00/ragnar/internal/metrics"
	"github.com/Xenn-00/ragnar/internal/store"
	"github.com/Xenn-00/ragnar/pkg/provider/embedder"
	"github.com/google/uuid"
)

type IngestionPipeline struct {
	parser        *ingestion.Parser
	chunker       ingestion.Chunker
	embedder      embedder.Embedder
	documentStore store.DocumentStoreInterface
	chunkStore    *store.ChunkStore
	logger        *slog.Logger
}

type IngestionPipelineInterface interface {
	Run(ctx context.Context, documentID uuid.UUID, filename string, data []byte, mimeType string) error
}

func NewIngestionPipeline(
	parser *ingestion.Parser,
	chunker ingestion.Chunker,
	embedder embedder.Embedder,
	documentStore store.DocumentStoreInterface,
	chunkStore *store.ChunkStore,
	logger *slog.Logger,
) *IngestionPipeline {
	return &IngestionPipeline{
		parser:        parser,
		chunker:       chunker,
		embedder:      embedder,
		documentStore: documentStore,
		chunkStore:    chunkStore,
		logger:        logger,
	}
}

// Run execute full ingestion pipeline for one document
// Called by Asynq worker, not directly from HTTP handler.
func (p *IngestionPipeline) Run(ctx context.Context, documentID uuid.UUID, filename string, data []byte, mimeType string) error {
	log := p.logger.With("document_id", documentID, "filename", filename)

	start := time.Now()

	// 1. Update status -> processing
	if err := p.documentStore.UpdateStatus(ctx, documentID, "processing", nil); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}
	log.InfoContext(ctx, "ingestion started")

	// wrap all step into helper, in order to the error handler is being centralize
	if err := p.run(ctx, documentID, filename, data, mimeType, log); err != nil {
		errMsg := err.Error()
		// update status -> failed, but don't shadow error it
		_ = p.documentStore.UpdateStatus(ctx, documentID, "failed", &errMsg)
		log.ErrorContext(ctx, "ingestion failed", "error", err)

		metrics.IngestionTotal.WithLabelValues("failed").Inc()
		metrics.IngestionDuration.Observe(time.Since(start).Seconds())

		return err
	}

	metrics.IngestionTotal.WithLabelValues("success").Inc()
	metrics.IngestionDuration.Observe(time.Since(start).Seconds())

	log.InfoContext(ctx, "ingestion completed")
	return nil
}

func (p *IngestionPipeline) run(ctx context.Context, documentID uuid.UUID, filename string, data []byte, mimeType string, log *slog.Logger) error {
	// 2. parse
	log.InfoContext(ctx, "parsing document")
	parsed, err := p.parser.Parse(ctx, filename, data, mimeType)
	if err != nil {
		return fmt.Errorf("parse document: %w", err)
	}

	// 3. chunk
	log.InfoContext(ctx, "chunking document")
	chunks := p.chunker.Chunk(parsed)
	if len(chunks) == 0 {
		return fmt.Errorf("document produced no chunks after parsing")
	}

	metrics.ChunksPerDocument.Observe(float64(len(chunks)))
	log.InfoContext(ctx, "chunking done", "chunk_count", len(chunks))

	// 4. embed all of it (batch)
	log.InfoContext(ctx, "embedding chunks")
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	embeddings, err := p.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}

	// 5. build ChunkInputs - combine chunk + embedding
	inputs := make([]store.ChunkInput, len(chunks))
	for i, c := range chunks {
		inputs[i] = store.ChunkInput{
			DocumentID: documentID,
			Content:    c.Content,
			Embedding:  embeddings[i],
			ChunkIndex: c.ChunkIndex,
			Metadata:   c.Metadata,
		}
	}

	// 6. batch insert into pgvector
	log.InfoContext(ctx, "storing chunks")
	if err := p.chunkStore.BatchInsert(ctx, inputs); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}

	// 7. update chunk_count + status -> completed
	if err := p.documentStore.IncrementChunkCount(ctx, documentID, len(chunks)); err != nil {
		return fmt.Errorf("increment chunk count: %w", err)
	}

	if err := p.documentStore.UpdateStatus(ctx, documentID, "completed", nil); err != nil {
		return fmt.Errorf("update status to completed: %w", err)
	}

	return nil
}
