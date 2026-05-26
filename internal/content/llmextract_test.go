package content

import (
	"context"
	"strings"
	"testing"
)

// mockLLMProvider cycles through fixed responses for testing.
type mockLLMProvider struct {
	responses []string
	callCount int
}

func (m *mockLLMProvider) Complete(_ context.Context, _ string) (string, error) {
	idx := m.callCount % len(m.responses)
	m.callCount++
	return m.responses[idx], nil
}

func TestLLMExtractionStrategy_Name(t *testing.T) {
	s := NewLLMExtractionStrategy(&mockLLMProvider{responses: []string{"ok"}})
	if got := s.Name(); got != "llm" {
		t.Errorf("Name() = %q, want %q", got, "llm")
	}
}

func TestLLMExtractionStrategy_EmptyInput(t *testing.T) {
	provider := &mockLLMProvider{responses: []string{"response"}}
	s := NewLLMExtractionStrategy(provider)
	results, err := s.Extract(context.Background(), "", ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty input, got %d", len(results))
	}
	if provider.callCount != 0 {
		t.Errorf("expected no LLM calls for empty input, got %d", provider.callCount)
	}
}

func TestLLMExtractionStrategy_Extract_ChunksSent(t *testing.T) {
	provider := &mockLLMProvider{responses: []string{"extracted content here"}}
	s := NewLLMExtractionStrategy(provider)

	html := "<html><body>" + strings.Repeat("<p>Some content paragraph with words. </p>", 20) + "</body></html>"
	results, err := s.Extract(context.Background(), html, ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// At least one chunk should have been processed.
	if provider.callCount == 0 {
		t.Error("expected at least one LLM call")
	}
	// Results should correspond to non-empty responses.
	for i, r := range results {
		if strings.TrimSpace(r.Content) == "" {
			t.Errorf("result[%d] has empty content", i)
		}
	}
}

func TestLLMExtractionStrategy_Extract_EmptyResponse(t *testing.T) {
	// Provider returns empty string — should produce no results.
	provider := &mockLLMProvider{responses: []string{"   "}}
	s := NewLLMExtractionStrategy(provider)

	html := "<p>Some real content paragraph with enough text to chunk.</p>"
	results, err := s.Extract(context.Background(), html, ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty LLM responses, got %d", len(results))
	}
}

func TestLLMExtractionStrategy_ContextCancel(t *testing.T) {
	provider := &mockLLMProvider{responses: []string{"some response"}}
	s := NewLLMExtractionStrategy(provider)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	html := "<p>" + strings.Repeat("word ", 200) + "</p>"
	_, err := s.Extract(ctx, html, ExtractionConfig{})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

func TestLLMExtractionStrategy_TokenCounting(t *testing.T) {
	provider := &mockLLMProvider{responses: []string{"response with some words"}}
	s := NewLLMExtractionStrategy(provider)

	html := "<p>" + strings.Repeat("word ", 50) + "</p>"
	_, err := s.Extract(context.Background(), html, ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.callCount > 0 {
		// Token counters should have been updated.
		if s.InputTokens == 0 {
			t.Error("InputTokens should be > 0 after extraction")
		}
		if s.OutputTokens == 0 {
			t.Error("OutputTokens should be > 0 after extraction")
		}
	}
}

func TestLLMExtractionStrategy_SchemaPrompt(t *testing.T) {
	var capturedPrompt string
	provider := &capturingProvider{}
	s := NewLLMExtractionStrategy(provider)
	s.Schema = `{"type":"object"}`

	html := "<p>Product name: Acme Widget. Price: $9.99.</p>"
	_, err := s.Extract(context.Background(), html, ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	capturedPrompt = provider.lastPrompt
	if !strings.Contains(capturedPrompt, s.Schema) {
		t.Errorf("prompt should contain schema, got: %q", capturedPrompt)
	}
}

// capturingProvider records the last prompt it received.
type capturingProvider struct {
	lastPrompt string
}

func (c *capturingProvider) Complete(_ context.Context, prompt string) (string, error) {
	c.lastPrompt = prompt
	return "extracted", nil
}
