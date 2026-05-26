package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- helpers ----------------------------------------------------------------

func openAIResponse(content, model string, promptTok, compTok int) []byte {
	resp := map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"content": content}},
		},
		"model": model,
		"usage": map[string]any{
			"prompt_tokens":     promptTok,
			"completion_tokens": compTok,
			"total_tokens":      promptTok + compTok,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func anthropicResponse(text, model string, inputTok, outputTok int) []byte {
	resp := map[string]any{
		"content": []map[string]string{{"text": text}},
		"model":   model,
		"usage": map[string]any{
			"input_tokens":  inputTok,
			"output_tokens": outputTok,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func embeddingResponse(vecs [][]float64, model string, totalTok int) []byte {
	data := make([]map[string]any, len(vecs))
	for i, v := range vecs {
		data[i] = map[string]any{"embedding": v}
	}
	resp := map[string]any{
		"data":  data,
		"model": model,
		"usage": map[string]any{"total_tokens": totalTok},
	}
	b, _ := json.Marshal(resp)
	return b
}

// ---- TestDefaultConfig ------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"Provider", cfg.Provider, "openai"},
		{"Model", cfg.Model, "gpt-4o-mini"},
		{"MaxTokens", cfg.MaxTokens, 4096},
		{"Temperature", cfg.Temperature, 0.0},
		{"Timeout", cfg.Timeout, 30 * time.Second},
		{"MaxRetries", cfg.MaxRetries, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

// ---- TestClient_Complete_OpenAI ---------------------------------------------

func TestClient_Complete_OpenAI(t *testing.T) {
	var capturedAuth, capturedCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.Write(openAIResponse("Hello!", "gpt-test", 10, 5))
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider:   "openai",
		Model:      "gpt-test",
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("content: got %q, want %q", resp.Content, "Hello!")
	}
	if resp.Model != "gpt-test" {
		t.Errorf("model: got %q, want %q", resp.Model, "gpt-test")
	}
	if resp.PromptTokens != 10 {
		t.Errorf("prompt tokens: got %d, want 10", resp.PromptTokens)
	}
	if resp.CompletionTokens != 5 {
		t.Errorf("completion tokens: got %d, want 5", resp.CompletionTokens)
	}
	if resp.TotalTokens != 15 {
		t.Errorf("total tokens: got %d, want 15", resp.TotalTokens)
	}
	if capturedAuth != "Bearer test-key" {
		t.Errorf("Authorization header: got %q, want %q", capturedAuth, "Bearer test-key")
	}
	if capturedCT != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", capturedCT, "application/json")
	}
}

// ---- TestClient_Complete_Anthropic ------------------------------------------

func TestClient_Complete_Anthropic(t *testing.T) {
	var capturedAPIKey, capturedVersion string
	var reqBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("x-api-key")
		capturedVersion = r.Header.Get("anthropic-version")
		json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(anthropicResponse("Anthropic says hi", "claude-test", 8, 4))
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider:   "anthropic",
		Model:      "claude-test",
		APIKey:     "sk-ant-test",
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Anthropic says hi" {
		t.Errorf("content: got %q, want %q", resp.Content, "Anthropic says hi")
	}
	if resp.PromptTokens != 8 || resp.CompletionTokens != 4 || resp.TotalTokens != 12 {
		t.Errorf("unexpected token counts: %+v", resp)
	}
	if capturedAPIKey != "sk-ant-test" {
		t.Errorf("x-api-key: got %q, want %q", capturedAPIKey, "sk-ant-test")
	}
	if capturedVersion != "2023-06-01" {
		t.Errorf("anthropic-version: got %q, want %q", capturedVersion, "2023-06-01")
	}
	// System message should have been extracted from the messages array.
	if sys, ok := reqBody["system"].(string); !ok || sys != "You are helpful." {
		t.Errorf("system field in request: %v", reqBody["system"])
	}
}

// ---- TestClient_Complete_Ollama ---------------------------------------------

func TestClient_Complete_Ollama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(openAIResponse("Ollama response", "llama3", 3, 7))
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider:   "ollama",
		Model:      "llama3",
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Ollama response" {
		t.Errorf("content: got %q, want %q", resp.Content, "Ollama response")
	}
}

// ---- TestClient_Complete_Error ----------------------------------------------

func TestClient_Complete_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider:   "openai",
		Model:      "gpt-test",
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention 500, got: %v", err)
	}
}

// ---- TestClient_Complete_Retry ----------------------------------------------

func TestClient_Complete_Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(openAIResponse("OK on retry", "gpt-retry", 1, 1))
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider:   "openai",
		Model:      "gpt-retry",
		BaseURL:    srv.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
	})

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if resp.Content != "OK on retry" {
		t.Errorf("content: got %q, want %q", resp.Content, "OK on retry")
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}
}

// ---- TestClient_UnsupportedProvider -----------------------------------------

func TestClient_UnsupportedProvider(t *testing.T) {
	client := NewClient(Config{
		Provider:   "unknown_provider",
		Model:      "whatever",
		Timeout:    5 * time.Second,
		MaxRetries: 0,
	})

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for unsupported provider, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("error should mention unsupported provider, got: %v", err)
	}
}

// ---- TestClient_ContextCancellation -----------------------------------------

func TestClient_ContextCancellation(t *testing.T) {
	slow := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-slow // block until test cancels
	}))
	defer func() {
		close(slow)
		srv.Close()
	}()

	client := NewClient(Config{
		Provider:   "openai",
		Model:      "gpt-test",
		BaseURL:    srv.URL,
		Timeout:    10 * time.Second,
		MaxRetries: 0,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := client.Complete(ctx, CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}
}

// ---- TestClient_Embed -------------------------------------------------------

func TestClient_Embed(t *testing.T) {
	vecs := [][]float64{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/embeddings") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(embeddingResponse(vecs, "text-embedding-3-small", 20))
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider: "openai",
		APIKey:   "test-key",
		BaseURL:  srv.URL,
		Timeout:  5 * time.Second,
	})

	resp, err := client.Embed(context.Background(), EmbeddingRequest{
		Input: []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(resp.Embeddings))
	}
	if resp.TotalTokens != 20 {
		t.Errorf("total tokens: got %d, want 20", resp.TotalTokens)
	}
	if len(resp.Embeddings[0]) != 3 {
		t.Errorf("embedding dim: got %d, want 3", len(resp.Embeddings[0]))
	}
}

// ---- TestClient_Embed_Error -------------------------------------------------

func TestClient_Embed_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewClient(Config{
		Provider: "openai",
		BaseURL:  srv.URL,
		Timeout:  5 * time.Second,
	})

	_, err := client.Embed(context.Background(), EmbeddingRequest{
		Input: []string{"test"},
	})
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to mention 400, got: %v", err)
	}
}
