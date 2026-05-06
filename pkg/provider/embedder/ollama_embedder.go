package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ollamaEmbedder struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// embedRequest is the request body for Ollma /api/embed endpoint
type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embedResponse is the response body from Ollama /api/embed endpoint
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func NewOllama(baseURL, model string) Embedder {
	return &ollamaEmbedder{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (o *ollamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := o.embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned empty embeddings")
	}

	return embeddings[0], nil
}

func (o *ollamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	return o.embed(ctx, texts)
}

func (o *ollamaEmbedder) embed(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody, err := json.Marshal(embedRequest{
		Model: o.model,
		Input: texts,
	})

	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed returned status %d", resp.StatusCode)
	}

	var embedResp embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decaode embed response: %w", err)
	}

	if len(embedResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embedResp.Embeddings))
	}

	return embedResp.Embeddings, nil
}
