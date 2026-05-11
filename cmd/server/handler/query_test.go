package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Xenn-00/ragnar/internal/pipeline"
	"github.com/Xenn-00/ragnar/internal/store"
	"github.com/Xenn-00/ragnar/pkg/provider/llm"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mock ---

type MockQueryPipeline struct {
	mock.Mock
}

func (m *MockQueryPipeline) Run(ctx context.Context, req pipeline.QueryRequest) (*pipeline.QueryResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pipeline.QueryResponse), args.Error(1)
}

func (m *MockQueryPipeline) RunStream(ctx context.Context, req pipeline.QueryRequest, chunks chan<- llm.StreamChunk) ([]store.SearchResult, error) {
	args := m.Called(ctx, req, chunks)
	// simulate sending a few chunks then closing
	if fn, ok := args.Get(0).(func(chan<- llm.StreamChunk)); ok {
		fn(chunks)
	}
	if args.Get(1) == nil {
		return []store.SearchResult{}, nil
	}
	return nil, args.Error(1)
}

// --- Helpers ---

func newQueryTestApp(h *QueryHandler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Post("/v1/query", h.Query)
	app.Get("/v1/query/stream", h.Stream)
	return app
}

// --- Query tests ---

func TestQueryHandler_Query_InvalidBody(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestQueryHandler_Query_EmptyQuery(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	body, _ := json.Marshal(map[string]any{"query": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestQueryHandler_Query_MissingQueryField(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	body, _ := json.Marshal(map[string]any{"top_k": 5})
	req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestQueryHandler_Query_PipelineFails(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	mockPipeline.On("Run", mock.Anything, mock.Anything).
		Return(nil, errors.New("retrieval failed"))

	body, _ := json.Marshal(map[string]any{"query": "what is RAG?"})
	req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
}

func TestQueryHandler_Query_Success(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	mockPipeline.On("Run", mock.Anything, pipeline.QueryRequest{
		Query:       "what is RAG?",
		TopK:        5,
		MinScore:    0.3,
		Temperature: 0.7,
	}).Return(&pipeline.QueryResponse{
		Answer:  "RAG stands for Retrieval-Augmented Generation.",
		Sources: []store.SearchResult{},
	}, nil)

	body, _ := json.Marshal(map[string]any{
		"query":       "what is RAG?",
		"top_k":       5,
		"min_score":   0.3,
		"temperature": 0.7,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var respBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&respBody))
	assert.Equal(t, "RAG stands for Retrieval-Augmented Generation.", respBody["answer"])
	assert.NotNil(t, respBody["sources"])
}

func TestQueryHandler_Query_ResponseContainsAnswerAndSources(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	mockPipeline.On("Run", mock.Anything, mock.Anything).
		Return(&pipeline.QueryResponse{
			Answer: "42",
			Sources: []store.SearchResult{
				{Content: "source chunk", Score: 0.9},
			},
		}, nil)

	body, _ := json.Marshal(map[string]any{"query": "meaning of life"})
	req := httptest.NewRequest(http.MethodPost, "/v1/query", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var respBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&respBody))
	assert.Equal(t, "42", respBody["answer"])

	sources, ok := respBody["sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
}

// --- Stream tests ---

func TestQueryHandler_Stream_MissingQueryParam(t *testing.T) {
	mockPipeline := new(MockQueryPipeline)
	h := NewQueryHandler(mockPipeline, slog.Default())
	app := newQueryTestApp(h)

	req := httptest.NewRequest(http.MethodGet, "/v1/query/stream", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}
