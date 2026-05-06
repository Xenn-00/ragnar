package task

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeIngestDocument = "document:ingest"

// IngestPaylaod is a data that be serialized to Asynq task queue.
// Worker would deserialized it for running ingestion pipeline.
type IngestPayload struct {
	DocumentID uuid.UUID `json:"document_id"`
	Filename   string    `json:"filename"`
	Data       []byte    `json:"data"`
	MimeType   string    `json:"mime_type"`
}

func NewIngestTask(documentID uuid.UUID, filename string, data []byte, mimeType string) (*asynq.Task, error) {
	payload, err := json.Marshal(IngestPayload{
		DocumentID: documentID,
		Filename:   filename,
		Data:       data,
		MimeType:   mimeType,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ingest payload: %w", err)
	}

	return asynq.NewTask(TypeIngestDocument, payload), nil
}
