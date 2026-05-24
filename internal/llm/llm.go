package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

// Config holds LLM provider configuration.
type Config struct {
	Provider    string        `json:"provider"` // openai, anthropic, ollama
	Model       string        `json:"model"`
	APIKey      string        `json:"api_key"`
	BaseURL     string        `json:"base_url,omitempty"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	TopP        float64       `json:"top_p,omitempty"`
	Timeout     time.Duration `json:"timeout"`
	MaxRetries  int           `json:"max_retries"`
}

// DefaultConfig returns a default LLM config.
func DefaultConfig() Config {
	return Config{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		MaxTokens:   4096,
		Temperature: 0.0,
		Timeout:     30 * time.Second,
		MaxRetries:  3,
	}
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompletionRequest is the request to the LLM.
type CompletionRequest struct {
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

// CompletionResponse is the LLM response.
type CompletionResponse struct {
	Content          string `json:"content"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

// Client wraps an LLM provider with retry logic.
type Client struct {
	config     Config
	httpClient *http.Client
}

// NewClient creates an LLM client.
func NewClient(cfg Config) *Client {
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// Complete sends a completion request with exponential backoff retry.
func (c *Client) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if req.MaxTokens == 0 {
		req.MaxTokens = c.config.MaxTokens
	}

	var lastErr error
	maxRetries := c.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			jitter := time.Duration(rand.Float64() * float64(backoff) * 0.25)
			select {
			case <-time.After(backoff + jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := c.doRequest(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("llm completion failed after %d attempts: %w", maxRetries+1, lastErr)
}

func (c *Client) doRequest(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	endpoint, body, err := c.buildRequest(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	switch c.config.Provider {
	case "openai":
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	case "anthropic":
		httpReq.Header.Set("x-api-key", c.config.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm api error %d: %s", resp.StatusCode, string(respBody))
	}

	return c.parseResponse(respBody)
}

func (c *Client) buildRequest(req CompletionRequest) (string, []byte, error) {
	baseURL := c.config.BaseURL

	switch c.config.Provider {
	case "openai", "ollama":
		if baseURL == "" {
			if c.config.Provider == "ollama" {
				baseURL = "http://localhost:11434/v1"
			} else {
				baseURL = "https://api.openai.com/v1"
			}
		}
		payload := map[string]any{
			"model":       c.config.Model,
			"messages":    req.Messages,
			"max_tokens":  req.MaxTokens,
			"temperature": req.Temperature,
		}
		body, err := json.Marshal(payload)
		return baseURL + "/chat/completions", body, err

	case "anthropic":
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		// Convert messages: extract system message
		var system string
		var msgs []map[string]string
		for _, m := range req.Messages {
			if m.Role == "system" {
				system = m.Content
			} else {
				msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
			}
		}
		payload := map[string]any{
			"model":      c.config.Model,
			"messages":   msgs,
			"max_tokens": req.MaxTokens,
		}
		if system != "" {
			payload["system"] = system
		}
		body, err := json.Marshal(payload)
		return baseURL + "/messages", body, err

	default:
		return "", nil, fmt.Errorf("unsupported provider: %s", c.config.Provider)
	}
}

func (c *Client) parseResponse(body []byte) (*CompletionResponse, error) {
	switch c.config.Provider {
	case "openai", "ollama":
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Model string `json:"model"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		content := ""
		if len(resp.Choices) > 0 {
			content = resp.Choices[0].Message.Content
		}
		return &CompletionResponse{
			Content:          content,
			Model:            resp.Model,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}, nil

	case "anthropic":
		var resp struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Model string `json:"model"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		content := ""
		if len(resp.Content) > 0 {
			content = resp.Content[0].Text
		}
		return &CompletionResponse{
			Content:          content,
			Model:            resp.Model,
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", c.config.Provider)
	}
}

// EmbeddingRequest holds the input for embedding generation.
type EmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model,omitempty"`
}

// EmbeddingResponse holds embedding vectors.
type EmbeddingResponse struct {
	Embeddings  [][]float64 `json:"embeddings"`
	Model       string      `json:"model"`
	TotalTokens int         `json:"total_tokens"`
}

// Embed generates text embeddings via the configured provider.
func (c *Client) Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	if req.Model == "" {
		req.Model = "text-embedding-3-small"
	}

	baseURL := c.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	payload := map[string]any{
		"model": req.Model,
		"input": req.Input,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding api error %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, err
	}

	embeddings := make([][]float64, len(embResp.Data))
	for i, d := range embResp.Data {
		embeddings[i] = d.Embedding
	}

	return &EmbeddingResponse{
		Embeddings:  embeddings,
		Model:       embResp.Model,
		TotalTokens: embResp.Usage.TotalTokens,
	}, nil
}
