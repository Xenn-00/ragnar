package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Xenn-00/ragnar/internal/pipeline"
	"github.com/Xenn-00/ragnar/pkg/provider/llm"
	"github.com/gofiber/fiber/v2"
)

type QueryHandler struct {
	queryPipeline pipeline.QueryPipelineInterface
	logger        *slog.Logger
}

func NewQueryHandler(queryPipeline pipeline.QueryPipelineInterface, logger *slog.Logger) *QueryHandler {
	return &QueryHandler{
		queryPipeline: queryPipeline,
		logger:        logger,
	}
}

type queryRequest struct {
	Query       string  `json:"query"`
	TopK        int     `json:"top_k"`
	MinScore    float64 `json:"min_score"`
	Temperature float64 `json:"temperature"`
}

// Query godoc
// POST /v1/query
// Non-streaming - wait the fully answer then return
func (h *QueryHandler) Query(c *fiber.Ctx) error {
	var req queryRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if req.Query == "" {
		return fiber.NewError(fiber.StatusBadRequest, "field 'query' is required")
	}

	result, err := h.queryPipeline.Run(c.Context(), pipeline.QueryRequest{
		Query:       req.Query,
		TopK:        req.TopK,
		MinScore:    req.MinScore,
		Temperature: req.Temperature,
	})

	if err != nil {
		h.logger.ErrorContext(c.Context(), "query pipeline failed", "error", err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to process query")
	}

	return c.JSON(fiber.Map{
		"answer":  result.Answer,
		"sources": result.Sources,
	})
}

// Stream godoc
// GET /v1/query/:id/stream?q=<query>
// Streaming via SSE - token is sent one by one
// Using GET + query param in order to easily consume via browser / EventSource API
func (h *QueryHandler) Stream(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return fiber.NewError(fiber.StatusBadRequest, "query param 'q' is required")
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	chunks := make(chan llm.StreamChunk, 32)

	// run pipeline in seperate goroutine
	errCh := make(chan error, 1)
	go func() {
		_, err := h.queryPipeline.RunStream(c.Context(), pipeline.QueryRequest{
			Query: query,
		}, chunks)
		errCh <- err
		close(chunks)
	}()

	// stream response using Fiber's StreamWriter
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		for chunk := range chunks {
			data, err := json.Marshal(fiber.Map{
				"content": chunk.Content,
				"done":    chunk.Done,
			})

			if err != nil {
				continue
			}

			// SSE format: "data: <json\n\n>"
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			_ = w.Flush()
		}

		// check error from pipeline after channel being drained
		if err := <-errCh; err != nil {
			h.logger.ErrorContext(c.Context(), "stream pipeline failed", "error", err)
			// sent error event to client
			_, _ = fmt.Fprintf(w, "event: error\ndata:{\"error\": \"%s\"}\n\n", err.Error())
			_ = w.Flush()
		}
	})

	return nil
}
