package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Xenn-00/ragnar/internal/generation"
	"github.com/Xenn-00/ragnar/internal/metrics"
	"github.com/Xenn-00/ragnar/internal/retrieval"
	"github.com/Xenn-00/ragnar/internal/store"
	"github.com/Xenn-00/ragnar/pkg/provider/llm"
)

// QueryRequest is an input for query pipeline
type QueryRequest struct {
	Query       string
	TopK        int
	MinScore    float64
	Temperature float64
}

// QueryResponse is a result from non-streaming query
type QueryResponse struct {
	Answer  string
	Sources []store.SearchResult
}

type QueryPipeline struct {
	retriever *retrieval.Retriever
	generator *generation.Generator
	logger    *slog.Logger
}

type QueryPipelineInterface interface {
	Run(ctx context.Context, req QueryRequest) (*QueryResponse, error)
	RunStream(ctx context.Context, req QueryRequest, chunks chan<- llm.StreamChunk) ([]store.SearchResult, error)
}

func NewQueryPipeline(
	retriever *retrieval.Retriever,
	generator *generation.Generator,
	logger *slog.Logger,
) *QueryPipeline {
	return &QueryPipeline{
		retriever: retriever,
		generator: generator,
		logger:    logger,
	}
}

// Run execute full query pipeline and return a complete answer (non-streaming).
func (p *QueryPipeline) Run(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	log := p.logger.With("query", req.Query, "top_k", req.TopK)
	start := time.Now()

	// 1. retrieve
	log.InfoContext(ctx, "embedding query")
	retrievalStart := time.Now()
	sources, err := p.retriever.Retrieve(ctx, req.Query, retrieval.RetrieveRequest{
		TopK:     req.TopK,
		MinScore: req.MinScore,
	})
	metrics.RetrievalDuration.Observe(float64(time.Since(retrievalStart).Seconds()))
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}
	log.InfoContext(ctx, "sources retrieved", "count", len(sources))

	// 2. generate
	log.InfoContext(ctx, "generating answer")
	generationStart := time.Now()
	result, err := p.generator.Generate(ctx, generation.GenerateRequest{
		Query:       req.Query,
		Sources:     sources,
		Temperature: req.Temperature,
	})
	metrics.GenerationDuration.Observe(time.Since(generationStart).Seconds())

	if err != nil {
		return nil, fmt.Errorf("generate answer: %w", err)
	}

	metrics.QueryDuration.Observe(time.Since(start).Seconds())

	return &QueryResponse{
		Answer:  result.Answer,
		Sources: result.Sources,
	}, nil
}

// RunStream execute a query pipeline with streaming response
// Token is being sent to channel chunks one by one
func (p *QueryPipeline) RunStream(ctx context.Context, req QueryRequest, chunks chan<- llm.StreamChunk) ([]store.SearchResult, error) {
	log := p.logger.With("query", req.Query)
	start := time.Now()

	// 1. retrieve
	log.InfoContext(ctx, "retrieving sources")
	retrievalStart := time.Now()
	sources, err := p.retriever.Retrieve(ctx, req.Query, retrieval.RetrieveRequest{
		TopK:     req.TopK,
		MinScore: req.MinScore,
	})
	metrics.RetrievalDuration.Observe(float64(time.Since(retrievalStart).Seconds()))
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}
	log.InfoContext(ctx, "sources retrieved", "count", len(sources))

	// 2. generate stream
	log.InfoContext(ctx, "streaming answer")
	if err := p.generator.GenerateStream(ctx, generation.GenerateRequest{
		Query:       req.Query,
		Sources:     sources,
		Temperature: req.Temperature,
	}, chunks); err != nil {
		return nil, fmt.Errorf("stream answer: %w", err)
	}

	metrics.QueryDuration.Observe(time.Since(start).Seconds())

	return sources, nil
}
