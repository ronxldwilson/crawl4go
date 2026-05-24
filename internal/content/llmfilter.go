package content

import (
	"context"
	"fmt"
	"strings"
)

// DefaultLLMFilterInstruction is the default prompt sent to the LLM when
// filtering content blocks for relevance.
const DefaultLLMFilterInstruction = `You are a content relevance filter. Given the following HTML content block and a search query, determine if the content is relevant. If relevant, extract the key information as clean markdown. If not relevant, respond with exactly "NOT_RELEVANT".

Query: %s

Content:
%s

Respond with the extracted markdown or "NOT_RELEVANT".`

// LLMCompleter is the interface required by LLMContentFilter to call an LLM.
type LLMCompleter interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// LLMContentFilter uses a large language model to score and filter content
// blocks by relevance to a query. It implements the ContentFilter interface.
type LLMContentFilter struct {
	llmClient   LLMCompleter
	Instruction string
	Threshold   float64
}

// NewLLMContentFilter creates an LLMContentFilter with the given client and
// default settings.
func NewLLMContentFilter(client LLMCompleter) *LLMContentFilter {
	return &LLMContentFilter{
		llmClient:   client,
		Instruction: DefaultLLMFilterInstruction,
		Threshold:   0.0,
	}
}

func (f *LLMContentFilter) Name() string { return "llm" }

// Filter sends each block to the LLM and keeps those that the model deems
// relevant. Blocks for which the LLM returns "NOT_RELEVANT" are marked as
// not kept.
func (f *LLMContentFilter) Filter(ctx context.Context, blocks []string, query string) ([]FilteredBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	results := make([]FilteredBlock, len(blocks))

	for i, block := range blocks {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		prompt := fmt.Sprintf(f.Instruction, query, block)

		resp, err := f.llmClient.Complete(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("llm filter block %d: %w", i, err)
		}

		trimmed := strings.TrimSpace(resp)
		kept := !strings.EqualFold(trimmed, "NOT_RELEVANT")

		score := 0.0
		if kept {
			score = 1.0
		}

		content := block
		if kept && trimmed != "" {
			content = trimmed // use the LLM-extracted markdown
		}

		results[i] = FilteredBlock{
			Content: content,
			Score:   score,
			Index:   i,
			Kept:    kept,
		}
	}

	return results, nil
}
