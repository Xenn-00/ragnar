package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ollamaLLM struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// ollamaGenerateRequest is the request body for Ollama /api/chat endpoint
type ollamaGenerateRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	Think       bool    `json:"think"`
}

// ollamaGenerateResponse is a one line NDJSON from streaming response.
type ollamaGenerateResponse struct {
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
}

func NewOllama(baseURL, model string) LLM {
	return &ollamaLLM{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// Timeout could be more longer for LLM generation, especially for larger models or longer outputs.
			Timeout: 5 * time.Minute,
		},
	}
}

func (o *ollamaLLM) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Generate using stream=false - wait for full response
	body, err := o.buildRequestBody(req, false)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))

	if err != nil {
		return nil, fmt.Errorf("create generate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do generate request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama generate returned status %d", resp.StatusCode)
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode generate response: %w", err)
	}

	return &CompletionResponse{
		Content: ollamaResp.Message.Content,
	}, nil
}

func (o *ollamaLLM) GenerateStream(ctx context.Context, req CompletionRequest, chunks chan<- StreamChunk) error {
	body, err := o.buildRequestBody(req, true)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))

	if err != nil {
		return fmt.Errorf("create generate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do generate request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama generate returned status %d", resp.StatusCode)
	}

	// Ollama streaming using NDJSON - each JSON object per line
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var streamResp ollamaGenerateResponse
		if err := json.Unmarshal(line, &streamResp); err != nil {
			return fmt.Errorf("decode stream chunk: %w", err)
		}

		chunks <- StreamChunk{
			Content: streamResp.Message.Content,
			Done:    streamResp.Done,
		}

		if streamResp.Done {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan stream response: %w", err)
	}

	return nil
}

func (o *ollamaLLM) buildRequestBody(req CompletionRequest, stream bool) ([]byte, error) {
	messages := []ollamaMessage{
		{Role: "system", Content: req.SystemPrompt},
		{Role: "user", Content: req.UserPrompt},
	}

	body, err := json.Marshal(ollamaGenerateRequest{
		Model:    o.model,
		Messages: messages,
		Stream:   stream,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			Think:       false,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("marshal generate request: %w", err)
	}
	return body, nil
}
