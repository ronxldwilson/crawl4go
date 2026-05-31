package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/browser"
	"github.com/ronxldwilson/crawl4go/internal/content"
	"github.com/ronxldwilson/crawl4go/internal/crawl"
	"github.com/ronxldwilson/crawl4go/internal/llm"
	"github.com/ronxldwilson/crawl4go/internal/storage"
)

// Deps is the shared dependency bundle threaded into HTTP handlers. Optional
// subsystems (LLM, cache) are nil when not configured; handlers must nil-check
// them and degrade gracefully.
type Deps struct {
	Cfg         Config
	CDP         *browser.CDPClient
	HTTP        *http.Client
	Pruner      *content.PruningFilter
	Robots      *crawl.RobotsChecker
	RateLimiter *crawl.RateLimiter

	// LLM is the universal adapter over *llm.Client. It is nil when no LLM
	// provider is configured. When non-nil it satisfies content.LLMProvider,
	// content.LLMCompleter, and content.Embedder simultaneously.
	LLM *LLMAdapter

	// Cache is the persistent crawl-result store (dependency-free filesystem
	// cache). Nil when caching is disabled.
	Cache *storage.FileCache
}

// LLMEnabled reports whether an LLM provider is wired up.
func (d *Deps) LLMEnabled() bool { return d != nil && d.LLM != nil }

// CacheEnabled reports whether persistent caching is active.
func (d *Deps) CacheEnabled() bool { return d != nil && d.Cache != nil }

// --- LLM adapter ---------------------------------------------------------

// LLMAdapter exposes the small method set the content strategies depend on,
// backed by injectable function fields. A single value satisfies all three
// content interfaces (LLMProvider, LLMCompleter, Embedder). The func-field
// design lets tests construct an adapter with fakes, while production wires it
// to an *llm.Client via buildLLMAdapter.
type LLMAdapter struct {
	completer func(ctx context.Context, prompt string) (string, error)
	embedder  func(ctx context.Context, text string) ([]float64, error)
}

// Compile-time guarantees that the adapter satisfies every interface the
// previously-dead content strategies require.
var (
	_ content.LLMProvider  = (*LLMAdapter)(nil)
	_ content.LLMCompleter = (*LLMAdapter)(nil)
	_ content.Embedder     = (*LLMAdapter)(nil)
)

// Complete satisfies content.LLMProvider and content.LLMCompleter.
func (a *LLMAdapter) Complete(ctx context.Context, prompt string) (string, error) {
	return a.completer(ctx, prompt)
}

// Embed satisfies content.Embedder.
func (a *LLMAdapter) Embed(ctx context.Context, text string) ([]float64, error) {
	return a.embedder(ctx, text)
}

// --- builders ------------------------------------------------------------

// buildDeps assembles the shared dependency bundle. The CDP client, HTTP
// client, pruner, robots checker, and rate limiter are constructed by the
// caller (they are always present); the LLM adapter and cache are built here
// from configuration and may be nil.
func buildDeps(cfg Config, cdp *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter, robots *crawl.RobotsChecker, rl *crawl.RateLimiter) *Deps {
	return &Deps{
		Cfg:         cfg,
		CDP:         cdp,
		HTTP:        httpClient,
		Pruner:      pruner,
		Robots:      robots,
		RateLimiter: rl,
		LLM:         buildLLMAdapter(cfg),
		Cache:       buildCache(cfg),
	}
}

// buildLLMAdapter constructs the LLM adapter from config, or returns nil when
// no provider is usable. An API key is required for openai/anthropic; ollama
// runs locally and needs none.
func buildLLMAdapter(cfg Config) *LLMAdapter {
	if cfg.LLMProvider == "" {
		return nil
	}
	if cfg.LLMAPIKey == "" && cfg.LLMProvider != "ollama" {
		return nil
	}

	lc := llm.DefaultConfig()
	lc.Provider = cfg.LLMProvider
	if cfg.LLMModel != "" {
		lc.Model = cfg.LLMModel
	}
	lc.APIKey = cfg.LLMAPIKey
	lc.BaseURL = cfg.LLMBaseURL

	client := llm.NewClient(lc)
	embedModel := cfg.LLMEmbedModel

	slog.Info("llm enabled", "provider", lc.Provider, "model", lc.Model)
	return &LLMAdapter{
		completer: func(ctx context.Context, prompt string) (string, error) {
			resp, err := client.Complete(ctx, llm.CompletionRequest{
				Messages: []llm.Message{{Role: "user", Content: prompt}},
			})
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		},
		embedder: func(ctx context.Context, text string) ([]float64, error) {
			resp, err := client.Embed(ctx, llm.EmbeddingRequest{
				Input: []string{text},
				Model: embedModel,
			})
			if err != nil {
				return nil, err
			}
			if len(resp.Embeddings) == 0 {
				return nil, nil
			}
			return resp.Embeddings[0], nil
		},
	}
}

// buildCache opens the persistent crawl-result store, or returns nil when
// caching is disabled or initialization fails (failures degrade to no-cache).
func buildCache(cfg Config) *storage.FileCache {
	if !cfg.CacheEnabled || cfg.CachePath == "" {
		return nil
	}
	ttl := time.Duration(cfg.CacheTTLSeconds) * time.Second
	fc, err := storage.NewFileCache(cfg.CachePath, ttl)
	if err != nil {
		slog.Error("cache disabled: open failed", "path", cfg.CachePath, "error", err)
		return nil
	}
	slog.Info("cache enabled", "path", cfg.CachePath, "ttl_seconds", cfg.CacheTTLSeconds)
	return fc
}
