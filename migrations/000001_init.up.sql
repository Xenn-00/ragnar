-- ============================================================
-- 001_init.sql
-- Enable pgvector, create documents and chunks tables
-- ============================================================

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- ENUM: document processing status
-- ============================================================

CREATE TYPE document_status AS ENUM (
    'pending', -- Job received, but not yet processed
    'processing', -- being parsed/chunked/embedded
    'completed', -- all chunks successfully stored
    'failed' -- error during processing
);

-- ============================================================
-- TABLE: documents
-- Store metadata about each document
-- ============================================================

CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    filename TEXT NOT NULL,
    mime_type TEXT NOT NULL,    -- e.g "application/pdf" | "text/markdown" | "text/plain"
    status document_status NOT NULL DEFAULT 'pending',
    error_msg TEXT, -- if processing failed, store error message here
    chunk_count INT NOT NULL DEFAULT 0, -- number of chunks created for this document
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- TABLE: chunks
-- Store text chunks and their embeddings
-- nomic-embed-text generates 768-dimentional embeddings for text chunks
-- ============================================================
CREATE TABLE chunks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    content TEXT NOT NULL, -- the text chunk
    embedding vector(768) NOT NULL, -- the embedding vector comes from nomic-embed-text
    chunk_index INT NOT NULL, -- the order of the chunk within the document
    token_count INT, -- estimated token count for the chunk, useful for cost estimation with LLMs
    metadata JSONB NOT NULL DEFAULT '{}', -- store any additional metadata about the chunk here (e.g. source page number, section header, etc.)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- INDEXES
-- ============================================================

-- Index for lookup chunks by document_id
CREATE INDEX idx_chunks_document_id ON chunks(document_id);

-- Index for filter document by status
CREATE INDEX idx_documents_status ON documents(status);

-- IVFFlat index for approximate nearest neighbor search
-- lists = sqrt(total rows) - for early development/testing, you can set lists to a smaller number (e.g. 100) for faster indexing and querying, but for production with larger datasets, you may want to set it to a higher number (e.g. 1000 or more) for better search accuracy
-- probes in query time, setted via: SET ivfflat.probes = 10;
-- CREATE INDEX idx_chunks_embedding ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- ============================================================
-- TRIGGER: auto-update updated_at timestamp on documents
-- ============================================================
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_documents_updated_at
    BEFORE UPDATE ON documents
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();