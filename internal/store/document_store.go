package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Document struct {
	ID         uuid.UUID `db:"id"`
	Filename   string    `db:"filename"`
	MimeType   string    `db:"mime_type"`
	Status     string    `db:"status"`
	ErrorMsg   *string   `db:"error_msg"`
	ChunkCount int       `db:"chunk_count"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type DocumentStore struct {
	db *DB
}

func NewDocumentStore(db *DB) *DocumentStore {
	return &DocumentStore{db: db}
}

func (s *DocumentStore) Create(ctx context.Context, filename, mimeType string) (*Document, error) {
	query := `
		INSERT INTO documents (filename, mime_type)
		VALUES ($1, $2)
		RETURNING id, filename, mime_type, status, error_msg, chunk_count, created_at, updated_at
	`

	doc := &Document{}
	err := s.db.Pool.QueryRow(ctx, query, filename, mimeType).Scan(
		&doc.ID,
		&doc.Filename,
		&doc.MimeType,
		&doc.Status,
		&doc.ErrorMsg,
		&doc.ChunkCount,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}

	return doc, nil
}

func (s *DocumentStore) GetByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	query := `
		SELECT id, filename, mime_type, status, error_msg, chunk_count, created_at, updated_at
		FROM documents
		WHERE id = $1
	`

	doc := &Document{}
	err := s.db.Pool.QueryRow(ctx, query, id).Scan(
		&doc.ID,
		&doc.Filename,
		&doc.MimeType,
		&doc.Status,
		&doc.ErrorMsg,
		&doc.ChunkCount,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("get document by ID: %w", err)
	}

	return doc, nil
}

func (s *DocumentStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMsg *string) error {
	query := `
		UPDATE documents
		SET status = $2, error_msg = $3
		WHERE id = $1
	`
	_, err := s.db.Pool.Exec(ctx, query, id, status, errorMsg)
	if err != nil {
		return fmt.Errorf("update document status: %w", err)
	}
	return nil
}

func (s *DocumentStore) IncrementChunkCount(ctx context.Context, id uuid.UUID, count int) error {
	query := `
		UPDATE documents
		SET chunk_count = chunk_count + $2
		WHERE id = $1
	`

	_, err := s.db.Pool.Exec(ctx, query, id, count)
	if err != nil {
		return fmt.Errorf("increment chunk count: %w", err)
	}
	return nil
}

func (s *DocumentStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM documents WHERE id = $1`

	_, err := s.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}

	return nil
}
