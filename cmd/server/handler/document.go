package handler

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"

	"github.com/Xenn-00/rag-engine/internal/store"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	maxUploadSize = 20 << 20 // 20 MB
)

type DocumentHandler struct {
	documentStore store.DocumentStoreInterface
	asynqClient   *asynq.Client
	logger        *slog.Logger
}

func NewDocumentHandler(
	documentStore store.DocumentStoreInterface,
	asynqClient *asynq.Client,
	logger *slog.Logger,
) *DocumentHandler {
	return &DocumentHandler{
		documentStore: documentStore,
		asynqClient:   asynqClient,
		logger:        logger,
	}
}

// Update godoc
// POST /v1/documents
// Content-Type: multipart/form-data
// field: "field" - PDF or Markdown
func (h *DocumentHandler) Upload(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "field 'file' is required")
	}

	if fileHeader.Size > maxUploadSize {
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, fmt.Sprintf("file too large: max %d MB", maxUploadSize>>20))
	}

	mimeType := fileHeader.Header.Get("Content-Type")
	if !isSupportedMimeType(mimeType) {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, fmt.Sprintf("unsupported file type: %s (supported: application/pdf, text/markdown, text/plain)", mimeType))
	}

	// read file bytes
	file, err := fileHeader.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to open uploaded file")
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	log.Printf("read %d bytes from upload", len(data))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to read uploaded file")
	}

	// create a record document in DB with status pending
	doc, err := h.documentStore.Create(c.Context(), fileHeader.Filename, mimeType)
	if err != nil {
		h.logger.ErrorContext(c.Context(), "failed to create document record", "error", err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create document record")
	}

	// enqueue ingestion job to asynq
	task, err := newIngestTask(doc.ID, fileHeader.Filename, data, mimeType)

	if _, err := h.asynqClient.EnqueueContext(c.Context(), task); err != nil {
		h.logger.ErrorContext(c.Context(), "failed to enqueue ingestion task", "document_id", doc.ID, "error", err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to enqueue ingestion task")
	}

	h.logger.InfoContext(c.Context(), "document uploaded", "document_id", doc.ID, "filename", doc.Filename)

	return c.Status(http.StatusAccepted).JSON(fiber.Map{
		"id":         doc.ID,
		"filename":   doc.Filename,
		"mime_type":  doc.MimeType,
		"status":     doc.Status,
		"created_at": doc.CreatedAt,
	})
}

// GetByID godoc
// GET /v1/documents/:id
func (h *DocumentHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid document id")
	}

	doc, err := h.documentStore.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "document not found")
	}

	return c.JSON(fiber.Map{
		"id":          doc.ID,
		"filename":    doc.Filename,
		"mime_type":   doc.MimeType,
		"status":      doc.Status,
		"chunk_count": doc.ChunkCount,
		"error_msg":   doc.ErrorMsg,
		"created_at":  doc.CreatedAt,
		"updated_at":  doc.UpdatedAt,
	})
}

// Delete godoc
// DELETE /v1/documents/:id
// chunks automatically deleted via ON DELETE CASCADE
func (h *DocumentHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid document id")
	}

	if err := h.documentStore.Delete(c.Context(), id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to delete document")
	}

	h.logger.InfoContext(c.Context(), "document deleted", "document_id", id)

	return c.SendStatus(http.StatusNoContent)
}

func isSupportedMimeType(mimeType string) bool {
	switch mimeType {
	case "application/pdf", "text/markdown", "text/plain":
		return true
	}
	return false
}
