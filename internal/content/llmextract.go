package content

import (
	"context"
	"fmt"
	"strings"
)

// LLMProvider is the interface required by LLMExtractionStrategy.
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// LLMExtractionStrategy uses a large language model to extract structured
// content from HTML. It implements the ExtractionStrategy interface.
type LLMExtractionStrategy struct {
	provider    LLMProvider
	ChunkSize   int
	Overlap     int
	Schema      string // JSON schema describing the desired output structure
	InputTokens  int    // estimated cumulative input tokens across calls
	OutputTokens int    // estimated cumulative output tokens across calls
}

// NewLLMExtractionStrategy creates an LLMExtractionStrategy with sensible
// defaults.
func NewLLMExtractionStrategy(provider LLMProvider) *LLMExtractionStrategy {
	return &LLMExtractionStrategy{
		provider:  provider,
		ChunkSize: 4000,
		Overlap:   200,
	}
}

func (s *LLMExtractionStrategy) Name() string { return "llm" }

// Extract chunks the HTML input, sends each chunk to the LLM with extraction
// instructions, and merges all results into a single ExtractionResult slice.
func (s *LLMExtractionStrategy) Extract(ctx context.Context, html string, config ExtractionConfig) ([]ExtractionResult, error) {
	chunks := ChunkDocuments(html, ChunkConfig{
		MaxTokens: s.ChunkSize,
		Overlap:   s.Overlap,
	})
	if len(chunks) == 0 {
		return nil, nil
	}

	var results []ExtractionResult
	idx := 0

	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		prompt := s.buildPrompt(chunk)

		resp, err := s.provider.Complete(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("llm extraction chunk %d: %w", i, err)
		}

		// Estimate token usage from word counts.
		s.InputTokens += estimateTokens(prompt)
		s.OutputTokens += estimateTokens(resp)

		trimmed := strings.TrimSpace(resp)
		if trimmed == "" {
			continue
		}

		results = append(results, ExtractionResult{
			Content: trimmed,
			Index:   idx,
		})
		idx++
	}

	return results, nil
}

func (s *LLMExtractionStrategy) buildPrompt(chunk string) string {
	var sb strings.Builder
	sb.WriteString("Extract the relevant content from the following HTML chunk.\n")
	if s.Schema != "" {
		sb.WriteString("Return the result as JSON conforming to this schema:\n")
		sb.WriteString(s.Schema)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("Return clean, well-structured markdown.\n\n")
	}
	sb.WriteString("HTML:\n")
	sb.WriteString(chunk)
	return sb.String()
}

// estimateTokens is defined in chunk.go
