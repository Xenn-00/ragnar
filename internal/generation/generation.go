package generation

import (
	"context"
	"fmt"
	"strings"

	"github.com/Xenn-00/rag-engine/internal/store"
	"github.com/Xenn-00/rag-engine/pkg/provider/llm"
)

const (
	defaultTemperature = 0.2 // niedrig, damit die Antwort mehr sachlich wird. less hallucinate

	systemPrompt = `You are a helpful assistant that answer question based strictly on the provided context. If the answer cannot be found in the context, say so clearly - do not make up information. Always concise and accurate.`
)

type GenerateRequest struct {
	Query       string
	Sources     []store.SearchResult
	Temperature float64
}

type GenerateResponse struct {
	Answer  string
	Sources []store.SearchResult
}

type Generator struct {
	llm llm.LLM
}

func NewGenerator(llm llm.LLM) *Generator {
	return &Generator{llm: llm}
}

// Generate build prompt from sources then return the fully answer (non-streaming).
func (g *Generator) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	req = g.applyDefaults(req)

	if len(req.Sources) == 0 {
		return &GenerateResponse{
			Answer:  "I couldn't find any relevant information to answer your question.",
			Sources: nil,
		}, nil
	}

	completion, err := g.llm.Generate(ctx, llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   g.buildPrompt(req.Query, req.Sources),
		Temperature:  req.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("generate completion: %w", err)
	}

	return &GenerateResponse{
		Answer:  completion.Content,
		Sources: req.Sources,
	}, nil
}

// GenerateStream build prompt then stream token by token into channel
func (g *Generator) GenerateStream(ctx context.Context, req GenerateRequest, chunks chan<- llm.StreamChunk) error {
	req = g.applyDefaults(req)

	if len(req.Sources) == 0 {
		chunks <- llm.StreamChunk{
			Content: "I couldn't find any relevant information to answer your question",
			Done:    true,
		}
		return nil
	}

	if err := g.llm.GenerateStream(ctx, llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   g.buildPrompt(req.Query, req.Sources),
		Temperature:  req.Temperature,
	}, chunks); err != nil {
		return fmt.Errorf("stream completion: %w", err)
	}

	return nil
}

// buildPrompt inject retrieved context inside prompt.
func (g *Generator) buildPrompt(query string, sources []store.SearchResult) string {
	var sb strings.Builder

	sb.WriteString("Context:\n")
	for i, src := range sources {
		sb.WriteString(fmt.Sprintf("[%d] %s\n\n", i+1, src.Content))
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("Question: %s", query))

	return sb.String()
}

func (g *Generator) applyDefaults(req GenerateRequest) GenerateRequest {
	if req.Temperature <= 0 {
		req.Temperature = defaultTemperature
	}

	return req
}
