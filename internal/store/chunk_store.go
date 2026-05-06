package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type Chunk struct {
	ID         uuid.UUID       `db:"id"`
	DocumentID uuid.UUID       `db:"document_id"`
	Content    string          `db:"content"`
	Embedding  pgvector.Vector `db:"embedding"`
	ChunkIndex int             `db:"chunk_index"`
	TokenCount *int            `db:"token_count"`
	Metadata   json.RawMessage `db:"metadata"`
	CreatedAt  time.Time       `db:"created_at"`
}

// ChunkInput is being used when batch insert - embedding that had been calculated and ready to be inserted into the database
type ChunkInput struct {
	DocumentID uuid.UUID
	Content    string
	Embedding  []float32
	ChunkIndex int
	TokenCount *int
	Metadata   map[string]any
}

// SearchResult is a result of vector similarity search query, it contains the chunk and its similarity score
type SearchResult struct {
	ChunkID    uuid.UUID
	DocumentID uuid.UUID
	Content    string
	Metadata   json.RawMessage
	Score      float64 // cosine similarity (1 = identical, 0 = orthogonal, -1 = opposite)
}

type ChunkStore struct {
	db *DB
}

func NewChunkStore(db *DB) *ChunkStore {
	return &ChunkStore{db: db}
}

// BatchInsert insert multiple chunks into the database in a single transaction.
// More efficient than inserting one by one, especially when the number of chunks is large.
func (s *ChunkStore) BatchInsert(ctx context.Context, inputs []ChunkInput) error {
	if len(inputs) == 0 {
		return nil
	}

	query := `
		INSERT INTO chunks (document_id, content, embedding, chunk_index, token_count, metadata) VALUES ($1, $2, $3, $4, $5, $6)
	`

	batch := &pgx.Batch{}
	for _, input := range inputs {
		meta, err := json.Marshal(input.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata for chunk %d: %w", input.ChunkIndex, err)
		}

		batch.Queue(query, input.DocumentID, input.Content, pgvector.NewVector(input.Embedding), input.ChunkIndex, input.TokenCount, meta)
	}

	br := s.db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := range inputs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert chunk %d: %w", i, err)
		}
	}

	return nil
}

// SimilaritySearch perform top-k chunks that are most similar to the given embedding vector
// Using cosine distance, then the result is converted to similarity score.
func (s *ChunkStore) SimilaritySearch(ctx context.Context, queryEmbedding []float32, topK int) ([]SearchResult, error) {
	_, err := s.db.Pool.Exec(ctx, "SET ivfflat.probes = 10")
	if err != nil {
		return nil, fmt.Errorf("set ivfflat probes: %w", err)
	}

	query := `
		SELECT id, document_id, content, metadata, 1 - (embedding <=> $1) AS score
		FROM chunks
		ORDER BY embedding <=> $1
		LIMIT $2
	`

	rows, err := s.db.Pool.Query(ctx, query, pgvector.NewVector(queryEmbedding), topK)

	if err != nil {
		return nil, fmt.Errorf("similarity search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ChunkID, &r.DocumentID, &r.Content, &r.Metadata, &r.Score); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	return results, nil
}

// DeleteByDocumentID delete all chunks that belong to the given document ID, used when we want to re-process a document and replace all its chunks with new ones
func (s *ChunkStore) DeleteByDocumentID(ctx context.Context, documentID uuid.UUID) error {
	_, err := s.db.Pool.Exec(ctx, "DELETE FROM chunks WHERE document_id = $1", documentID)
	if err != nil {
		return fmt.Errorf("delete chunks by document id: %w", err)
	}

	return nil
}
