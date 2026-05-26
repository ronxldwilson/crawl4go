package content

import (
	"context"
	"testing"
)

// mockLLMCompleter implements LLMCompleter for filter tests.
type mockLLMCompleter struct {
	responses []string
	callCount int
}

func (m *mockLLMCompleter) Complete(_ context.Context, _ string) (string, error) {
	idx := m.callCount % len(m.responses)
	m.callCount++
	return m.responses[idx], nil
}

func TestLLMContentFilter_Name(t *testing.T) {
	f := NewLLMContentFilter(&mockLLMCompleter{responses: []string{"ok"}})
	if got := f.Name(); got != "llm" {
		t.Errorf("Name() = %q, want %q", got, "llm")
	}
}

func TestLLMContentFilter_Filter_Empty(t *testing.T) {
	f := NewLLMContentFilter(&mockLLMCompleter{responses: []string{"ok"}})
	results, err := f.Filter(context.Background(), nil, "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty blocks, got %v", results)
	}
}

func TestLLMContentFilter_Filter_Relevant(t *testing.T) {
	client := &mockLLMCompleter{responses: []string{"## Relevant content here"}}
	f := NewLLMContentFilter(client)

	blocks := []string{"Some block of text about the query topic."}
	results, err := f.Filter(context.Background(), blocks, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Kept {
		t.Error("block should be kept when LLM returns content")
	}
	if results[0].Score != 1.0 {
		t.Errorf("score = %f, want 1.0", results[0].Score)
	}
}

func TestLLMContentFilter_Filter_NotRelevant(t *testing.T) {
	client := &mockLLMCompleter{responses: []string{"NOT_RELEVANT"}}
	f := NewLLMContentFilter(client)

	blocks := []string{"Unrelated block about something else entirely."}
	results, err := f.Filter(context.Background(), blocks, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Kept {
		t.Error("block should not be kept when LLM returns NOT_RELEVANT")
	}
	if results[0].Score != 0.0 {
		t.Errorf("score = %f, want 0.0", results[0].Score)
	}
}

func TestLLMContentFilter_Filter_NotRelevantCaseInsensitive(t *testing.T) {
	client := &mockLLMCompleter{responses: []string{"not_relevant"}}
	f := NewLLMContentFilter(client)

	blocks := []string{"Some block."}
	results, err := f.Filter(context.Background(), blocks, "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Kept {
		t.Error("block should not be kept for case-insensitive NOT_RELEVANT")
	}
}

func TestLLMContentFilter_Filter_Mixed(t *testing.T) {
	client := &mockLLMCompleter{
		responses: []string{
			"## Relevant content",
			"NOT_RELEVANT",
			"More relevant content",
		},
	}
	f := NewLLMContentFilter(client)

	blocks := []string{
		"First block relevant to query.",
		"Second block unrelated.",
		"Third block also relevant.",
	}
	results, err := f.Filter(context.Background(), blocks, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Kept {
		t.Error("block 0 should be kept")
	}
	if results[1].Kept {
		t.Error("block 1 should not be kept")
	}
	if !results[2].Kept {
		t.Error("block 2 should be kept")
	}
}

func TestLLMContentFilter_Filter_IndexPreserved(t *testing.T) {
	client := &mockLLMCompleter{responses: []string{"relevant", "NOT_RELEVANT", "relevant"}}
	f := NewLLMContentFilter(client)

	blocks := []string{"a", "b", "c"}
	results, err := f.Filter(context.Background(), blocks, "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, r := range results {
		if r.Index != i {
			t.Errorf("result[%d].Index = %d, want %d", i, r.Index, i)
		}
	}
}
