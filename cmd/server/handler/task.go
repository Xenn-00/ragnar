package handler

import (
	"github.com/Xenn-00/rag-engine/pkg/task"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func newIngestTask(documentID uuid.UUID, filename string, data []byte, mimeType string) (*asynq.Task, error) {
	return task.NewIngestTask(documentID, filename, data, mimeType)
}
