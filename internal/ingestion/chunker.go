package ingestion

// Chunk is a one piece from the text that ready for embedded
type Chunk struct {
	Content    string
	ChunkIndex int
	Metadata   map[string]any
}

// Chunker is the interface that all chunking strategies must implement
type Chunker interface {
	Chunk(doc *ParsedDocument) []Chunk
}
